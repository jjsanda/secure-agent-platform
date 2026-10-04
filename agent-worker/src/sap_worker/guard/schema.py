"""Per-tool argument-schema guard — validate arguments before dispatch.

Each registered tool has a Pydantic model describing its arguments (types,
required keys, and a per-string length bound). Unknown keys, missing keys,
mistyped values, and oversized payloads are rejected here so malformed calls
fail fast worker-side rather than round-tripping to the proxy.

Contracts mirrored from the Go tool registry:

* ``echo`` requires a string ``input``.
* ``http_get`` requires a string ``url``.

A tool with no registered model is left to the other guards (allow-list, SSRF,
path) — only the global argument-size ceiling still applies to it. The Go tool
proxy remains authoritative; this is worker-local defense-in-depth.
"""

from __future__ import annotations

import json
from collections.abc import Mapping
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from sap_worker.exceptions import GuardBlocked

__all__ = [
    "MAX_STRING_CHARS",
    "MAX_ARGUMENTS_BYTES",
    "SchemaViolation",
    "EchoArgs",
    "HttpGetArgs",
    "check",
]

# Bound a single string argument and the whole serialized argument object, so an
# "oversized args" injection attempt is rejected rather than forwarded.
MAX_STRING_CHARS = 8_192
MAX_ARGUMENTS_BYTES = 16_384


class SchemaViolation(GuardBlocked):
    """A tool's arguments did not match its declared schema."""

    def __init__(self, reason: str, *, tool_name: str | None = None) -> None:
        super().__init__("schema", reason, tool_name=tool_name)


class EchoArgs(BaseModel):
    """Arguments for the ``echo`` demo tool: a single string ``input``."""

    model_config = ConfigDict(extra="forbid")

    input: str = Field(max_length=MAX_STRING_CHARS)


class HttpGetArgs(BaseModel):
    """Arguments for the guarded ``http_get`` tool: a single string ``url``."""

    model_config = ConfigDict(extra="forbid")

    url: str = Field(max_length=MAX_STRING_CHARS)


_MODELS: dict[str, type[BaseModel]] = {
    "echo": EchoArgs,
    "http_get": HttpGetArgs,
}


def check(tool_name: str, arguments: Mapping[str, Any]) -> None:
    """Validate ``arguments`` for ``tool_name``; raise :class:`SchemaViolation` on any problem."""
    try:
        raw = json.dumps(dict(arguments), ensure_ascii=False, default=str)
    except (TypeError, ValueError) as exc:
        raise SchemaViolation(
            f"arguments are not JSON-serializable: {exc}", tool_name=tool_name
        ) from exc
    if len(raw.encode("utf-8")) > MAX_ARGUMENTS_BYTES:
        raise SchemaViolation(
            f"arguments exceed the {MAX_ARGUMENTS_BYTES}-byte ceiling", tool_name=tool_name
        )

    model = _MODELS.get(tool_name)
    if model is None:
        return  # No declared schema; other guards still apply.
    try:
        model.model_validate(dict(arguments))
    except ValidationError as exc:
        errors = "; ".join(
            f"{'.'.join(str(p) for p in err['loc']) or '(root)'}: {err['msg']}"
            for err in exc.errors()
        )
        raise SchemaViolation(errors, tool_name=tool_name) from exc
