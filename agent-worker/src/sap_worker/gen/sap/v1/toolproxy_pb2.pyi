from google.protobuf import struct_pb2 as _struct_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ErrorCode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ERROR_CODE_UNSPECIFIED: _ClassVar[ErrorCode]
    ERROR_CODE_UNAUTHENTICATED: _ClassVar[ErrorCode]
    ERROR_CODE_OUT_OF_SCOPE: _ClassVar[ErrorCode]
    ERROR_CODE_POLICY_BLOCKED: _ClassVar[ErrorCode]
    ERROR_CODE_INVALID_ARGS: _ClassVar[ErrorCode]
    ERROR_CODE_TOOL_FAILED: _ClassVar[ErrorCode]
    ERROR_CODE_RATE_LIMITED: _ClassVar[ErrorCode]
ERROR_CODE_UNSPECIFIED: ErrorCode
ERROR_CODE_UNAUTHENTICATED: ErrorCode
ERROR_CODE_OUT_OF_SCOPE: ErrorCode
ERROR_CODE_POLICY_BLOCKED: ErrorCode
ERROR_CODE_INVALID_ARGS: ErrorCode
ERROR_CODE_TOOL_FAILED: ErrorCode
ERROR_CODE_RATE_LIMITED: ErrorCode

class ExecuteToolRequest(_message.Message):
    __slots__ = ("run_id", "tool_name", "arguments", "step")
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    TOOL_NAME_FIELD_NUMBER: _ClassVar[int]
    ARGUMENTS_FIELD_NUMBER: _ClassVar[int]
    STEP_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    tool_name: str
    arguments: _struct_pb2.Struct
    step: int
    def __init__(self, run_id: _Optional[str] = ..., tool_name: _Optional[str] = ..., arguments: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., step: _Optional[int] = ...) -> None: ...

class ExecuteToolResponse(_message.Message):
    __slots__ = ("ok", "error")
    OK_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    ok: ToolOk
    error: ToolError
    def __init__(self, ok: _Optional[_Union[ToolOk, _Mapping]] = ..., error: _Optional[_Union[ToolError, _Mapping]] = ...) -> None: ...

class ToolOk(_message.Message):
    __slots__ = ("output", "output_sha256")
    OUTPUT_FIELD_NUMBER: _ClassVar[int]
    OUTPUT_SHA256_FIELD_NUMBER: _ClassVar[int]
    output: _struct_pb2.Struct
    output_sha256: str
    def __init__(self, output: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., output_sha256: _Optional[str] = ...) -> None: ...

class ToolError(_message.Message):
    __slots__ = ("code", "message")
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    code: ErrorCode
    message: str
    def __init__(self, code: _Optional[_Union[ErrorCode, str]] = ..., message: _Optional[str] = ...) -> None: ...
