"""Tests for the XBIN text-mode screen format module."""

from __future__ import annotations

import struct

import pytest

from gravedigger.xbin import XbinFile, build, parse

_MAGIC = b"XBIN"
_EOF = 0x1A
_HEADER_SIZE = 11
_FLAG_PALETTE = 0b00000001
_FLAG_FONT = 0b00000010
_PALETTE_SIZE = 48


class TestBuildParse:
    """Roundtrip tests: build -> parse."""

    def test_roundtrip_no_font(self) -> None:
        width, height = 80, 25
        image_data = bytes(range(256)) * (width * height * 2 // 256) + bytes(
            (width * height * 2) % 256
        )
        raw = build(width, height, image_data)
        result = parse(raw)
        assert result.width == width
        assert result.height == height
        assert result.font is None
        assert result.image_data == image_data

    def test_roundtrip_with_font(self) -> None:
        width, height = 40, 12
        font_height = 16
        image_data = bytes(width * height * 2)
        font = bytes(range(256)) * (256 * font_height // 256)
        raw = build(width, height, image_data, font=font, font_height=font_height)
        result = parse(raw)
        assert result.width == width
        assert result.height == height
        assert result.font_height == font_height
        assert result.font == font
        assert result.image_data == image_data

    def test_roundtrip_font_height_8(self) -> None:
        width, height = 10, 5
        font_height = 8
        image_data = bytes(width * height * 2)
        font = bytes(256 * font_height)
        raw = build(width, height, image_data, font=font, font_height=font_height)
        result = parse(raw)
        assert result.font_height == font_height
        assert result.font == font

    def test_roundtrip_font_height_14(self) -> None:
        width, height = 10, 5
        font_height = 14
        image_data = bytes(width * height * 2)
        font = bytes(256 * font_height)
        raw = build(width, height, image_data, font=font, font_height=font_height)
        result = parse(raw)
        assert result.font_height == font_height
        assert result.font == font

    def test_roundtrip_1x1_screen(self) -> None:
        """Edge case: minimum 1x1 screen."""
        width, height = 1, 1
        image_data = b"\x41\x07"  # 'A' with white-on-black attr
        raw = build(width, height, image_data)
        result = parse(raw)
        assert result.width == 1
        assert result.height == 1
        assert result.image_data == image_data

    def test_roundtrip_large_screen(self) -> None:
        """Edge case: large screen size."""
        width, height = 160, 50
        image_data = bytes(width * height * 2)
        raw = build(width, height, image_data)
        result = parse(raw)
        assert result.width == width
        assert result.height == height
        assert result.image_data == image_data

    def test_image_data_integrity(self) -> None:
        """Image data bytes are preserved exactly through roundtrip."""
        width, height = 4, 3
        # Each cell has distinct char+attr pair
        image_data = bytes((i % 256) for i in range(width * height * 2))
        raw = build(width, height, image_data)
        result = parse(raw)
        assert result.image_data == image_data


class TestHeaderBytes:
    """Verify the raw bytes of the header are correct."""

    def test_magic_bytes(self) -> None:
        raw = build(80, 25, bytes(80 * 25 * 2))
        assert raw[:4] == _MAGIC

    def test_eof_char(self) -> None:
        raw = build(80, 25, bytes(80 * 25 * 2))
        assert raw[4] == _EOF

    def test_width_height_little_endian(self) -> None:
        raw = build(80, 25, bytes(80 * 25 * 2))
        width = struct.unpack_from("<H", raw, 5)[0]
        height = struct.unpack_from("<H", raw, 7)[0]
        assert width == 80
        assert height == 25

    def test_no_font_font_height_zero(self) -> None:
        raw = build(80, 25, bytes(80 * 25 * 2))
        assert raw[9] == 0  # font_height == 0 when no font

    def test_no_font_flag_not_set(self) -> None:
        raw = build(80, 25, bytes(80 * 25 * 2))
        assert raw[10] & _FLAG_FONT == 0

    def test_with_font_font_height_in_header(self) -> None:
        font_height = 16
        font = bytes(256 * font_height)
        raw = build(4, 4, bytes(4 * 4 * 2), font=font, font_height=font_height)
        assert raw[9] == font_height

    def test_with_font_flag_set(self) -> None:
        font_height = 16
        font = bytes(256 * font_height)
        raw = build(4, 4, bytes(4 * 4 * 2), font=font, font_height=font_height)
        assert raw[10] & _FLAG_FONT != 0

    def test_header_size(self) -> None:
        raw = build(1, 1, b"\x00\x00")
        # Without font, total = 11 (header) + 2 (image)
        assert len(raw) == _HEADER_SIZE + 2

    def test_total_size_with_font(self) -> None:
        font_height = 8
        font = bytes(256 * font_height)
        width, height = 2, 2
        image_data = bytes(width * height * 2)
        raw = build(width, height, image_data, font=font, font_height=font_height)
        expected = _HEADER_SIZE + 256 * font_height + width * height * 2
        assert len(raw) == expected


class TestBuildErrors:
    """Error cases in build()."""

    def test_wrong_image_data_length(self) -> None:
        with pytest.raises(ValueError, match="image_data"):
            build(80, 25, bytes(80 * 25 * 2 - 1))

    def test_wrong_font_length(self) -> None:
        font_height = 16
        bad_font = bytes(256 * font_height - 1)
        with pytest.raises(ValueError, match="font"):
            build(4, 4, bytes(4 * 4 * 2), font=bad_font, font_height=font_height)


class TestParseErrors:
    """Error cases in parse()."""

    def test_wrong_magic(self) -> None:
        raw = bytearray(build(4, 4, bytes(4 * 4 * 2)))
        raw[:4] = b"NOPE"
        with pytest.raises(ValueError, match="magic"):
            parse(bytes(raw))

    def test_truncated_header(self) -> None:
        with pytest.raises((ValueError, struct.error)):
            parse(b"XBIN\x1a\x04")

    def test_truncated_image_data(self) -> None:
        raw = build(4, 4, bytes(4 * 4 * 2))
        # Chop off last byte of image data
        with pytest.raises(ValueError, match="image_data"):
            parse(raw[:-1])

    def test_truncated_font_data(self) -> None:
        font_height = 16
        font = bytes(256 * font_height)
        raw = build(4, 4, bytes(4 * 4 * 2), font=font, font_height=font_height)
        # Chop data after header so font is truncated
        with pytest.raises(ValueError, match="font"):
            parse(raw[: _HEADER_SIZE + 10])

    def test_parse_result_is_xbinfile(self) -> None:
        raw = build(2, 2, bytes(2 * 2 * 2))
        result = parse(raw)
        assert isinstance(result, XbinFile)


class TestParsePalette:
    """Parsing XBIN files with palette flag (e.g. saved by Moebius)."""

    def _build_with_palette(
        self,
        width: int,
        height: int,
        image_data: bytes,
        *,
        font: bytes | None = None,
        font_height: int = 16,
    ) -> bytes:
        """Build an XBIN with palette flag set (not produced by our build())."""
        flags = _FLAG_PALETTE
        header_font_height = 0
        palette = bytes(range(_PALETTE_SIZE))  # dummy 48-byte palette

        if font is not None:
            flags |= _FLAG_FONT
            header_font_height = font_height

        header = struct.pack(
            "<4sBHHBB",
            b"XBIN",
            _EOF,
            width,
            height,
            header_font_height,
            flags,
        )

        parts: list[bytes] = [header, palette]
        if font is not None:
            parts.append(font)
        parts.append(image_data)
        return b"".join(parts)

    def test_parse_palette_no_font(self) -> None:
        width, height = 80, 25
        image_data = bytes(width * height * 2)
        raw = self._build_with_palette(width, height, image_data)
        result = parse(raw)
        assert result.width == width
        assert result.height == height
        assert result.font is None
        assert result.image_data == image_data

    def test_parse_palette_with_font(self) -> None:
        width, height = 80, 25
        font_height = 16
        font = bytes(b ^ 0x55 for b in range(256)) * font_height
        image_data = bytes(i % 256 for i in range(width * height * 2))
        raw = self._build_with_palette(
            width, height, image_data, font=font, font_height=font_height
        )
        result = parse(raw)
        assert result.font == font
        assert result.image_data == image_data

    def test_parse_palette_with_trailing_sauce(self) -> None:
        """Moebius appends a 129-byte SAUCE record; parser should ignore it."""
        width, height = 80, 25
        font_height = 16
        font = bytes(256 * font_height)
        image_data = bytes(width * height * 2)
        raw = self._build_with_palette(
            width, height, image_data, font=font, font_height=font_height
        )
        sauce = b"\x1a" + b"SAUCE" + bytes(123)  # 129-byte SAUCE stub
        raw_with_sauce = raw + sauce
        result = parse(raw_with_sauce)
        assert result.image_data == image_data
        assert result.font == font
