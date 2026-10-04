// Package tenancy is the single choke point for tenant-scoped database access.
//
// Row-level security is the isolation guarantee; this package is what activates
// it per request. Nothing else in the codebase may touch the pool directly — an
// architecture test (tenancy_arch_test.go) fails the build if it does. Because
// every query runs inside WithTenant, application code never writes a tenant_id
// filter by hand, so it can never forget one.
package tenancy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "github.com/jjsanda/secure-agent-platform/control-plane/internal/db/gen"
)

// Store wraps the runtime pool (sap_app, NOBYPASSRLS) and exposes only
// tenant-scoped access.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps a runtime pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Ping checks database connectivity (used by the health endpoint).
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// WithTenant runs fn inside a transaction whose app.current_tenant GUC is set to
// tenantID, so that row-level security filters every statement to that tenant.
//
//   - set_config(..., is_local => true) binds the tenant id as a PARAMETER (no
//     SQL string building — no injection surface) and scopes it to the
//     transaction, so a pooled connection can never leak one tenant's context
//     into another request.
//   - If fn returns an error the transaction is rolled back and the GUC is
//     discarded with it. Callers get all-or-nothing semantics for free.
func (s *Store) WithTenant(ctx context.Context, tenantID uuid.UUID, fn func(q *dbgen.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tenancy: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once committed

	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.current_tenant', $1, true)`,
		tenantID.String(),
	); err != nil {
		return fmt.Errorf("tenancy: set current_tenant: %w", err)
	}

	if err := fn(dbgen.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("tenancy: commit: %w", err)
	}
	return nil
}
