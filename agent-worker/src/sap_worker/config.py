"""Typed worker settings (12-factor, pydantic-settings v2).

Every value has a safe default, so an empty environment runs the fully-offline,
deterministic stack (the mock LLM, no API key). The environment variable names
match the control-plane / compose contract exactly and are read *unprefixed*
(e.g. ``WORKER_GRPC_LISTEN``, ``TOOLPROXY_GRPC_ADDR``).

Example:
    >>> Settings(_env_file=None).llm_engine
    'mock'
"""

from __future__ import annotations

from typing import Literal

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict

__all__ = ["Settings", "LLMEngine", "AgentVariant"]

LLMEngine = Literal["mock", "anthropic"]
AgentVariant = Literal["custom", "langgraph"]


class Settings(BaseSettings):
    """Validated settings loaded from the environment / ``.env``."""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
        extra="ignore",
    )

    # --- gRPC wiring ---
    # Address this worker's RunnerService listens on (host omitted => all interfaces).
    worker_grpc_listen: str = ":50051"
    # Address of the control-plane ToolProxyService this worker dials (plaintext, local).
    toolproxy_grpc_addr: str = "control-plane:9090"

    # --- Agent brain ---
    llm_engine: LLMEngine = "mock"
    llm_model: str = "claude-sonnet-5"  # ignored by the mock engine
    # Reasoning effort for the Claude engine (sonnet-5 / opus-4-8); ignored by mock
    # and by haiku-4-5, which supports neither thinking nor effort.
    llm_effort: str = "medium"
    llm_max_tokens: int = Field(default=16000, ge=1)
    agent_variant: AgentVariant = "custom"
    max_steps: int = Field(default=8, ge=1, le=50)

    # --- Tool proxy client behaviour ---
    toolproxy_timeout_s: float = Field(default=10.0, gt=0)
    toolproxy_max_attempts: int = Field(default=3, ge=1, le=10)

    # --- Observability ---
    log_level: str = "INFO"
    log_json: bool = True
    # OpenTelemetry: an empty endpoint is a no-op (default stack ships no collector).
    # When set (observability profile), traces export over OTLP/gRPC and a single
    # trace flows control-plane -> worker -> tool-proxy via W3C trace-context.
    otel_exporter_otlp_endpoint: str = ""
    otel_service_name: str = "agent-worker"
    otel_service_namespace: str = "sap"

    def public_info(self) -> dict[str, object]:
        """Non-secret configuration summary (for startup logs)."""
        return {
            "worker_grpc_listen": self.worker_grpc_listen,
            "toolproxy_grpc_addr": self.toolproxy_grpc_addr,
            "llm_engine": self.llm_engine,
            "llm_model": self.llm_model if self.llm_engine != "mock" else "mock-deterministic",
            "agent_variant": self.agent_variant,
            "max_steps": self.max_steps,
            "otel_enabled": bool(self.otel_exporter_otlp_endpoint),
        }
