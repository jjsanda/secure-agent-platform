"""Path-traversal guard — confine file-ish tool arguments to a sandbox root.

Rejects file-ish tool arguments that escape their intended root via ``..``
segments, an absolute path, or a null byte, before the call is dispatched.
:func:`confine` returns the confined absolute path so a caller can operate on it
safely. Mirrors the authoritative ``ConfinePath`` check in the Go control plane;
this is worker-local defense-in-depth.

Note: the demo tool registry (``echo``, ``http_get``) exposes no filesystem tool
yet, so in the default run this guard is a no-op. It fires for any tool whose
arguments carry a path-shaped key, keeping the check ready for the next
filesystem-backed tool without another round-trip through the proxy.
"""

from __future__ import annotations

import posixpath
from collections.abc import Mapping
from pathlib import PurePosixPath
from typing import Any

from sap_worker.exceptions import GuardBlocked

__all__ = [
    "PATH_ARGUMENT_KEYS",
    "DEFAULT_SANDBOX_ROOT",
    "PathTraversal",
    "confine",
    "check",
]

# The nominal sandbox root a filesystem tool would confine reads/writes to.
DEFAULT_SANDBOX_ROOT = "/srv/sandbox"

# Argument keys whose string value is treated as a path and confined.
PATH_ARGUMENT_KEYS: frozenset[str] = frozenset(
    {"path", "file", "filepath", "file_path", "filename", "dir", "directory"}
)


class PathTraversal(GuardBlocked):
    """A path argument escaped the sandbox root (traversal, absolute, or null byte)."""

    def __init__(self, reason: str, *, tool_name: str | None = None) -> None:
        super().__init__("path", reason, tool_name=tool_name)


def confine(root: str, raw_path: str, *, tool_name: str | None = None) -> str:
    """Confine ``raw_path`` to ``root``; return the confined absolute path.

    Raises :class:`PathTraversal` on a null byte, an absolute path (the task's
    "absolute escape"), or any ``..`` traversal that leaves ``root``.
    """
    if "\x00" in raw_path:
        raise PathTraversal("null byte in path", tool_name=tool_name)
    if PurePosixPath(raw_path).is_absolute() or raw_path.startswith("\\"):
        raise PathTraversal(f"absolute path {raw_path!r} is not allowed", tool_name=tool_name)

    abs_root = posixpath.normpath(root)
    joined = posixpath.normpath(posixpath.join(abs_root, raw_path))
    if joined != abs_root and not joined.startswith(abs_root + "/"):
        raise PathTraversal(f"path {raw_path!r} escapes the sandbox root", tool_name=tool_name)
    return joined


def check(
    tool_name: str, arguments: Mapping[str, Any], *, root: str = DEFAULT_SANDBOX_ROOT
) -> None:
    """Confine every path-shaped argument of ``tool_name``; raise on the first bad one."""
    for key, value in arguments.items():
        if key in PATH_ARGUMENT_KEYS and isinstance(value, str):
            confine(root, value, tool_name=tool_name)
