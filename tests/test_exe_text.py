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
def unpacked_dir(handler: ExeTextHandler, exe_path: Path, tmp_path: Path) -> Path:
    out = tmp_path / "unpacked"
    out.mkdir()
    handler.unpack(exe_path, out)
    return out


class TestUnpack:
    def test_produces_json(self, unpacked_dir: Path) -> None:
        """Unpack should produce a strings.json file."""
        assert (unpacked_dir / "strings.json").exists()

    def test_produces_manifest(self, unpacked_dir: Path) -> None:
        """Unpack should produce a manifest.json file."""
        manifest = Manifest.from_json(unpacked_dir / "manifest.json")
        assert manifest.handler == "ExeTextHandler"

    def test_json_contains_dangerous_dave(self, unpacked_dir: Path) -> None:
        """JSON should contain the 'Dangerous Dave Commands' string."""
        data = json.loads((unpacked_dir / "strings.json").read_text())
        texts = [entry["text"] for entry in data["strings"]]
        assert any("Dangerous Dave Commands" in t for t in texts)

    def test_json_contains_game_over(self, unpacked_dir: Path) -> None:
        """JSON should contain the 'G A M E   O V E R' string."""
        data = json.loads((unpacked_dir / "strings.json").read_text())
        texts = [entry["text"] for entry in data["strings"]]
        assert any("G A M E   O V E R" in t for t in texts)

    def test_json_contains_copyright(self, unpacked_dir: Path) -> None:
        """JSON should contain the Softdisk copyright string."""
        data = json.loads((unpacked_dir / "strings.json").read_text())
        texts = [entry["text"] for entry in data["strings"]]
        assert any("Softdisk" in t for t in texts)

    def test_json_contains_level_names(self, unpacked_dir: Path) -> None:
        """JSON should contain level name strings."""
        data = json.loads((unpacked_dir / "strings.json").read_text())
        texts = [entry["text"] for entry in data["strings"]]
        assert any("LEVEL 1" in t for t in texts)
        assert any("LEVEL 8" in t for t in texts)

    def test_json_contains_congratulations(self, unpacked_dir: Path) -> None:
        """JSON should contain the win screen congratulations text."""
        data = json.loads((unpacked_dir / "strings.json").read_text())
        texts = [entry["text"] for entry in data["strings"]]
        assert any("You have freed Delbert" in t for t in texts)

    def test_json_entries_have_offset_and_max_length(self, unpacked_dir: Path) -> None:
        """Each JSON entry should have offset, text, and max_length fields."""
        data = json.loads((unpacked_dir / "strings.json").read_text())
        for entry in data["strings"]:
            assert "offset" in entry
            assert "text" in entry
            assert "max_length" in entry
            assert isinstance(entry["offset"], int)
            assert isinstance(entry["text"], str)
            assert isinstance(entry["max_length"], int)
            assert len(entry["text"].encode("ascii")) <= entry["max_length"]

    def test_json_string_count(self, unpacked_dir: Path) -> None:
        """JSON should contain the expected number of strings."""
        data = json.loads((unpacked_dir / "strings.json").read_text())
        # We expect a reasonable number of game strings
        assert len(data["strings"]) >= 50

    def test_manifest_has_original_exe(self, unpacked_dir: Path) -> None:
        """Manifest should store the original EXE for repacking."""
        manifest = Manifest.from_json(unpacked_dir / "manifest.json")
        assert "original_exe" in manifest.metadata


class TestRepack:
    def test_roundtrip_byte_exact(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        """Unpack then repack with original strings should produce identical EXE."""
        unpack_dir = tmp_path / "unpack"
        unpack_dir.mkdir()
        manifest = handler.unpack(exe_path, unpack_dir)

        repack_path = tmp_path / "repacked.exe"
        handler.repack(manifest, unpack_dir, repack_path)

        assert repack_path.read_bytes() == exe_path.read_bytes()

    def test_string_too_long_raises(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        """A string longer than max_length should raise ValueError."""
        unpack_dir = tmp_path / "unpack"
        unpack_dir.mkdir()
        manifest = handler.unpack(exe_path, unpack_dir)

        strings_path = unpack_dir / "strings.json"
        data = json.loads(strings_path.read_text())

        # Make a string way too long
        for entry in data["strings"]:
            if entry["text"] == "You win!":
                entry["text"] = "A" * (entry["max_length"] + 10)
                break

        strings_path.write_text(json.dumps(data, indent=2))

        repack_path = tmp_path / "repacked.exe"
        with pytest.raises(ValueError, match="exceeds maximum length"):
            handler.repack(manifest, unpack_dir, repack_path)

    def test_patching_decompressed_strings(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        """Verify that string patching works correctly at the decompressed level."""
        import struct

        exe_data = exe_path.read_bytes()
        decompressed = decompress(exe_data)

        header_para = struct.unpack_from("<H", decompressed, 8)[0]
        code_start = header_para * 16

        # Simulate what repack does internally: patch strings in decompressed EXE
        patched = bytearray(decompressed)

        # Patch "You win!" (8 bytes) -> "U win!!!" (8 bytes) - same length
        win_offset = code_start + 0x27646
        assert patched[win_offset : win_offset + 8] == b"You win!"
        patched[win_offset : win_offset + 8] = b"U win!!!"
        assert patched[win_offset : win_offset + 9] == b"U win!!!\x00"

        # Patch "God mode off" (12 bytes) -> "Off" (3 bytes) + NUL pad
        god_offset = code_start + 0x273AE
        assert patched[god_offset : god_offset + 12] == b"God mode off"
        patched[god_offset : god_offset + 13] = b"Off" + b"\x00" * 10
        assert patched[god_offset : god_offset + 13] == b"Off" + b"\x00" * 10

        # Verify changes are in the patched EXE
        assert b"U win!!!" in patched
        assert b"You win!" not in patched
        assert b"Off" in patched[god_offset : god_offset + 13]

    def test_modify_string_and_repack(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        """Modifying a string in JSON and repacking should produce changed EXE."""
        unpack_dir = tmp_path / "unpack"
        unpack_dir.mkdir()
        manifest = handler.unpack(exe_path, unpack_dir)

        # Modify "Softdisk" -> "Xoftdisk" in the copyright string.
        # The 'S' byte is a literal in the compressed stream, so PKLITE
        # byte-patching can handle this single-byte change.
        strings_path = unpack_dir / "strings.json"
        data = json.loads(strings_path.read_text())

        for entry in data["strings"]:
            if entry["id"] == "copyright":
                entry["text"] = entry["text"].replace("Softdisk", "Xoftdisk")
                break

        strings_path.write_text(json.dumps(data, indent=2))

        repack_path = tmp_path / "repacked.exe"
        handler.repack(manifest, unpack_dir, repack_path)

        repacked_dec = decompress(repack_path.read_bytes())
        assert b"Xoftdisk" in repacked_dec
        assert b"Softdisk" not in repacked_dec

    def test_repack_preserves_other_data(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        """Repacking should not change any non-string data in the EXE."""
        import struct

        unpack_dir = tmp_path / "unpack"
        unpack_dir.mkdir()
        manifest = handler.unpack(exe_path, unpack_dir)

        repack_path = tmp_path / "repacked.exe"
        handler.repack(manifest, unpack_dir, repack_path)

        original_dec = decompress(exe_path.read_bytes())
        repacked_dec = decompress(repack_path.read_bytes())

        # Death sequence data at 0x18A60 should be identical
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
        """Handler should match known EXE file patterns."""
        assert "DAVE.EXE" in handler.file_patterns
        assert "1.EXE" in handler.file_patterns
