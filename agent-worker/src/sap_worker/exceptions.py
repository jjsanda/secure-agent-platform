"""Typed exceptions for the agent worker.

A small, explicit hierarchy lets the agent core and the gRPC layer distinguish a
misconfiguration from a guardrail trip from a transient transport failure, and
map each cleanly onto a ``RunError`` event.
"""

from __future__ import annotations

__all__ = [
    "SapWorkerError",
    "ConfigError",
    "ToolProxyError",
    "ToolExecutionFailed",
    "GuardBlocked",
]


class SapWorkerError(Exception):
    """Base class for every error raised by ``sap_worker``."""


class ConfigError(SapWorkerError):
    """Invalid or insufficient configuration (e.g. a provider selected without its extra)."""


class GuardBlocked(SapWorkerError):
    """A worker-side security guard rejected a tool call before it left the worker.

    Raised by the SSRF, path-traversal, and argument-schema guards. Carries the
    ``guard`` that fired and a human-readable ``reason`` so the agent core can
    surface both in the run's ``RunError`` trace. The Go tool proxy remains the
    authoritative gate; this is defense-in-depth that fails fast worker-side.
    """

    def __init__(self, guard: str, reason: str, *, tool_name: str | None = None) -> None:
        self.guard = guard
        self.reason = reason
        self.tool_name = tool_name
        where = f" for tool {tool_name!r}" if tool_name else ""
        super().__init__(f"{guard} guard blocked the call{where}: {reason}")


class ToolProxyError(SapWorkerError):
    """The tool proxy was unreachable (transport failure) after retries.

    This is distinct from a *policy* denial, which the proxy returns as a normal
    :class:`~sap_worker.toolproxy_client.ToolResult` with ``ok=False`` and an
    error code — never as a transport error, and never retried.
    """


class ToolExecutionFailed(SapWorkerError):
    """The tool proxy executed but returned a failure (denial, invalid args, tool error).

    Carries the frozen ``ErrorCode`` name and the proxy's message so the agent
    can surface both in the run trace.
    """

    def __init__(self, code: str, message: str) -> None:
        self.code = code
        self.message = message
        super().__init__(f"{code}: {message}")
