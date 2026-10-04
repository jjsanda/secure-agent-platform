"""The LLM port and its deterministic implementations.

:mod:`sap_worker.llm.base` defines the small, structured decision surface the
agent core depends on (``plan`` + ``decide_action``). :mod:`sap_worker.llm.mock`
is the deterministic, no-API-key default. A future ``ClaudeLLM`` (behind the
``anthropic`` extra) drops in behind the same port.
"""

from __future__ import annotations

from sap_worker.llm.base import LLM, Action, FinalAnswer, Observation, ToolCall
from sap_worker.llm.mock import MockLLM

__all__ = ["LLM", "Action", "FinalAnswer", "Observation", "ToolCall", "MockLLM"]
