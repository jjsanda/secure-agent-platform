# ADR-0001: Record architecture decisions

- **Status:** accepted
- **Date:** 2026-07-01

## Context

This project makes several non-obvious design choices (row-level security for tenant isolation, PASETO instead of JWT for scoped credentials, two gRPC services rather than one, Terraform that applies to a local `kind` cluster). A reader should be able to understand *why* without archaeology.

## Decision

We keep lightweight Architecture Decision Records in `docs/adr/`, one file per decision, using the template in `ADR-0000`. ADRs are immutable once accepted; a later decision supersedes an earlier one rather than editing it.

## Consequences

- The reasoning behind each choice is discoverable and reviewable.
- A small ongoing cost: each significant decision gets a short write-up.

## Alternatives considered

- **No ADRs** — reasoning lives only in commit messages and code comments, which scatter and rot. Rejected: this repo's value is partly in explaining itself.
