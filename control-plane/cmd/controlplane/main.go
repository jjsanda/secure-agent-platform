// Command controlplane is the secure-agent-platform control plane: it exposes
// the REST + SSE API and the internal gRPC tool proxy, mints scoped credentials,
// dispatches runs to the agent worker, and enforces tenant isolation via
// PostgreSQL row-level security.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/api"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/auth"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/config"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/credential"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/db"
	dbgen "github.com/jjsanda/secure-agent-platform/control-plane/internal/db/gen"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/devseed"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/dispatch"
	sapv1 "github.com/jjsanda/secure-agent-platform/control-plane/internal/gen/proto/sap/v1"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/telemetry"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/tenancy"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/toolproxy"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/tools"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(); err != nil {
		slog.Error("control-plane exited with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.FromEnv()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 0. OpenTelemetry (no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set).
	shutdownTel, err := telemetry.Setup(ctx, "control-plane", cfg.ServiceNamespace, cfg.OTLPEndpoint)
	if err != nil {
		return err
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTel(sctx)
	}()
	metrics, err := telemetry.NewMetrics()
	if err != nil {
		return err
	}

	// 1. Migrations run as the schema owner (DDL).
	slog.Info("running database migrations")
	if err := db.Migrate(cfg.MigrateURL); err != nil {
		return err
	}

	// 2. Dev seed via the BYPASSRLS migrator role. Idempotent.
	if cfg.SeedDemo {
		if err := seedDemo(ctx, cfg.MigratorURL); err != nil {
			slog.Warn("dev seed failed (continuing without it)", "err", err)
		}
	}

	// 3. Runtime pool — the non-privileged, NOBYPASSRLS app role.
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := tenancy.NewStore(pool)

	// 4. Scoped-credential signer (mint) + verifier (used by the tool proxy).
	signer, err := buildSigner(cfg)
	if err != nil {
		return err
	}
	credVerifier := signer.Verifier()

	// 5. Global tool registry (the outermost tool allow-list). http_get is a
	//    deliberately guarded tool: an unguarded fetch is an SSRF primitive.
	registry := tools.NewRegistry(tools.Echo{}, tools.HTTPGet{})

	// 6. OIDC verifier — provider-agnostic. Blocks (with retry) until the IdP
	//    is reachable, since it may still be starting in compose.
	slog.Info("discovering OIDC provider", "issuer", cfg.OIDCIssuerURL, "discovery", discoveryURL(cfg))
	oidcVerifier, err := auth.NewVerifier(ctx, cfg.OIDCIssuerURL, cfg.OIDCDiscoveryURL, cfg.OIDCAudience)
	if err != nil {
		return err
	}

	// 7. Agent-worker gRPC client (lazy connect; the worker may start after us).
	workerConn, err := grpc.NewClient(cfg.WorkerGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return err
	}
	defer func() { _ = workerConn.Close() }()
	workerClient := sapv1.NewRunnerServiceClient(workerConn)

	// 8. Run dispatcher + SSE hub.
	hub := dispatch.NewHub()
	dispatcher := dispatch.New(store, signer, workerClient, hub, metrics)

	// 9. gRPC ToolProxy server — the credential interceptor authorizes every call;
	//    the otelgrpc stats handler ties it into the run's trace.
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			toolproxy.CredentialInterceptor(credVerifier, store, metrics),
		),
	)
	sapv1.RegisterToolProxyServiceServer(grpcServer, toolproxy.NewServer(store, registry, metrics))

	// 10. HTTP API (otelhttp wraps the router so requests start a trace).
	apiSrv := api.New(store, dispatcher, hub, oidcVerifier, metrics, cfg.CORSOrigin)
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           otelhttp.NewHandler(apiSrv.Router(), "control-plane"),
		ReadHeaderTimeout: 10 * time.Second,
	}

	grpcLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 2)
	go func() {
		slog.Info("gRPC ToolProxy listening", "addr", cfg.GRPCAddr)
		errCh <- grpcServer.Serve(grpcLis)
	}()
	go func() {
		slog.Info("HTTP API listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			slog.Error("server stopped", "err", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)

	// Drain in-flight gRPC calls, but don't hang forever on a stuck handler.
	stopped := make(chan struct{})
	go func() { grpcServer.GracefulStop(); close(stopped) }()
	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
	}
	slog.Info("control-plane stopped")
	return nil
}

func buildSigner(cfg config.Config) (*credential.Signer, error) {
	if cfg.PASETOPrivateKey != "" {
		return credential.NewSignerFromSeedHex(cfg.PASETOPrivateKey)
	}
	slog.Warn("PASETO_PRIVATE_KEY not set — generating an ephemeral dev keypair (run `make keys` for a stable one)")
	return credential.NewEphemeralSigner(), nil
}

func discoveryURL(cfg config.Config) string {
	if cfg.OIDCDiscoveryURL != "" {
		return cfg.OIDCDiscoveryURL
	}
	return cfg.OIDCIssuerURL
}

// seedDemo inserts the two demo tenants using the BYPASSRLS migrator role, so the
// zero-config profile is immediately usable with two isolated tenants.
func seedDemo(ctx context.Context, migratorURL string) error {
	pool, err := db.NewPool(ctx, migratorURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := dbgen.New(pool)
	for _, t := range devseed.Tenants {
		id, err := uuid.Parse(t.ID)
		if err != nil {
			return err
		}
		if err := q.SeedTenant(ctx, dbgen.SeedTenantParams{ID: id, Slug: t.Slug, Name: t.Name}); err != nil {
			return err
		}
	}
	slog.Info("seeded demo tenants", "count", len(devseed.Tenants))
	return nil
}
