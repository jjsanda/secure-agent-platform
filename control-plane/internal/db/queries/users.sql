-- name: GetUserBySubject :one
SELECT * FROM app.account
WHERE subject = $1;

-- name: UpsertUser :one
INSERT INTO app.account (tenant_id, subject, email, display_name, roles)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (subject) DO UPDATE
SET email = EXCLUDED.email,
    display_name = EXCLUDED.display_name,
    roles = EXCLUDED.roles
RETURNING *;
