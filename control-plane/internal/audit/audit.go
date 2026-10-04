// Package audit records security-relevant events to the append-only, tenant-
// scoped audit log. Records are always written inside a tenant transaction, so
// row-level security stamps them to the acting tenant automatically.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	dbgen "github.com/jjsanda/secure-agent-platform/control-plane/internal/db/gen"
)

// Action is a security-relevant event kind.
type Action string

const (
	ActionLogin             Action = "auth.login"
	ActionAuthzDenied       Action = "authz.denied"
	ActionRunCreated        Action = "run.created"
	ActionRunFinished       Action = "run.finished"
	ActionCredentialMinted  Action = "credential.minted"
	ActionCredentialRevoked Action = "credential.revoked"
	ActionToolRequested     Action = "tool.requested"
	ActionToolAllowed       Action = "tool.allowed"
	ActionToolDenied        Action = "tool.denied"
	ActionToolExecuted      Action = "tool.executed"
)

// Entry is one audit record. RunID and JTI are optional; ToolName is optional.
type Entry struct {
	Actor    string
	Action   Action
	RunID    uuid.NullUUID
	JTI      uuid.NullUUID
	ToolName string
	Detail   map[string]any
}

// Record writes an audit entry using the given tenant-scoped Queries. Because it
// runs inside the caller's WithTenant transaction, RLS binds it to tenantID and
// WITH CHECK rejects any attempt to write into another tenant.
func Record(ctx context.Context, q *dbgen.Queries, tenantID uuid.UUID, e Entry) error {
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("audit: encode detail: %w", err)
	}
	_, err = q.InsertAuditLog(ctx, dbgen.InsertAuditLogParams{
		TenantID: tenantID,
		Actor:    e.Actor,
		Action:   string(e.Action),
		RunID:    e.RunID,
		Jti:      e.JTI,
		ToolName: pgtype.Text{String: e.ToolName, Valid: e.ToolName != ""},
		Detail:   raw,
	})
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}
