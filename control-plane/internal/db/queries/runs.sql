-- name: CreateRun :one
INSERT INTO app.run (tenant_id, created_by, objective, variant, allowed_tools, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetRun :one
-- No tenant_id filter: RLS supplies the predicate so it cannot be forgotten.
SELECT * FROM app.run
WHERE id = $1;

-- name: ListRuns :many
SELECT * FROM app.run
ORDER BY created_at DESC
LIMIT $1;

-- name: UpdateRunStatus :exec
UPDATE app.run
SET status = $2, updated_at = now()
WHERE id = $1;

-- name: FinishRun :exec
UPDATE app.run
SET status = $2, final_answer = $3, error = $4, updated_at = now()
WHERE id = $1;
