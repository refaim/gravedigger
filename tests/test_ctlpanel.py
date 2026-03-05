from __future__ import annotations

import json
from typing import TYPE_CHECKING

import pytest

from gravedigger.core.handler import Manifest
from gravedigger.handlers.ctlpanel import CtlPanelHandler

if TYPE_CHECKING:
    from pathlib import Path


@pytest.fixture()
def handler() -> CtlPanelHandler:
    return CtlPanelHandler()


class TestCtlPanelHandler:
    def test_file_patterns(self, handler: CtlPanelHandler) -> None:
        assert "CTLPANEL.DD2" in handler.file_patterns

    def test_unpack_creates_json(
        self,
        handler: CtlPanelHandler,
        game_dir: Path,
        tmp_output: Path,
    ) -> None:
        manifest = handler.unpack(game_dir / "CTLPANEL.DD2", tmp_output)
        assert manifest.handler == "CtlPanelHandler"
        assert manifest.source_file == "CTLPANEL.DD2"

        json_path = tmp_output / "ctlpanel.json"
        assert json_path.exists()

        data = json.loads(json_path.read_text())
        # 10 key scan codes
        assert len(data["keys"]) == 10
        # All values should be integers (scan codes)
        assert all(isinstance(v, int) for v in data["keys"])
        # Known scan codes from the default file
        assert data["keys"] == [0x48, 0x49, 0x4D, 0x51, 0x50, 0x4F, 0x4B, 0x47, 0x39, 0x38]
        # Last 4 bytes as uint32 LE
        assert isinstance(data["extra"], int)
        assert data["extra"] == 0x0003D7FC

    def test_repack_byte_exact(
        self,
        handler: CtlPanelHandler,
        game_dir: Path,
        tmp_output: Path,
    ) -> None:
        original_path = game_dir / "CTLPANEL.DD2"
        original_data = original_path.read_bytes()

        handler.unpack(original_path, tmp_output)
        manifest = Manifest.from_json(tmp_output / "manifest.json")
        repacked_path = tmp_output / "repacked.dd2"
        handler.repack(manifest, tmp_output, repacked_path)

        repacked_data = repacked_path.read_bytes()
        assert repacked_data == original_data

    def test_unpack_repack_roundtrip_minimal(
        self,
        handler: CtlPanelHandler,
        tmp_path: Path,
    ) -> None:
        """Test roundtrip with synthetic data."""
        import struct

        keys = [0x48, 0x49, 0x4D, 0x51, 0x50, 0x4F, 0x4B, 0x47, 0x39, 0x38]
        extra = 12345
        raw = bytes(keys) + struct.pack("<I", extra)

        input_file = tmp_path / "CTLPANEL.DD2"
        input_file.write_bytes(raw)

        out_dir = tmp_path / "out"
        out_dir.mkdir()
        handler.unpack(input_file, out_dir)
        manifest_loaded = Manifest.from_json(out_dir / "manifest.json")
        repacked = tmp_path / "repacked.dd2"
        handler.repack(manifest_loaded, out_dir, repacked)

        assert repacked.read_bytes() == raw

    def test_unpack_wrong_size_raises(
        self,
        handler: CtlPanelHandler,
        tmp_path: Path,
    ) -> None:
        bad_file = tmp_path / "CTLPANEL.DD2"
        bad_file.write_bytes(b"\x00" * 5)
        out_dir = tmp_path / "out"
        out_dir.mkdir()
        with pytest.raises(ValueError, match="Expected 14 bytes"):
            handler.unpack(bad_file, out_dir)
