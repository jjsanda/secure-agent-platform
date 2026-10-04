"""The custom plan -> act -> observe agent loop.

Given a ``RunTaskRequest``, :meth:`Agent.run` yields the run's ``RunEvent`` trace
in this fixed order for the happy path::

    StatusChange(PLANNING)
    PlanProduced(steps)
    LlmMessage(assistant, ...)
    StatusChange(ACTING)
      # for each chosen tool, up to max_steps:
      ToolCallRequested(tool, args)
      # -> worker-side allow-list + schema/SSRF/path guards
      # -> tool proxy ExecuteTool
      # -> injection scan + secret redaction of the result
      ToolCallResult(tool, ok, detail)
      StatusChange(OBSERVING)
    FinalAnswer(answer)          # secret-redacted
    StatusChange(SUCCEEDED)

On an allow-list denial, a guard rejection, a tool failure returned by the proxy,
or any unexpected error, the loop emits ``RunError(message)`` then
``StatusChange(FAILED)`` and stops. Every event is stamped with ``at`` (UTC
timestamp) and ``step``.
"""

from __future__ import annotations

from collections.abc import Iterator, Sequence
from typing import Any

from sap_worker.core.steps import (
    Clock,
    finalize_answer,
    plan_message,
    run_tool_step,
    to_timestamp,
)
from sap_worker.core.steps import default_clock as _default_clock
from sap_worker.exceptions import SapWorkerError, ToolExecutionFailed
from sap_worker.gen.sap.v1 import runner_pb2
from sap_worker.guard import injection
from sap_worker.llm.base import LLM, FinalAnswer, Observation
from sap_worker.logging import get_logger
from sap_worker.structpb import dict_to_struct
from sap_worker.telemetry import get_tracer
from sap_worker.toolproxy_client import ToolProxy

__all__ = ["Agent"]


class Agent:
    """The hand-written agent. Reentrant: :meth:`run` keeps all state in locals."""

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
        self._log = logger or get_logger("sap_worker.agent")
        self._tracer = get_tracer("sap_worker.agent")

    # -- event construction --------------------------------------------------
    def _now(self) -> Any:
        return to_timestamp(self._clock)

    def _genai_attrs(self, span: Any) -> None:
        """Stamp GenAI semantic-convention attributes on an ``llm.call`` span."""
        span.set_attribute("gen_ai.system", self._llm.engine)
        span.set_attribute("gen_ai.request.model", self._llm.model)

    # -- the loop ------------------------------------------------------------
    def run(self, request: runner_pb2.RunTaskRequest) -> Iterator[runner_pb2.RunEvent]:
        """Drive the loop for ``request``, yielding its ``RunEvent`` trace."""
        run_id = request.run_id
        objective = request.objective
        allowed = list(request.allowed_tools)
        credential = request.scoped_credential
        max_steps = int(request.max_steps) or self._default_max_steps

        step = 0
        # Screen the objective for injection patterns. We flag (and log) rather
        # than refuse: the deterministic core treats the objective as data, so an
        # override attempt cannot subvert it — but it must be visible in the trace.
        objective_verdict = injection.check_objective(objective)
        self._log.info(
            "agent run started",
            run_id=run_id,
            tenant_id=request.tenant_id,
            objective=objective,
            allowed_tools=allowed,
            max_steps=max_steps,
            engine=self._llm.engine,
            objective_suspicious=objective_verdict.suspicious,
        )
        try:
            # --- PLAN ---
            yield runner_pb2.RunEvent(
                at=self._now(),
                step=step,
                status=runner_pb2.StatusChange(status=runner_pb2.RUN_STATUS_PLANNING),
            )
            with self._tracer.start_as_current_span("llm.call") as span:
                self._genai_attrs(span)
                span.set_attribute("gen_ai.operation.name", "plan")
                plan = self._llm.plan(objective, allowed)
            yield runner_pb2.RunEvent(
                at=self._now(), step=step, plan=runner_pb2.PlanProduced(steps=plan)
            )
            yield runner_pb2.RunEvent(
                at=self._now(),
                step=step,
                llm_message=runner_pb2.LlmMessage(role="assistant", content=plan_message(plan)),
            )

            # --- ACT / OBSERVE ---
            step = 1
            yield runner_pb2.RunEvent(
                at=self._now(),
                step=step,
                status=runner_pb2.StatusChange(status=runner_pb2.RUN_STATUS_ACTING),
            )
            observations: list[Observation] = []
            final: FinalAnswer | None = None
            for step in range(1, max_steps + 1):
                with self._tracer.start_as_current_span("agent.step") as step_span:
                    step_span.set_attribute("sap.step", step)
                    with self._tracer.start_as_current_span("llm.call") as span:
                        self._genai_attrs(span)
                        span.set_attribute("gen_ai.operation.name", "decide_action")
                        action = self._llm.decide_action(objective, allowed, observations)
                    if isinstance(action, FinalAnswer):
                        final = action
                        break
                    step_span.set_attribute("sap.tool_name", action.tool)

                    # A tool call: announce it, then guard + dispatch through the proxy.
                    yield runner_pb2.RunEvent(
                        at=self._now(),
                        step=step,
                        tool_requested=runner_pb2.ToolCallRequested(
                            tool_name=action.tool, arguments=dict_to_struct(action.arguments)
                        ),
                    )
                    outcome = run_tool_step(
                        tool_proxy=self._tool_proxy,
                        run_id=run_id,
                        tool_name=action.tool,
                        arguments=dict(action.arguments),
                        step=step,
                        credential=credential,
                        allowed_tools=allowed,
                    )
                    if outcome.suspicious:
                        self._log.warning(
                            "quarantined suspicious tool output",
                            run_id=run_id,
                            tool=action.tool,
                            step=step,
                        )
                    yield runner_pb2.RunEvent(
                        at=self._now(),
                        step=step,
                        tool_result=runner_pb2.ToolCallResult(
                            tool_name=action.tool,
                            ok=outcome.result.ok,
                            detail=outcome.detail,
                        ),
                    )
                    if not outcome.result.ok:
                        raise ToolExecutionFailed(
                            outcome.result.error_code or "ERROR_CODE_UNSPECIFIED",
                            outcome.result.detail,
                        )
                    observations.append(outcome.observation)
                    yield runner_pb2.RunEvent(
                        at=self._now(),
                        step=step,
                        status=runner_pb2.StatusChange(status=runner_pb2.RUN_STATUS_OBSERVING),
                    )

            if final is None:  # ran out of step budget without a final answer
                final = self._force_final(objective, allowed, observations)

            # --- FINALIZE --- (redact any leaked secret from the answer)
            answer = finalize_answer(final.answer, credential=credential)
            yield runner_pb2.RunEvent(
                at=self._now(),
                step=step,
                final=runner_pb2.FinalAnswer(answer=answer),
            )
            yield runner_pb2.RunEvent(
                at=self._now(),
                step=step,
                status=runner_pb2.StatusChange(status=runner_pb2.RUN_STATUS_SUCCEEDED),
            )
            self._log.info("agent run succeeded", run_id=run_id, steps=step)
        except SapWorkerError as exc:
            # Expected failures: allow-list denial, guard block, tool failure, transport error.
            self._log.warning("agent run failed", run_id=run_id, error=str(exc))
            yield from self._fail(step, str(exc))
        except Exception as exc:  # last-resort: still emit a clean FAILED trace
            self._log.error("agent run crashed", run_id=run_id, error=str(exc))
            yield from self._fail(step, str(exc))

    def _fail(self, step: int, message: str) -> Iterator[runner_pb2.RunEvent]:
        yield runner_pb2.RunEvent(
            at=self._now(), step=step, error=runner_pb2.RunError(message=message)
        )
        yield runner_pb2.RunEvent(
            at=self._now(),
            step=step,
            status=runner_pb2.StatusChange(status=runner_pb2.RUN_STATUS_FAILED),
        )

    def _force_final(
        self, objective: str, allowed: Sequence[str], observations: Sequence[Observation]
    ) -> FinalAnswer:
        action = self._llm.decide_action(objective, allowed, observations)
        if isinstance(action, FinalAnswer):
            return action
        return FinalAnswer(
            answer="Stopped: reached the maximum step budget before completing the objective."
        )
