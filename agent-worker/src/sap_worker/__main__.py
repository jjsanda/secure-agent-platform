"""Entry point: start the RunnerService gRPC server and block until signalled.

    $ sap-worker

Boots the worker, logs a listening line, and serves until SIGINT/SIGTERM, then
shuts down gracefully. It needs no secrets: the default stack is the mock LLM,
and the scoped credential arrives per-request over gRPC.
"""

from __future__ import annotations

import signal
import threading
from types import FrameType

from sap_worker import telemetry
from sap_worker.config import Settings
from sap_worker.exceptions import ConfigError
from sap_worker.grpc_server.server import make_server
from sap_worker.logging import configure_logging, get_logger

__all__ = ["main"]

_SHUTDOWN_GRACE_S = 5.0


def main() -> None:
    """Start the worker and serve until a shutdown signal arrives."""
    settings = Settings()
    configure_logging(level=settings.log_level, json=settings.log_json)
    log = get_logger("sap_worker")

    # Install telemetry BEFORE building the server: with an OTLP endpoint set, the
    # gRPC instrumentation patches grpc.server()/grpc.insecure_channel() so the
    # RunnerService extracts the incoming trace context and the tool-proxy client
    # injects it onward. With the endpoint unset it is a no-op (the W3C propagator
    # is still installed so context would flow even without exporting).
    otel_on = telemetry.setup(
        endpoint=settings.otel_exporter_otlp_endpoint or None,
        service_name=settings.otel_service_name,
        namespace=settings.otel_service_namespace,
    )

    server, bound_port, selector = make_server(settings)
    if bound_port == 0:
        selector.close()
        telemetry.shutdown()
        raise ConfigError(f"failed to bind worker gRPC listener to {settings.worker_grpc_listen!r}")

    server.start()
    log.info(
        "agent worker listening",
        bound_port=bound_port,
        otel_exporting=otel_on,
        **settings.public_info(),
    )

    stop = threading.Event()

    def _handle(signum: int, _frame: FrameType | None) -> None:
        log.info("shutdown signal received", signal=signal.Signals(signum).name)
        stop.set()

    signal.signal(signal.SIGINT, _handle)
    signal.signal(signal.SIGTERM, _handle)

    stop.wait()

    log.info("agent worker shutting down", grace_s=_SHUTDOWN_GRACE_S)
    server.stop(_SHUTDOWN_GRACE_S).wait()
    selector.close()  # close the tool-proxy channel the selector owns
    telemetry.shutdown()
    log.info("agent worker stopped")


if __name__ == "__main__":
    main()
