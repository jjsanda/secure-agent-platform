"""The LangGraph variant must produce an event trace equivalent to the custom core.

Both cores are driven by the same deterministic ``MockLLM`` and the same fake
tool proxy, so their ``RunEvent`` traces — kinds, statuses, step numbers, tool
arguments, tool-result details, final answer, and the sequence of proxy calls —
must match exactly. (See ``graph/agent.py``: the only intentional difference is
that LangGraph materializes the whole trace before yielding it.)
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from datetime import UTC, datetime
from typing import Any

import pytest

from sap_worker.core.agent import Agent
from sap_worker.core.steps import AgentRunner
from sap_worker.gen.sap.v1 import runner_pb2
from sap_worker.graph import LangGraphAgent
from sap_worker.llm.base import Action, Observation, ToolCall
from sap_worker.llm.mock import MockLLM
from sap_worker.structpb import struct_to_dict
from sap_worker.toolproxy_client import ToolResult

pytestmark = pytest.mark.unit


def _fixed_clock() -> datetime:
    return datetime(2026, 7, 1, 12, 0, 0, tzinfo=UTC)


class FakeToolProxy:
    def __init__(self, result: ToolResult | None = None) -> None:
        self.calls: list[dict[str, Any]] = []
        self._result = result or ToolResult(
            ok=True, output={"echoed": {"input": "hi"}}, detail="ok"
        )

    def execute(
        self,
        run_id: str,
        tool_name: str,
        arguments: Mapping[str, Any],
        step: int,
        credential: str,
    ) -> ToolResult:
        self.calls.append({"tool_name": tool_name, "arguments": dict(arguments), "step": step})
        return self._result


class ForbiddenToolLLM:
    engine = "test"
    model = "test"

    def plan(self, objective: str, allowed_tools: Sequence[str]) -> list[str]:
        return ["look", "act", "answer"]

    def decide_action(
        self, objective: str, allowed_tools: Sequence[str], observations: Sequence[Observation]
    ) -> Action:
        return ToolCall(tool="danger", arguments={"payload": "x"})


class RaisingToolProxy:
    """A proxy whose execute raises a *non*-``SapWorkerError`` (an unexpected fault)."""

    def __init__(self) -> None:
        self.calls: list[dict[str, Any]] = []

    def execute(
        self,
        run_id: str,
        tool_name: str,
        arguments: Mapping[str, Any],
        step: int,
        credential: str,
    ) -> ToolResult:
        self.calls.append({"tool_name": tool_name, "arguments": dict(arguments), "step": step})
        raise RuntimeError("tool proxy exploded")


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


def _signature(events: Sequence[runner_pb2.RunEvent], proxy: FakeToolProxy) -> dict[str, Any]:
    return {
        "kinds": [e.WhichOneof("event") for e in events],
        "statuses": [e.status.status for e in events if e.WhichOneof("event") == "status"],
        "steps": [e.step for e in events],
        "final": next((e.final.answer for e in events if e.WhichOneof("event") == "final"), None),
        "error": next((e.error.message for e in events if e.WhichOneof("event") == "error"), None),
        "tool_args": [
            struct_to_dict(e.tool_requested.arguments)
            for e in events
            if e.WhichOneof("event") == "tool_requested"
        ],
        "tool_details": [
            e.tool_result.detail for e in events if e.WhichOneof("event") == "tool_result"
        ],
        "calls": proxy.calls,
    }


def _run(
    agent: AgentRunner, proxy: FakeToolProxy, request: runner_pb2.RunTaskRequest
) -> dict[str, Any]:
    events = list(agent.run(request))
    return _signature(events, proxy)


def _both(
    *, llm_factory: Any, result: ToolResult | None, request: runner_pb2.RunTaskRequest
) -> tuple[dict[str, Any], dict[str, Any]]:
    custom_proxy = FakeToolProxy(result)
    custom = Agent(llm=llm_factory(), tool_proxy=custom_proxy, clock=_fixed_clock)
    graph_proxy = FakeToolProxy(result)
    graph = LangGraphAgent(llm=llm_factory(), tool_proxy=graph_proxy, clock=_fixed_clock)
    return _run(custom, custom_proxy, request), _run(graph, graph_proxy, request)


def test_langgraph_smoke() -> None:
    proxy = FakeToolProxy()
    agent = LangGraphAgent(llm=MockLLM(), tool_proxy=proxy, clock=_fixed_clock)
    events = list(agent.run(_request()))
    kinds = [e.WhichOneof("event") for e in events]
    assert kinds == [
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
    assert events[-1].status.status == runner_pb2.RUN_STATUS_SUCCEEDED


def test_happy_path_traces_are_equivalent() -> None:
    custom_sig, graph_sig = _both(llm_factory=MockLLM, result=None, request=_request())
    assert custom_sig == graph_sig


def test_denial_path_traces_are_equivalent() -> None:
    custom_sig, graph_sig = _both(
        llm_factory=ForbiddenToolLLM, result=None, request=_request(allowed_tools=["echo"])
    )
    assert custom_sig == graph_sig
    assert graph_sig["kinds"][-1] == "status"
    assert "danger" in (graph_sig["error"] or "")
    assert graph_sig["calls"] == []  # guard denied before any proxy call


def test_tool_failure_traces_are_equivalent() -> None:
    result = ToolResult(
        ok=False, error_code="ERROR_CODE_POLICY_BLOCKED", detail="blocked by policy"
    )
    custom_sig, graph_sig = _both(llm_factory=MockLLM, result=result, request=_request())
    assert custom_sig == graph_sig
    assert "POLICY_BLOCKED" in (graph_sig["error"] or "")


def test_max_steps_traces_are_equivalent() -> None:
    custom_sig, graph_sig = _both(llm_factory=MockLLM, result=None, request=_request(max_steps=1))
    assert custom_sig == graph_sig
    # Exactly one tool call, then a clean finish.
    assert len(graph_sig["calls"]) == 1
    assert graph_sig["statuses"][-1] == runner_pb2.RUN_STATUS_SUCCEEDED


def test_unexpected_error_traces_are_equivalent() -> None:
    # A non-SapWorkerError escaping the tool act must NOT collapse the LangGraph
    # trace to error@0/FAILED@0 (dropping the whole prior trace). Both cores stream
    # what happened, then fail: the RunError/FAILED land on the acting step.
    custom_proxy = RaisingToolProxy()
    custom = Agent(llm=MockLLM(), tool_proxy=custom_proxy, clock=_fixed_clock)
    graph_proxy = RaisingToolProxy()
    graph = LangGraphAgent(llm=MockLLM(), tool_proxy=graph_proxy, clock=_fixed_clock)

    custom_sig = _run(custom, custom_proxy, _request())
    graph_sig = _run(graph, graph_proxy, _request())

    assert custom_sig == graph_sig
    assert graph_sig["kinds"] == [
        "status",  # PLANNING
        "plan",
        "llm_message",
        "status",  # ACTING
        "tool_requested",
        "error",
        "status",  # FAILED
    ]
    assert graph_sig["statuses"][-1] == runner_pb2.RUN_STATUS_FAILED
    assert "exploded" in (graph_sig["error"] or "")
    # The error/FAILED are attributed to the acting step (1), never step 0.
    kinds, steps = graph_sig["kinds"], graph_sig["steps"]
    assert steps[kinds.index("error")] == 1
    assert steps[-1] == 1
