package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/audit"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/auth"
	dbgen "github.com/jjsanda/secure-agent-platform/control-plane/internal/db/gen"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/httpx"
)

// caller extracts the verified claims and tenant UUID, or writes an error.
func (a *API) caller(w http.ResponseWriter, r *http.Request) (auth.Claims, uuid.UUID, bool) {
	c, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "no claims")
		return auth.Claims{}, uuid.Nil, false
	}
	tid, err := c.TenantUUID()
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid tenant claim")
		return auth.Claims{}, uuid.Nil, false
	}
	return c, tid, true
}

// lookupError distinguishes a genuine not-found from a real database error. A
// lookup-by-id that returns NO ROWS is, in a multi-tenant system, usually a
// cross-tenant / IDOR attempt that row-level security filtered out — a 404 plus
// the RLS metric. Any other error (a DB outage, a cancelled context) is a 500
// and must NOT pollute the isolation signal.
func (a *API) lookupError(w http.ResponseWriter, r *http.Request, err error, notFoundMsg string) {
	if errors.Is(err, pgx.ErrNoRows) {
		a.metrics.RLSDenied(r.Context())
		httpx.WriteError(w, http.StatusNotFound, notFoundMsg)
		return
	}
	slog.Error("api: lookup failed", "err", err)
	httpx.WriteError(w, http.StatusInternalServerError, "lookup failed")
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	c, tid, ok := a.caller(w, r)
	if !ok {
		return
	}
	// Provision the user row (idempotent) and record the login.
	_ = a.store.WithTenant(r.Context(), tid, func(q *dbgen.Queries) error {
		if _, err := q.UpsertUser(r.Context(), dbgen.UpsertUserParams{
			TenantID:    tid,
			Subject:     c.Subject,
			Email:       textFrom(c.Email),
			DisplayName: textFrom(c.Name),
			Roles:       c.Roles,
		}); err != nil {
			return err
		}
		return audit.Record(r.Context(), q, tid, audit.Entry{
			Actor:  c.Subject,
			Action: audit.ActionLogin,
			Detail: map[string]any{"email": c.Email},
		})
	})
	httpx.WriteJSON(w, http.StatusOK, MeDTO{
		Subject: c.Subject, TenantID: c.TenantID, Email: c.Email, Name: c.Name, Roles: c.Roles,
	})
}

func (a *API) listTenants(w http.ResponseWriter, r *http.Request) {
	_, tid, ok := a.caller(w, r)
	if !ok {
		return
	}
	var rows []dbgen.AppTenant
	if err := a.store.WithTenant(r.Context(), tid, func(q *dbgen.Queries) error {
		var e error
		rows, e = q.ListTenants(r.Context())
		return e
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list tenants")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mapTenants(rows))
}

func (a *API) getTenant(w http.ResponseWriter, r *http.Request) {
	_, tid, ok := a.caller(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid tenant id")
		return
	}
	var t dbgen.AppTenant
	if err := a.store.WithTenant(r.Context(), tid, func(q *dbgen.Queries) error {
		var e error
		t, e = q.GetTenant(r.Context(), id)
		return e
	}); err != nil {
		a.lookupError(w, r, err, "tenant not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mapTenant(t))
}

func (a *API) listRuns(w http.ResponseWriter, r *http.Request) {
	_, tid, ok := a.caller(w, r)
	if !ok {
		return
	}
	var rows []dbgen.AppRun
	if err := a.store.WithTenant(r.Context(), tid, func(q *dbgen.Queries) error {
		var e error
		rows, e = q.ListRuns(r.Context(), 100)
		return e
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list runs")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mapRuns(rows))
}

type createRunRequest struct {
	Objective    string   `json:"objective"`
	Variant      string   `json:"variant"`
	AllowedTools []string `json:"allowedTools"`
}

func (a *API) createRun(w http.ResponseWriter, r *http.Request) {
	c, tid, ok := a.caller(w, r)
	if !ok {
		return
	}
	var body createRunRequest
	// Cap the request body; a run request is small.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Objective) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "objective is required")
		return
	}
	variant := "custom"
	if body.Variant == "langgraph" {
		variant = "langgraph"
	}
	toolset := body.AllowedTools
	if len(toolset) == 0 {
		toolset = []string{"echo"}
	}

	var run dbgen.AppRun
	if err := a.store.WithTenant(r.Context(), tid, func(q *dbgen.Queries) error {
		user, err := q.UpsertUser(r.Context(), dbgen.UpsertUserParams{
			TenantID: tid, Subject: c.Subject, Email: textFrom(c.Email),
			DisplayName: textFrom(c.Name), Roles: c.Roles,
		})
		if err != nil {
			return err
		}
		run, err = q.CreateRun(r.Context(), dbgen.CreateRunParams{
			TenantID:     tid,
			CreatedBy:    uuid.NullUUID{UUID: user.ID, Valid: true},
			Objective:    body.Objective,
			Variant:      variant,
			AllowedTools: toolset,
			Status:       "pending",
		})
		if err != nil {
			return err
		}
		return audit.Record(r.Context(), q, tid, audit.Entry{
			Actor:  c.Subject,
			Action: audit.ActionRunCreated,
			RunID:  uuid.NullUUID{UUID: run.ID, Valid: true},
			Detail: map[string]any{"objective": body.Objective, "allowed_tools": toolset, "variant": variant},
		})
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "create run")
		return
	}

	a.dispatcher.Start(run.ID, tid, run.Objective, run.Variant, run.AllowedTools)
	httpx.WriteJSON(w, http.StatusCreated, mapRun(run))
}

func (a *API) getRun(w http.ResponseWriter, r *http.Request) {
	_, tid, ok := a.caller(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	var run dbgen.AppRun
	if err := a.store.WithTenant(r.Context(), tid, func(q *dbgen.Queries) error {
		var e error
		run, e = q.GetRun(r.Context(), id)
		return e
	}); err != nil {
		a.lookupError(w, r, err, "run not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mapRun(run))
}

func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	_, tid, ok := a.caller(w, r)
	if !ok {
		return
	}
	var rows []dbgen.AppAuditLog
	if err := a.store.WithTenant(r.Context(), tid, func(q *dbgen.Queries) error {
		var e error
		rows, e = q.ListAuditLog(r.Context(), 200)
		return e
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list audit")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mapAudits(rows))
}

// runEvents streams a run's event trace as Server-Sent Events. A completed run
// is replayed from the durable log; an active run is served live from the hub.
// The browser client authenticates with a fetch-based reader (EventSource cannot
// set the Authorization header).
func (a *API) runEvents(w http.ResponseWriter, r *http.Request) {
	_, tid, ok := a.caller(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	var run dbgen.AppRun
	if err := a.store.WithTenant(r.Context(), tid, func(q *dbgen.Queries) error {
		var e error
		run, e = q.GetRun(r.Context(), id)
		return e
	}); err != nil {
		a.lookupError(w, r, err, "run not found")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if run.Status == "succeeded" || run.Status == "failed" {
		var events []dbgen.AppRunEvent
		_ = a.store.WithTenant(r.Context(), tid, func(q *dbgen.Queries) error {
			var e error
			events, e = q.ListRunEvents(r.Context(), id)
			return e
		})
		for _, e := range events {
			a.sendSSE(w, flusher, mapRunEvent(e))
		}
		a.sendDone(w, flusher)
		return
	}

	history, ch, done, cancel := a.hub.Subscribe(id.String())
	defer cancel()
	for _, e := range history {
		a.sendSSE(w, flusher, eventToDTO(e))
	}
	if done {
		a.sendDone(w, flusher)
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				a.sendDone(w, flusher)
				return
			}
			a.sendSSE(w, flusher, eventToDTO(ev))
		}
	}
}

func (a *API) sendSSE(w http.ResponseWriter, flusher http.Flusher, ev RunEventDTO) {
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Kind, raw)
	flusher.Flush()
}

func (a *API) sendDone(w http.ResponseWriter, flusher http.Flusher) {
	_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
}
