# Security Policy

`secure-agent-platform` is a portfolio project that demonstrates secure design patterns (multi-tenant isolation, scoped credentials, prompt-injection hardening). It is **not** a production service, but security is the whole point, so issues are taken seriously.

## Reporting a vulnerability

Please open a private report or email **pepek.svanda@gmail.com** with:

- a description of the issue and its impact,
- steps to reproduce (a failing test is ideal), and
- any suggested remediation.

Please do not open a public issue for anything that could expose tenant data or allow credential/tool-policy bypass until it has been addressed.

## Scope & threat model

The design, trust boundaries, and **explicit limitations** are documented in [`docs/security-model.md`](docs/security-model.md) and [`docs/threat-model.md`](docs/threat-model.md). In short: the platform aims for *defense in depth* (Postgres row-level security, capability-scoped short-lived credentials, an allow-listed tool proxy, and an auditable trail) — not for "prompt-injection-proof" guarantees, which are not achievable with heuristics alone.
