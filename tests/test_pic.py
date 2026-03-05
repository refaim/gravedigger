from pathlib import Path

import pytest
from PIL import Image

from gravedigger.handlers.pic import PicHandler

PIC_FILES = ["TITLE1.DD2", "TITLE2.DD2", "PROGPIC.DD2", "STARPIC.DD2"]

# Reference pixel values at (0,0), (160,100), (319,199) for each PIC file
REFERENCE_PIXELS: dict[str, tuple[int, int, int]] = {
    "TITLE1.DD2": (8, 15, 2),
    "TITLE2.DD2": (8, 6, 8),
    "PROGPIC.DD2": (3, 6, 6),
    "STARPIC.DD2": (0, 0, 0),
}


@pytest.fixture()
def handler() -> PicHandler:
    return PicHandler()


class TestPicUnpack:
    @pytest.mark.parametrize("filename", PIC_FILES)
    def test_unpack_produces_320x200_png(
        self, handler: PicHandler, game_dir: Path, tmp_output: Path, filename: str
    ) -> None:
        input_path = game_dir / filename
        manifest = handler.unpack(input_path, tmp_output)

        png_path = tmp_output / manifest.metadata["image"]
        assert png_path.exists()

        img = Image.open(png_path)
        assert img.size == (320, 200)
        assert img.mode == "P"

    @pytest.mark.parametrize("filename", PIC_FILES)
    def test_unpack_manifest_metadata(
        self, handler: PicHandler, game_dir: Path, tmp_output: Path, filename: str
    ) -> None:
        input_path = game_dir / filename
        manifest = handler.unpack(input_path, tmp_output)

        assert manifest.handler == "PicHandler"
        assert manifest.source_file == filename
        assert manifest.metadata["width"] == 320
        assert manifest.metadata["height"] == 200

    @pytest.mark.parametrize("filename", PIC_FILES)
    def test_unpack_pixel_values_match_reference(
        self, handler: PicHandler, game_dir: Path, tmp_output: Path, filename: str
    ) -> None:
        input_path = game_dir / filename
        manifest = handler.unpack(input_path, tmp_output)

        png_path = tmp_output / manifest.metadata["image"]
        img = Image.open(png_path)
        px = img.load()
        assert px is not None

        p00, p_mid, p_last = REFERENCE_PIXELS[filename]
        assert px[0, 0] == p00
        assert px[160, 100] == p_mid
        assert px[319, 199] == p_last


class TestPicRepack:
    @pytest.mark.parametrize("filename", PIC_FILES)
    def test_repack_byte_exact(
        self, handler: PicHandler, game_dir: Path, tmp_output: Path, filename: str
    ) -> None:
        input_path = game_dir / filename
        original = input_path.read_bytes()

        manifest = handler.unpack(input_path, tmp_output)

        output_path = tmp_output / "repacked" / filename
        output_path.parent.mkdir(parents=True, exist_ok=True)
        handler.repack(manifest, tmp_output, output_path)

        repacked = output_path.read_bytes()
        assert repacked == original


class TestPicErrors:
    def test_bad_pic_signature_raises(
        self, handler: PicHandler, game_dir: Path, tmp_path: Path
    ) -> None:
        """File that decompresses to non-PIC data should raise ValueError."""
        from gravedigger.compression.huff import compress, decompress

        # Get a real tree from any HUFF file
        raw = (game_dir / "TITLE1.DD2").read_bytes()
        _data, tree = decompress(raw)

        # Compress data with wrong signature
        bad_data = b"BAD\x00" + b"\x28\x00\xc8\x00" + b"\x00" * (40 * 200 * 4)
        bad_dd2 = compress(bad_data, tree)

        bad_file = tmp_path / "BAD.DD2"
        bad_file.write_bytes(bad_dd2)
        out = tmp_path / "out"
        out.mkdir()

        with pytest.raises(ValueError, match="PIC signature"):
            handler.unpack(bad_file, out)
