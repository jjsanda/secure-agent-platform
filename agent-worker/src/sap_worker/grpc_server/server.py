"""The ``RunnerService`` gRPC server.

``RunTask`` is a server-streaming RPC: it drives the agent core for one task and
yields each ``RunEvent`` as the run progresses. The credential to forward to the
tool proxy is taken from ``request.scoped_credential`` — the worker never sees it
by any other route.
"""

from __future__ import annotations

from collections.abc import Iterator
from concurrent import futures
from typing import Any

import grpc

from sap_worker.config import Settings
from sap_worker.factory import AgentSelector
from sap_worker.gen.sap.v1 import runner_pb2, runner_pb2_grpc
from sap_worker.logging import get_logger

__all__ = ["RunnerService", "make_server"]


def _normalize_listen(addr: str) -> str:
    """Make a listen address bindable by grpc-python.

    The wire contract with the control plane uses the host-omitted form
    (``":50051"``), which Go's ``net.Listen`` accepts but grpc-python rejects
    ("host must not be empty"). Bind all interfaces (dual-stack) in that case.
    """
    if addr.startswith(":"):
        return f"[::]{addr}"
    return addr


class RunnerService(runner_pb2_grpc.RunnerServiceServicer):  # type: ignore[misc]
    """Streams an agent run back to the control plane, one ``RunEvent`` at a time."""

    def __init__(self, selector: AgentSelector, *, logger: Any | None = None) -> None:
        self._selector = selector
        self._log = logger or get_logger("sap_worker.runner")

    def RunTask(  # noqa: N802 - method name fixed by the gRPC service contract
        self,
        request: runner_pb2.RunTaskRequest,
        context: grpc.ServicerContext,
    ) -> Iterator[runner_pb2.RunEvent]:
        """Server-streaming RPC: yield the run's ``RunEvent`` trace."""
        self._log.info(
            "RunTask received",
            run_id=request.run_id,
            tenant_id=request.tenant_id,
            variant=request.variant,
        )
        # Honour the run's requested variant (custom | langgraph), falling back to
        # the configured default when the request leaves it unspecified.
        agent = self._selector.for_variant(request.variant)
        # The agent core converts every failure into a RunError + FAILED trace, so
        # a well-behaved run never raises out of here.
        yield from agent.run(request)


def make_server(
    settings: Settings,
    *,
    selector: AgentSelector | None = None,
    max_workers: int = 8,
) -> tuple[Any, int, AgentSelector]:
    """Build a gRPC server bound to ``settings.worker_grpc_listen``.

    Returns the (not-yet-started) server, the bound port (``0`` means the bind
    failed), and the :class:`AgentSelector`. The caller owns ``start()`` /
    ``stop()`` and signal handling, and must ``selector.close()`` on shutdown to
    release the tool-proxy channel.
    """
    selector = selector or AgentSelector(settings)
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers))
    runner_pb2_grpc.add_RunnerServiceServicer_to_server(RunnerService(selector), server)
    bound_port: int = server.add_insecure_port(_normalize_listen(settings.worker_grpc_listen))
    return server, bound_port, selector
