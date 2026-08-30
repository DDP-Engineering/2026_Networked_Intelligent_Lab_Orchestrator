"""Strict models for the versioned NILO integration runner protocol."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from math import isfinite
from typing import Any, Literal, TypeAlias

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

PROTOCOL_V1 = "nilo.integration/1"
MAX_LINE_BYTES = 1 << 20
MAX_REFERENCED_LOG_BYTES = 64 << 10

Method: TypeAlias = Literal[
    "hello", "describe", "discover", "health", "configure", "execute", "cancel", "safe_shutdown"
]
Kind: TypeAlias = Literal["request", "response", "event"]


class StrictModel(BaseModel):
    """Base model that rejects fields not defined by the protocol."""

    model_config = ConfigDict(extra="forbid")


def _non_empty(value: str) -> str:
    if not value.strip():
        raise ValueError("must not be empty")
    return value


class Envelope(StrictModel):
    protocol: Literal["nilo.integration/1"]
    kind: Kind
    message_id: str
    request_id: str
    method: Method
    timestamp: datetime
    payload: dict[str, Any]

    @field_validator("message_id")
    @classmethod
    def message_id_is_not_empty(cls, value: str) -> str:
        return _non_empty(value)

    @field_validator("timestamp")
    @classmethod
    def timestamp_is_utc(cls, value: datetime) -> datetime:
        if value.tzinfo is None or value.utcoffset() != timedelta(0):
            raise ValueError("must be UTC")
        return value.astimezone(UTC)

    @model_validator(mode="after")
    def request_correlation_matches_kind(self) -> Envelope:
        if self.kind == "request" and self.request_id:
            raise ValueError("requests must have an empty request_id")
        if self.kind in {"response", "event"} and not self.request_id.strip():
            raise ValueError("responses and events require request_id")
        if self.kind == "event" and self.method != "execute":
            raise ValueError("only execute may emit events")
        return self


class HelloRequest(StrictModel):
    type: Literal["hello"]
    client: str

    _client_is_not_empty = field_validator("client")(_non_empty)


class ControlRequest(StrictModel):
    type: Literal["describe", "discover", "health", "safe_shutdown"]


class ConfigureRequest(StrictModel):
    type: Literal["configure"]
    configuration: dict[str, Any]


class ExecuteRequest(StrictModel):
    type: Literal["execute"]
    operation: str
    parameters: Any

    _operation_is_not_empty = field_validator("operation")(_non_empty)


class CancelRequest(StrictModel):
    type: Literal["cancel"]
    target_request_id: str

    _target_request_id_is_not_empty = field_validator("target_request_id")(_non_empty)


class HelloResponse(StrictModel):
    type: Literal["hello"] = "hello"
    integration: str

    _integration_is_not_empty = field_validator("integration")(_non_empty)


class ControlResult(StrictModel):
    type: Literal["describe", "discover", "health", "configure", "cancel", "safe_shutdown"]
    status: str = "ok"

    _status_is_not_empty = field_validator("status")(_non_empty)


class DescribeResult(ControlResult):
    type: Literal["describe"] = "describe"


class DiscoverResult(ControlResult):
    type: Literal["discover"] = "discover"


class HealthResult(ControlResult):
    type: Literal["health"] = "health"


class ConfigureResult(ControlResult):
    type: Literal["configure"] = "configure"


class CancelResult(ControlResult):
    type: Literal["cancel"] = "cancel"


class SafeShutdownResult(ControlResult):
    type: Literal["safe_shutdown"] = "safe_shutdown"


class Progress(StrictModel):
    type: Literal["progress"] = "progress"
    fraction: float
    message: str
    stage: str | None = None

    _message_is_not_empty = field_validator("message")(_non_empty)

    @field_validator("fraction")
    @classmethod
    def fraction_is_a_unit_interval(cls, value: float) -> float:
        if not isfinite(value) or not 0 <= value <= 1:
            raise ValueError("must be between 0 and 1")
        return value


class Measurement(StrictModel):
    type: Literal["measurement"] = "measurement"
    name: str
    value: float
    unit: str
    timestamp: datetime

    _name_is_not_empty = field_validator("name")(_non_empty)
    _unit_is_not_empty = field_validator("unit")(_non_empty)
    _timestamp_is_utc = field_validator("timestamp")(Envelope.timestamp_is_utc)

    @field_validator("value")
    @classmethod
    def value_is_finite(cls, value: float) -> float:
        if not isfinite(value):
            raise ValueError("must be finite")
        return value


class DomainEvent(StrictModel):
    type: Literal["domain_event"] = "domain_event"
    name: str
    data: Any
    timestamp: datetime

    _name_is_not_empty = field_validator("name")(_non_empty)
    _timestamp_is_utc = field_validator("timestamp")(Envelope.timestamp_is_utc)


class WarningEvent(StrictModel):
    type: Literal["warning"] = "warning"
    code: str
    message: str

    _code_is_not_empty = field_validator("code")(_non_empty)
    _message_is_not_empty = field_validator("message")(_non_empty)


class LogReference(StrictModel):
    type: Literal["log_reference"] = "log_reference"
    artifact_id: str
    offset: int = Field(ge=0)
    length: int = Field(ge=0, le=MAX_REFERENCED_LOG_BYTES)

    _artifact_id_is_not_empty = field_validator("artifact_id")(_non_empty)


class ArtifactDeclaration(StrictModel):
    type: Literal["artifact"] = "artifact"
    id: str
    media_type: str
    size_bytes: int = Field(ge=0)
    digest: str

    _id_is_not_empty = field_validator("id")(_non_empty)
    _media_type_is_not_empty = field_validator("media_type")(_non_empty)
    _digest_is_not_empty = field_validator("digest")(_non_empty)


class ExecuteResult(StrictModel):
    type: Literal["result"] = "result"
    status: str
    data: Any

    _status_is_not_empty = field_validator("status")(_non_empty)


class ErrorResult(StrictModel):
    type: Literal["error"] = "error"
    code: str
    message: str
    retryable: bool = False

    _code_is_not_empty = field_validator("code")(_non_empty)
    _message_is_not_empty = field_validator("message")(_non_empty)


RequestPayload: TypeAlias = (
    HelloRequest | ControlRequest | ConfigureRequest | ExecuteRequest | CancelRequest
)
EventPayload: TypeAlias = (
    Progress | Measurement | DomainEvent | WarningEvent | LogReference | ArtifactDeclaration
)

_REQUEST_MODELS: dict[str, type[StrictModel]] = {
    "hello": HelloRequest,
    "describe": ControlRequest,
    "discover": ControlRequest,
    "health": ControlRequest,
    "configure": ConfigureRequest,
    "execute": ExecuteRequest,
    "cancel": CancelRequest,
    "safe_shutdown": ControlRequest,
}


def parse_request_payload(method: Method, payload: dict[str, Any]) -> RequestPayload:
    """Validate a request payload against the only model allowed for its method."""

    model = _REQUEST_MODELS[method]
    result = model.model_validate(payload)
    if result.type != method:
        raise ValueError(f"payload type {result.type!r} does not match method {method!r}")
    return result  # type: ignore[return-value]
