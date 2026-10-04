// Package config loads control-plane configuration from the environment. The
// default profile runs with zero secrets (mock LLM + mock OIDC), so every value
// has a sensible local default.
package config

import "os"

// Config is the resolved control-plane configuration.
type Config struct {
	HTTPAddr       string // REST + SSE listen address
	GRPCAddr       string // ToolProxy gRPC listen address
	WorkerGRPCAddr string // agent worker RunnerService address

	DatabaseURL string // runtime pool — sap_app (NOBYPASSRLS)
	MigrateURL  string // migrations — sap_owner (DDL), pgx5 scheme
	MigratorURL string // BYPASSRLS pool — sap_migrator, dev seed only
	SeedDemo    bool   // seed the two demo tenants on startup (dev default)

	OIDCIssuerURL    string // token `iss` / browser-facing issuer
	OIDCDiscoveryURL string // where the backend fetches discovery (may differ in Docker)
	OIDCAudience     string // expected access-token audience

	PASETOPrivateKey string // hex Ed25519 seed (empty in dev -> ephemeral keypair)
	PASETOPublicKey  string // hex Ed25519 public key

	CORSOrigin string // SPA origin allowed to call the API

	OTLPEndpoint     string // empty -> telemetry disabled (no-op)
	ServiceNamespace string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// FromEnv builds a Config from environment variables, applying local defaults.
func FromEnv() Config {
	return Config{
		HTTPAddr:       env("CONTROL_PLANE_HTTP_ADDR", ":8080"),
		GRPCAddr:       env("CONTROL_PLANE_GRPC_ADDR", ":9090"),
		WorkerGRPCAddr: env("WORKER_GRPC_ADDR", "localhost:50051"),

		DatabaseURL: env("DATABASE_URL",
			"postgres://sap_app:sap_app_dev_pw@localhost:5432/sap?sslmode=disable"),
		MigrateURL: env("MIGRATE_DATABASE_URL",
			"pgx5://sap_owner:sap_owner_dev_pw@localhost:5432/sap?sslmode=disable"),
		MigratorURL: env("MIGRATOR_DATABASE_URL",
			"postgres://sap_migrator:sap_migrator_dev_pw@localhost:5432/sap?sslmode=disable"),
		SeedDemo: env("SEED_DEMO", "true") == "true",

		OIDCIssuerURL:    env("OIDC_ISSUER_URL", "http://localhost:9000"),
		OIDCDiscoveryURL: os.Getenv("OIDC_DISCOVERY_URL"), // empty -> use issuer
		OIDCAudience:     env("OIDC_AUDIENCE", "sap-control-plane"),

		PASETOPrivateKey: os.Getenv("PASETO_PRIVATE_KEY"),
		PASETOPublicKey:  os.Getenv("PASETO_PUBLIC_KEY"),

		CORSOrigin: env("CORS_ORIGIN", "http://localhost:5173"),

		OTLPEndpoint:     os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		ServiceNamespace: env("OTEL_SERVICE_NAMESPACE", "sap"),
	}
}
