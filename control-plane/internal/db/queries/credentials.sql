-- name: InsertScopedCredential :one
INSERT INTO app.scoped_credential (jti, tenant_id, run_id, allowed_tools, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetScopedCredential :one
SELECT * FROM app.scoped_credential
WHERE jti = $1;

-- name: TouchScopedCredential :exec
UPDATE app.scoped_credential
SET last_used_at = now()
WHERE jti = $1;

-- name: RevokeScopedCredential :exec
UPDATE app.scoped_credential
SET revoked_at = now()
WHERE jti = $1 AND revoked_at IS NULL;

-- name: RevokeRunCredentials :exec
UPDATE app.scoped_credential
SET revoked_at = now()
WHERE run_id = $1 AND revoked_at IS NULL;
