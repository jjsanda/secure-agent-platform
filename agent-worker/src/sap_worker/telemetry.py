"""Worker-side OpenTelemetry — a no-op unless ``OTEL_EXPORTER_OTLP_ENDPOINT`` is set.

This completes the platform's one-trace story: the Go control plane's otelgrpc
client injects ``traceparent`` on ``RunTask``; this worker EXTRACTs that context
on its ``RunnerService`` server, opens per-run spans (``agent.step``,
``llm.call``, ``tool.execute``), and re-INJECTs the context on the outbound
``ExecuteTool`` call, so a single trace flows control-plane → worker →
tool-proxy.

Everything here is behind the optional ``otel`` extra and degrades safely:

* the SDK and gRPC instrumentation are imported lazily; if the extra is not
  installed, :func:`get_tracer` returns a no-op tracer and :func:`setup` is inert;
* when installed, :func:`setup` still leaves the tracer provider as the SDK's
  no-op unless an OTLP endpoint is configured — but it *always* installs the W3C
  trace-context propagator, so context propagates even when this process does not
  export (matching the Go side).

:func:`get_tracer` is safe to import and call from the core with no extra
installed and no endpoint configured — spans are then cheap no-ops.
"""

from __future__ import annotations

import contextlib
from collections.abc import Iterator
from typing import Any

__all__ = [
    "OTEL_ENDPOINT_ENV",
    "setup",
    "shutdown",
    "get_tracer",
    "add_genai_usage",
    "instrument_channel",
    "server_interceptors",
]

OTEL_ENDPOINT_ENV = "OTEL_EXPORTER_OTLP_ENDPOINT"

try:  # The whole SDK lives behind the ``otel`` extra.
    from opentelemetry import trace as _otel_trace

    _HAVE_OTEL = True
except ImportError:  # pragma: no cover - exercised only without the extra
    _HAVE_OTEL = False


class _NoopSpan:
    """Minimal span shim used when OpenTelemetry is not installed."""

    def set_attribute(self, key: str, value: Any) -> None:
        return None


class _NoopTracer:
    """A tracer whose spans do nothing — the fallback when the extra is absent."""

    @contextlib.contextmanager
    def start_as_current_span(self, name: str, **_: Any) -> Iterator[_NoopSpan]:
        yield _NoopSpan()


_NOOP_TRACER = _NoopTracer()

# Populated by ``setup`` when an endpoint is configured, so ``shutdown`` can flush.
_provider: Any | None = None


def get_tracer(name: str = "sap_worker") -> Any:
    """Return a tracer for ``name``.

    When the ``otel`` extra is installed this is the real global tracer (a no-op
    until :func:`setup` installs a provider); otherwise it is a local no-op
    tracer. Either way, ``tracer.start_as_current_span(...)`` is safe to use.
    """
    if _HAVE_OTEL:
        return _otel_trace.get_tracer(name)
    return _NOOP_TRACER


def setup(
    *,
    endpoint: str | None,
    service_name: str = "agent-worker",
    namespace: str = "sap",
) -> bool:
    """Install the W3C propagator and (if ``endpoint`` is set) an OTLP exporter.

    Returns ``True`` when a real tracer provider + gRPC instrumentation were
    installed, ``False`` for the no-op path (extra missing, or endpoint unset).
    Also patches the sync gRPC server/channel via the instrumentors so the
    worker's ``RunnerService`` extracts and its tool-proxy client injects
    ``traceparent`` automatically.
    """
    global _provider
    if not _HAVE_OTEL:
        return False

    # Always install the composite W3C propagator (trace-context + baggage), even
    # without an exporter, so incoming context is honored and outgoing context is
    # injected. Mirrors control-plane/internal/telemetry.Setup.
    from opentelemetry import propagate
    from opentelemetry.propagators.composite import CompositePropagator
    from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator

    propagators: list[Any] = [TraceContextTextMapPropagator()]
    with contextlib.suppress(ImportError):
        from opentelemetry.baggage.propagation import W3CBaggagePropagator

        propagators.append(W3CBaggagePropagator())
    propagate.set_global_textmap(CompositePropagator(propagators))

    if not endpoint:
        return False

    from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
    from opentelemetry.sdk.resources import Resource
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor

    resource = Resource.create({"service.name": service_name, "service.namespace": namespace})
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=endpoint)))
    _otel_trace.set_tracer_provider(provider)
    _provider = provider

    # Patch grpc.server()/grpc.insecure_channel() so the RunnerService extracts
    # the incoming traceparent and the tool-proxy client injects it outbound.
    from opentelemetry.instrumentation.grpc import (
        GrpcInstrumentorClient,
        GrpcInstrumentorServer,
    )

    GrpcInstrumentorServer().instrument()  # type: ignore[no-untyped-call]
    GrpcInstrumentorClient().instrument()  # type: ignore[no-untyped-call]
    return True


def add_genai_usage(input_tokens: int | None, output_tokens: int | None) -> None:
    """Record GenAI token usage on the currently-active span (no-op if disabled).

    Called by the Claude engine after a ``messages.create`` so the core's
    ``llm.call`` span carries ``gen_ai.usage.*`` when the model reports it.
    """
    if not _HAVE_OTEL:
        return
    span = _otel_trace.get_current_span()
    if input_tokens is not None:
        span.set_attribute("gen_ai.usage.input_tokens", input_tokens)
    if output_tokens is not None:
        span.set_attribute("gen_ai.usage.output_tokens", output_tokens)


def shutdown() -> None:
    """Flush and shut down the tracer provider, if one was installed."""
    global _provider
    if _provider is not None:
        _provider.shutdown()
        _provider = None


def instrument_channel(channel: Any) -> Any:
    """Return ``channel`` (the client instrumentor patches channels globally)."""
    return channel


def server_interceptors() -> list[Any]:
    """Server interceptors to add (empty; the server instrumentor patches globally)."""
    return []
