"""The RunnerService must honour a run's requested agent variant.

The worker builds one shared LLM engine + tool-proxy client and selects the agent
core per call from ``request.variant`` (falling back to the configured default for
``AGENT_VARIANT_UNSPECIFIED``), so choosing "langgraph" in the dashboard actually
runs the LangGraph variant. On shutdown the shared tool-proxy channel is closed.
"""

from __future__ import annotations

from collections.abc import Iterator, Mapping
from typing import Any

import pytest

from sap_worker.config import Settings
from sap_worker.core.agent import Agent
from sap_worker.factory import AgentSelector
from sap_worker.gen.sap.v1 import common_pb2, runner_pb2
from sap_worker.graph import LangGraphAgent
from sap_worker.grpc_server.server import RunnerService, make_server
from sap_worker.llm.mock import MockLLM
from sap_worker.toolproxy_client import ToolResult

pytestmark = pytest.mark.unit


class FakeToolProxy:
    """A tool proxy that owns no channel — it has no ``close()``."""

    def execute(
        self,
        run_id: str,
        tool_name: str,
        arguments: Mapping[str, Any],
        step: int,
        credential: str,
    ) -> ToolResult:
        return ToolResult(ok=True, output={"echoed": dict(arguments)}, detail="ok")


class ClosableToolProxy(FakeToolProxy):
    """A tool proxy that owns a channel — ``close()`` must be called on shutdown."""

    def __init__(self) -> None:
        self.closed = False

    def close(self) -> None:
        self.closed = True


class _SpyAgent:
    """Records that it ran and emits a single terminal event."""

    def __init__(self) -> None:
        self.ran = False

    def run(self, request: runner_pb2.RunTaskRequest) -> Iterator[runner_pb2.RunEvent]:
        self.ran = True
        yield runner_pb2.RunEvent(
            step=0,
            status=runner_pb2.StatusChange(status=runner_pb2.RUN_STATUS_SUCCEEDED),
        )


class _RecordingSelector:
    """A stand-in selector that records the variant RunTask asks it to resolve."""

    def __init__(self, agent: _SpyAgent) -> None:
        self._agent = agent
        self.seen: list[int] = []

    def for_variant(self, variant: int) -> _SpyAgent:
        self.seen.append(variant)
        return self._agent


def _settings(**overrides: Any) -> Settings:
    return Settings(_env_file=None, **overrides)


def _request(variant: int, **overrides: Any) -> runner_pb2.RunTaskRequest:
    params: dict[str, Any] = {
        "run_id": "11111111-1111-1111-1111-111111111111",
        "tenant_id": "22222222-2222-2222-2222-222222222222",
        "objective": "say hi",
        "allowed_tools": ["echo"],
        "scoped_credential": "paseto-token-abc",
        "variant": variant,
        "max_steps": 8,
    }
    params.update(overrides)
    return runner_pb2.RunTaskRequest(**params)


def _selector(**overrides: Any) -> AgentSelector:
    return AgentSelector(_settings(**overrides), llm=MockLLM(), tool_proxy=FakeToolProxy())


# --- variant -> agent resolution -----------------------------------------------------


def test_selector_maps_variant_to_agent_type() -> None:
    selector = _selector(agent_variant="custom")
    assert isinstance(selector.for_variant(common_pb2.AGENT_VARIANT_CUSTOM), Agent)
    assert isinstance(selector.for_variant(common_pb2.AGENT_VARIANT_LANGGRAPH), LangGraphAgent)


@pytest.mark.parametrize(
    ("default", "expected"),
    [("custom", Agent), ("langgraph", LangGraphAgent)],
)
def test_selector_unspecified_uses_configured_default(default: str, expected: type) -> None:
    selector = _selector(agent_variant=default)
    assert isinstance(selector.for_variant(common_pb2.AGENT_VARIANT_UNSPECIFIED), expected)


def test_selector_caches_one_instance_per_variant() -> None:
    selector = _selector(agent_variant="custom")
    first = selector.for_variant(common_pb2.AGENT_VARIANT_LANGGRAPH)
    again = selector.for_variant(common_pb2.AGENT_VARIANT_LANGGRAPH)
    other = selector.for_variant(common_pb2.AGENT_VARIANT_CUSTOM)
    assert first is again  # cached, not rebuilt each call
    assert other is not first


# --- RunTask honours the request's variant -------------------------------------------


def test_run_task_selects_agent_by_request_variant() -> None:
    spy = _SpyAgent()
    selector = _RecordingSelector(spy)
    service = RunnerService(selector)  # type: ignore[arg-type]

    events = list(service.RunTask(_request(common_pb2.AGENT_VARIANT_LANGGRAPH), None))

    # RunTask forwarded the request's variant to the selector and streamed its agent.
    assert selector.seen == [common_pb2.AGENT_VARIANT_LANGGRAPH]
    assert spy.ran is True
    assert events[-1].status.status == runner_pb2.RUN_STATUS_SUCCEEDED


def test_run_task_runs_langgraph_end_to_end() -> None:
    # End-to-end: a langgraph request resolves to a LangGraphAgent (asserted via the
    # cache) and produces a full successful trace through the real service path.
    selector = _selector(agent_variant="custom")
    resolved = selector.for_variant(common_pb2.AGENT_VARIANT_LANGGRAPH)
    service = RunnerService(selector)

    events = list(service.RunTask(_request(common_pb2.AGENT_VARIANT_LANGGRAPH), None))

    assert isinstance(resolved, LangGraphAgent)  # the cached instance RunTask reuses
    kinds = [e.WhichOneof("event") for e in events]
    assert kinds[-2:] == ["final", "status"]
    assert events[-1].status.status == runner_pb2.RUN_STATUS_SUCCEEDED


# --- shutdown closes the shared tool-proxy channel (fix: leaked channel) --------------


def test_selector_close_closes_the_tool_proxy() -> None:
    proxy = ClosableToolProxy()
    selector = AgentSelector(_settings(), llm=MockLLM(), tool_proxy=proxy)
    selector.close()
    assert proxy.closed is True


def test_selector_close_tolerates_proxy_without_close() -> None:
    # An injected fake proxy owns no channel and exposes no close(); shutdown must
    # not raise on it.
    selector = AgentSelector(_settings(), llm=MockLLM(), tool_proxy=FakeToolProxy())
    selector.close()  # no-op, no AttributeError


def test_make_server_returns_selector_for_shutdown() -> None:
    # make_server must hand back the selector so __main__ can close it on stop.
    selector = _selector(worker_grpc_listen=":0")
    server, port, returned = make_server(_settings(worker_grpc_listen=":0"), selector=selector)
    try:
        assert returned is selector
        assert port > 0  # bound an ephemeral port
    finally:
        server.stop(None)
