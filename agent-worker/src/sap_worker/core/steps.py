"""Shared plan/act/observe building blocks used by both agent cores.

The hand-written core (:mod:`sap_worker.core.agent`) and the LangGraph variant
(:mod:`sap_worker.graph`) both import these, so they apply the *same* guards,
route every tool call through the *same* tool-proxy client, neutralize
suspicious tool output the *same* way, and therefore emit an equivalent
``RunEvent`` trace regardless of which core runs.
"""

from __future__ import annotations

from collections.abc import Callable, Iterator, Sequence
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Protocol, runtime_checkable

from google.protobuf.timestamp_pb2 import Timestamp

from sap_worker.gen.sap.v1 import runner_pb2
from sap_worker.guard import enforce_tool_arguments, ensure_allowed, injection
from sap_worker.llm.base import Observation
from sap_worker.telemetry import get_tracer
from sap_worker.toolproxy_client import ToolProxy, ToolResult

__all__ = [
    "Clock",
    "AgentRunner",
    "plan_message",
    "to_timestamp",
    "ToolStepOutcome",
    "run_tool_step",
    "finalize_answer",
]

Clock = Callable[[], datetime]


@runtime_checkable
class AgentRunner(Protocol):
    """The shared agent surface: both cores stream a ``RunEvent`` trace for a run.

    Lets the composition root and gRPC layer treat the custom
    :class:`~sap_worker.core.agent.Agent` and the LangGraph variant
    interchangeably.
    """

    def run(self, request: runner_pb2.RunTaskRequest) -> Iterator[runner_pb2.RunEvent]:
        """Drive one run and yield its ordered ``RunEvent`` trace."""


def plan_message(steps: Sequence[str]) -> str:
    """Render a plan as a short assistant message."""
    numbered = "; ".join(f"{i}) {s}" for i, s in enumerate(steps, start=1))
    return f"Plan ({len(steps)} steps): {numbered}"


def to_timestamp(clock: Clock) -> Timestamp:
    """Stamp an event with the current UTC time from ``clock``."""
    ts = Timestamp()
    ts.FromDatetime(clock())
    return ts


def default_clock() -> datetime:
    """The wall-clock used when a core is constructed without one."""
    return datetime.now(UTC)


@dataclass(frozen=True)
class ToolStepOutcome:
    """The result of a single guarded, proxied tool call."""

    result: ToolResult
    observation: Observation
    detail: str
    suspicious: bool


def run_tool_step(
    *,
    tool_proxy: ToolProxy,
    run_id: str,
    tool_name: str,
    arguments: dict[str, object],
    step: int,
    credential: str,
    allowed_tools: Sequence[str],
) -> ToolStepOutcome:
    """Guard, dispatch, and post-process one tool call.

    Order: allow-list → argument guards (schema/SSRF/path) → proxy → injection
    scan + secret redaction on the result. Raises
    :class:`~sap_worker.guard.ToolNotAllowed` or a
    :class:`~sap_worker.exceptions.GuardBlocked` subclass *before* any proxy call
    on a denial; the caller has already emitted the ``ToolCallRequested`` event,
    so the run fails cleanly after announcing the attempt.

    On success the tool output is treated as untrusted: it is scanned for
    injection patterns (a hit annotates ``detail`` with the quarantine note) and
    always run through secret redaction so the scoped credential can never
    survive into the observation that re-enters the loop or the final answer.
    """
    ensure_allowed(tool_name, allowed_tools)
    enforce_tool_arguments(tool_name, arguments)

    # A worker-side ``tool.execute`` span wraps the outbound call; the gRPC client
    # instrumentation (when telemetry is enabled) nests the ExecuteTool RPC under
    # it and injects the trace context onward to the tool proxy.
    with get_tracer().start_as_current_span("tool.execute") as span:
        span.set_attribute("sap.tool_name", tool_name)
        span.set_attribute("sap.run_id", run_id)
        span.set_attribute("sap.step", step)
        result = tool_proxy.execute(run_id, tool_name, arguments, step, credential)
        span.set_attribute("sap.tool_ok", result.ok)

    detail = result.detail
    suspicious = False
    if result.ok:
        verdict = injection.scan_tool_output(tool_name, result.output)
        if verdict.suspicious:
            suspicious = True
            detail = f"{detail} · {injection.QUARANTINE_NOTE}"

    # Neutralize the credential (and any secret-shaped token) out of the output
    # before it becomes an observation — defensively, even when not flagged.
    safe_output = injection.neutralize_output(result.output, secrets=(credential,))
    observation = Observation(tool=tool_name, ok=result.ok, output=safe_output, detail=detail)
    return ToolStepOutcome(
        result=result, observation=observation, detail=detail, suspicious=suspicious
    )


def finalize_answer(answer: str, *, credential: str) -> str:
    """Redact any leaked scoped credential / secret out of the final answer."""
    return injection.redact_secrets(answer, secrets=(credential,))
