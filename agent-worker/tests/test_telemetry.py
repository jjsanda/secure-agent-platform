"""Worker OpenTelemetry: verify the no-op default and the instrument path.

The default stack ships no collector, so with ``OTEL_EXPORTER_OTLP_ENDPOINT``
unset everything must be a cheap no-op and stay green. When an endpoint is set,
``setup`` installs an exporter and patches the sync gRPC server/client so the
worker extracts and injects the W3C trace context.
"""

from __future__ import annotations

import pytest

from sap_worker import telemetry

pytestmark = pytest.mark.unit


def test_setup_is_noop_without_endpoint() -> None:
    assert telemetry.setup(endpoint=None) is False
    assert telemetry.setup(endpoint="") is False


def test_get_tracer_spans_are_always_usable() -> None:
    tracer = telemetry.get_tracer("test")
    # Whether or not a provider is installed, opening a span and setting GenAI
    # semconv attributes must not raise — the core relies on this.
    with tracer.start_as_current_span("llm.call") as span:
        span.set_attribute("gen_ai.system", "mock")
        span.set_attribute("gen_ai.request.model", "mock-deterministic")


def test_setup_installs_w3c_propagator() -> None:
    # Always install the composite W3C propagator so trace context flows even when
    # this process does not export (mirrors the Go control plane).
    telemetry.setup(endpoint=None)
    from opentelemetry import propagate

    assert "traceparent" in propagate.get_global_textmap().fields


def test_setup_with_endpoint_instruments_grpc() -> None:
    from opentelemetry.instrumentation.grpc import (
        GrpcInstrumentorClient,
        GrpcInstrumentorServer,
    )

    installed = telemetry.setup(endpoint="http://localhost:4317", service_name="agent-worker")
    try:
        assert installed is True
        # The sync server + client instrumentors are active after setup.
        assert GrpcInstrumentorServer().is_instrumented_by_opentelemetry
        assert GrpcInstrumentorClient().is_instrumented_by_opentelemetry
    finally:
        # Restore global gRPC state so instrumentation doesn't leak across tests.
        GrpcInstrumentorServer().uninstrument()
        GrpcInstrumentorClient().uninstrument()
        telemetry.shutdown()
