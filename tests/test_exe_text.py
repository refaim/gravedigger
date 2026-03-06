"""Tests for ExeTextHandler — text strings from EXE."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from gravedigger.compression.pklite import decompress
from gravedigger.core.handler import Manifest
from gravedigger.handlers.exe_text import ExeTextHandler

GAME_DIR = Path(__file__).resolve().parent.parent / "game"
EXE_PATH = GAME_DIR / "1.EXE"


@pytest.fixture()
def handler() -> ExeTextHandler:
    return ExeTextHandler()


@pytest.fixture()
def exe_path() -> Path:
    if not EXE_PATH.exists():
        pytest.skip("game/1.EXE not found")
    return EXE_PATH


@pytest.fixture()
def unpacked_dirs(handler: ExeTextHandler, exe_path: Path, tmp_path: Path) -> tuple[Path, Path]:
    translatable = tmp_path / "translatable"
    meta = tmp_path / "meta"
    translatable.mkdir()
    meta.mkdir()
    handler.unpack(exe_path, translatable, meta)
    return translatable, meta


class TestUnpack:
    def test_produces_json(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        assert (translatable / "strings.json").exists()

    def test_produces_manifest(self, unpacked_dirs: tuple[Path, Path]) -> None:
        _, meta = unpacked_dirs
        manifest = Manifest.from_json(meta / "manifest.json")
        assert manifest.handler == "ExeTextHandler"

    def test_json_contains_dangerous_dave(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        data = json.loads((translatable / "strings.json").read_text())
        texts = [entry["text"] for entry in data["strings"]]
        assert any("Dangerous Dave Commands" in t for t in texts)

    def test_json_contains_game_over(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        data = json.loads((translatable / "strings.json").read_text())
        texts = [entry["text"] for entry in data["strings"]]
        assert any("G A M E   O V E R" in t for t in texts)

    def test_json_contains_copyright(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        data = json.loads((translatable / "strings.json").read_text())
        texts = [entry["text"] for entry in data["strings"]]
        assert any("Softdisk" in t for t in texts)

    def test_json_contains_level_names(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        data = json.loads((translatable / "strings.json").read_text())
        texts = [entry["text"] for entry in data["strings"]]
        assert any("LEVEL 1" in t for t in texts)
        assert any("LEVEL 8" in t for t in texts)

    def test_json_contains_congratulations(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        data = json.loads((translatable / "strings.json").read_text())
        texts = [entry["text"] for entry in data["strings"]]
        assert any("You have freed Delbert" in t for t in texts)

    def test_json_entries_have_offset_and_max_length(
        self, unpacked_dirs: tuple[Path, Path]
    ) -> None:
        translatable, _ = unpacked_dirs
        data = json.loads((translatable / "strings.json").read_text())
        for entry in data["strings"]:
            assert "offset" in entry
            assert "text" in entry
            assert "max_length" in entry
            assert isinstance(entry["offset"], int)
            assert isinstance(entry["text"], str)
            assert isinstance(entry["max_length"], int)
            assert len(entry["text"].encode("ascii")) <= entry["max_length"]

    def test_json_string_count(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        data = json.loads((translatable / "strings.json").read_text())
        assert len(data["strings"]) >= 50

    def test_manifest_has_original_exe(self, unpacked_dirs: tuple[Path, Path]) -> None:
        _, meta = unpacked_dirs
        manifest = Manifest.from_json(meta / "manifest.json")
        assert "original_exe" in manifest.metadata


class TestRepack:
    def test_roundtrip_byte_exact(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        translatable = tmp_path / "translatable"
        meta = tmp_path / "meta"
        translatable.mkdir()
        meta.mkdir()
        manifest = handler.unpack(exe_path, translatable, meta)

        repack_path = tmp_path / "repacked.exe"
        handler.repack(manifest, translatable, meta, repack_path)

        assert repack_path.read_bytes() == exe_path.read_bytes()

    def test_string_too_long_raises(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        translatable = tmp_path / "translatable"
        meta = tmp_path / "meta"
        translatable.mkdir()
        meta.mkdir()
        manifest = handler.unpack(exe_path, translatable, meta)

        strings_path = translatable / "strings.json"
        data = json.loads(strings_path.read_text())

        for entry in data["strings"]:
            if entry["text"] == "You win!":
                entry["text"] = "A" * (entry["max_length"] + 10)
                break

        strings_path.write_text(json.dumps(data, indent=2))

        repack_path = tmp_path / "repacked.exe"
        with pytest.raises(ValueError, match="exceeds maximum length"):
            handler.repack(manifest, translatable, meta, repack_path)

    def test_patching_decompressed_strings(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        import struct

        exe_data = exe_path.read_bytes()
        decompressed = decompress(exe_data)

        header_para = struct.unpack_from("<H", decompressed, 8)[0]
        code_start = header_para * 16

        patched = bytearray(decompressed)

        win_offset = code_start + 0x27646
        assert patched[win_offset : win_offset + 8] == b"You win!"
        patched[win_offset : win_offset + 8] = b"U win!!!"
        assert patched[win_offset : win_offset + 9] == b"U win!!!\x00"

        god_offset = code_start + 0x273AE
        assert patched[god_offset : god_offset + 12] == b"God mode off"
        patched[god_offset : god_offset + 13] = b"Off" + b"\x00" * 10
        assert patched[god_offset : god_offset + 13] == b"Off" + b"\x00" * 10

        assert b"U win!!!" in patched
        assert b"You win!" not in patched
        assert b"Off" in patched[god_offset : god_offset + 13]

    def test_modify_string_and_repack(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        translatable = tmp_path / "translatable"
        meta = tmp_path / "meta"
        translatable.mkdir()
        meta.mkdir()
        manifest = handler.unpack(exe_path, translatable, meta)

        strings_path = translatable / "strings.json"
        data = json.loads(strings_path.read_text())

        for entry in data["strings"]:
            if entry["id"] == "copyright":
                entry["text"] = entry["text"].replace("Softdisk", "Xoftdisk")
                break

        strings_path.write_text(json.dumps(data, indent=2))

        repack_path = tmp_path / "repacked.exe"
        handler.repack(manifest, translatable, meta, repack_path)

        repacked_dec = decompress(repack_path.read_bytes())
        assert b"Xoftdisk" in repacked_dec
        assert b"Softdisk" not in repacked_dec

    def test_repack_preserves_other_data(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        import struct

        translatable = tmp_path / "translatable"
        meta = tmp_path / "meta"
        translatable.mkdir()
        meta.mkdir()
        manifest = handler.unpack(exe_path, translatable, meta)

        repack_path = tmp_path / "repacked.exe"
        handler.repack(manifest, translatable, meta, repack_path)

        original_dec = decompress(exe_path.read_bytes())
        repacked_dec = decompress(repack_path.read_bytes())

        header_para = struct.unpack_from("<H", original_dec, 8)[0]
        code_start = header_para * 16
        death_offset = code_start + 0x18A60
        block_size = 0x33B0 * 4

        assert (
            original_dec[death_offset : death_offset + block_size]
            == repacked_dec[death_offset : death_offset + block_size]
        )


class TestFilePatterns:
    def test_file_patterns(self, handler: ExeTextHandler) -> None:
        assert "DAVE.EXE" in handler.file_patterns
        assert "1.EXE" in handler.file_patterns
