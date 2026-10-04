"""A LangGraph ``StateGraph`` implementation of the plan -> act -> observe loop.

Selected by ``AGENT_VARIANT=langgraph`` (behind the ``langgraph`` extra), this is
a drop-in alternative to :class:`sap_worker.core.agent.Agent`: same constructor
shape, same ``run(request) -> Iterator[RunEvent]`` signature, and — driven by the
same ``LLM`` port, tool-proxy client, and shared guards — an *equivalent*
``RunEvent`` trace.

The graph nodes accumulate events into the graph state; because LangGraph runs a
graph to completion rather than yielding incrementally, :meth:`run` executes the
graph and then replays the accumulated event list. The trace order, step
numbering, guard behavior, and secret redaction are identical to the custom core
(they share :mod:`sap_worker.core.steps`), so the two variants are
interchangeable to the control plane. This is the one documented difference:
the LangGraph variant materializes the full trace before yielding it, whereas
the custom core streams each event as it is produced.
"""

from __future__ import annotations

import operator
from collections.abc import Iterator, Sequence
from typing import Annotated, Any, TypedDict

from langgraph.graph import END, START, StateGraph

from sap_worker.core.steps import (
    Clock,
    finalize_answer,
    plan_message,
    run_tool_step,
    to_timestamp,
)
from sap_worker.core.steps import default_clock as _default_clock
from sap_worker.gen.sap.v1 import runner_pb2
from sap_worker.guard import injection
from sap_worker.llm.base import LLM, FinalAnswer, Observation
from sap_worker.logging import get_logger
from sap_worker.structpb import dict_to_struct
from sap_worker.telemetry import get_tracer
from sap_worker.toolproxy_client import ToolProxy

__all__ = ["LangGraphAgent"]


class _State(TypedDict, total=False):
    """The graph's accumulating state. ``events`` / ``observations`` use an ``add``
    reducer so each node appends; the rest are last-write-wins."""

    events: Annotated[list[runner_pb2.RunEvent], operator.add]
    observations: Annotated[list[Observation], operator.add]
    objective: str
    allowed: list[str]
    credential: str
    run_id: str
    max_steps: int
    step: int
    pending_tool_name: str | None
    pending_tool_args: dict[str, Any] | None
    final_answer: str | None
    error: str | None


class LangGraphAgent:
    """The LangGraph variant. Reentrant: all run state lives in the graph state."""

    def __init__(
        self,
        *,
        llm: LLM,
        tool_proxy: ToolProxy,
        max_steps: int = 8,
        clock: Clock | None = None,
        logger: Any | None = None,
    ) -> None:
        self._llm = llm
        self._tool_proxy = tool_proxy
        self._default_max_steps = max_steps
        self._clock: Clock = clock or _default_clock
        self._log = logger or get_logger("sap_worker.graph")
        self._tracer = get_tracer("sap_worker.graph")
        self._graph = self._build_graph()

    # -- graph construction --------------------------------------------------
    def _build_graph(self) -> Any:
        graph = StateGraph(_State)
        graph.add_node("plan", self._plan_node)
        graph.add_node("act_status", self._act_status_node)
        graph.add_node("decide", self._decide_node)
        graph.add_node("act", self._act_node)
        graph.add_node("finalize", self._finalize_node)
        graph.add_node("fail", self._fail_node)

        graph.add_edge(START, "plan")
        graph.add_conditional_edges(
            "plan", self._route_after_plan, {"act_status": "act_status", "fail": "fail"}
        )
        graph.add_edge("act_status", "decide")
        graph.add_conditional_edges(
            "decide",
            self._route_after_decide,
            {"act": "act", "finalize": "finalize", "fail": "fail"},
        )
        graph.add_conditional_edges(
            "act", self._route_after_act, {"decide": "decide", "fail": "fail"}
        )
        graph.add_edge("finalize", END)
        graph.add_edge("fail", END)
        return graph.compile()

    # -- event helpers -------------------------------------------------------
    def _now(self) -> Any:
        return to_timestamp(self._clock)

    def _status(self, step: int, status: runner_pb2.RunStatus) -> runner_pb2.RunEvent:
        return runner_pb2.RunEvent(
            at=self._now(), step=step, status=runner_pb2.StatusChange(status=status)
        )

    def _genai_attrs(self, span: Any, operation: str) -> None:
        span.set_attribute("gen_ai.system", self._llm.engine)
        span.set_attribute("gen_ai.request.model", self._llm.model)
        span.set_attribute("gen_ai.operation.name", operation)

    # -- nodes ---------------------------------------------------------------
    def _plan_node(self, state: _State) -> _State:
        events: list[runner_pb2.RunEvent] = [self._status(0, runner_pb2.RUN_STATUS_PLANNING)]
        try:
            with self._tracer.start_as_current_span("llm.call") as span:
                self._genai_attrs(span, "plan")
                plan = self._llm.plan(state["objective"], state["allowed"])
        except Exception as exc:
            return {"events": events, "step": 0, "error": str(exc)}
        events.append(
            runner_pb2.RunEvent(at=self._now(), step=0, plan=runner_pb2.PlanProduced(steps=plan))
        )
        events.append(
            runner_pb2.RunEvent(
                at=self._now(),
                step=0,
                llm_message=runner_pb2.LlmMessage(role="assistant", content=plan_message(plan)),
            )
        )
        return {"events": events, "step": 0}

    def _act_status_node(self, state: _State) -> _State:
        return {"events": [self._status(1, runner_pb2.RUN_STATUS_ACTING)], "step": 1}

    def _decide_node(self, state: _State) -> _State:
        step = state["step"]
        observations = state.get("observations", [])
        if step > state["max_steps"]:  # out of budget -> force a final answer
            try:
                final = self._force_final(state["objective"], state["allowed"], observations)
            except Exception as exc:  # a forced-final LLM call may still fail
                return {"error": str(exc)}
            return {
                "final_answer": final.answer,
                "pending_tool_name": None,
                "pending_tool_args": None,
            }
        try:
            with self._tracer.start_as_current_span("agent.step") as step_span:
                step_span.set_attribute("sap.step", step)
                with self._tracer.start_as_current_span("llm.call") as span:
                    self._genai_attrs(span, "decide_action")
                    action = self._llm.decide_action(
                        state["objective"], state["allowed"], observations
                    )
        except Exception as exc:
            return {"error": str(exc)}
        if isinstance(action, FinalAnswer):
            return {
                "final_answer": action.answer,
                "pending_tool_name": None,
                "pending_tool_args": None,
            }
        return {
            "pending_tool_name": action.tool,
            "pending_tool_args": dict(action.arguments),
            "final_answer": None,
        }

    def _act_node(self, state: _State) -> _State:
        step = state["step"]
        tool = state["pending_tool_name"]
        args = state.get("pending_tool_args") or {}
        assert tool is not None  # routing guarantees a pending tool call
        events: list[runner_pb2.RunEvent] = [
            runner_pb2.RunEvent(
                at=self._now(),
                step=step,
                tool_requested=runner_pb2.ToolCallRequested(
                    tool_name=tool, arguments=dict_to_struct(args)
                ),
            )
        ]
        try:
            outcome = run_tool_step(
                tool_proxy=self._tool_proxy,
                run_id=state["run_id"],
                tool_name=tool,
                arguments=dict(args),
                step=step,
                credential=state["credential"],
                allowed_tools=state["allowed"],
            )
        except Exception as exc:
            # Allow-list denial, guard block, transport error, or any unexpected
            # failure: no ToolCallResult, straight to fail — but carry the already
            # built ToolCallRequested so the trace matches the custom core, which
            # has streamed it before raising (core/agent.py:205-211).
            return {"events": events, "error": str(exc)}
        if outcome.suspicious:
            self._log.warning(
                "quarantined suspicious tool output",
                run_id=state["run_id"],
                tool=tool,
                step=step,
            )
        events.append(
            runner_pb2.RunEvent(
                at=self._now(),
                step=step,
                tool_result=runner_pb2.ToolCallResult(
                    tool_name=tool, ok=outcome.result.ok, detail=outcome.detail
                ),
            )
        )
        if not outcome.result.ok:
            code = outcome.result.error_code or "ERROR_CODE_UNSPECIFIED"
            return {"events": events, "error": f"{code}: {outcome.result.detail}"}
        events.append(self._status(step, runner_pb2.RUN_STATUS_OBSERVING))
        return {"events": events, "observations": [outcome.observation], "step": step + 1}

    def _finalize_node(self, state: _State) -> _State:
        # The custom core attributes a forced final (step budget exhausted) to the
        # last loop step (max_steps), while an early final keeps its decide step;
        # clamp to reconcile both, matching the custom core's step numbering.
        step = min(state["step"], state["max_steps"])
        answer = finalize_answer(state.get("final_answer") or "", credential=state["credential"])
        self._log.info("agent run succeeded", run_id=state["run_id"], steps=step)
        return {
            "events": [
                runner_pb2.RunEvent(
                    at=self._now(), step=step, final=runner_pb2.FinalAnswer(answer=answer)
                ),
                self._status(step, runner_pb2.RUN_STATUS_SUCCEEDED),
            ]
        }

    def _fail_node(self, state: _State) -> _State:
        # Clamp like _finalize_node: after the final tool act the loop step sits one
        # past max_steps, yet the custom core attributes a failure to the last real
        # step, so a forced-final failure must report max_steps, not max_steps + 1.
        step = state.get("step", 0)
        max_steps = state.get("max_steps")
        if max_steps is not None:
            step = min(step, max_steps)
        message = state.get("error") or "unknown error"
        self._log.warning("agent run failed", run_id=state.get("run_id"), error=message)
        return {
            "events": [
                runner_pb2.RunEvent(
                    at=self._now(), step=step, error=runner_pb2.RunError(message=message)
                ),
                self._status(step, runner_pb2.RUN_STATUS_FAILED),
            ]
        }

    # -- routers -------------------------------------------------------------
    @staticmethod
    def _route_after_plan(state: _State) -> str:
        return "fail" if state.get("error") else "act_status"

    @staticmethod
    def _route_after_decide(state: _State) -> str:
        if state.get("error"):
            return "fail"
        if state.get("final_answer") is not None:
            return "finalize"
        return "act"

    @staticmethod
    def _route_after_act(state: _State) -> str:
        return "fail" if state.get("error") else "decide"

    def _force_final(
        self, objective: str, allowed: Sequence[str], observations: Sequence[Observation]
    ) -> FinalAnswer:
        action = self._llm.decide_action(objective, allowed, observations)
        if isinstance(action, FinalAnswer):
            return action
        return FinalAnswer(
            answer="Stopped: reached the maximum step budget before completing the objective."
        )

    # -- the loop ------------------------------------------------------------
    def run(self, request: runner_pb2.RunTaskRequest) -> Iterator[runner_pb2.RunEvent]:
        """Drive the graph for ``request`` and yield its accumulated ``RunEvent`` trace."""
        max_steps = int(request.max_steps) or self._default_max_steps
        initial: _State = {
            "events": [],
            "observations": [],
            "objective": request.objective,
            "allowed": list(request.allowed_tools),
            "credential": request.scoped_credential,
            "run_id": request.run_id,
            "max_steps": max_steps,
            "step": 0,
            "pending_tool_name": None,
            "pending_tool_args": None,
            "final_answer": None,
            "error": None,
        }
        # Screen the objective for injection patterns — flag (and log) rather than
        # refuse, matching the custom core: the deterministic loop treats the
        # objective as data, so an override attempt cannot subvert it, but it must
        # still be visible in the trace/logs.
        objective_verdict = injection.check_objective(request.objective)
        self._log.info(
            "agent run started",
            run_id=request.run_id,
            tenant_id=request.tenant_id,
            objective=request.objective,
            allowed_tools=list(request.allowed_tools),
            max_steps=max_steps,
            engine=self._llm.engine,
            objective_suspicious=objective_verdict.suspicious,
            variant="langgraph",
        )
        # Bound the loop generously: ~2 node visits per step plus the plan/finalize
        # scaffolding. Mirrors the custom core's max_steps ceiling.
        config = {"recursion_limit": 2 * max_steps + 10}
        # Stream in "values" mode so we always hold the events accumulated so far,
        # even if a node escapes. The guarded nodes route every expected/unexpected
        # failure to ``fail`` (a clean RunError + FAILED), so the stream normally
        # runs to completion and this replays the full trace. The ``except`` is
        # last-resort defense-in-depth: on an unguarded escape it still yields the
        # partial trace before a terminal RunError/FAILED — never a bare error@0,
        # matching the custom core's stream-then-fail contract.
        events: list[runner_pb2.RunEvent] = []
        last_step = 0
        try:
            for state in self._graph.stream(initial, config=config, stream_mode="values"):
                snapshot: _State = state
                events = snapshot.get("events", events)
                last_step = snapshot.get("step", last_step)
            yield from events
        except Exception as exc:
            self._log.error("agent run crashed", run_id=request.run_id, error=str(exc))
            yield from events
            fail_step = min(last_step, max_steps)
            yield runner_pb2.RunEvent(
                at=self._now(), step=fail_step, error=runner_pb2.RunError(message=str(exc))
            )
            yield self._status(fail_step, runner_pb2.RUN_STATUS_FAILED)
