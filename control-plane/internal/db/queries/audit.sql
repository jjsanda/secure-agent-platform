-- name: InsertAuditLog :one
INSERT INTO app.audit_log (tenant_id, actor, action, run_id, jti, tool_name, detail)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListAuditLog :many
SELECT * FROM app.audit_log
ORDER BY at DESC
LIMIT $1;

-- name: ListAuditLogForRun :many
SELECT * FROM app.audit_log
WHERE run_id = $1
ORDER BY at DESC;
