"""The deterministic, no-API-key default engine.

:class:`MockLLM` makes the whole worker reproducible offline: the same objective
and allowed-tools list always yield the same plan, the same tool call, and the
same final answer. It is intentionally trivial — a demo brain, not a model:

* ``plan`` returns a fixed 3-step plan referencing the objective and first tool.
* ``decide_action`` calls ``allowed_tools[0]`` once (arguments ``{"input":
  objective}``), then, once it has observed a result, returns a final answer that
  references that tool result. With no allowed tools it answers directly.

This mirrors the P2 demo, whose single tool is ``echo``.
"""

from __future__ import annotations

import json
from collections.abc import Sequence

from sap_worker.llm.base import Action, FinalAnswer, Observation, ToolCall

__all__ = ["MockLLM"]

_MAX_DETAIL_CHARS = 200


def _compact(output: dict[str, object]) -> str:
    """Deterministic, compact rendering of a tool output for the final answer."""
    text = json.dumps(output, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    if len(text) > _MAX_DETAIL_CHARS:
        text = text[:_MAX_DETAIL_CHARS] + "..."
    return text


class MockLLM:
    """A pure, reproducible engine — same input in, same decisions out, everywhere."""

    engine = "mock"

    def __init__(self, model: str = "mock-deterministic") -> None:
        self._model = model

    @property
    def model(self) -> str:
        return self._model

    def plan(self, objective: str, allowed_tools: Sequence[str]) -> list[str]:
        objective = objective.strip()
        if allowed_tools:
            tool = allowed_tools[0]
            return [
                f"Understand the objective: {objective}",
                f"Call the {tool!r} tool with the objective as input",
                "Summarize the tool result into a final answer",
            ]
        return [
            f"Understand the objective: {objective}",
            "Reason about it directly (no tools are permitted for this run)",
            "Produce a final answer",
        ]

    def decide_action(
        self,
        objective: str,
        allowed_tools: Sequence[str],
        observations: Sequence[Observation],
    ) -> Action:
        objective = objective.strip()

        # First move: if a tool is available and we have not run one yet, call it.
        if allowed_tools and not observations:
            return ToolCall(tool=allowed_tools[0], arguments={"input": objective})

        # We have a tool result: answer, referencing it.
        if observations:
            last = observations[-1]
            return FinalAnswer(
                answer=(
                    f"Objective addressed via the {last.tool!r} tool. "
                    f"Tool result: {_compact(last.output)}."
                )
            )

        # No tools were permitted: answer directly.
        return FinalAnswer(answer=f"{objective} — answered directly (no tools permitted).")
