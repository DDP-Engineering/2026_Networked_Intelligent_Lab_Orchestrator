import json
from datetime import UTC, datetime
from pathlib import Path

import pytest
from pydantic import ValidationError

from nilo_integration.models import Envelope, HelloRequest, Measurement, validate_envelope_payload


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


def test_shared_golden_fixture_validates_and_round_trips() -> None:
    fixture = (
        Path(__file__).parents[4]
        / "tests"
        / "contract"
        / "fixtures"
        / "protocol-v1"
        / "hello.ndjson"
    )

    for line in fixture.read_bytes().splitlines():
        source = json.loads(line)
        envelope = Envelope.model_validate(source)
        validate_envelope_payload(envelope)

        assert (
            json.loads(json.dumps(envelope.model_dump(mode="json"), separators=(",", ":")))
            == source
        )
