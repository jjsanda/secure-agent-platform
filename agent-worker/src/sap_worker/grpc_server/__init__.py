"""The RunnerService gRPC server that the control plane calls to run a task."""

from __future__ import annotations

from sap_worker.grpc_server.server import RunnerService, make_server

__all__ = ["RunnerService", "make_server"]
