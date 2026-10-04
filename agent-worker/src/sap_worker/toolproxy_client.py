"""gRPC client for the control-plane ``ToolProxyService``.

This is the worker's *only* outbound side-effect channel. Every tool call the
agent wants to make is forwarded here; the worker presents its short-lived scoped
credential as gRPC metadata and consumes whatever the proxy returns. The proxy is
the authoritative enforcement point — it validates the credential, re-checks the
allow-list, and runs its policy guard before any tool executes.

Contract details that must match the Go side exactly:

* the credential travels as metadata key ``x-scoped-credential`` (lowercase);
* arguments and outputs are ``google.protobuf.Struct`` on the wire;
* a *policy denial* comes back as a normal ``ExecuteToolResponse`` with the
  ``error`` oneof set (mapped to :class:`~sap_worker.exceptions.ToolExecutionFailed`
  by the caller) — it is **not** a transport error and is **never** retried.

Only transient transport failures (gRPC ``UNAVAILABLE``) are retried, with
bounded exponential backoff via ``tenacity``.
"""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any, Protocol, runtime_checkable

import grpc
from pydantic import BaseModel, ConfigDict, Field
from tenacity import Retrying, retry_if_exception, stop_after_attempt, wait_exponential

from sap_worker.exceptions import ToolProxyError
from sap_worker.gen.sap.v1 import toolproxy_pb2, toolproxy_pb2_grpc
from sap_worker.structpb import dict_to_struct, struct_to_dict

__all__ = [
    "CREDENTIAL_METADATA_KEY",
    "ToolResult",
    "ToolProxy",
    "ToolProxyClient",
    "credential_metadata",
]

# Frozen contract with the Go control plane — do not change casing.
CREDENTIAL_METADATA_KEY = "x-scoped-credential"


class ToolResult(BaseModel):
    """The worker-side view of an ``ExecuteToolResponse``.

    On success, ``output`` is the tool's result mapping. On failure, ``error_code``
    is the frozen ``ErrorCode`` enum name (e.g. ``"ERROR_CODE_OUT_OF_SCOPE"``) and
    ``detail`` carries the proxy's human-readable message.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    ok: bool
    output: dict[str, Any] = Field(default_factory=dict)
    error_code: str | None = None
    detail: str = ""


def credential_metadata(credential: str) -> list[tuple[str, str]]:
    """Assemble the gRPC metadata carrying the scoped credential."""
    return [(CREDENTIAL_METADATA_KEY, credential)]


@runtime_checkable
class ToolProxy(Protocol):
    """The outbound tool-execution port the agent core depends on."""

    def execute(
        self,
        run_id: str,
        tool_name: str,
        arguments: Mapping[str, Any],
        step: int,
        credential: str,
    ) -> ToolResult:
        """Execute a tool through the proxy and return the mapped result."""


def _is_transient(exc: BaseException) -> bool:
    """True only for a transient gRPC transport failure (``UNAVAILABLE``).

    Duck-typed on ``.code()`` so it matches ``grpc.RpcError`` and its private
    call-error subclasses without depending on grpc's (untyped) class objects.
    Policy denials never reach here — they are ordinary responses.
    """
    code_getter = getattr(exc, "code", None)
    if not callable(code_getter):
        return False
    return bool(code_getter() == grpc.StatusCode.UNAVAILABLE)


class ToolProxyClient:
    """Dials the tool proxy over an insecure (local plaintext) channel."""

    def __init__(
        self,
        addr: str,
        *,
        stub: Any | None = None,
        timeout_s: float = 10.0,
        max_attempts: int = 3,
    ) -> None:
        """Connect to ``addr``. Pass ``stub`` to inject a fake in tests."""
        self._addr = addr
        self._timeout_s = timeout_s
        if stub is not None:
            self._channel = None
            self._stub = stub
        else:
            self._channel = grpc.insecure_channel(addr)
            self._stub = toolproxy_pb2_grpc.ToolProxyServiceStub(self._channel)
        self._max_attempts = max_attempts

    def _make_retrying(self) -> Retrying:
        """Build a fresh retry controller for one ``execute`` call.

        ``tenacity.Retrying`` carries per-call mutable state, so the 8-thread server
        pool must not share a single instance; a new one per call keeps concurrent
        executes from racing on the retry statistics.
        """
        return Retrying(
            retry=retry_if_exception(_is_transient),
            stop=stop_after_attempt(self._max_attempts),
            wait=wait_exponential(multiplier=0.1, max=2.0),
            reraise=True,
        )

    def execute(
        self,
        run_id: str,
        tool_name: str,
        arguments: Mapping[str, Any],
        step: int,
        credential: str,
    ) -> ToolResult:
        """Call ``ExecuteTool`` with the credential in metadata; map the response."""
        request = toolproxy_pb2.ExecuteToolRequest(
            run_id=run_id,
            tool_name=tool_name,
            arguments=dict_to_struct(arguments),
            step=step,
        )
        metadata = credential_metadata(credential)
        try:
            response = self._make_retrying()(self._invoke, request, metadata)
        except grpc.RpcError as exc:  # transport failure after retries — not a policy denial
            code = exc.code() if callable(getattr(exc, "code", None)) else None
            raise ToolProxyError(
                f"tool proxy at {self._addr!r} unavailable for tool {tool_name!r}: {code}"
            ) from exc
        return _to_result(response)

    def _invoke(
        self,
        request: toolproxy_pb2.ExecuteToolRequest,
        metadata: list[tuple[str, str]],
    ) -> toolproxy_pb2.ExecuteToolResponse:
        response: toolproxy_pb2.ExecuteToolResponse = self._stub.ExecuteTool(
            request, metadata=metadata, timeout=self._timeout_s
        )
        return response

    def close(self) -> None:
        """Close the underlying channel (no-op when a stub was injected)."""
        if self._channel is not None:
            self._channel.close()


def _error_code_name(code: int) -> str:
    """Frozen ``ErrorCode`` name for ``code``, tolerating an unknown wire value.

    A newer proxy may send an ``ErrorCode`` these stubs don't define; ``.Name`` would
    raise ``ValueError`` on it, so map any out-of-range value to the unspecified
    code rather than crash the whole run.
    """
    if code in toolproxy_pb2.ErrorCode.values():
        return str(toolproxy_pb2.ErrorCode.Name(code))
    return "ERROR_CODE_UNSPECIFIED"


def _to_result(response: toolproxy_pb2.ExecuteToolResponse) -> ToolResult:
    """Map the ``ExecuteToolResponse`` oneof to a :class:`ToolResult`."""
    which = response.WhichOneof("result")
    if which == "ok":
        output = struct_to_dict(response.ok.output)
        sha = response.ok.output_sha256
        detail = f"ok · sha256={sha[:12]}" if sha else "ok"
        return ToolResult(ok=True, output=output, detail=detail)
    if which == "error":
        return ToolResult(
            ok=False,
            error_code=_error_code_name(response.error.code),
            detail=response.error.message,
        )
    # Neither oneof arm set — treat as a malformed/unspecified failure.
    return ToolResult(
        ok=False,
        error_code="ERROR_CODE_UNSPECIFIED",
        detail="tool proxy returned an empty result",
    )
