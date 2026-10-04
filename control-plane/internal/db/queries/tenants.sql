-- name: GetTenant :one
SELECT * FROM app.tenant
WHERE id = $1;

-- name: GetTenantBySlug :one
SELECT * FROM app.tenant
WHERE slug = $1;

-- name: ListTenants :many
-- RLS scopes this to the caller's own tenant; a platform-admin path uses a
-- separate BYPASSRLS connection for cross-tenant listing.
SELECT * FROM app.tenant
ORDER BY created_at;

-- name: CreateTenant :one
-- Only usable on a BYPASSRLS connection (seeding/admin): WITH CHECK on the
-- tenant table would otherwise reject inserting a row for a not-yet-current tenant.
INSERT INTO app.tenant (slug, name)
VALUES ($1, $2)
RETURNING *;

-- name: SeedTenant :exec
-- Insert a tenant with an explicit id (BYPASSRLS seeding only). Idempotent, so
-- the dev seed can run on every startup.
INSERT INTO app.tenant (id, slug, name)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO NOTHING;
