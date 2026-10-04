#!/usr/bin/env bash
# Runs ONCE, as the bootstrap superuser, on first Postgres start.
#
# Establishes the three-tier role model the RLS design depends on:
#   sap_owner    — owns the schema/tables; runs migrations (DDL). NOT a superuser.
#   sap_app      — the runtime connection role. DML only, NOBYPASSRLS, NOT a table owner.
#   sap_migrator — BYPASSRLS, for cross-tenant seed/admin tooling ONLY (never the app).
#
# Why it matters: superusers and table owners bypass RLS. Keeping the runtime
# role non-superuser, non-owner, and NOBYPASSRLS is what makes FORCE ROW LEVEL
# SECURITY actually enforce tenant isolation.
set -euo pipefail

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
	CREATE ROLE sap_owner    LOGIN PASSWORD '${POSTGRES_OWNER_PASSWORD}'    NOSUPERUSER NOBYPASSRLS;
	CREATE ROLE sap_app      LOGIN PASSWORD '${POSTGRES_APP_PASSWORD}'      NOSUPERUSER NOBYPASSRLS;
	CREATE ROLE sap_migrator LOGIN PASSWORD '${POSTGRES_MIGRATOR_PASSWORD}' NOSUPERUSER BYPASSRLS;

	-- The application database is owned by sap_owner so migrations (run as
	-- sap_owner) can manage schema objects.
	ALTER DATABASE ${POSTGRES_DB} OWNER TO sap_owner;
	GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO sap_app, sap_migrator;

	-- Application schema, owned by sap_owner. Migrations create tables here and
	-- grant DML to sap_app; sap_app never owns anything.
	CREATE SCHEMA IF NOT EXISTS app AUTHORIZATION sap_owner;
	GRANT USAGE ON SCHEMA app TO sap_app, sap_migrator;

	ALTER ROLE sap_owner    IN DATABASE ${POSTGRES_DB} SET search_path TO app, public;
	ALTER ROLE sap_app      IN DATABASE ${POSTGRES_DB} SET search_path TO app, public;
	ALTER ROLE sap_migrator IN DATABASE ${POSTGRES_DB} SET search_path TO app, public;

	-- Lock down the public schema (defense in depth).
	REVOKE ALL ON SCHEMA public FROM PUBLIC;
	GRANT USAGE ON SCHEMA public TO sap_owner, sap_app, sap_migrator;
SQL

echo "sap: roles (sap_owner / sap_app / sap_migrator) and schema 'app' initialized"
