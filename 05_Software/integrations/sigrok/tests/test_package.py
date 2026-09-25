"""@file    test_package.py
@brief   Final-product contract for the first installed sigrok package.
@details Defines the package import boundary.
"""

import json
from pathlib import Path


def test_package_declares_digital_capture_contract():
    root = Path(__file__).parents[1]
    manifest = root / "manifest.json"
    assert manifest.is_file(), "The sigrok package has no installable operation contract"
    value = json.loads(manifest.read_text())
    assert value["protocol"] == "nilo.integration/1"
    assert {op["id"] for op in value["operations"]} == {
        "discover_devices",
        "capture",
        "decode_uart",
    }
    for operation in value["operations"]:
        schema = json.loads((root / operation["input_schema"]).read_text())
        assert schema["additionalProperties"] is False
        assert "pwm" not in schema.get("properties", {})
    assert value["artifacts"] == [
        {
            "id": "capture",
            "schema": "schemas/event.schema.json",
            "media_types": ["application/x-sigrok-session"],
        }
    ]


# END OF FILE
