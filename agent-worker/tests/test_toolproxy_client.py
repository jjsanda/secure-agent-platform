"""Unit-test Struct<->dict conversion, metadata assembly, and response mapping."""

from __future__ import annotations

from typing import Any

import grpc
import pytest

from sap_worker.exceptions import ToolProxyError
from sap_worker.gen.sap.v1 import toolproxy_pb2
from sap_worker.structpb import dict_to_struct, struct_to_dict
from sap_worker.toolproxy_client import (
    CREDENTIAL_METADATA_KEY,
    ToolProxyClient,
    credential_metadata,
)

pytestmark = pytest.mark.unit


class FakeStub:
    """A stand-in for ToolProxyServiceStub that records calls and scripts responses."""

    def __init__(
        self,
        response: Any = None,
        raises: list[BaseException] | None = None,
    ) -> None:
        self.response = response
        self.raises = list(raises or [])
        self.received: list[dict[str, Any]] = []

    def ExecuteTool(self, request: Any, metadata: Any = None, timeout: Any = None) -> Any:
        self.received.append({"request": request, "metadata": metadata, "timeout": timeout})
        if self.raises:
            raise self.raises.pop(0)
        return self.response


class FakeRpcError(grpc.RpcError):
    """A gRPC error with a scriptable status code (matches the duck-typed retry check)."""

    def __init__(self, code: grpc.StatusCode) -> None:
        self._code = code

    def code(self) -> grpc.StatusCode:
        return self._code


def _ok_response(output: dict[str, Any], sha: str = "deadbeefcafe0000") -> Any:
    return toolproxy_pb2.ExecuteToolResponse(
        ok=toolproxy_pb2.ToolOk(output=dict_to_struct(output), output_sha256=sha)
    )


# --- Struct <-> dict -----------------------------------------------------------------


def test_struct_dict_roundtrip() -> None:
    original = {
        "input": "hello",
        "flag": True,
        "nested": {"k": "v"},
        "items": ["a", "b"],
        "empty": None,
    }
    assert struct_to_dict(dict_to_struct(original)) == original


def test_struct_numbers_become_floats() -> None:
    # google.protobuf.Struct stores every number as a double.
    assert struct_to_dict(dict_to_struct({"n": 1})) == {"n": 1.0}


# --- Metadata ------------------------------------------------------------------------


def test_credential_metadata_uses_frozen_key() -> None:
    assert CREDENTIAL_METADATA_KEY == "x-scoped-credential"
    assert credential_metadata("tok") == [("x-scoped-credential", "tok")]


# --- execute() request / response mapping --------------------------------------------


def test_execute_success_maps_output_and_sends_metadata() -> None:
    stub = FakeStub(response=_ok_response({"echoed": "hi"}))
    client = ToolProxyClient("unused:0", stub=stub)

    result = client.execute("run-1", "echo", {"input": "hi"}, 3, "cred-xyz")

    assert result.ok is True
    assert result.output == {"echoed": "hi"}
    assert result.error_code is None

    sent = stub.received[0]
    assert ("x-scoped-credential", "cred-xyz") in sent["metadata"]
    request = sent["request"]
    assert request.run_id == "run-1"
    assert request.tool_name == "echo"
    assert request.step == 3
    assert struct_to_dict(request.arguments) == {"input": "hi"}


def test_execute_error_maps_error_code_name_and_message() -> None:
    response = toolproxy_pb2.ExecuteToolResponse(
        error=toolproxy_pb2.ToolError(
            code=toolproxy_pb2.ERROR_CODE_OUT_OF_SCOPE, message="tool not in scope"
        )
    )
    client = ToolProxyClient("unused:0", stub=FakeStub(response=response))

    result = client.execute("run-1", "danger", {}, 1, "cred")

    assert result.ok is False
    assert result.error_code == "ERROR_CODE_OUT_OF_SCOPE"
    assert result.detail == "tool not in scope"


def test_execute_tolerates_unknown_error_code() -> None:
    # A proxy newer than these stubs may send an ErrorCode value we don't define;
    # ErrorCode.Name() would raise ValueError. The client must map it to the
    # unspecified code and surface the message, never crash the run.
    response = toolproxy_pb2.ExecuteToolResponse()
    response.error.code = 999  # out of range for the committed ErrorCode enum
    response.error.message = "from the future"
    client = ToolProxyClient("unused:0", stub=FakeStub(response=response))

    result = client.execute("run-1", "danger", {}, 1, "cred")

    assert result.ok is False
    assert result.error_code == "ERROR_CODE_UNSPECIFIED"
    assert result.detail == "from the future"


# --- retry semantics -----------------------------------------------------------------


def test_transient_unavailable_is_retried_then_succeeds() -> None:
    stub = FakeStub(
        response=_ok_response({"ok": "yes"}),
        raises=[FakeRpcError(grpc.StatusCode.UNAVAILABLE)],  # fail once, then succeed
    )
    client = ToolProxyClient("unused:0", stub=stub, max_attempts=3)

    result = client.execute("run-1", "echo", {}, 1, "cred")

    assert result.ok is True
    assert len(stub.received) == 2  # one retry


def test_non_transient_error_is_not_retried() -> None:
    stub = FakeStub(raises=[FakeRpcError(grpc.StatusCode.INTERNAL)])
    client = ToolProxyClient("unused:0", stub=stub, max_attempts=3)

    with pytest.raises(ToolProxyError):
        client.execute("run-1", "echo", {}, 1, "cred")

    assert len(stub.received) == 1  # a policy/other error is surfaced, never retried


def test_retry_controller_is_built_per_call() -> None:
    # The 8-thread server pool must not share one tenacity.Retrying (it holds mutable
    # per-call state); each execute() builds a fresh controller instead.
    client = ToolProxyClient("unused:0", stub=FakeStub(response=_ok_response({"ok": "yes"})))
    assert client._make_retrying() is not client._make_retrying()
