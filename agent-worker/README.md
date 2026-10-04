# agent-worker

The **Python agent worker** for the Secure Agent Platform. The Go control plane dispatches a task to this worker over gRPC (`RunnerService.RunTask`) and hands it a short-lived **scoped credential**. The worker runs an agent loop and streams a trace of `RunEvent`s back as it plans, calls tools, observes results, and finishes.

## Security posture

- **The worker holds only a scoped credential** — an opaque PASETO capability token bound to `{run_id, tenant_id, allowed_tools, exp}`, delivered per-request in `RunTaskRequest.scoped_credential`. No database access, no tool secrets, no API keys.
- **Every side effect goes through the tool proxy.** When the agent wants to run a tool it calls the control plane's `ToolProxyService.ExecuteTool`, presenting the credential as gRPC metadata (`x-scoped-credential`). The proxy is the single, authoritative choke point: it validates the credential, re-checks the allow-list, and runs its policy guard before any tool executes. The worker only consumes the result.
- **Defense-in-depth on the worker side.** Before any call, the worker asserts the tool is in the run's `allowed_tools` (`guard/allowlist.py`), validates the arguments against the tool's schema (`guard/schema.py`), and rejects SSRF/private-egress URLs (`guard/ssrf.py`) and path-traversal (`guard/paths.py`). Tool results are treated as untrusted data: the prompt-injection heuristics (`guard/injection.py`) flag suspicious output, quarantine it in the trace, and redact any leaked secret before it can re-enter the loop or the final answer.
- **Deterministic by default.** The default `mock` LLM engine needs no API key and produces the same trace every time, so the whole stack runs offline.

## Architecture

```
RunnerService.RunTask (stream)      <- control plane calls in
  └─ core/agent.py   plan -> act -> observe loop, yields RunEvents
        │             (or graph/ — the LangGraph variant, same trace)
        ├─ llm/       LLM port (base) + MockLLM (default) | ClaudeLLM
        ├─ guard/     allow-list, schema, SSRF, path, injection heuristics
        └─ toolproxy_client.py  -> ToolProxyService.ExecuteTool  (control plane)
```

Arguments and tool outputs are `google.protobuf.Struct` on the wire; `run_id` and `tenant_id` are UUID strings. Configuration is read from the environment (`WORKER_GRPC_LISTEN`, `TOOLPROXY_GRPC_ADDR`, `LLM_ENGINE`, `LLM_MODEL`, `AGENT_VARIANT`, `MAX_STEPS`); see `config.py`.

## Develop

```bash
uv sync --all-extras --dev      # install with the anthropic / langgraph / otel extras
uv run pytest -q                # run the whole suite
uv run pytest -m injection -q   # run just the prompt-injection corpus
uv run ruff check . && uv run black --check . && uv run mypy src
uv run sap-worker               # start the gRPC server on :50051
```

The generated gRPC stubs live under `src/sap_worker/gen/` and are produced by `make proto` at the repo root (`buf generate` followed by `scripts/fix-python-protos.sh`, which rewrites the stubs' imports to the vendored package path).

## Variants & extras

All four extras are implemented; the default stack (mock engine, custom core, no telemetry) needs none of them.

- **Security guard + injection corpus** (`guard/`, `security/corpus/*.yaml`). The headline: schema/SSRF/path guards run before every tool call and the injection heuristics treat tool results as data. The corpus (one YAML per category — `direct_override`, `indirect_tool_result`, `exfiltration`, `jailbreak`, `tool_abuse`) is executable via `uv run pytest -m injection`.
- **LangGraph variant** (`AGENT_VARIANT=langgraph`, `langgraph` extra). A `StateGraph` that shares the same LLM port, tool-proxy client, and guards and emits an equivalent `RunEvent` trace to the custom core.
- **Real Claude engine** (`LLM_ENGINE=anthropic`, `anthropic` extra). `ClaudeLLM` runs a mediated tool-use loop via the `anthropic` SDK — every `tool_use` is routed through the tool proxy so the credential and guards still apply. The mock stays the default so the repo runs with no API key.
- **OpenTelemetry** (`OTEL_EXPORTER_OTLP_ENDPOINT`, `otel` extra). Extracts the control plane's incoming trace context on `RunTask` and injects it on `ExecuteTool`, so one trace flows control-plane → worker → tool-proxy. A no-op when the endpoint is unset.
