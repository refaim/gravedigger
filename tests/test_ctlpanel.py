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
        tmp_translatable: Path,
        tmp_meta: Path,
    ) -> None:
        manifest = handler.unpack(game_dir / "CTLPANEL.DD2", tmp_translatable, tmp_meta)
        assert manifest.handler == "CtlPanelHandler"
        assert manifest.source_file == "CTLPANEL.DD2"

        json_path = tmp_translatable / "ctlpanel.json"
        assert json_path.exists()

        data = json.loads(json_path.read_text())
        assert len(data["keys"]) == 10
        assert all(isinstance(v, int) for v in data["keys"])
        assert isinstance(data["extra"], int)
        if game_dir.name == "softdisk":
            assert data["keys"] == [0x48, 0x49, 0x4D, 0x51, 0x50, 0x4F, 0x4B, 0x47, 0x39, 0x38]
            assert data["extra"] == 0x0003D7FC

    def test_repack_byte_exact(
        self,
        handler: CtlPanelHandler,
        game_dir: Path,
        tmp_translatable: Path,
        tmp_meta: Path,
    ) -> None:
        original_path = game_dir / "CTLPANEL.DD2"
        original_data = original_path.read_bytes()

        handler.unpack(original_path, tmp_translatable, tmp_meta)
        manifest = Manifest.from_json(tmp_meta / "manifest.json")
        repacked_path = tmp_meta / "repacked.dd2"
        handler.repack(manifest, tmp_translatable, tmp_meta, repacked_path)

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

        translatable = tmp_path / "translatable"
        meta = tmp_path / "meta"
        translatable.mkdir()
        meta.mkdir()
        handler.unpack(input_file, translatable, meta)
        manifest_loaded = Manifest.from_json(meta / "manifest.json")
        repacked = tmp_path / "repacked.dd2"
        handler.repack(manifest_loaded, translatable, meta, repacked)

        assert repacked.read_bytes() == raw

    def test_unpack_wrong_size_raises(
        self,
        handler: CtlPanelHandler,
        tmp_path: Path,
    ) -> None:
        bad_file = tmp_path / "CTLPANEL.DD2"
        bad_file.write_bytes(b"\x00" * 5)
        translatable = tmp_path / "translatable"
        meta = tmp_path / "meta"
        translatable.mkdir()
        meta.mkdir()
        with pytest.raises(ValueError, match="Expected 14 bytes"):
            handler.unpack(bad_file, translatable, meta)
