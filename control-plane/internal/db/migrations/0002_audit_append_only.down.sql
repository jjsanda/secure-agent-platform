-- Restore the broad DML grant on the audit log for the runtime role.
GRANT UPDATE, DELETE ON app.audit_log TO sap_app;
