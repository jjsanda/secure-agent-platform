"""``sap_worker``: the Secure Agent Platform Python agent worker.

The Go control plane dispatches a task to this worker over gRPC
(:class:`RunnerService`) together with a short-lived **scoped credential** — an
opaque PASETO capability token bound to ``{run_id, tenant_id, allowed_tools,
exp}``. The worker holds *only* that token: no database access, no tool secrets,
no API keys.

It then runs an agent loop (:mod:`sap_worker.core`). Whenever the agent wants to
run a tool it calls back to the control plane's ``ToolProxyService.ExecuteTool``
(:mod:`sap_worker.toolproxy_client`), presenting the credential as gRPC
metadata. The proxy validates and executes; the worker only consumes the result.
Throughout, the worker streams a trace of ``RunEvent`` messages back to the
control plane.

Layout::

    config.py            typed settings (12-factor)
    logging.py           structlog configuration
    exceptions.py        typed error hierarchy
    structpb.py          google.protobuf.Struct <-> dict helpers
    llm/                 the LLM port (base) + deterministic MockLLM
    guard/               worker-side defense-in-depth (allowlist now; more next)
    core/                the custom plan -> act -> observe agent loop
    toolproxy_client.py  gRPC client for the control-plane tool proxy
    grpc_server/         the RunnerService gRPC server
    factory.py           composition root wiring settings -> agent
    gen/                 generated gRPC stubs (buf) — do not edit by hand
"""

from __future__ import annotations

__all__ = ["__version__"]

__version__ = "0.1.0"
