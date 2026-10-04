"""Prompt-injection corpus — the platform's adversarial showcase.

The corpus under :mod:`sap_worker.security.corpus` is a set of realistic
prompt-injection payloads, one YAML file per category. :func:`load_corpus`
parses and validates every item into a typed :class:`CorpusItem`. The
``injection`` pytest suite drives the agent with each item and asserts its
declared ``expected`` outcome, so the corpus is executable documentation of the
worker's defenses, not a static list.

Each item declares:

* ``id`` — unique, stable identifier;
* ``category`` — ``direct_override`` | ``indirect_tool_result`` | ``exfiltration``
  | ``jailbreak`` | ``tool_abuse``;
* ``delivery`` — how the payload reaches the agent: ``user_input`` (the run
  objective), ``tool_result`` (a tool's output), or ``document`` (a fetched
  document body, handled like a tool result);
* ``payload`` — the adversarial text;
* ``expected`` — ``blocked`` (a guard refuses before dispatch), ``neutralized``
  (a secret is redacted / content quoted so nothing leaks), or ``not_acted_on``
  (the agent treats the text as data and never obeys it);
* ``severity`` — ``low`` | ``medium`` | ``high`` | ``critical``.

``tool_abuse`` items additionally carry ``tool`` and ``args`` — the concrete,
guard-tripping call an attacker coaxes the agent into. The literal
``__OVERSIZED__`` placeholder in ``args`` is expanded by the test harness into a
string that exceeds the argument-size guard, keeping the YAML readable.
"""

from __future__ import annotations

from importlib import resources
from typing import Any, Literal

import yaml
from pydantic import BaseModel, ConfigDict

__all__ = [
    "Category",
    "Delivery",
    "Expected",
    "Severity",
    "CorpusItem",
    "OVERSIZED_PLACEHOLDER",
    "load_corpus",
    "CATEGORIES",
]

Category = Literal[
    "direct_override", "indirect_tool_result", "exfiltration", "jailbreak", "tool_abuse"
]
Delivery = Literal["user_input", "tool_result", "document"]
Expected = Literal["blocked", "neutralized", "not_acted_on"]
Severity = Literal["low", "medium", "high", "critical"]

CATEGORIES: tuple[Category, ...] = (
    "direct_override",
    "indirect_tool_result",
    "exfiltration",
    "jailbreak",
    "tool_abuse",
)

# Placeholder the test harness expands into an over-length argument value.
OVERSIZED_PLACEHOLDER = "__OVERSIZED__"


class CorpusItem(BaseModel):
    """One validated prompt-injection corpus entry."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    id: str
    category: Category
    delivery: Delivery
    payload: str
    expected: Expected
    severity: Severity
    # Present only for ``tool_abuse`` items: the concrete guard-tripping call.
    tool: str | None = None
    args: dict[str, Any] | None = None


def load_corpus() -> list[CorpusItem]:
    """Load, validate, and return every corpus item across all category files.

    Raises ``ValueError`` on a duplicate id, a category/file mismatch, or a
    ``tool_abuse`` item missing its ``tool``/``args`` — the corpus must stay
    internally consistent for the executable tests to mean anything.
    """
    corpus_dir = resources.files("sap_worker.security").joinpath("corpus")
    items: list[CorpusItem] = []
    seen: set[str] = set()
    for entry in sorted(corpus_dir.iterdir(), key=lambda p: p.name):
        if not entry.name.endswith(".yaml"):
            continue
        doc = yaml.safe_load(entry.read_text(encoding="utf-8")) or {}
        file_category = doc.get("category")
        for raw in doc.get("items", []):
            item = CorpusItem.model_validate(raw)
            if item.id in seen:
                raise ValueError(f"duplicate corpus id {item.id!r}")
            seen.add(item.id)
            if file_category is not None and item.category != file_category:
                raise ValueError(
                    f"item {item.id!r} category {item.category!r} != file category "
                    f"{file_category!r} in {entry.name}"
                )
            if item.category == "tool_abuse" and (item.tool is None or item.args is None):
                raise ValueError(f"tool_abuse item {item.id!r} must declare 'tool' and 'args'")
            items.append(item)
    if not items:  # pragma: no cover - guards against a packaging regression
        raise ValueError("prompt-injection corpus is empty")
    return items
