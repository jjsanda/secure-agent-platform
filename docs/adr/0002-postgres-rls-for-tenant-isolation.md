# ADR-0002: PostgreSQL row-level security for tenant isolation

- **Status:** accepted
- **Date:** 2026-07-01

## Context

Multi-tenant isolation can be enforced at the application layer (every query filters on `tenant_id`), at the database layer (row-level security), or by physical separation (a database per tenant). Application- layer filtering is the most common and the easiest to get subtly wrong — one forgotten `WHERE tenant_id = ?` is a cross-tenant leak. A database per tenant is operationally heavy and overkill here.

## Decision

Use PostgreSQL **row-level security** as the primary isolation mechanism, in depth:

- Every tenant table `ENABLE`s and **`FORCE`s** RLS with a `USING` + `WITH CHECK` policy keyed on a per-request session variable, `app.current_tenant`.
- The runtime connection uses a **non-superuser, non-owner, `NOBYPASSRLS`** role so the policy always applies.
- A single `WithTenant` transaction helper sets the variable with a parameterized, transaction-scoped `set_config`. Application queries carry **no** `tenant_id` filter — the database supplies it.
- The predicate is fail-closed via `NULLIF(current_setting(...), '')`, so an unset or pooled-reset context denies rather than errors.

## Consequences

- Isolation cannot be forgotten in a query, and holds even against the table owner.
- It is testable end-to-end against a real database (and is tested).
- Cost: all DB access must go through `WithTenant` (enforced by an architecture test), and truly cross-tenant operations (seeding) need a separate `BYPASSRLS` role.

## Alternatives considered

- **Application-layer filtering** — rejected as the sole mechanism: too easy to get wrong; no defense in depth. (It is still present as the belt to RLS's suspenders — inserts set `tenant_id` from context.)
- **Database-per-tenant** — rejected: operationally heavy for the scale this demonstrates.
