// Package api exposes the control plane's REST + SSE surface: tenants, agent
// runs, the live run-event stream, and the audit log. Every /api/v1 route is
// authenticated and runs its database work inside the caller's tenant scope, so
// row-level security is always in force.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/auth"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/dispatch"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/httpx"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/telemetry"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/tenancy"
)

// API bundles the dependencies the HTTP handlers need.
type API struct {
	store      *tenancy.Store
	dispatcher *dispatch.Dispatcher
	hub        *dispatch.Hub
	verifier   *auth.Verifier
	metrics    *telemetry.Metrics
	corsOrigin string
}

// New builds an API.
func New(store *tenancy.Store, d *dispatch.Dispatcher, hub *dispatch.Hub, v *auth.Verifier, metrics *telemetry.Metrics, corsOrigin string) *API {
	return &API{store: store, dispatcher: d, hub: hub, verifier: v, metrics: metrics, corsOrigin: corsOrigin}
}

// Router assembles the chi router. CORS is applied at the top so SPA preflight
// requests are answered before authentication.
func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(httpx.CORS(a.corsOrigin))
	r.Use(httpx.Logger)

	r.Get("/healthz", a.health)
	r.Get("/readyz", a.ready)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(a.verifier.Middleware)
		r.Get("/me", a.me)
		r.Get("/tenants", a.listTenants)
		r.Get("/tenants/{id}", a.getTenant)
		r.Get("/runs", a.listRuns)
		r.Post("/runs", a.createRun)
		r.Get("/runs/{id}", a.getRun)
		r.Get("/runs/{id}/events", a.runEvents)
		r.Get("/audit", a.listAudit)
	})
	return r
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Ping(r.Context()); err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
