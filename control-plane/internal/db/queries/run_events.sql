-- name: AppendRunEvent :one
INSERT INTO app.run_event (tenant_id, run_id, step, kind, payload)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListRunEvents :many
SELECT * FROM app.run_event
WHERE run_id = $1
ORDER BY id;
