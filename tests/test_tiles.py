from pathlib import Path

from PIL import Image

from gravedigger.core.handler import Manifest
from gravedigger.handlers.tiles import TileHandler


class TestTileHandlerUnpack:
    def test_unpack_creates_pngs(
        self, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        handler = TileHandler()
        manifest = handler.unpack(game_dir / "EGATILES.DD2", tmp_translatable, tmp_meta)
        assert manifest.handler == "TileHandler"
        assert manifest.source_file == "EGATILES.DD2"
        assert manifest.metadata["total_tiles"] == 858
        tiles_dir = tmp_translatable / "tiles"
        for i in range(858):
            png = tiles_dir / f"tile_{i:04d}.png"
            assert png.exists(), f"Missing {png.name}"

    def test_tile_dimensions(
        self, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        handler = TileHandler()
        handler.unpack(game_dir / "EGATILES.DD2", tmp_translatable, tmp_meta)
        tiles_dir = tmp_translatable / "tiles"
        img = Image.open(tiles_dir / "tile_0000.png")
        assert img.size == (16, 16)
        img = Image.open(tiles_dir / "tile_0857.png")
        assert img.size == (16, 16)

    def test_manifest_file_written(
        self, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        handler = TileHandler()
        handler.unpack(game_dir / "EGATILES.DD2", tmp_translatable, tmp_meta)
        manifest_path = tmp_meta / "manifest.json"
        assert manifest_path.exists()
        loaded = Manifest.from_json(manifest_path)
        assert loaded.handler == "TileHandler"
        assert loaded.metadata["total_tiles"] == 858


class TestTileHandlerRepack:
    def test_repack_byte_exact(
        self, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        original = (game_dir / "EGATILES.DD2").read_bytes()
        handler = TileHandler()
        handler.unpack(game_dir / "EGATILES.DD2", tmp_translatable, tmp_meta)

        manifest = Manifest.from_json(tmp_meta / "manifest.json")
        repack_path = tmp_meta / "EGATILES.DD2"
        handler.repack(manifest, tmp_translatable, tmp_meta, repack_path)

        repacked = repack_path.read_bytes()
        assert repacked == original


class TestTileHandlerPatterns:
    def test_file_patterns(self) -> None:
        handler = TileHandler()
        assert "EGATILES.DD2" in handler.file_patterns
