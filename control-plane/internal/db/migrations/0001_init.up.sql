-- 0001_init: tenant tables + row-level security.
-- Runs as sap_owner (owns schema `app`). The RLS block is what makes tenant
-- isolation a database-enforced guarantee rather than an application convention.

CREATE SCHEMA IF NOT EXISTS app;

CREATE TABLE app.tenant (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       text NOT NULL UNIQUE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.account (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES app.tenant (id) ON DELETE CASCADE,
    subject      text NOT NULL UNIQUE,
    email        text,
    display_name text,
    roles        text[] NOT NULL DEFAULT '{}',
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX account_tenant_idx ON app.account (tenant_id);

CREATE TABLE app.run (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES app.tenant (id) ON DELETE CASCADE,
    created_by    uuid REFERENCES app.account (id) ON DELETE SET NULL,
    objective     text NOT NULL,
    variant       text NOT NULL DEFAULT 'custom',
    allowed_tools text[] NOT NULL DEFAULT '{}',
    status        text NOT NULL DEFAULT 'pending',
    final_answer  text,
    error         text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX run_tenant_created_idx ON app.run (tenant_id, created_at DESC);

CREATE TABLE app.run_event (
    id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES app.tenant (id) ON DELETE CASCADE,
    run_id    uuid NOT NULL REFERENCES app.run (id) ON DELETE CASCADE,
    step      integer NOT NULL DEFAULT 0,
    kind      text NOT NULL,
    payload   jsonb NOT NULL DEFAULT '{}'::jsonb,
    at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX run_event_run_idx ON app.run_event (run_id, id);

CREATE TABLE app.scoped_credential (
    jti           uuid PRIMARY KEY,
    tenant_id     uuid NOT NULL REFERENCES app.tenant (id) ON DELETE CASCADE,
    run_id        uuid NOT NULL REFERENCES app.run (id) ON DELETE CASCADE,
    allowed_tools text[] NOT NULL,
    issued_at     timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    revoked_at    timestamptz,
    last_used_at  timestamptz
);
CREATE INDEX scoped_credential_run_idx ON app.scoped_credential (run_id);

CREATE TABLE app.audit_log (
    id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES app.tenant (id) ON DELETE CASCADE,
    at        timestamptz NOT NULL DEFAULT now(),
    actor     text NOT NULL DEFAULT 'system',
    action    text NOT NULL,
    run_id    uuid,
    jti       uuid,
    tool_name text,
    detail    jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX audit_log_tenant_at_idx ON app.audit_log (tenant_id, at DESC);

-- ---------------------------------------------------------------------------
-- Row-Level Security.
--
-- Kept inside a DO block so the statements apply at runtime but stay opaque to
-- static schema parsers (sqlc). ENABLE + FORCE means the policy applies even to
-- the table owner; combined with a NOBYPASSRLS, non-owner runtime role, tenant
-- isolation cannot be bypassed and cannot be forgotten in a query.
--
-- NULLIF(current_setting('app.current_tenant', true), '')::uuid is the
-- fail-closed predicate:
--   * unset GUC          -> current_setting(..., true) returns NULL
--   * pooled connection   -> a prior transaction-local set reverts the custom
--     GUC to an EMPTY STRING (not NULL) once the tx ends; NULLIF folds '' to
--     NULL so we deny cleanly instead of raising on ''::uuid
--   * NULL = <uuid>       -> NULL -> false -> zero rows, insert rejected
-- The two-argument current_setting avoids the exception the one-arg form raises
-- on an unknown GUC.
-- ---------------------------------------------------------------------------
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['account', 'run', 'run_event', 'scoped_credential', 'audit_log'] LOOP
        EXECUTE format('ALTER TABLE app.%I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE app.%I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation ON app.%I '
            'USING (tenant_id = NULLIF(current_setting(''app.current_tenant'', true), '''')::uuid) '
            'WITH CHECK (tenant_id = NULLIF(current_setting(''app.current_tenant'', true), '''')::uuid)',
            t);
    END LOOP;

    -- The tenant table isolates on its own id (each tenant sees only itself).
    EXECUTE 'ALTER TABLE app.tenant ENABLE ROW LEVEL SECURITY';
    EXECUTE 'ALTER TABLE app.tenant FORCE ROW LEVEL SECURITY';
    EXECUTE 'CREATE POLICY tenant_isolation ON app.tenant '
        || 'USING (id = NULLIF(current_setting(''app.current_tenant'', true), '''')::uuid) '
        || 'WITH CHECK (id = NULLIF(current_setting(''app.current_tenant'', true), '''')::uuid)';
END $$;

-- Runtime DML for the non-privileged app role and the (BYPASSRLS) seed role.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA app TO sap_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA app TO sap_migrator;
