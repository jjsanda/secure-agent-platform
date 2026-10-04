//go:build integration

// Package integration spins up a real PostgreSQL via testcontainers and proves
// the row-level-security tenant isolation end to end. These tests need Docker
// and run only under `-tags=integration` (see the Security Gates CI workflow).
package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/db"
	dbgen "github.com/jjsanda/secure-agent-platform/control-plane/internal/db/gen"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/tenancy"
)

const (
	testOwnerPw    = "owner_pw"
	testAppPw      = "app_pw"
	testMigratorPw = "migrator_pw"
)

// rolesSQL mirrors deploy/compose/initdb/00-init-roles.sh with fixed test
// credentials. It establishes the three-tier role model the RLS design needs.
const rolesSQL = `
CREATE ROLE sap_owner    LOGIN PASSWORD '` + testOwnerPw + `'    NOSUPERUSER NOBYPASSRLS;
CREATE ROLE sap_app      LOGIN PASSWORD '` + testAppPw + `'      NOSUPERUSER NOBYPASSRLS;
CREATE ROLE sap_migrator LOGIN PASSWORD '` + testMigratorPw + `' NOSUPERUSER BYPASSRLS;
ALTER DATABASE sap OWNER TO sap_owner;
GRANT CONNECT ON DATABASE sap TO sap_app, sap_migrator;
CREATE SCHEMA IF NOT EXISTS app AUTHORIZATION sap_owner;
GRANT USAGE ON SCHEMA app TO sap_app, sap_migrator;
ALTER ROLE sap_owner    IN DATABASE sap SET search_path TO app, public;
ALTER ROLE sap_app      IN DATABASE sap SET search_path TO app, public;
ALTER ROLE sap_migrator IN DATABASE sap SET search_path TO app, public;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO sap_owner, sap_app, sap_migrator;
`

type harness struct {
	appPool      *pgxpool.Pool // runtime role: sap_app (NOBYPASSRLS)
	migratorPool *pgxpool.Pool // BYPASSRLS: seeding + meta queries only
	store        *tenancy.Store
}

// setup starts Postgres, bootstraps roles, migrates, and returns runtime pools.
func setup(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	pgC, err := tcpostgres.Run(ctx, "postgres:16",
		tcpostgres.WithDatabase("sap"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("bootstrap"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(120*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	testcontainers.CleanupContainer(t, pgC)

	host, err := pgC.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := pgC.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("container port: %v", err)
	}
	dsn := func(user, pw, scheme string) string {
		return fmt.Sprintf("%s://%s:%s@%s:%s/sap?sslmode=disable", scheme, user, pw, host, port.Port())
	}

	// 1. Bootstrap roles as the superuser. Simple protocol lets a single Exec
	//    run the whole multi-statement batch.
	superCfg, err := pgx.ParseConfig(dsn("postgres", "bootstrap", "postgres"))
	if err != nil {
		t.Fatalf("parse super dsn: %v", err)
	}
	superCfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	superConn, err := pgx.ConnectConfig(ctx, superCfg)
	if err != nil {
		t.Fatalf("connect super: %v", err)
	}
	if _, err := superConn.Exec(ctx, rolesSQL); err != nil {
		t.Fatalf("bootstrap roles: %v", err)
	}
	_ = superConn.Close(ctx)

	// 2. Migrations run as the schema owner (DDL) via golang-migrate's pgx5 driver.
	if err := db.Migrate(dsn("sap_owner", testOwnerPw, "pgx5")); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 3. Runtime pools.
	appPool, err := db.NewPool(ctx, dsn("sap_app", testAppPw, "postgres"))
	if err != nil {
		t.Fatalf("app pool: %v", err)
	}
	migratorPool, err := db.NewPool(ctx, dsn("sap_migrator", testMigratorPw, "postgres"))
	if err != nil {
		t.Fatalf("migrator pool: %v", err)
	}
	t.Cleanup(func() {
		appPool.Close()
		migratorPool.Close()
	})

	return &harness{appPool: appPool, migratorPool: migratorPool, store: tenancy.NewStore(appPool)}
}

// seedTenant inserts a tenant plus one run using the BYPASSRLS migrator pool
// (the only place cross-tenant writes are legitimate).
func (h *harness) seedTenant(t *testing.T, slug string) (dbgen.AppTenant, dbgen.AppRun) {
	t.Helper()
	ctx := context.Background()
	q := dbgen.New(h.migratorPool)

	ten, err := q.CreateTenant(ctx, dbgen.CreateTenantParams{Slug: slug, Name: slug})
	if err != nil {
		t.Fatalf("seed tenant %s: %v", slug, err)
	}
	run, err := q.CreateRun(ctx, dbgen.CreateRunParams{
		TenantID:     ten.ID,
		Objective:    "objective for " + slug,
		Variant:      "custom",
		AllowedTools: []string{"echo"},
		Status:       "pending",
	})
	if err != nil {
		t.Fatalf("seed run %s: %v", slug, err)
	}
	return ten, run
}
