package toolproxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/audit"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/credential"
	dbgen "github.com/jjsanda/secure-agent-platform/control-plane/internal/db/gen"
	sapv1 "github.com/jjsanda/secure-agent-platform/control-plane/internal/gen/proto/sap/v1"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/guard"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/telemetry"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/tenancy"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/tools"
)

// Server implements sapv1.ToolProxyServiceServer. By the time a handler runs, the
// CredentialInterceptor has already validated the credential; the handler then
// enforces the per-request checks that need the request body: run binding, the
// credential's tool allow-list, and the global tool registry — and audits the
// outcome. (The full policy guard — SSRF, path traversal, arg schema — is layered
// on here alongside the richer tools.)
type Server struct {
	sapv1.UnimplementedToolProxyServiceServer
	store    *tenancy.Store
	registry *tools.Registry
	metrics  *telemetry.Metrics
}

// NewServer builds a ToolProxy server.
func NewServer(store *tenancy.Store, registry *tools.Registry, metrics *telemetry.Metrics) *Server {
	return &Server{store: store, registry: registry, metrics: metrics}
}

// ExecuteTool validates and runs a single tool call for a run.
func (s *Server) ExecuteTool(ctx context.Context, req *sapv1.ExecuteToolRequest) (*sapv1.ExecuteToolResponse, error) {
	scoped, ok := scopedFrom(ctx)
	if !ok { // should be impossible: the interceptor injects it
		return toolErr(sapv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "no validated credential in context"), nil
	}
	tool := req.GetToolName()

	// The credential is bound to exactly one run.
	if req.GetRunId() != scoped.RunID.String() {
		s.record(ctx, scoped, tool, audit.ActionToolDenied, map[string]any{"reason": "run mismatch"})
		return toolErr(sapv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "credential is not valid for this run"), nil
	}

	// The credential carries a per-run allow-list.
	if !scoped.Allows(tool) {
		s.record(ctx, scoped, tool, audit.ActionToolDenied, map[string]any{"reason": "out of scope"})
		return toolErr(sapv1.ErrorCode_ERROR_CODE_OUT_OF_SCOPE, "tool is not permitted for this run"), nil
	}

	// The platform's global registry is the outermost allow-list.
	impl, ok := s.registry.Get(tool)
	if !ok {
		s.record(ctx, scoped, tool, audit.ActionToolDenied, map[string]any{"reason": "unknown tool"})
		return toolErr(sapv1.ErrorCode_ERROR_CODE_OUT_OF_SCOPE, "unknown tool"), nil
	}

	args := req.GetArguments().AsMap()

	// Policy guard: argument schema + SSRF/path checks run BEFORE the call is
	// recorded as allowed, so a blocked call audits only tool.denied (never both
	// allowed and denied).
	if err := impl.Validate(args); err != nil {
		code := sapv1.ErrorCode_ERROR_CODE_INVALID_ARGS
		if errors.Is(err, guard.ErrBlocked) {
			code = sapv1.ErrorCode_ERROR_CODE_POLICY_BLOCKED
			s.metrics.GuardBlock(ctx, "policy")
		}
		s.record(ctx, scoped, tool, audit.ActionToolDenied, map[string]any{"reason": err.Error()})
		return toolErr(code, err.Error()), nil
	}

	s.record(ctx, scoped, tool, audit.ActionToolAllowed, nil)

	out, err := impl.Execute(ctx, args)
	if err != nil {
		// The guard also runs inside Execute (e.g. http_get re-checks and pins the
		// resolved IP to defeat DNS rebinding); a rejection there is POLICY_BLOCKED,
		// anything else is an ordinary tool failure.
		code := sapv1.ErrorCode_ERROR_CODE_TOOL_FAILED
		if errors.Is(err, guard.ErrBlocked) {
			code = sapv1.ErrorCode_ERROR_CODE_POLICY_BLOCKED
			s.metrics.GuardBlock(ctx, "policy")
		}
		s.record(ctx, scoped, tool, audit.ActionToolDenied, map[string]any{"reason": err.Error()})
		return toolErr(code, err.Error()), nil
	}

	outStruct, err := structpb.NewStruct(out)
	if err != nil {
		return toolErr(sapv1.ErrorCode_ERROR_CODE_TOOL_FAILED, "encode output: "+err.Error()), nil
	}
	sum := sha256Hex(out)
	s.record(ctx, scoped, tool, audit.ActionToolExecuted, map[string]any{"output_sha256": sum})

	return &sapv1.ExecuteToolResponse{
		Result: &sapv1.ExecuteToolResponse_Ok{Ok: &sapv1.ToolOk{
			Output:       outStruct,
			OutputSha256: sum,
		}},
	}, nil
}

func (s *Server) record(ctx context.Context, sc credential.Scoped, tool string, action audit.Action, detail map[string]any) {
	switch action {
	case audit.ActionToolExecuted:
		s.metrics.ToolCall(ctx, tool, "executed")
	case audit.ActionToolAllowed:
		s.metrics.ToolCall(ctx, tool, "allowed")
	case audit.ActionToolDenied:
		s.metrics.ToolCall(ctx, tool, "denied")
	}
	_ = s.store.WithTenant(ctx, sc.TenantID, func(q *dbgen.Queries) error {
		return audit.Record(ctx, q, sc.TenantID, audit.Entry{
			Actor:    "run:" + sc.RunID.String(),
			Action:   action,
			RunID:    uuid.NullUUID{UUID: sc.RunID, Valid: true},
			JTI:      uuid.NullUUID{UUID: sc.JTI, Valid: true},
			ToolName: tool,
			Detail:   detail,
		})
	})
}

func toolErr(code sapv1.ErrorCode, msg string) *sapv1.ExecuteToolResponse {
	return &sapv1.ExecuteToolResponse{
		Result: &sapv1.ExecuteToolResponse_Error{Error: &sapv1.ToolError{Code: code, Message: msg}},
	}
}

func sha256Hex(v map[string]any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
