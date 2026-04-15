"""
Tests for the open-file limit helper in the HTTP scaffolding.
"""
import sys
import importlib
import logging
from unittest.mock import patch, MagicMock

# Ensure the service package is importable from the scaffolding root.
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from service._ulimit import configure


def test_raises_soft_limit_to_hard():
    """configure() should raise the soft limit to the hard limit when soft < hard."""
    mock_resource = MagicMock()
    mock_resource.getrlimit.return_value = (1024, 65536)
    mock_resource.RLIMIT_NOFILE = 7  # canonical Linux value

    with patch.dict(sys.modules, {"resource": mock_resource}):
        # Re-import to pick up the patched module inside the function.
        importlib.reload(sys.modules["service._ulimit"])
        from service._ulimit import configure as _configure
        _configure()

    mock_resource.setrlimit.assert_called_once_with(
        mock_resource.RLIMIT_NOFILE, (65536, 65536)
    )


def test_no_change_when_soft_equals_hard():
    """configure() should not call setrlimit when soft == hard."""
    mock_resource = MagicMock()
    mock_resource.getrlimit.return_value = (65536, 65536)
    mock_resource.RLIMIT_NOFILE = 7

    with patch.dict(sys.modules, {"resource": mock_resource}):
        importlib.reload(sys.modules["service._ulimit"])
        from service._ulimit import configure as _configure
        _configure()

    mock_resource.setrlimit.assert_not_called()


def test_import_error_is_silently_skipped():
    """configure() should do nothing when resource module is unavailable (non-Unix)."""
    with patch.dict(sys.modules, {"resource": None}):
        importlib.reload(sys.modules["service._ulimit"])
        from service._ulimit import configure as _configure
        # Must not raise.
        _configure()


def test_os_error_logs_warning(caplog):
    """configure() should log a warning and not propagate an OSError."""
    mock_resource = MagicMock()
    mock_resource.getrlimit.return_value = (1024, 65536)
    mock_resource.RLIMIT_NOFILE = 7
    mock_resource.setrlimit.side_effect = OSError("operation not permitted")

    with patch.dict(sys.modules, {"resource": mock_resource}):
        importlib.reload(sys.modules["service._ulimit"])
        from service._ulimit import configure as _configure
        with caplog.at_level(logging.WARNING):
            _configure()  # must not raise

    assert "Could not raise open-file limit" in caplog.text


def test_value_error_logs_warning(caplog):
    """configure() should log a warning and not propagate a ValueError."""
    mock_resource = MagicMock()
    mock_resource.getrlimit.return_value = (1024, 65536)
    mock_resource.RLIMIT_NOFILE = 7
    mock_resource.setrlimit.side_effect = ValueError("invalid argument")

    with patch.dict(sys.modules, {"resource": mock_resource}):
        importlib.reload(sys.modules["service._ulimit"])
        from service._ulimit import configure as _configure
        with caplog.at_level(logging.WARNING):
            _configure()  # must not raise

    assert "Could not raise open-file limit" in caplog.text
