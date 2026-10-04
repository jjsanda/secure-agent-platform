package api

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	dbgen "github.com/jjsanda/secure-agent-platform/control-plane/internal/db/gen"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/dispatch"
)

// TenantDTO is the public shape of a tenant.
type TenantDTO struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

func mapTenant(t dbgen.AppTenant) TenantDTO {
	return TenantDTO{ID: t.ID.String(), Slug: t.Slug, Name: t.Name, CreatedAt: t.CreatedAt.Time}
}

func mapTenants(ts []dbgen.AppTenant) []TenantDTO {
	out := make([]TenantDTO, len(ts))
	for i, t := range ts {
		out[i] = mapTenant(t)
	}
	return out
}

// RunDTO is the public shape of an agent run.
type RunDTO struct {
	ID           string    `json:"id"`
	Objective    string    `json:"objective"`
	Variant      string    `json:"variant"`
	AllowedTools []string  `json:"allowedTools"`
	Status       string    `json:"status"`
	FinalAnswer  *string   `json:"finalAnswer"`
	Error        *string   `json:"error"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func mapRun(r dbgen.AppRun) RunDTO {
	return RunDTO{
		ID:           r.ID.String(),
		Objective:    r.Objective,
		Variant:      r.Variant,
		AllowedTools: r.AllowedTools,
		Status:       r.Status,
		FinalAnswer:  textPtr(r.FinalAnswer),
		Error:        textPtr(r.Error),
		CreatedAt:    r.CreatedAt.Time,
		UpdatedAt:    r.UpdatedAt.Time,
	}
}

func mapRuns(rs []dbgen.AppRun) []RunDTO {
	out := make([]RunDTO, len(rs))
	for i, r := range rs {
		out[i] = mapRun(r)
	}
	return out
}

// RunEventDTO is one item in a run's event trace (matches app.run_event).
type RunEventDTO struct {
	ID      int64           `json:"id"`
	Kind    string          `json:"kind"`
	Step    int32           `json:"step"`
	Payload json.RawMessage `json:"payload"`
	At      time.Time       `json:"at"`
}

func mapRunEvent(e dbgen.AppRunEvent) RunEventDTO {
	return RunEventDTO{ID: e.ID, Kind: e.Kind, Step: e.Step, Payload: e.Payload, At: e.At.Time}
}

func eventToDTO(e dispatch.Event) RunEventDTO {
	return RunEventDTO{ID: e.ID, Kind: e.Kind, Step: e.Step, Payload: e.Payload, At: e.At}
}

// AuditDTO is one append-only audit record.
type AuditDTO struct {
	ID       int64           `json:"id"`
	At       time.Time       `json:"at"`
	Actor    string          `json:"actor"`
	Action   string          `json:"action"`
	RunID    *string         `json:"runId"`
	ToolName *string         `json:"toolName"`
	Detail   json.RawMessage `json:"detail"`
}

func mapAudit(a dbgen.AppAuditLog) AuditDTO {
	return AuditDTO{
		ID:       a.ID,
		At:       a.At.Time,
		Actor:    a.Actor,
		Action:   a.Action,
		RunID:    nullUUIDStr(a.RunID),
		ToolName: textPtr(a.ToolName),
		Detail:   a.Detail,
	}
}

func mapAudits(as []dbgen.AppAuditLog) []AuditDTO {
	out := make([]AuditDTO, len(as))
	for i, a := range as {
		out[i] = mapAudit(a)
	}
	return out
}

// MeDTO is the caller's identity, straight from the verified token.
type MeDTO struct {
	Subject  string   `json:"subject"`
	TenantID string   `json:"tenantId"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Roles    []string `json:"roles"`
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func textFrom(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func nullUUIDStr(u uuid.NullUUID) *string {
	if !u.Valid {
		return nil
	}
	s := u.UUID.String()
	return &s
}
