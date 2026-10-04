"""Determinism and plan/action shape of the deterministic MockLLM."""

from __future__ import annotations

import pytest

from sap_worker.llm.base import LLM, FinalAnswer, Observation, ToolCall
from sap_worker.llm.mock import MockLLM

pytestmark = pytest.mark.unit


def test_mock_conforms_to_llm_port() -> None:
    llm = MockLLM()
    assert isinstance(llm, LLM)
    assert llm.engine == "mock"
    assert isinstance(llm.model, str)


def test_plan_is_three_steps() -> None:
    plan = MockLLM().plan("summarize the incident", ["echo"])
    assert isinstance(plan, list)
    assert len(plan) == 3
    assert all(isinstance(step, str) and step for step in plan)


def test_plan_is_deterministic() -> None:
    llm = MockLLM()
    assert llm.plan("obj", ["echo", "search"]) == llm.plan("obj", ["echo", "search"])


def test_decide_action_calls_first_allowed_tool() -> None:
    action = MockLLM().decide_action("do the thing", ["echo", "search"], [])
    assert isinstance(action, ToolCall)
    assert action.tool == "echo"
    assert action.arguments == {"input": "do the thing"}


def test_decide_action_finalizes_after_observation() -> None:
    obs = [Observation(tool="echo", ok=True, output={"echoed": {"input": "hi"}}, detail="ok")]
    action = MockLLM().decide_action("hi", ["echo"], obs)
    assert isinstance(action, FinalAnswer)
    assert "echo" in action.answer
    assert "echoed" in action.answer  # references the tool result


def test_decide_action_answers_directly_without_tools() -> None:
    action = MockLLM().decide_action("what is 2+2", [], [])
    assert isinstance(action, FinalAnswer)
    assert "what is 2+2" in action.answer


def test_decide_action_is_deterministic() -> None:
    llm = MockLLM()
    obs = [Observation(tool="echo", ok=True, output={"k": "v"}, detail="ok")]
    assert llm.decide_action("obj", ["echo"], obs) == llm.decide_action("obj", ["echo"], obs)
