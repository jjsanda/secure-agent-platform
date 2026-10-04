//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	dbgen "github.com/jjsanda/secure-agent-platform/control-plane/internal/db/gen"
)

// TestTenantIsolation_DBLayer is the headline proof: with row-level security in
// force, tenant A cannot read, insert-for, or modify tenant B — even though both
// share one database, one connection pool, and one application role.
func TestTenantIsolation_DBLayer(t *testing.T) {
	h := setup(t)
	ctx := context.Background()

	tenA, runA := h.seedTenant(t, "tenant-a")
	tenB, runB := h.seedTenant(t, "tenant-b")

	t.Run("A lists only its own runs", func(t *testing.T) {
		var got []dbgen.AppRun
		if err := h.store.WithTenant(ctx, tenA.ID, func(q *dbgen.Queries) error {
			var e error
			got, e = q.ListRuns(ctx, 100)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != runA.ID {
			t.Fatalf("expected exactly run A visible, got %d run(s)", len(got))
		}
	})

	t.Run("A cannot read B's run (returns no rows)", func(t *testing.T) {
		err := h.store.WithTenant(ctx, tenA.ID, func(q *dbgen.Queries) error {
			_, e := q.GetRun(ctx, runB.ID)
			return e
		})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("expected pgx.ErrNoRows for cross-tenant read, got %v", err)
		}
	})

	t.Run("A cannot INSERT a row owned by B (WITH CHECK)", func(t *testing.T) {
		err := h.store.WithTenant(ctx, tenA.ID, func(q *dbgen.Queries) error {
			_, e := q.CreateRun(ctx, dbgen.CreateRunParams{
				TenantID:     tenB.ID, // smuggled tenant id
				Objective:    "smuggled",
				Variant:      "custom",
				AllowedTools: []string{},
				Status:       "pending",
			})
			return e
		})
		if err == nil {
			t.Fatal("expected WITH CHECK to reject inserting a row for another tenant")
		}
	})

	t.Run("A's UPDATE cannot touch B's run", func(t *testing.T) {
		// Under A, RLS's USING clause filters B's row out, so the UPDATE matches
		// zero rows and silently affects nothing.
		if err := h.store.WithTenant(ctx, tenA.ID, func(q *dbgen.Queries) error {
			return q.UpdateRunStatus(ctx, dbgen.UpdateRunStatusParams{ID: runB.ID, Status: "hijacked"})
		}); err != nil {
			t.Fatalf("update under A errored: %v", err)
		}
		// Read B's run under B: it must be untouched.
		var b dbgen.AppRun
		if err := h.store.WithTenant(ctx, tenB.ID, func(q *dbgen.Queries) error {
			var e error
			b, e = q.GetRun(ctx, runB.ID)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		if b.Status != "pending" {
			t.Fatalf("tenant B's run was modified across the tenant boundary: status=%q", b.Status)
		}
	})

	t.Run("no tenant context denies everything (fail-closed)", func(t *testing.T) {
		// A bare pool query with app.current_tenant unset: the policy predicate is
		// NULL -> false, so nothing is visible.
		var count int
		if err := h.appPool.QueryRow(ctx, "SELECT count(*) FROM app.run").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("expected 0 rows visible without a tenant context, got %d", count)
		}
	})
}

// TestAppRole_HasNoRLSBypass guards the invariants that make FORCE RLS
// meaningful: the runtime role is neither a superuser, nor a BYPASSRLS role, nor
// the owner of the tables. Any of those would silently defeat isolation.
func TestAppRole_HasNoRLSBypass(t *testing.T) {
	h := setup(t)
	ctx := context.Background()

	var super, bypass bool
	if err := h.migratorPool.QueryRow(ctx,
		`SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'sap_app'`,
	).Scan(&super, &bypass); err != nil {
		t.Fatal(err)
	}
	if super || bypass {
		t.Fatalf("sap_app must be NOSUPERUSER and NOBYPASSRLS; got super=%v bypass=%v", super, bypass)
	}

	var owner string
	if err := h.migratorPool.QueryRow(ctx,
		`SELECT tableowner FROM pg_tables WHERE schemaname = 'app' AND tablename = 'run'`,
	).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner == "sap_app" {
		t.Fatal("sap_app must not own app tables, or FORCE RLS could be bypassed")
	}
}
