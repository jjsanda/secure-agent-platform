"""Helpers to convert between ``dict`` and ``google.protobuf.Struct``.

Tool arguments and tool outputs travel on the wire as ``google.protobuf.Struct``
(see the ``ExecuteToolRequest.arguments`` / ``ToolOk.output`` fields and
``ToolCallRequested.arguments``). These two helpers are the single conversion
point, so the rest of the worker works with plain Python ``dict`` objects.

Note the JSON value model of ``Struct``: numbers are stored as ``double``, so an
``int`` round-trips as a ``float``. String keys, strings, booleans, ``None``,
nested objects, and lists round-trip faithfully.
"""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from google.protobuf import json_format
from google.protobuf.struct_pb2 import Struct

__all__ = ["dict_to_struct", "struct_to_dict"]


def dict_to_struct(data: Mapping[str, Any]) -> Struct:
    """Build a ``Struct`` from a plain mapping."""
    return json_format.ParseDict(dict(data), Struct())


def struct_to_dict(struct: Struct) -> dict[str, Any]:
    """Convert a ``Struct`` back to a plain ``dict``."""
    return json_format.MessageToDict(struct)
