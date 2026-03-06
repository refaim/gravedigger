"""Tests for SpriteHandler — sprite files (HUFF + EGA)."""

from __future__ import annotations

from typing import TYPE_CHECKING

import pytest

if TYPE_CHECKING:
    from pathlib import Path
from PIL import Image

from gravedigger.core.handler import Manifest
from gravedigger.handlers.sprites import SPRITE_SIZES, SpriteHandler

SPRITE_FILES = ["S_DAVE.DD2", "S_CHUNK1.DD2", "S_CHUNK2.DD2", "S_FRANK.DD2", "S_MASTER.DD2"]


@pytest.fixture()
def handler() -> SpriteHandler:
    return SpriteHandler()


class TestUnpackDave:
    """Unpack S_DAVE.DD2 and verify sprite properties."""

    def test_unpack_sprite_count(
        self, handler: SpriteHandler, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        manifest = handler.unpack(game_dir / "S_DAVE.DD2", tmp_translatable, tmp_meta)
        sprites = manifest.metadata["sprites"]
        assert len(sprites) == len(SPRITE_SIZES["S_DAVE.DD2"])

    def test_first_sprite_size(
        self, handler: SpriteHandler, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        handler.unpack(game_dir / "S_DAVE.DD2", tmp_translatable, tmp_meta)
        img = Image.open(tmp_translatable / "sprites" / "dave_0000.png")
        assert img.size == (24, 32)

    def test_manifest_contains_sprite_sizes(
        self, handler: SpriteHandler, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        manifest = handler.unpack(game_dir / "S_DAVE.DD2", tmp_translatable, tmp_meta)
        sprites = manifest.metadata["sprites"]
        assert sprites[0]["width"] == 24
        assert sprites[0]["height"] == 32

    def test_manifest_saved_to_disk(
        self, handler: SpriteHandler, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        handler.unpack(game_dir / "S_DAVE.DD2", tmp_translatable, tmp_meta)
        loaded = Manifest.from_json(tmp_meta / "manifest.json")
        assert loaded.handler == "SpriteHandler"
        assert loaded.source_file == "S_DAVE.DD2"
        assert len(loaded.metadata["sprites"]) == len(SPRITE_SIZES["S_DAVE.DD2"])

    def test_unknown_sprite_file_raises(self, tmp_path: Path) -> None:
        handler = SpriteHandler()
        bad_file = tmp_path / "S_UNKNOWN.DD2"
        bad_file.write_bytes(b"\x00" * 100)
        with pytest.raises(ValueError, match="Unknown sprite file"):
            handler.unpack(bad_file, tmp_path / "out", tmp_path / "meta")


class TestRepackRoundtrip:
    """Repack sprites and verify byte-exact match with original."""

    @pytest.mark.parametrize("filename", SPRITE_FILES)
    def test_repack_byte_exact(
        self,
        handler: SpriteHandler,
        game_dir: Path,
        tmp_path: Path,
        filename: str,
    ) -> None:
        translatable = tmp_path / "translatable"
        meta = tmp_path / "meta"
        translatable.mkdir()
        meta.mkdir()
        original = (game_dir / filename).read_bytes()
        manifest = handler.unpack(game_dir / filename, translatable, meta)
        repacked_path = tmp_path / "repacked.DD2"
        handler.repack(manifest, translatable, meta, repacked_path)
        repacked = repacked_path.read_bytes()
        assert repacked == original, f"Repacked {filename} differs from original"


class TestAllSpriteFiles:
    """Test unpack for all 5 sprite files."""

    @pytest.mark.parametrize("filename", SPRITE_FILES)
    def test_unpack_creates_pngs(
        self,
        handler: SpriteHandler,
        game_dir: Path,
        tmp_path: Path,
        filename: str,
    ) -> None:
        translatable = tmp_path / "translatable"
        meta = tmp_path / "meta"
        translatable.mkdir()
        meta.mkdir()
        manifest = handler.unpack(game_dir / filename, translatable, meta)
        sprites = manifest.metadata["sprites"]
        expected = len(SPRITE_SIZES[filename])
        assert len(sprites) == expected

        prefix = filename.removesuffix(".DD2").lower().removeprefix("s_")
        sprites_dir = translatable / "sprites"
        for i, sp in enumerate(sprites):
            img = Image.open(sprites_dir / f"{prefix}_{i:04d}.png")
            assert img.size == (sp["width"], sp["height"]), (
                f"{filename} sprite {i}: expected {sp['width']}x{sp['height']}, got {img.size}"
            )
