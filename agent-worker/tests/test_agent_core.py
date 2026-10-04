"""Drive the agent core with a fake tool proxy and assert the RunEvent trace."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from datetime import UTC, datetime
from typing import Any

import pytest

from sap_worker.core.agent import Agent
from sap_worker.gen.sap.v1 import runner_pb2
from sap_worker.llm.base import Action, Observation, ToolCall
from sap_worker.llm.mock import MockLLM
from sap_worker.toolproxy_client import ToolResult

pytestmark = pytest.mark.unit


def _fixed_clock() -> datetime:
    return datetime(2026, 7, 1, 12, 0, 0, tzinfo=UTC)


class FakeToolProxy:
    """Records calls and returns a canned result (OK by default)."""

    def __init__(self, result: ToolResult | None = None) -> None:
        self.calls: list[dict[str, Any]] = []
        self._result = result or ToolResult(
            ok=True, output={"echoed": {"input": "hi"}}, detail="ok · sha256=abc123"
        )

    def execute(
        self,
        run_id: str,
        tool_name: str,
        arguments: Mapping[str, Any],
        step: int,
        credential: str,
    ) -> ToolResult:
        self.calls.append(
            {
                "run_id": run_id,
                "tool_name": tool_name,
                "arguments": dict(arguments),
                "step": step,
                "credential": credential,
            }
        )
        return self._result


class ForbiddenToolLLM:
    """An LLM that insists on calling a tool outside the allow-list."""

    engine = "test"
    model = "test"

    def plan(self, objective: str, allowed_tools: Sequence[str]) -> list[str]:
        return ["look", "act", "answer"]

    def decide_action(
        self, objective: str, allowed_tools: Sequence[str], observations: Sequence[Observation]
    ) -> Action:
        return ToolCall(tool="danger", arguments={"payload": "x"})


def _kinds(events: list[runner_pb2.RunEvent]) -> list[str]:
    return [e.WhichOneof("event") for e in events]


def _statuses(events: list[runner_pb2.RunEvent]) -> list[int]:
    return [e.status.status for e in events if e.WhichOneof("event") == "status"]


def _request(**overrides: Any) -> runner_pb2.RunTaskRequest:
    params: dict[str, Any] = {
        "run_id": "11111111-1111-1111-1111-111111111111",
        "tenant_id": "22222222-2222-2222-2222-222222222222",
        "objective": "say hi",
        "allowed_tools": ["echo"],
        "scoped_credential": "paseto-token-abc",
        "max_steps": 8,
    }
    params.update(overrides)
    return runner_pb2.RunTaskRequest(**params)


def test_happy_path_event_sequence() -> None:
    fake = FakeToolProxy()
    agent = Agent(llm=MockLLM(), tool_proxy=fake, clock=_fixed_clock)

    events = list(agent.run(_request()))

    assert _kinds(events) == [
        "status",  # PLANNING
        "plan",
        "llm_message",
        "status",  # ACTING
        "tool_requested",
        "tool_result",
        "status",  # OBSERVING
        "final",
        "status",  # SUCCEEDED
    ]
    assert _statuses(events) == [
        runner_pb2.RUN_STATUS_PLANNING,
        runner_pb2.RUN_STATUS_ACTING,
        runner_pb2.RUN_STATUS_OBSERVING,
        runner_pb2.RUN_STATUS_SUCCEEDED,
    ]


def test_every_event_is_stamped() -> None:
    agent = Agent(llm=MockLLM(), tool_proxy=FakeToolProxy(), clock=_fixed_clock)
    events = list(agent.run(_request()))

    assert all(e.HasField("at") for e in events)
    steps = [e.step for e in events]
    assert steps == sorted(steps)  # monotonic non-decreasing
    assert steps[0] == 0  # planning phase


def test_tool_call_is_forwarded_with_credential_and_args() -> None:
    fake = FakeToolProxy()
    agent = Agent(llm=MockLLM(), tool_proxy=fake, clock=_fixed_clock)

    list(agent.run(_request(objective="do a thing")))

    assert len(fake.calls) == 1
    call = fake.calls[0]
    assert call["tool_name"] == "echo"
    assert call["arguments"] == {"input": "do a thing"}
    assert call["credential"] == "paseto-token-abc"
    assert call["step"] == 1


def test_final_answer_references_tool_result() -> None:
    agent = Agent(llm=MockLLM(), tool_proxy=FakeToolProxy(), clock=_fixed_clock)
    events = list(agent.run(_request()))

    final = next(e for e in events if e.WhichOneof("event") == "final")
    assert "echo" in final.final.answer


def test_denial_path_emits_run_error_then_failed() -> None:
    fake = FakeToolProxy()
    agent = Agent(llm=ForbiddenToolLLM(), tool_proxy=fake, clock=_fixed_clock)

    events = list(agent.run(_request(allowed_tools=["echo"])))

    assert _kinds(events) == [
        "status",  # PLANNING
        "plan",
        "llm_message",
        "status",  # ACTING
        "tool_requested",
        "error",
        "status",  # FAILED
    ]
    assert events[-1].status.status == runner_pb2.RUN_STATUS_FAILED
    error = next(e for e in events if e.WhichOneof("event") == "error")
    assert "danger" in error.error.message
    assert fake.calls == []  # guard denied before any proxy call


def test_tool_failure_from_proxy_fails_the_run() -> None:
    fake = FakeToolProxy(
        ToolResult(ok=False, error_code="ERROR_CODE_POLICY_BLOCKED", detail="blocked by policy")
    )
    agent = Agent(llm=MockLLM(), tool_proxy=fake, clock=_fixed_clock)

    events = list(agent.run(_request()))

    assert _kinds(events) == [
        "status",  # PLANNING
        "plan",
        "llm_message",
        "status",  # ACTING
        "tool_requested",
        "tool_result",  # ok=False
        "error",
        "status",  # FAILED
    ]
    tool_result = next(e for e in events if e.WhichOneof("event") == "tool_result")
    assert tool_result.tool_result.ok is False
    assert events[-1].status.status == runner_pb2.RUN_STATUS_FAILED
    error = next(e for e in events if e.WhichOneof("event") == "error")
    assert "POLICY_BLOCKED" in error.error.message


def test_max_steps_is_respected() -> None:
    # max_steps=1 permits exactly one tool call; the run still finalizes cleanly.
    fake = FakeToolProxy()
    agent = Agent(llm=MockLLM(), tool_proxy=fake, clock=_fixed_clock)

    events = list(agent.run(_request(max_steps=1)))

    assert len(fake.calls) == 1
    assert events[-1].status.status == runner_pb2.RUN_STATUS_SUCCEEDED
