// Package dispatch orchestrates an agent run: it mints a scoped credential,
// dispatches the task to the Python worker over gRPC, and relays the streamed
// events to the database (durable trace), the run's status, and the SSE hub. It
// revokes the credential the instant the run ends.
package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/audit"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/credential"
	dbgen "github.com/jjsanda/secure-agent-platform/control-plane/internal/db/gen"
	sapv1 "github.com/jjsanda/secure-agent-platform/control-plane/internal/gen/proto/sap/v1"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/telemetry"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/tenancy"
)

// Dispatcher runs agent tasks and relays their event streams.
type Dispatcher struct {
	store   *tenancy.Store
	signer  *credential.Signer
	worker  sapv1.RunnerServiceClient
	hub     *Hub
	metrics *telemetry.Metrics
}

// New builds a Dispatcher.
func New(store *tenancy.Store, signer *credential.Signer, worker sapv1.RunnerServiceClient, hub *Hub, metrics *telemetry.Metrics) *Dispatcher {
	return &Dispatcher{store: store, signer: signer, worker: worker, hub: hub, metrics: metrics}
}

// Start executes a created run asynchronously.
func (d *Dispatcher) Start(runID, tenantID uuid.UUID, objective, variant string, allowedTools []string) {
	go func() {
		// A panic on this detached goroutine is NOT caught by the HTTP
		// middleware, so without this recover it would crash the whole control
		// plane. Recover, finalize the run as failed (revoking its credential),
		// and keep serving.
		defer func() {
			if r := recover(); r != nil {
				slog.Error("dispatch: run panicked", "run", runID, "panic", r, "stack", string(debug.Stack()))
				d.finalize(context.Background(), runID, tenantID, "", fmt.Sprintf("run panicked: %v", r))
			}
		}()
		if err := d.run(context.Background(), runID, tenantID, objective, variant, allowedTools); err != nil {
			slog.Error("dispatch: run ended with error", "run", runID, "err", err)
		}
	}()
}

func (d *Dispatcher) run(ctx context.Context, runID, tenantID uuid.UUID, objective, variant string, allowedTools []string) error {
	// Bound the whole run, and let the worker call wait for the worker to become
	// ready (WaitForReady below) rather than failing on a startup race.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	defer d.hub.Close(runID.String())
	d.metrics.RunStarted(ctx)

	// 1. Mint, persist, and audit the scoped credential.
	token, scoped, err := d.signer.Mint(runID, tenantID, allowedTools, time.Now())
	if err != nil {
		return d.failEarly(ctx, runID, tenantID, "mint credential: "+err.Error())
	}
	if err := d.store.WithTenant(ctx, tenantID, func(q *dbgen.Queries) error {
		if _, err := q.InsertScopedCredential(ctx, dbgen.InsertScopedCredentialParams{
			Jti:          scoped.JTI,
			TenantID:     tenantID,
			RunID:        runID,
			AllowedTools: allowedTools,
			ExpiresAt:    pgtype.Timestamptz{Time: scoped.ExpiresAt, Valid: true},
		}); err != nil {
			return err
		}
		return audit.Record(ctx, q, tenantID, audit.Entry{
			Actor:  "system",
			Action: audit.ActionCredentialMinted,
			RunID:  uuid.NullUUID{UUID: runID, Valid: true},
			JTI:    uuid.NullUUID{UUID: scoped.JTI, Valid: true},
			Detail: map[string]any{"allowed_tools": allowedTools, "expires_at": scoped.ExpiresAt},
		})
	}); err != nil {
		return d.failEarly(ctx, runID, tenantID, "persist credential: "+err.Error())
	}

	// 2. Dispatch to the worker. The worker holds ONLY the token below.
	stream, err := d.worker.RunTask(ctx, &sapv1.RunTaskRequest{
		RunId:            runID.String(),
		TenantId:         tenantID.String(),
		Objective:        objective,
		AllowedTools:     allowedTools,
		ScopedCredential: token,
		Variant:          variantEnum(variant),
		MaxSteps:         8,
		// Engine/model are the worker's deploy-time setting (LLM_ENGINE); the
		// per-run variant IS honored by the worker.
	}, grpc.WaitForReady(true))
	if err != nil {
		return d.failEarly(ctx, runID, tenantID, "dispatch to worker: "+err.Error())
	}

	// 3. Relay the streamed events.
	var finalAnswer, runErr string
	for {
		ev, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			runErr = "worker stream: " + err.Error()
			break
		}
		kind, payload := mapEvent(ev)
		if err := d.persistAndPublish(ctx, runID, tenantID, int32(ev.GetStep()), kind, payload); err != nil {
			slog.Error("dispatch: persist event", "run", runID, "err", err)
		}
		switch kind {
		case "final":
			finalAnswer = ev.GetFinal().GetAnswer()
		case "error":
			runErr = ev.GetError().GetMessage()
		case "status":
			d.updateStatus(ctx, runID, tenantID, ev.GetStatus().GetStatus())
		}
	}

	// 4. Finalize: set terminal status, revoke credentials, audit.
	d.finalize(ctx, runID, tenantID, finalAnswer, runErr)
	return nil
}

func (d *Dispatcher) persistAndPublish(ctx context.Context, runID, tenantID uuid.UUID, step int32, kind string, payload []byte) error {
	var row dbgen.AppRunEvent
	if err := d.store.WithTenant(ctx, tenantID, func(q *dbgen.Queries) error {
		var e error
		row, e = q.AppendRunEvent(ctx, dbgen.AppendRunEventParams{
			TenantID: tenantID,
			RunID:    runID,
			Step:     step,
			Kind:     kind,
			Payload:  payload,
		})
		return e
	}); err != nil {
		return err
	}
	d.hub.Publish(runID.String(), Event{
		ID:      row.ID,
		RunID:   runID.String(),
		Kind:    row.Kind,
		Step:    row.Step,
		Payload: row.Payload,
		At:      row.At.Time,
	})
	return nil
}

func (d *Dispatcher) updateStatus(ctx context.Context, runID, tenantID uuid.UUID, s sapv1.RunStatus) {
	status := statusString(s)
	if status == "succeeded" || status == "failed" {
		return // finalize handles terminal state (with the final answer / error)
	}
	if err := d.store.WithTenant(ctx, tenantID, func(q *dbgen.Queries) error {
		return q.UpdateRunStatus(ctx, dbgen.UpdateRunStatusParams{ID: runID, Status: status})
	}); err != nil {
		slog.Error("dispatch: update status", "run", runID, "err", err)
	}
}

func (d *Dispatcher) finalize(ctx context.Context, runID, tenantID uuid.UUID, finalAnswer, runErr string) {
	// Detach from the run's context — which may already be cancelled or past its
	// 5-minute deadline — with a fresh timeout, so the terminal status is set and
	// the credential is ALWAYS revoked. That guarantee is the whole point of
	// finalization; skipping it would leave a live credential until its TTL.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()

	status := "succeeded"
	if runErr != "" {
		status = "failed"
	}
	if err := d.store.WithTenant(ctx, tenantID, func(q *dbgen.Queries) error {
		if err := q.FinishRun(ctx, dbgen.FinishRunParams{
			ID:          runID,
			Status:      status,
			FinalAnswer: pgtype.Text{String: finalAnswer, Valid: finalAnswer != ""},
			Error:       pgtype.Text{String: runErr, Valid: runErr != ""},
		}); err != nil {
			return err
		}
		if err := q.RevokeRunCredentials(ctx, runID); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, tenantID, audit.Entry{
			Actor:  "system",
			Action: audit.ActionCredentialRevoked,
			RunID:  uuid.NullUUID{UUID: runID, Valid: true},
		}); err != nil {
			return err
		}
		return audit.Record(ctx, q, tenantID, audit.Entry{
			Actor:  "system",
			Action: audit.ActionRunFinished,
			RunID:  uuid.NullUUID{UUID: runID, Valid: true},
			Detail: map[string]any{"status": status},
		})
	}); err != nil {
		slog.Error("dispatch: finalize", "run", runID, "err", err)
	}
}

// failEarly records a failure that happened before or without a worker stream.
func (d *Dispatcher) failEarly(ctx context.Context, runID, tenantID uuid.UUID, msg string) error {
	payload, _ := json.Marshal(map[string]any{"message": msg})
	_ = d.persistAndPublish(ctx, runID, tenantID, 0, "error", payload)
	d.finalize(ctx, runID, tenantID, "", msg)
	return errors.New(msg)
}

func mapEvent(ev *sapv1.RunEvent) (kind string, payload []byte) {
	var m map[string]any
	switch e := ev.GetEvent().(type) {
	case *sapv1.RunEvent_Status:
		kind, m = "status", map[string]any{"status": statusString(e.Status.GetStatus())}
	case *sapv1.RunEvent_Plan:
		kind, m = "plan", map[string]any{"steps": e.Plan.GetSteps()}
	case *sapv1.RunEvent_LlmMessage:
		kind, m = "llm_message", map[string]any{"role": e.LlmMessage.GetRole(), "content": e.LlmMessage.GetContent()}
	case *sapv1.RunEvent_ToolRequested:
		kind, m = "tool_requested", map[string]any{"tool": e.ToolRequested.GetToolName(), "arguments": e.ToolRequested.GetArguments().AsMap()}
	case *sapv1.RunEvent_ToolResult:
		kind, m = "tool_result", map[string]any{"tool": e.ToolResult.GetToolName(), "ok": e.ToolResult.GetOk(), "detail": e.ToolResult.GetDetail()}
	case *sapv1.RunEvent_Final:
		kind, m = "final", map[string]any{"answer": e.Final.GetAnswer()}
	case *sapv1.RunEvent_Error:
		kind, m = "error", map[string]any{"message": e.Error.GetMessage()}
	default:
		kind, m = "unknown", map[string]any{}
	}
	payload, _ = json.Marshal(m)
	return kind, payload
}

func statusString(s sapv1.RunStatus) string {
	switch s {
	case sapv1.RunStatus_RUN_STATUS_PLANNING:
		return "planning"
	case sapv1.RunStatus_RUN_STATUS_ACTING:
		return "acting"
	case sapv1.RunStatus_RUN_STATUS_OBSERVING:
		return "observing"
	case sapv1.RunStatus_RUN_STATUS_SUCCEEDED:
		return "succeeded"
	case sapv1.RunStatus_RUN_STATUS_FAILED:
		return "failed"
	default:
		return "pending"
	}
}

func variantEnum(v string) sapv1.AgentVariant {
	if v == "langgraph" {
		return sapv1.AgentVariant_AGENT_VARIANT_LANGGRAPH
	}
	return sapv1.AgentVariant_AGENT_VARIANT_CUSTOM
}
