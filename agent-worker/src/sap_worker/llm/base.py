"""The LLM port: a small, structured decision surface for the agent core.

Instead of exposing a raw "messages" loop, the engine offers exactly the two
decisions the plan/act/observe loop makes:

* :meth:`LLM.plan` — turn an objective (and the tools this run may call) into a
  short, human-readable plan.
* :meth:`LLM.decide_action` — given the objective, the allowed tools, and the
  observations gathered so far, decide the next :data:`Action`: either a tool
  call or a final answer.

Both are synchronous and pure with respect to their inputs, which keeps the core
loop trivial to test. The agent is identical whether the engine behind this port
is the deterministic :class:`~sap_worker.llm.mock.MockLLM` or a future
``ClaudeLLM`` (next phase, behind the ``anthropic`` extra).
"""

from __future__ import annotations

from collections.abc import Sequence
from typing import Any, Protocol, runtime_checkable

from pydantic import BaseModel, ConfigDict, Field

__all__ = ["ToolCall", "FinalAnswer", "Action", "Observation", "LLM"]


class ToolCall(BaseModel):
    """The agent should call ``tool`` with ``arguments`` (a JSON-object mapping)."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    tool: str
    arguments: dict[str, Any] = Field(default_factory=dict)


class FinalAnswer(BaseModel):
    """The agent is done; ``answer`` is the response to the objective."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    answer: str


# The next action is exactly one of: run a tool, or answer.
Action = ToolCall | FinalAnswer


class Observation(BaseModel):
    """The result of a completed tool call, fed back into :meth:`LLM.decide_action`."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    tool: str
    ok: bool
    output: dict[str, Any] = Field(default_factory=dict)
    detail: str = ""


@runtime_checkable
class LLM(Protocol):
    """The structured-LLM port the agent core depends on."""

    @property
    def engine(self) -> str:
        """Short engine id for traces, e.g. ``"mock"`` or ``"anthropic"``."""

    @property
    def model(self) -> str:
        """Model id for traces (ignored by the mock engine)."""

    def plan(self, objective: str, allowed_tools: Sequence[str]) -> list[str]:
        """Produce a short, ordered, human-readable plan for the objective."""

    def decide_action(
        self,
        objective: str,
        allowed_tools: Sequence[str],
        observations: Sequence[Observation],
    ) -> Action:
        """Decide the next action given the objective and observations so far."""
