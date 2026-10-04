"""Allow-list guard: the tool must be in this run's ``allowed_tools``.

``allowed_tools`` mirrors the scoped credential's claim. The tool proxy re-checks
it server-side (and never trusts the worker alone), but checking here means the
worker fails fast and never sends an out-of-scope request over the wire.
"""

from __future__ import annotations

from collections.abc import Sequence

from sap_worker.exceptions import SapWorkerError

__all__ = ["ToolNotAllowed", "ensure_allowed"]


class ToolNotAllowed(SapWorkerError):
    """A tool call was attempted for a tool outside the run's allow-list."""

    def __init__(self, tool_name: str, allowed_tools: Sequence[str]) -> None:
        self.tool_name = tool_name
        self.allowed_tools = list(allowed_tools)
        allowed = ", ".join(self.allowed_tools) or "(none)"
        super().__init__(f"tool {tool_name!r} is not in the run's allow-list [{allowed}]")


def ensure_allowed(tool_name: str, allowed_tools: Sequence[str]) -> None:
    """Raise :class:`ToolNotAllowed` unless ``tool_name`` is permitted for this run."""
    if tool_name not in allowed_tools:
        raise ToolNotAllowed(tool_name, allowed_tools)
