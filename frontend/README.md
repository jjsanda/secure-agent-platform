# Secure Agent Platform — Dashboard

The React single-page dashboard for `secure-agent-platform`. It signs in via OIDC (authorization-code + PKCE), then shows the caller's tenant, their agent runs with a **live event stream**, a run-creation form, and the security **audit log** — all scoped to the signed-in tenant by the control plane's row-level security.

Signing in as **Alice (Tenant A)** or **Bob (Tenant B)** and seeing different data is the whole demo, so the current user and tenant are always visible in the header.

## Stack

| Concern       | Choice                                                       |
| ------------- | ------------------------------------------------------------ |
| Build / dev   | [Vite](https://vite.dev) 6 + React 18 + TypeScript (strict)  |
| Styling       | Tailwind CSS 3 + shadcn/ui-style components (hand-rolled)    |
| Data fetching | [TanStack Query](https://tanstack.com/query) 5               |
| Auth          | `react-oidc-context` + `oidc-client-ts` (auth-code + PKCE)   |
| Routing       | `react-router-dom` 6                                         |
| Icons         | `lucide-react`                                               |
| Tooling       | ESLint 9 (flat config) · Prettier · `tsc` project references |

Requires **Node 22**.

## Quick start

```bash
cd frontend
npm install
npm run dev          # Vite dev server on http://localhost:5173
```

The dev server expects the rest of the stack (control plane + mock OIDC) to be running — from the repo root, `make up` starts the default zero-secret profile. You can then open <http://localhost:5173> and sign in.

### Scripts

| Script                 | Purpose                                                |
| ---------------------- | ------------------------------------------------------ |
| `npm run dev`          | Start the Vite dev server (HMR).                       |
| `npm run build`        | Type-check (`tsc -b`) then produce a production build. |
| `npm run preview`      | Serve the production build locally.                    |
| `npm run lint`         | ESLint over the whole project.                         |
| `npm run format`       | Format with Prettier.                                  |
| `npm run format:check` | Verify formatting (used in CI-style checks).           |

## Environment variables

All are read at **build time** (Vite inlines `import.meta.env.VITE_*`). Defaults match the default docker-compose profile, so no configuration is needed for the local stack. Copy `.env.example` to `.env` to override for `npm run dev`.

| Variable                 | Default                          | Meaning                             |
| ------------------------ | -------------------------------- | ----------------------------------- |
| `VITE_API_BASE_URL`      | `http://localhost:8080`          | Control-plane REST + SSE base URL.  |
| `VITE_OIDC_ISSUER`       | `http://localhost:9000`          | OIDC issuer (discovery authority).  |
| `VITE_OIDC_CLIENT_ID`    | `sap-dashboard`                  | Public OIDC client id.              |
| `VITE_OIDC_REDIRECT_URI` | `http://localhost:5173/callback` | Authorization-code redirect target. |

## Authentication (OIDC authorization-code + PKCE)

- Configured in [`src/auth/oidc.ts`](src/auth/oidc.ts); the provider is wired in [`src/main.tsx`](src/main.tsx) and gated in [`src/App.tsx`](src/App.tsx).
- `scope = "openid profile email"`, `response_type = "code"`. PKCE (S256) is automatic in `oidc-client-ts`.
- The authenticated **user is held in memory only** — no tokens are written to `localStorage`. The transient auth/PKCE state keeps the default `localStorage` store because it must survive the full-page redirect to the IdP and back. A hard refresh therefore returns to the sign-in screen (one click to re-auth).
- The IdP redirects to `/callback`; `react-oidc-context` completes the code exchange automatically, `onSigninCallback` strips the OAuth params from the URL, and the `/callback` route lands the router on `/`.
- Sign-out uses the IdP's `end_session_endpoint` via `signoutRedirect()`.
- The API client sends the **access token** as `Authorization: Bearer <token>` on every `/api/v1` call (see [`src/api/client.ts`](src/api/client.ts)).

## Live run events (header-authenticated SSE)

Native `EventSource` cannot set an `Authorization` header, so the run event stream is read with **`fetch` + a `ReadableStream` reader** and the SSE frames are parsed by hand — see [`src/api/sse.ts`](src/api/sse.ts) and the [`useRunEventStream`](src/api/useRunEventStream.ts) hook:

- `fetch(`${base}/api/v1/runs/${id}/events`, { headers: { Authorization } })`, then read `response.body` and split frames on the blank-line delimiter.
- Each frame's `data:` line is a JSON `RunEventDTO { id, kind, step, payload, at }`; `payload` is discriminated by `kind` (`status`, `plan`, `llm_message`, `tool_requested`, `tool_result`, `final`, `error`).
- A frame with `event: done` ends the stream. Completed runs are replayed from the durable log; active runs stream live.
- The reader is aborted (`AbortController`) on unmount / navigation, events are de-duplicated by id, and on completion the run, runs list, and audit log are refreshed. The timeline auto-scrolls as frames arrive.

## Routes

| Route       | View                                                                |
| ----------- | ------------------------------------------------------------------- |
| `/`         | Overview — identity, tenant card(s), and a "what this shows" intro. |
| `/runs`     | Runs list + "New run" form (objective, variant, tool multi-select). |
| `/runs/:id` | Run detail with the live event timeline and final answer / error.   |
| `/audit`    | Audit log, colour-coded for `*.denied` vs `*.allowed`/`*.executed`. |
| `/callback` | OIDC redirect landing (auto-redirects to `/`).                      |

## Project structure

```
src/
  api/          fetch client, TanStack Query hooks, fetch-based SSE reader
  auth/         react-oidc-context configuration
  components/   header, layout, timeline, forms, state views, ui/ primitives
  pages/        Overview · Runs · RunDetail · Audit · NotFound
  lib/          cn() helper, date/JSON formatting, status/kind visual mappings
  types.ts      DTOs mirroring the control-plane REST + SSE contract
  config.ts     env-derived runtime configuration
```

## Production build (Docker)

The [`Dockerfile`](Dockerfile) is multi-stage: a `node:22` stage runs `npm ci` and `npm run build`, then an `nginx:alpine` stage serves `dist/` on port 80. The [`nginx.conf`](nginx.conf) SPA fallback (`try_files $uri /index.html;`) means client-side routes like `/callback` and `/runs/:id` resolve on a hard refresh.

```bash
# from the repo root — build context is frontend/
docker build -f frontend/Dockerfile -t sap-frontend frontend
docker run --rm -p 5173:80 sap-frontend
```

`VITE_*` values are baked at build time from the defaults above, or pass `--build-arg VITE_API_BASE_URL=… ` etc. to target another environment.
