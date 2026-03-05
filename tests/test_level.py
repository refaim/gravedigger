"""Tests for the LevelHandler."""

import json
from pathlib import Path

import pytest

from gravedigger.handlers.level import LevelHandler

LEVEL_FILES = [f"LEVEL{i:02d}.DD2" for i in range(1, 9)]


@pytest.fixture()
def handler() -> LevelHandler:
    return LevelHandler()


class TestUnpackLevel01:
    """Test unpacking LEVEL01.DD2 to JSON."""

    def test_produces_json(self, handler: LevelHandler, game_dir: Path, tmp_output: Path) -> None:
        handler.unpack(game_dir / "LEVEL01.DD2", tmp_output)
        assert (tmp_output / "level.json").is_file()

    def test_manifest(self, handler: LevelHandler, game_dir: Path, tmp_output: Path) -> None:
        manifest = handler.unpack(game_dir / "LEVEL01.DD2", tmp_output)
        assert manifest.handler == "LevelHandler"
        assert manifest.source_file == "LEVEL01.DD2"
        assert manifest.metadata["width"] == 64
        assert manifest.metadata["height"] == 57

    def test_width_height(self, handler: LevelHandler, game_dir: Path, tmp_output: Path) -> None:
        handler.unpack(game_dir / "LEVEL01.DD2", tmp_output)
        data = json.loads((tmp_output / "level.json").read_text())
        assert data["width"] == 64
        assert data["height"] == 57

    def test_tile_map_size(self, handler: LevelHandler, game_dir: Path, tmp_output: Path) -> None:
        handler.unpack(game_dir / "LEVEL01.DD2", tmp_output)
        data = json.loads((tmp_output / "level.json").read_text())
        assert len(data["tile_map"]) == 64 * 57

    def test_object_map_contains_player_spawn(
        self, handler: LevelHandler, game_dir: Path, tmp_output: Path
    ) -> None:
        handler.unpack(game_dir / "LEVEL01.DD2", tmp_output)
        data = json.loads((tmp_output / "level.json").read_text())
        assert 0x00FF in data["object_map"]

    def test_bad_trailer_raises(self, tmp_path: Path) -> None:
        handler = LevelHandler()
        bad_file = tmp_path / "LEVEL01.DD2"
        bad_file.write_bytes(b"\x00" * 20)  # no MsDos trailer
        with pytest.raises(ValueError, match="MsDos trailer"):
            handler.unpack(bad_file, tmp_path / "out")


class TestRepackLevel01:
    """Test repacking LEVEL01.DD2 is byte-exact."""

    def test_byte_exact_roundtrip(
        self, handler: LevelHandler, game_dir: Path, tmp_output: Path, tmp_path: Path
    ) -> None:
        original = game_dir / "LEVEL01.DD2"
        manifest = handler.unpack(original, tmp_output)

        repacked = tmp_path / "LEVEL01_repacked.DD2"
        handler.repack(manifest, tmp_output, repacked)

        assert repacked.read_bytes() == original.read_bytes()


class TestAllLevels:
    """Test all 8 level files unpack and repack byte-exact."""

    @pytest.mark.parametrize("filename", LEVEL_FILES)
    def test_unpack_repack_roundtrip(
        self,
        handler: LevelHandler,
        game_dir: Path,
        tmp_output: Path,
        tmp_path: Path,
        filename: str,
    ) -> None:
        original = game_dir / filename
        if not original.exists():
            pytest.skip(f"{filename} not found")

        out_dir = tmp_output / filename
        out_dir.mkdir()
        manifest = handler.unpack(original, out_dir)

        repacked = tmp_path / f"{filename}_repacked"
        handler.repack(manifest, out_dir, repacked)

        assert repacked.read_bytes() == original.read_bytes()
