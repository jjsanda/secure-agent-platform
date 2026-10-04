"""LangGraph agent variant (behind the ``langgraph`` extra).

Importing this package requires the ``langgraph`` extra; the composition root
imports it lazily so the default (mock/custom) stack runs without it. Selected by
``AGENT_VARIANT=langgraph``.
"""

from __future__ import annotations

from sap_worker.graph.agent import LangGraphAgent

__all__ = ["LangGraphAgent"]
