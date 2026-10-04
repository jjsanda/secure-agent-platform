# ADR-0004: Two gRPC services and a tool proxy

- **Status:** accepted
- **Date:** 2026-07-01

## Context

The control plane (Go) and the agent worker (Python) must communicate: the control plane dispatches a task and streams back progress; the worker must perform side effects (tool calls) that have to be authorized and audited. We could give the worker direct access to tools/DB, or mediate everything.

## Decision

Two gRPC services, each hosted by the side that owns the capability:

- **`RunnerService.RunTask`** — implemented by the worker, called by the control plane; a server stream of run events. Carries the scoped credential in the request body.
- **`ToolProxyService.ExecuteTool`** — implemented by the control plane, called by the worker; the credential travels as gRPC **metadata** so a server interceptor authorizes it *before* the handler.

The worker gets **no** direct tool or database access. Every side effect is a call back through the tool proxy, which is the single place least privilege and auditing are enforced.

## Consequences

- One choke point for validation and audit; the worker's blast radius is exactly "what the tool proxy allows for this credential."
- A clean "both languages, both directions" story (external REST + internal gRPC).
- Credential placement is asymmetric by design (body for hand-off, metadata for per-call authorization), which is worth documenting so it doesn't look accidental.

## Alternatives considered

- **One bidirectional stream** — rejected: muddier ownership and harder to interpose an authorization interceptor per tool call.
- **Direct tool access in the worker** — rejected: it would put credentials and side-effect authority in the least-trusted component.
