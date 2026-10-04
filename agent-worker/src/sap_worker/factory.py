"""Composition root: wire :class:`~sap_worker.config.Settings` into an agent.

Keeping construction here means the gRPC layer and ``__main__`` stay thin, and
tests can build an agent with injected fakes. The LLM engine (``mock`` |
``anthropic``) and the agent variant (``custom`` | ``langgraph``) are selected
from settings; the real Claude engine and the LangGraph variant are imported
lazily so the default offline stack needs neither extra installed.

The default variant is a startup setting, but a run may request a specific one:
:class:`AgentSelector` resolves the per-request variant against a single shared
LLM engine and tool-proxy client, caching one agent instance per variant.
"""

from __future__ import annotations

from sap_worker.config import AgentVariant, Settings
from sap_worker.core.agent import Agent
from sap_worker.core.steps import AgentRunner
from sap_worker.exceptions import ConfigError
from sap_worker.gen.sap.v1 import common_pb2
from sap_worker.llm.base import LLM
from sap_worker.llm.mock import MockLLM
from sap_worker.toolproxy_client import ToolProxy, ToolProxyClient

__all__ = ["build_llm", "build_tool_proxy", "build_agent", "AgentSelector"]


def build_llm(settings: Settings) -> LLM:
    """Select the LLM engine. The deterministic mock is the default (no API key)."""
    if settings.llm_engine == "mock":
        return MockLLM(model=settings.llm_model)
    if settings.llm_engine == "anthropic":
        try:
            from sap_worker.llm.claude import ClaudeLLM
        except ImportError as exc:
            raise ConfigError(
                "LLM engine 'anthropic' requires the 'anthropic' extra "
                "(pip install 'sap-worker[anthropic]')"
            ) from exc
        return ClaudeLLM(
            model=settings.llm_model,
            effort=settings.llm_effort,
            max_tokens=settings.llm_max_tokens,
        )
    raise ConfigError(f"unknown LLM engine {settings.llm_engine!r}")


def build_tool_proxy(settings: Settings) -> ToolProxyClient:
    """Build the tool-proxy client from settings (insecure local channel)."""
    return ToolProxyClient(
        settings.toolproxy_grpc_addr,
        timeout_s=settings.toolproxy_timeout_s,
        max_attempts=settings.toolproxy_max_attempts,
    )


def build_agent(
    settings: Settings,
    *,
    variant: AgentVariant | None = None,
    llm: LLM | None = None,
    tool_proxy: ToolProxy | None = None,
) -> AgentRunner:
    """Assemble one agent core (``custom`` | ``langgraph``).

    ``variant`` overrides ``settings.agent_variant`` — :class:`AgentSelector` uses
    it to honour a per-request variant — and defaults to the configured variant.
    """
    engine = llm or build_llm(settings)
    proxy = tool_proxy or build_tool_proxy(settings)
    selected = variant or settings.agent_variant

    if selected == "custom":
        return Agent(llm=engine, tool_proxy=proxy, max_steps=settings.max_steps)
    if selected == "langgraph":
        try:
            from sap_worker.graph import LangGraphAgent
        except ImportError as exc:
            raise ConfigError(
                "agent variant 'langgraph' requires the 'langgraph' extra "
                "(pip install 'sap-worker[langgraph]')"
            ) from exc
        return LangGraphAgent(llm=engine, tool_proxy=proxy, max_steps=settings.max_steps)
    raise ConfigError(f"unknown agent variant {selected!r}")


# Map the wire ``AgentVariant`` enum onto the settings' variant name. UNSPECIFIED
# (and any unknown value) is intentionally absent so it falls back to the default.
_VARIANT_NAMES: dict[int, AgentVariant] = {
    common_pb2.AGENT_VARIANT_CUSTOM: "custom",
    common_pb2.AGENT_VARIANT_LANGGRAPH: "langgraph",
}


class AgentSelector:
    """Resolves the agent core for a run's requested variant, caching one each.

    The worker builds a single LLM engine and tool-proxy client and shares them
    across both variants; each variant's agent is built lazily on first use and
    then reused (both cores are reentrant). :meth:`for_variant` maps a request's
    ``AgentVariant`` onto its agent, falling back to the configured default when
    the request leaves it ``AGENT_VARIANT_UNSPECIFIED``. This makes the dashboard's
    per-run variant choice take effect, rather than the worker running a single
    agent fixed at startup.
    """

    def __init__(
        self,
        settings: Settings,
        *,
        llm: LLM | None = None,
        tool_proxy: ToolProxy | None = None,
    ) -> None:
        self._settings = settings
        self._llm = llm or build_llm(settings)
        self._tool_proxy: ToolProxy = tool_proxy or build_tool_proxy(settings)
        self._agents: dict[AgentVariant, AgentRunner] = {}

    def for_variant(self, variant: int) -> AgentRunner:
        """Return the agent for the wire ``AgentVariant`` (default for UNSPECIFIED)."""
        name = _VARIANT_NAMES.get(variant, self._settings.agent_variant)
        agent = self._agents.get(name)
        if agent is None:
            agent = build_agent(
                self._settings, variant=name, llm=self._llm, tool_proxy=self._tool_proxy
            )
            self._agents[name] = agent
        return agent

    def close(self) -> None:
        """Release shared resources — closes the tool-proxy channel if it owns one."""
        close = getattr(self._tool_proxy, "close", None)
        if callable(close):
            close()
