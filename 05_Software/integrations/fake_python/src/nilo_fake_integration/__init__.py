"""Deterministic, hardware-free integration used by NILO contract tests."""

from nilo_fake_integration.plugin import FakeIntegration, FakeOperationError

__all__ = ["FakeIntegration", "FakeOperationError"]
