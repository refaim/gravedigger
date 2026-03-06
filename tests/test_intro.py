from pathlib import Path

import pytest
from PIL import Image

from gravedigger.core.handler import Manifest
from gravedigger.handlers.intro import ESCAPE, IntroHandler, compress_rle, decompress_rle


class TestRleFunctions:
    def test_decompress_empty(self) -> None:
        data, offset = decompress_rle(b"", 0, 100)
        assert data == b""
        assert offset == 0

    def test_decompress_literal_bytes(self) -> None:
        data, _offset = decompress_rle(bytes([1, 2, 3]), 0, 3)
        assert data == bytes([1, 2, 3])

    def test_decompress_rle_escape(self) -> None:
        raw = bytes([ESCAPE, 5, 0xAA])
        data, _offset = decompress_rle(raw, 0, 100)
        assert data == bytes([0xAA] * 5)

    def test_decompress_truncated_escape_raises(self) -> None:
        with pytest.raises(ValueError, match="Truncated"):
            decompress_rle(bytes([ESCAPE]), 0, 100)

    def test_decompress_with_limit(self) -> None:
        raw = bytes([ESCAPE, 10, 0xBB])
        data, _offset = decompress_rle(raw, 0, 5)
        assert data == bytes([0xBB] * 5)

    def test_compress_empty(self) -> None:
        assert compress_rle(b"") == b""

    def test_compress_escape_byte(self) -> None:
        result = compress_rle(bytes([ESCAPE]))
        assert result == bytes([ESCAPE, 1, ESCAPE])

    def test_compress_short_run_literal(self) -> None:
        result = compress_rle(bytes([0x42] * 3))
        assert result == bytes([0x42] * 3)

    def test_compress_long_run_rle(self) -> None:
        result = compress_rle(bytes([0x42] * 5))
        assert result == bytes([ESCAPE, 5, 0x42])

    def test_compress_run_over_255_chunks(self) -> None:
        result = compress_rle(bytes([0x42] * 300))
        assert result == bytes([ESCAPE, 255, 0x42, ESCAPE, 45, 0x42])

    def test_roundtrip(self) -> None:
        original = bytes(range(256)) * 2
        compressed = compress_rle(original)
        decompressed, _ = decompress_rle(compressed, 0, len(original))
        assert decompressed == original


class TestIntroHandlerUnpack:
    def test_unpack_creates_png(
        self, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        handler = IntroHandler()
        manifest = handler.unpack(game_dir / "INTRO.DD2", tmp_translatable, tmp_meta)
        assert manifest.handler == "IntroHandler"
        assert manifest.source_file == "INTRO.DD2"
        png = tmp_translatable / "intro.png"
        assert png.exists()

    def test_image_dimensions(
        self, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        handler = IntroHandler()
        handler.unpack(game_dir / "INTRO.DD2", tmp_translatable, tmp_meta)
        img = Image.open(tmp_translatable / "intro.png")
        assert img.size == (256, 64)

    def test_manifest_file_written(
        self, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        handler = IntroHandler()
        handler.unpack(game_dir / "INTRO.DD2", tmp_translatable, tmp_meta)
        manifest_path = tmp_meta / "manifest.json"
        assert manifest_path.exists()
        loaded = Manifest.from_json(manifest_path)
        assert loaded.handler == "IntroHandler"


class TestIntroHandlerRepack:
    def test_repack_byte_exact(
        self, game_dir: Path, tmp_translatable: Path, tmp_meta: Path
    ) -> None:
        original = (game_dir / "INTRO.DD2").read_bytes()
        handler = IntroHandler()
        handler.unpack(game_dir / "INTRO.DD2", tmp_translatable, tmp_meta)

        manifest = Manifest.from_json(tmp_meta / "manifest.json")
        repack_path = tmp_meta / "INTRO.DD2"
        handler.repack(manifest, tmp_translatable, tmp_meta, repack_path)

        repacked = repack_path.read_bytes()
        assert repacked == original


class TestIntroHandlerPatterns:
    def test_file_patterns(self) -> None:
        handler = IntroHandler()
        assert "INTRO.DD2" in handler.file_patterns
