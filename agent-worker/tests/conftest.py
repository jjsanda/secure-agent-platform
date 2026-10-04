"""Shared pytest configuration for the worker test suite."""

from __future__ import annotations

import pytest


def pytest_sessionfinish(session: pytest.Session, exitstatus: int) -> None:
    """Make ``pytest -m injection`` exit 0 even if a filter selects nothing.

    The prompt-injection corpus now lands a meaningful set of tests, so a plain
    ``pytest -m injection`` collects and runs them. This safety net only matters
    when an additional ``-k`` filter narrows the injection selection to zero: it
    treats "nothing collected" as success *only* when the injection marker was
    requested, so the marker never trips exit code 5 (NO_TESTS_COLLECTED).
    """
    markexpr = session.config.option.markexpr or ""
    if exitstatus == int(pytest.ExitCode.NO_TESTS_COLLECTED) and "injection" in markexpr:
        session.exitstatus = int(pytest.ExitCode.OK)
