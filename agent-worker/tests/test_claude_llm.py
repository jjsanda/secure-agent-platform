"""Unit-test the Claude engine with the anthropic client mocked.

There is no API key in this environment, so the real path cannot run; these tests
mock ``client.messages.create`` to prove the engine (a) maps a ``tool_use`` block
to a ToolCall and text to a FinalAnswer, (b) drives exactly one tool-proxy call
per ``tool_use`` when wired into the agent core, and (c) sends the correct
model-specific request parameters (adaptive thinking + effort on sonnet-5/opus-4-8,
neither on haiku-4-5, and never ``budget_tokens``).
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from types import SimpleNamespace
from typing import Any

import pytest

from sap_worker.core.agent import Agent
from sap_worker.gen.sap.v1 import runner_pb2
from sap_worker.llm.base import LLM, FinalAnswer, ToolCall
from sap_worker.llm.claude import ClaudeLLM
from sap_worker.toolproxy_client import ToolResult

pytestmark = pytest.mark.unit


def _text_block(text: str) -> SimpleNamespace:
    return SimpleNamespace(type="text", text=text)


def _tool_use_block(name: str, tool_input: dict[str, Any]) -> SimpleNamespace:
    return SimpleNamespace(type="tool_use", name=name, input=tool_input)


def _response(content: list[SimpleNamespace], stop_reason: str) -> SimpleNamespace:
    return SimpleNamespace(
        content=content,
        stop_reason=stop_reason,
        usage=SimpleNamespace(input_tokens=11, output_tokens=7),
    )


class FakeMessages:
    def __init__(self, responses: list[SimpleNamespace]) -> None:
        self._responses = list(responses)
        self.calls: list[dict[str, Any]] = []

    def create(self, **kwargs: Any) -> SimpleNamespace:
        self.calls.append(kwargs)
        return self._responses.pop(0)


class FakeAnthropic:
    def __init__(self, responses: list[SimpleNamespace]) -> None:
        self.messages = FakeMessages(responses)


class FakeToolProxy:
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
        self.calls.append({"tool_name": tool_name, "arguments": dict(arguments)})
        return ToolResult(ok=True, output={"echoed": dict(arguments)}, detail="ok")


def _request(
    objective: str = "say hi", allowed: Sequence[str] = ("echo",)
) -> runner_pb2.RunTaskRequest:
    return runner_pb2.RunTaskRequest(
        run_id="11111111-1111-1111-1111-111111111111",
        tenant_id="22222222-2222-2222-2222-222222222222",
        objective=objective,
        allowed_tools=list(allowed),
        scoped_credential="paseto-token-abc",
        max_steps=8,
    )


def test_conforms_to_llm_port() -> None:
    llm = ClaudeLLM(client=FakeAnthropic([]))
    assert isinstance(llm, LLM)
    assert llm.engine == "anthropic"
    assert llm.model == "claude-sonnet-5"  # default


def test_decide_action_maps_tool_use_to_toolcall() -> None:
    fake = FakeAnthropic([_response([_tool_use_block("echo", {"input": "hi"})], "tool_use")])
    llm = ClaudeLLM(client=fake)
    action = llm.decide_action("say hi", ["echo"], [])
    assert isinstance(action, ToolCall)
    assert action.tool == "echo"
    assert action.arguments == {"input": "hi"}


def test_decide_action_maps_text_to_final_answer() -> None:
    fake = FakeAnthropic([_response([_text_block("All done.")], "end_turn")])
    llm = ClaudeLLM(client=fake)
    action = llm.decide_action("say hi", ["echo"], [])
    assert isinstance(action, FinalAnswer)
    assert action.answer == "All done."


def test_decide_action_disables_parallel_tool_use() -> None:
    # Parallel tool use is on by default; the port takes one action per step, so
    # decide_action must ask Claude for at most one tool_use per turn.
    fake = FakeAnthropic([_response([_tool_use_block("echo", {"input": "hi"})], "tool_use")])
    llm = ClaudeLLM(client=fake)
    llm.decide_action("say hi", ["echo"], [])
    kwargs = fake.messages.calls[0]
    assert kwargs["tool_choice"] == {"type": "auto", "disable_parallel_tool_use": True}


def test_tool_use_block_drives_exactly_one_proxy_call() -> None:
    # plan() -> text; decide() -> tool_use (one proxy call); decide() -> text (final).
    fake = FakeAnthropic(
        [
            _response([_text_block("1. understand\n2. echo\n3. answer")], "end_turn"),
            _response([_tool_use_block("echo", {"input": "say hi"})], "tool_use"),
            _response([_text_block("Done: echoed the input.")], "end_turn"),
        ]
    )
    proxy = FakeToolProxy()
    agent = Agent(llm=ClaudeLLM(client=fake), tool_proxy=proxy)

    events = list(agent.run(_request()))

    assert len(proxy.calls) == 1
    assert proxy.calls[0]["tool_name"] == "echo"
    assert proxy.calls[0]["arguments"] == {"input": "say hi"}
    assert events[-1].status.status == runner_pb2.RUN_STATUS_SUCCEEDED
    final = next(e for e in events if e.WhichOneof("event") == "final")
    assert "echoed" in final.final.answer or "Done" in final.final.answer


@pytest.mark.parametrize("model", ["claude-sonnet-5", "claude-opus-4-8"])
def test_thinking_and_effort_sent_for_thinking_models(model: str) -> None:
    fake = FakeAnthropic([_response([_text_block("hi")], "end_turn")])
    llm = ClaudeLLM(model=model, effort="high", client=fake)
    llm.decide_action("obj", ["echo"], [])
    kwargs = fake.messages.calls[0]
    assert kwargs["thinking"] == {"type": "adaptive"}
    assert kwargs["output_config"] == {"effort": "high"}
    assert kwargs["model"] == model
    assert kwargs["max_tokens"] == 16000


def test_haiku_omits_thinking_and_effort() -> None:
    fake = FakeAnthropic([_response([_text_block("hi")], "end_turn")])
    llm = ClaudeLLM(model="claude-haiku-4-5", client=fake)
    llm.decide_action("obj", ["echo"], [])
    kwargs = fake.messages.calls[0]
    assert "thinking" not in kwargs
    assert "output_config" not in kwargs


def test_budget_tokens_is_never_sent() -> None:
    fake = FakeAnthropic(
        [
            _response([_text_block("hi")], "end_turn"),
            _response([_text_block("hi")], "end_turn"),
            _response([_text_block("hi")], "end_turn"),
        ]
    )
    llm = ClaudeLLM(model="claude-opus-4-8", client=fake)
    llm.plan("obj", ["echo"])
    llm.decide_action("obj", ["echo"], [])
    for kwargs in fake.messages.calls:
        assert "budget_tokens" not in kwargs
        assert "budget_tokens" not in kwargs.get("thinking", {})


def test_plan_parses_numbered_lines() -> None:
    fake = FakeAnthropic(
        [_response([_text_block("1. Understand it\n2. Call echo\n3. Answer")], "end_turn")]
    )
    llm = ClaudeLLM(client=fake)
    plan = llm.plan("do the thing", ["echo"])
    assert plan == ["Understand it", "Call echo", "Answer"]
    # plan() must not offer tools (planning should not trigger a tool call), so it
    # also carries no tool_choice.
    assert "tools" not in fake.messages.calls[0]
    assert "tool_choice" not in fake.messages.calls[0]


def test_factory_builds_claude_when_configured(monkeypatch: pytest.MonkeyPatch) -> None:
    # The factory creates a real anthropic.Anthropic() (which needs a key); patch it
    # so we can verify the wiring picks the Claude engine for LLM_ENGINE=anthropic.
    import anthropic

    monkeypatch.setattr(anthropic, "Anthropic", lambda *a, **k: FakeAnthropic([]))
    from sap_worker.config import Settings
    from sap_worker.factory import build_llm

    settings = Settings(_env_file=None, llm_engine="anthropic", llm_model="claude-opus-4-8")
    llm = build_llm(settings)
    assert isinstance(llm, ClaudeLLM)
    assert llm.model == "claude-opus-4-8"
