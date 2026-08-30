"""Public integration implementation interfaces."""

from __future__ import annotations

from typing import Protocol
from uuid import UUID

from nilo_integration.models import (
    ConfigureRequest,
    ConfigureResult,
    DescribeResult,
    DiscoverResult,
    EventPayload,
    ExecuteRequest,
    ExecuteResult,
    HealthResult,
)


class EventSink(Protocol):
    """The only route an integration may use to publish execute events."""

    async def emit(self, event: EventPayload) -> None: ...


class Integration(Protocol):
    """An async implementation that is run behind the NILO protocol server.

    Each handler owns every child task it creates: it must await or cancel those
    tasks before returning. The server can cancel and await its direct handler
    tasks during teardown, but cannot safely discover arbitrary plugin tasks.
    """

    name: str

    async def describe(self) -> DescribeResult: ...

    async def discover(self) -> DiscoverResult: ...

    async def health(self) -> HealthResult: ...

    async def configure(self, request: ConfigureRequest) -> ConfigureResult: ...

    async def execute(self, request: ExecuteRequest, events: EventSink) -> ExecuteResult: ...

    async def cancel(self, request_id: UUID) -> None: ...

    async def safe_shutdown(self) -> None: ...
