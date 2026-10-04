"""The custom agent loop: a hand-written plan -> act -> observe state machine.

:class:`~sap_worker.core.agent.Agent` drives an :class:`~sap_worker.llm.base.LLM`
and a :class:`~sap_worker.toolproxy_client.ToolProxy`, enforces the worker-side
allow-list guard, and streams the run as ``RunEvent`` messages. The LangGraph
variant (next phase) will share this same LLM port, tool-proxy client, and guard.
"""

from __future__ import annotations

from sap_worker.core.agent import Agent

__all__ = ["Agent"]
