"""Worker-side security guard — defense-in-depth in front of every tool call.

The Go tool proxy is the **authoritative** enforcement point (it re-validates the
scoped credential, the allow-list, and its own policy guard before any tool
runs). This package is a *second*, worker-local line of defense so the worker
never even attempts an out-of-scope or obviously-unsafe call.

* :mod:`~sap_worker.guard.allowlist`  — the tool is in the run's ``allowed_tools``.
* :mod:`~sap_worker.guard.schema`     — arguments match the tool's declared schema.
* :mod:`~sap_worker.guard.ssrf`       — no private/loopback/link-local/metadata egress.
* :mod:`~sap_worker.guard.paths`      — file-ish arguments stay inside the sandbox root.
* :mod:`~sap_worker.guard.injection`  — prompt-injection heuristics on objectives/outputs.

:func:`enforce_tool_arguments` runs the argument-level guards (schema → SSRF →
paths) as one call; the agent core invokes it after the allow-list check and
before dispatching to the proxy.
"""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from sap_worker.exceptions import GuardBlocked
from sap_worker.guard import injection, paths, schema, ssrf
from sap_worker.guard.allowlist import ToolNotAllowed, ensure_allowed
from sap_worker.guard.injection import InjectionVerdict
from sap_worker.guard.paths import PathTraversal
from sap_worker.guard.schema import SchemaViolation
from sap_worker.guard.ssrf import SsrfBlocked

__all__ = [
    "GuardBlocked",
    "ToolNotAllowed",
    "SchemaViolation",
    "SsrfBlocked",
    "PathTraversal",
    "InjectionVerdict",
    "ensure_allowed",
    "enforce_tool_arguments",
    "injection",
    "schema",
    "ssrf",
    "paths",
]


def enforce_tool_arguments(
    tool_name: str,
    arguments: Mapping[str, Any],
    *,
    sandbox_root: str | None = None,
) -> None:
    """Run the argument-level guards for a pending tool call.

    Order matters: validate the schema first (cheap, rejects malformed/oversized
    args), then the SSRF check on URL arguments, then path confinement on file
    arguments. Any guard raises a :class:`~sap_worker.exceptions.GuardBlocked`
    subclass, which the agent core maps to a ``RunError`` + ``FAILED`` trace.
    """
    schema.check(tool_name, arguments)
    ssrf.check(tool_name, arguments)
    paths.check(
        tool_name,
        arguments,
        root=sandbox_root if sandbox_root is not None else paths.DEFAULT_SANDBOX_ROOT,
    )
