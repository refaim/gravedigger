from pathlib import Path

from PIL import Image

from gravedigger.compression.ega import (
    EGA_PALETTE,
    decode_planar,
    encode_planar,
    image_to_pixels,
    pixels_to_image,
)


class TestEGAPalette:
    def test_palette_length(self) -> None:
        assert len(EGA_PALETTE) == 16

    def test_black(self) -> None:
        assert EGA_PALETTE[0] == (0x00, 0x00, 0x00)

    def test_white(self) -> None:
        assert EGA_PALETTE[15] == (0xFF, 0xFF, 0xFF)

    def test_dark_red(self) -> None:
        assert EGA_PALETTE[4] == (0xA8, 0x00, 0x00)


class TestDecodePlanar:
    def test_all_zeros(self) -> None:
        """All-zero planes -> all-zero pixels."""
        data = b"\x00" * 128  # 4 planes * 32 bytes each for 16x16
        pixels = decode_planar(data, 16, 16)
        assert len(pixels) == 256
        assert all(p == 0 for p in pixels)

    def test_all_ones(self) -> None:
        """All-0xFF planes -> all color 15 pixels."""
        data = b"\xff" * 128
        pixels = decode_planar(data, 16, 16)
        assert len(pixels) == 256
        assert all(p == 15 for p in pixels)

    def test_known_vector(self) -> None:
        """Hand-crafted test: first 8 pixels from plane data."""
        # 8x1 image: plane_size = 1 byte per plane, 4 planes = 4 bytes
        # plane0 = 0b10101010 -> bits: 1,0,1,0,1,0,1,0
        # plane1 = 0b11001100 -> bits: 1,1,0,0,1,1,0,0
        # plane2 = 0b11110000 -> bits: 1,1,1,1,0,0,0,0
        # plane3 = 0b00000000 -> bits: 0,0,0,0,0,0,0,0
        data = bytes([0b10101010, 0b11001100, 0b11110000, 0b00000000])
        pixels = decode_planar(data, 8, 1)
        assert len(pixels) == 8
        # pixel 0: bit7 from each plane: p0=1, p1=1, p2=1, p3=0 -> 0b0111 = 7
        assert pixels[0] == 7
        # pixel 1: bit6: p0=0, p1=1, p2=1, p3=0 -> 0b0110 = 6
        assert pixels[1] == 6
        # pixel 2: bit5: p0=1, p1=0, p2=1, p3=0 -> 0b0101 = 5
        assert pixels[2] == 5
        # pixel 3: bit4: p0=0, p1=0, p2=1, p3=0 -> 0b0100 = 4
        assert pixels[3] == 4
        # pixel 4: bit3: p0=1, p1=1, p2=0, p3=0 -> 0b0011 = 3
        assert pixels[4] == 3
        # pixel 5: bit2: p0=0, p1=1, p2=0, p3=0 -> 0b0010 = 2
        assert pixels[5] == 2
        # pixel 6: bit1: p0=1, p1=0, p2=0, p3=0 -> 0b0001 = 1
        assert pixels[6] == 1
        # pixel 7: bit0: p0=0, p1=0, p2=0, p3=0 -> 0b0000 = 0
        assert pixels[7] == 0

    def test_output_length(self) -> None:
        data = b"\x00" * (4 * 40 * 200)  # 320x200 picture
        pixels = decode_planar(data, 320, 200)
        assert len(pixels) == 320 * 200


class TestEncodePlanar:
    def test_roundtrip_zeros(self) -> None:
        data = b"\x00" * 128
        pixels = decode_planar(data, 16, 16)
        encoded = encode_planar(pixels, 16, 16)
        assert encoded == data

    def test_roundtrip_ones(self) -> None:
        data = b"\xff" * 128
        pixels = decode_planar(data, 16, 16)
        encoded = encode_planar(pixels, 16, 16)
        assert encoded == data

    def test_roundtrip_known_vector(self) -> None:
        data = bytes([0b10101010, 0b11001100, 0b11110000, 0b00000000])
        pixels = decode_planar(data, 8, 1)
        encoded = encode_planar(pixels, 8, 1)
        assert encoded == data

    def test_roundtrip_random_tile(self) -> None:
        """Random 16x16 tile data roundtrips exactly."""
        import random

        rng = random.Random(42)
        data = bytes(rng.randint(0, 255) for _ in range(128))
        pixels = decode_planar(data, 16, 16)
        encoded = encode_planar(pixels, 16, 16)
        assert encoded == data


class TestValidation:
    def test_decode_planar_bad_width(self) -> None:
        import pytest

        with pytest.raises(ValueError, match="multiple of 8"):
            decode_planar(b"\x00" * 128, 13, 16)

    def test_encode_planar_bad_width(self) -> None:
        import pytest

        with pytest.raises(ValueError, match="multiple of 8"):
            encode_planar([0] * (13 * 16), 13, 16)

    def test_decode_planar_truncated_data(self) -> None:
        import pytest

        with pytest.raises(ValueError, match="Expected at least"):
            decode_planar(b"\x00" * 10, 16, 16)


class TestDecodePlanarFromFile:
    def test_first_tile_from_egatiles(self, game_dir: Path) -> None:
        """Decode first tile (128 bytes) from EGATILES.DD2."""
        raw = (game_dir / "EGATILES.DD2").read_bytes()
        tile_data = raw[:128]
        pixels = decode_planar(tile_data, 16, 16)
        assert len(pixels) == 256
        assert all(0 <= p <= 15 for p in pixels)

    def test_first_tile_roundtrip(self, game_dir: Path) -> None:
        """First tile encodes back to exact original bytes."""
        raw = (game_dir / "EGATILES.DD2").read_bytes()
        tile_data = raw[:128]
        pixels = decode_planar(tile_data, 16, 16)
        encoded = encode_planar(pixels, 16, 16)
        assert encoded == tile_data


class TestPixelsToImage:
    def test_creates_image(self) -> None:
        pixels = [0] * 256
        img = pixels_to_image(pixels, 16, 16)
        assert isinstance(img, Image.Image)
        assert img.size == (16, 16)
        assert img.mode == "P"

    def test_pixel_values(self) -> None:
        pixels = list(range(16)) * 16  # 16x16 image
        img = pixels_to_image(pixels, 16, 16)
        for i in range(16):
            assert img.getpixel((i, 0)) == i

    def test_palette_applied(self) -> None:
        pixels = [0] * 4
        img = pixels_to_image(pixels, 2, 2)
        palette = img.getpalette()
        assert palette is not None
        # First entry should be black (0,0,0)
        assert palette[0:3] == [0, 0, 0]
        # Last EGA color (index 15) should be white
        assert palette[45:48] == [255, 255, 255]


class TestImageToPixels:
    def test_roundtrip(self) -> None:
        original = list(range(16)) * 16
        img = pixels_to_image(original, 16, 16)
        result = image_to_pixels(img)
        assert result == original

    def test_single_color(self) -> None:
        pixels = [5] * 64
        img = pixels_to_image(pixels, 8, 8)
        result = image_to_pixels(img)
        assert result == pixels
