# Agent worker image. Build context is the REPO ROOT (compose uses
#   build: { context: ., dockerfile: agent-worker/Dockerfile }),
# so COPY paths are relative to the repo root.
FROM python:3.12-slim

# uv, copied from its official distroless image (no pip bootstrap needed).
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_COMPILE_BYTECODE=1 \
    UV_PROJECT_ENVIRONMENT=/app/agent-worker/.venv \
    PATH="/app/agent-worker/.venv/bin:${PATH}"

WORKDIR /app/agent-worker

# Install runtime deps with ALL extras (otel, langgraph, anthropic) so the single
# image supports every compose toggle without a rebuild: the observability profile
# (OTLP export), the LangGraph variant (AGENT_VARIANT=langgraph), and the Claude
# engine (LLM_ENGINE=anthropic). The default mock + custom path uses none of them.
COPY agent-worker/ /app/agent-worker/
RUN uv sync --no-dev --all-extras \
    && useradd --system --uid 10001 --home-dir /app worker \
    && chown -R worker:worker /app

USER worker
EXPOSE 50051

# Serves RunnerService on WORKER_GRPC_LISTEN (default :50051).
CMD ["sap-worker"]
