from datetime import UTC, datetime

import pytest
from pydantic import ValidationError

from nilo_integration.models import Envelope, HelloRequest, Measurement


def test_hello_request_rejects_extra_fields() -> None:
    with pytest.raises(ValidationError, match="Extra inputs are not permitted"):
        HelloRequest.model_validate({"type": "hello", "client": "nilo", "unexpected": True})


def test_envelope_rejects_unsupported_protocol() -> None:
    with pytest.raises(ValidationError, match="nilo.integration/1"):
        Envelope.model_validate(
            {
                "protocol": "nilo.integration/2",
                "kind": "request",
                "message_id": "request-hello-1",
                "request_id": "",
                "method": "hello",
                "timestamp": datetime.now(UTC),
                "payload": {"type": "hello", "client": "nilo"},
            }
        )


def test_measurement_rejects_non_finite_value() -> None:
    with pytest.raises(ValidationError, match="finite"):
        Measurement(
            name="voltage",
            value=float("inf"),
            unit="V",
            timestamp=datetime.now(UTC),
        )
