-- Make the audit log genuinely append-only for the runtime role.
--
-- The application only ever INSERTs and SELECTs audit rows (see internal/audit),
-- so revoking UPDATE and DELETE turns "append-only" from a code convention into a
-- database guarantee: even a compromised request path, within its own tenant,
-- cannot rewrite or erase history. The BYPASSRLS migrator/admin role keeps full
-- access for retention/administration tooling.
REVOKE UPDATE, DELETE ON app.audit_log FROM sap_app;
