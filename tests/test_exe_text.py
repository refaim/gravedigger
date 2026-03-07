"""Tests for ExeTextHandler — text strings from EXE."""

from __future__ import annotations

from pathlib import Path

import pytest
from openpyxl import load_workbook

from gravedigger.core.handler import Manifest
from gravedigger.handlers.exe_text import ExeTextHandler, _decompress_exe

_GAME_ROOT = Path(__file__).resolve().parent.parent / "game"
_EXE_VARIANTS = [
    ("softdisk", "1.EXE"),
    ("retail", "MANSION.EXE"),
]


def _available_exe_paths() -> list[Path]:
    return [
        _GAME_ROOT / variant / name
        for variant, name in _EXE_VARIANTS
        if (_GAME_ROOT / variant / name).exists()
    ]


@pytest.fixture()
def handler() -> ExeTextHandler:
    return ExeTextHandler()


@pytest.fixture(params=_available_exe_paths(), ids=lambda p: p.name)
def exe_path(request: pytest.FixtureRequest) -> Path:
    path: Path = request.param
    return path


@pytest.fixture()
def unpacked_dirs(handler: ExeTextHandler, exe_path: Path, tmp_path: Path) -> tuple[Path, Path]:
    translatable = tmp_path / "translatable"
    meta = tmp_path / "meta"
    translatable.mkdir()
    meta.mkdir()
    handler.unpack(exe_path, translatable, meta)
    return translatable, meta


def _read_xlsx(path: Path) -> dict[str, str]:
    """Read strings.xlsx into {id: text} dict."""
    wb = load_workbook(path)
    ws = wb.active
    assert ws is not None
    result: dict[str, str] = {}
    for row in ws.iter_rows(min_row=2, values_only=True):
        str_id, text = row[0], row[1]
        result[str(str_id)] = str(text) if text is not None else ""
    return result


def _write_xlsx(path: Path, strings: dict[str, str]) -> None:
    """Write {id: text} dict back to strings.xlsx."""
    from openpyxl import Workbook

    wb = Workbook()
    ws = wb.active
    assert ws is not None
    ws.title = "strings"
    ws.append(["id", "text"])
    for str_id, text in strings.items():
        ws.append([str_id, text])
    wb.save(path)


class TestUnpack:
    def test_produces_xlsx(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        assert (translatable / "strings.xlsx").exists()

    def test_produces_manifest(self, unpacked_dirs: tuple[Path, Path]) -> None:
        _, meta = unpacked_dirs
        manifest = Manifest.from_json(meta / "manifest.json")
        assert manifest.handler == "ExeTextHandler"

    def test_xlsx_contains_dangerous_dave(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        strings = _read_xlsx(translatable / "strings.xlsx")
        assert any("Dangerous Dave Commands" in t for t in strings.values())

    def test_xlsx_contains_game_over(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        strings = _read_xlsx(translatable / "strings.xlsx")
        assert any("G A M E   O V E R" in t for t in strings.values())

    def test_xlsx_contains_copyright(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        strings = _read_xlsx(translatable / "strings.xlsx")
        assert any("Softdisk" in t for t in strings.values())

    def test_xlsx_contains_level_names(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        strings = _read_xlsx(translatable / "strings.xlsx")
        assert any("LEVEL 1" in t for t in strings.values())
        assert any("LEVEL 8" in t for t in strings.values())

    def test_xlsx_contains_congratulations(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        strings = _read_xlsx(translatable / "strings.xlsx")
        assert any("You have freed Delbert" in t for t in strings.values())

    def test_xlsx_has_header_row(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        wb = load_workbook(translatable / "strings.xlsx")
        ws = wb.active
        assert ws is not None
        assert ws.cell(1, 1).value == "id"
        assert ws.cell(1, 2).value == "text"

    def test_xlsx_string_count(self, unpacked_dirs: tuple[Path, Path]) -> None:
        translatable, _ = unpacked_dirs
        strings = _read_xlsx(translatable / "strings.xlsx")
        assert len(strings) >= 50

    def test_manifest_has_original_exe(self, unpacked_dirs: tuple[Path, Path]) -> None:
        _, meta = unpacked_dirs
        manifest = Manifest.from_json(meta / "manifest.json")
        assert "original_exe" in manifest.metadata

    def test_manifest_has_strings_meta(self, unpacked_dirs: tuple[Path, Path]) -> None:
        _, meta = unpacked_dirs
        manifest = Manifest.from_json(meta / "manifest.json")
        strings_meta = manifest.metadata["strings"]
        assert len(strings_meta) >= 50
        for entry in strings_meta:
            assert "id" in entry
            assert "offset" in entry
            assert "max_length" in entry
            assert isinstance(entry["offset"], int)
            assert isinstance(entry["max_length"], int)


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

    def test_string_longer_than_original_relocates(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        """Strings longer than original slot are relocated to an appended block."""
        translatable = tmp_path / "translatable"
        meta = tmp_path / "meta"
        translatable.mkdir()
        meta.mkdir()
        manifest = handler.unpack(exe_path, translatable, meta)

        xlsx_path = translatable / "strings.xlsx"
        strings = _read_xlsx(xlsx_path)
        long_text = "A very long replacement string that exceeds the original"
        strings["win_line13"] = long_text
        _write_xlsx(xlsx_path, strings)

        repack_path = tmp_path / "repacked.exe"
        handler.repack(manifest, translatable, meta, repack_path)

        # Output is an uncompressed EXE when relocation is needed
        repacked = repack_path.read_bytes()
        assert long_text.encode("ascii") in repacked

    def test_patching_decompressed_strings(
        self, handler: ExeTextHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        import struct

        exe_data = exe_path.read_bytes()
        decompressed = _decompress_exe(exe_data)

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

        xlsx_path = translatable / "strings.xlsx"
        strings = _read_xlsx(xlsx_path)
        strings["copyright"] = strings["copyright"].replace("Softdisk", "Xoftdisk")
        _write_xlsx(xlsx_path, strings)

        repack_path = tmp_path / "repacked.exe"
        handler.repack(manifest, translatable, meta, repack_path)

        repacked_dec = _decompress_exe(repack_path.read_bytes())
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

        original_dec = _decompress_exe(exe_path.read_bytes())
        repacked_dec = _decompress_exe(repack_path.read_bytes())

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
        assert "MANSION.EXE" in handler.file_patterns
