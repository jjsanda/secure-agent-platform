// Package telemetry wires OpenTelemetry traces and metrics for the control plane.
// When OTEL_EXPORTER_OTLP_ENDPOINT is empty (the default profile) it is a no-op,
// so observability adds zero overhead unless you opt in.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Setup installs global tracer + meter providers exporting over OTLP/gRPC, and
// the W3C trace-context propagator so a trace flows control-plane → worker →
// tool-proxy as one. Returns a shutdown func. If otlpEndpoint is empty the
// providers are left as the SDK's no-ops (but the propagator is still set, so
// context propagates even when this process doesn't export).
func Setup(ctx context.Context, serviceName, namespace, otlpEndpoint string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	if otlpEndpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(serviceName),
		semconv.ServiceNamespace(namespace),
	))
	if err != nil {
		return nil, fmt.Errorf("telemetry: resource: %w", err)
	}

	traceExp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(otlpEndpoint))
	if err != nil {
		return nil, fmt.Errorf("telemetry: trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	metricExp, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpointURL(otlpEndpoint))
	if err != nil {
		return nil, fmt.Errorf("telemetry: metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(15*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}, nil
}

// Metrics holds the platform's security-relevant counters. They are backed by
// the global meter, so they are safe no-ops when telemetry is disabled. Metric
// names omit the `_total` suffix; the collector's Prometheus exporter adds it.
type Metrics struct {
	runs        metric.Int64Counter
	toolCalls   metric.Int64Counter
	guardBlocks metric.Int64Counter
	credDenied  metric.Int64Counter
	rlsDenied   metric.Int64Counter
}

// NewMetrics constructs the platform counters.
func NewMetrics() (*Metrics, error) {
	m := otel.Meter("secure-agent-platform")
	runs, err := m.Int64Counter("sap_runs", metric.WithDescription("Agent runs dispatched."))
	if err != nil {
		return nil, err
	}
	toolCalls, err := m.Int64Counter("sap_tool_calls", metric.WithDescription("Tool calls, by tool and outcome."))
	if err != nil {
		return nil, err
	}
	guardBlocks, err := m.Int64Counter("sap_guard_blocks", metric.WithDescription("Policy-guard blocks, by reason."))
	if err != nil {
		return nil, err
	}
	credDenied, err := m.Int64Counter("sap_credential_denied", metric.WithDescription("Credential validation denials, by reason."))
	if err != nil {
		return nil, err
	}
	rlsDenied, err := m.Int64Counter("sap_rls_denied", metric.WithDescription("Resource lookups filtered by row-level security (cross-tenant / IDOR attempts)."))
	if err != nil {
		return nil, err
	}
	return &Metrics{runs: runs, toolCalls: toolCalls, guardBlocks: guardBlocks, credDenied: credDenied, rlsDenied: rlsDenied}, nil
}

// RunStarted records a dispatched run.
func (m *Metrics) RunStarted(ctx context.Context) {
	if m != nil {
		m.runs.Add(ctx, 1)
	}
}

// ToolCall records a tool call outcome (allowed, executed, denied, blocked).
func (m *Metrics) ToolCall(ctx context.Context, tool, outcome string) {
	if m != nil {
		m.toolCalls.Add(ctx, 1, metric.WithAttributes(
			attribute.String("tool", tool), attribute.String("outcome", outcome)))
	}
}

// GuardBlock records a policy-guard block.
func (m *Metrics) GuardBlock(ctx context.Context, reason string) {
	if m != nil {
		m.guardBlocks.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
	}
}

// CredentialDenied records a credential validation denial.
func (m *Metrics) CredentialDenied(ctx context.Context, reason string) {
	if m != nil {
		m.credDenied.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
	}
}

// RLSDenied records a resource lookup that row-level security filtered out. In a
// multi-tenant system a 404-by-id is typically a cross-tenant / IDOR attempt.
func (m *Metrics) RLSDenied(ctx context.Context) {
	if m != nil {
		m.rlsDenied.Add(ctx, 1)
	}
}
