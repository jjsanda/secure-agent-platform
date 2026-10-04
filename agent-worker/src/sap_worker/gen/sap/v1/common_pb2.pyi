from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor

class AgentVariant(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    AGENT_VARIANT_UNSPECIFIED: _ClassVar[AgentVariant]
    AGENT_VARIANT_CUSTOM: _ClassVar[AgentVariant]
    AGENT_VARIANT_LANGGRAPH: _ClassVar[AgentVariant]
AGENT_VARIANT_UNSPECIFIED: AgentVariant
AGENT_VARIANT_CUSTOM: AgentVariant
AGENT_VARIANT_LANGGRAPH: AgentVariant

class ModelConfig(_message.Message):
    __slots__ = ("engine", "model", "effort")
    ENGINE_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    EFFORT_FIELD_NUMBER: _ClassVar[int]
    engine: str
    model: str
    effort: str
    def __init__(self, engine: _Optional[str] = ..., model: _Optional[str] = ..., effort: _Optional[str] = ...) -> None: ...
