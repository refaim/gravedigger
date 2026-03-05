from pathlib import Path

from PIL import Image

from gravedigger.core.handler import Manifest
from gravedigger.handlers.tiles import TileHandler


class TestTileHandlerUnpack:
    def test_unpack_creates_pngs(self, game_dir: Path, tmp_output: Path) -> None:
        handler = TileHandler()
        manifest = handler.unpack(game_dir / "EGATILES.DD2", tmp_output)
        assert manifest.handler == "TileHandler"
        assert manifest.source_file == "EGATILES.DD2"
        assert manifest.metadata["total_tiles"] == 858
        # Check all 858 PNGs exist
        for i in range(858):
            png = tmp_output / f"tile_{i:04d}.png"
            assert png.exists(), f"Missing {png.name}"

    def test_tile_dimensions(self, game_dir: Path, tmp_output: Path) -> None:
        handler = TileHandler()
        handler.unpack(game_dir / "EGATILES.DD2", tmp_output)
        # First tile
        img = Image.open(tmp_output / "tile_0000.png")
        assert img.size == (16, 16)
        # Last tile
        img = Image.open(tmp_output / "tile_0857.png")
        assert img.size == (16, 16)

    def test_manifest_file_written(self, game_dir: Path, tmp_output: Path) -> None:
        handler = TileHandler()
        handler.unpack(game_dir / "EGATILES.DD2", tmp_output)
        manifest_path = tmp_output / "manifest.json"
        assert manifest_path.exists()
        loaded = Manifest.from_json(manifest_path)
        assert loaded.handler == "TileHandler"
        assert loaded.metadata["total_tiles"] == 858


class TestTileHandlerRepack:
    def test_repack_byte_exact(self, game_dir: Path, tmp_output: Path) -> None:
        original = (game_dir / "EGATILES.DD2").read_bytes()
        handler = TileHandler()
        handler.unpack(game_dir / "EGATILES.DD2", tmp_output)

        manifest = Manifest.from_json(tmp_output / "manifest.json")
        repack_path = tmp_output / "EGATILES.DD2"
        handler.repack(manifest, tmp_output, repack_path)

        repacked = repack_path.read_bytes()
        assert repacked == original


class TestTileHandlerPatterns:
    def test_file_patterns(self) -> None:
        handler = TileHandler()
        assert "EGATILES.DD2" in handler.file_patterns
