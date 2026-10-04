import datetime

from google.protobuf import struct_pb2 as _struct_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from sap_worker.gen.sap.v1 import common_pb2 as _common_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class RunStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    RUN_STATUS_UNSPECIFIED: _ClassVar[RunStatus]
    RUN_STATUS_PLANNING: _ClassVar[RunStatus]
    RUN_STATUS_ACTING: _ClassVar[RunStatus]
    RUN_STATUS_OBSERVING: _ClassVar[RunStatus]
    RUN_STATUS_SUCCEEDED: _ClassVar[RunStatus]
    RUN_STATUS_FAILED: _ClassVar[RunStatus]
RUN_STATUS_UNSPECIFIED: RunStatus
RUN_STATUS_PLANNING: RunStatus
RUN_STATUS_ACTING: RunStatus
RUN_STATUS_OBSERVING: RunStatus
RUN_STATUS_SUCCEEDED: RunStatus
RUN_STATUS_FAILED: RunStatus

class RunTaskRequest(_message.Message):
    __slots__ = ("run_id", "tenant_id", "objective", "allowed_tools", "scoped_credential", "model", "variant", "max_steps")
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    OBJECTIVE_FIELD_NUMBER: _ClassVar[int]
    ALLOWED_TOOLS_FIELD_NUMBER: _ClassVar[int]
    SCOPED_CREDENTIAL_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    VARIANT_FIELD_NUMBER: _ClassVar[int]
    MAX_STEPS_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    tenant_id: str
    objective: str
    allowed_tools: _containers.RepeatedScalarFieldContainer[str]
    scoped_credential: str
    model: _common_pb2.ModelConfig
    variant: _common_pb2.AgentVariant
    max_steps: int
    def __init__(self, run_id: _Optional[str] = ..., tenant_id: _Optional[str] = ..., objective: _Optional[str] = ..., allowed_tools: _Optional[_Iterable[str]] = ..., scoped_credential: _Optional[str] = ..., model: _Optional[_Union[_common_pb2.ModelConfig, _Mapping]] = ..., variant: _Optional[_Union[_common_pb2.AgentVariant, str]] = ..., max_steps: _Optional[int] = ...) -> None: ...

class RunEvent(_message.Message):
    __slots__ = ("at", "step", "status", "plan", "llm_message", "tool_requested", "tool_result", "final", "error")
    AT_FIELD_NUMBER: _ClassVar[int]
    STEP_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    PLAN_FIELD_NUMBER: _ClassVar[int]
    LLM_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    TOOL_REQUESTED_FIELD_NUMBER: _ClassVar[int]
    TOOL_RESULT_FIELD_NUMBER: _ClassVar[int]
    FINAL_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    at: _timestamp_pb2.Timestamp
    step: int
    status: StatusChange
    plan: PlanProduced
    llm_message: LlmMessage
    tool_requested: ToolCallRequested
    tool_result: ToolCallResult
    final: FinalAnswer
    error: RunError
    def __init__(self, at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., step: _Optional[int] = ..., status: _Optional[_Union[StatusChange, _Mapping]] = ..., plan: _Optional[_Union[PlanProduced, _Mapping]] = ..., llm_message: _Optional[_Union[LlmMessage, _Mapping]] = ..., tool_requested: _Optional[_Union[ToolCallRequested, _Mapping]] = ..., tool_result: _Optional[_Union[ToolCallResult, _Mapping]] = ..., final: _Optional[_Union[FinalAnswer, _Mapping]] = ..., error: _Optional[_Union[RunError, _Mapping]] = ...) -> None: ...

class StatusChange(_message.Message):
    __slots__ = ("status",)
    STATUS_FIELD_NUMBER: _ClassVar[int]
    status: RunStatus
    def __init__(self, status: _Optional[_Union[RunStatus, str]] = ...) -> None: ...

class PlanProduced(_message.Message):
    __slots__ = ("steps",)
    STEPS_FIELD_NUMBER: _ClassVar[int]
    steps: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, steps: _Optional[_Iterable[str]] = ...) -> None: ...

class LlmMessage(_message.Message):
    __slots__ = ("role", "content")
    ROLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    role: str
    content: str
    def __init__(self, role: _Optional[str] = ..., content: _Optional[str] = ...) -> None: ...

class ToolCallRequested(_message.Message):
    __slots__ = ("tool_name", "arguments")
    TOOL_NAME_FIELD_NUMBER: _ClassVar[int]
    ARGUMENTS_FIELD_NUMBER: _ClassVar[int]
    tool_name: str
    arguments: _struct_pb2.Struct
    def __init__(self, tool_name: _Optional[str] = ..., arguments: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ...) -> None: ...

class ToolCallResult(_message.Message):
    __slots__ = ("tool_name", "ok", "detail")
    TOOL_NAME_FIELD_NUMBER: _ClassVar[int]
    OK_FIELD_NUMBER: _ClassVar[int]
    DETAIL_FIELD_NUMBER: _ClassVar[int]
    tool_name: str
    ok: bool
    detail: str
    def __init__(self, tool_name: _Optional[str] = ..., ok: _Optional[bool] = ..., detail: _Optional[str] = ...) -> None: ...

class FinalAnswer(_message.Message):
    __slots__ = ("answer",)
    ANSWER_FIELD_NUMBER: _ClassVar[int]
    answer: str
    def __init__(self, answer: _Optional[str] = ...) -> None: ...

class RunError(_message.Message):
    __slots__ = ("message",)
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    message: str
    def __init__(self, message: _Optional[str] = ...) -> None: ...
