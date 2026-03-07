"""XBIN text-mode screen format (subset: no compression, no palette)."""

from __future__ import annotations

import struct
from dataclasses import dataclass

_MAGIC = b"XBIN"
_EOF = 0x1A
_HEADER_SIZE = 11
_FLAG_PALETTE = 0b00000001
_FLAG_FONT = 0b00000010
_PALETTE_SIZE = 48  # 16 colors x 3 bytes (RGB)


@dataclass(frozen=True, slots=True)
class XbinFile:
    """Parsed XBIN file."""

    width: int
    height: int
    font_height: int
    font: bytes | None
    image_data: bytes


def build(
    width: int,
    height: int,
    image_data: bytes,
    *,
    font: bytes | None = None,
    font_height: int = 16,
) -> bytes:
    """Build an XBIN file from components."""
    expected_image = width * height * 2
    if len(image_data) != expected_image:
        msg = f"image_data length {len(image_data)} != {expected_image} (width*height*2)"
        raise ValueError(msg)

    flags = 0
    header_font_height = 0

    if font is not None:
        expected_font = 256 * font_height
        if len(font) != expected_font:
            msg = f"font length {len(font)} != {expected_font} (256 * font_height)"
            raise ValueError(msg)
        flags |= _FLAG_FONT
        header_font_height = font_height

    header = struct.pack(
        "<4sBHHBB",
        _MAGIC,
        _EOF,
        width,
        height,
        header_font_height,
        flags,
    )

    parts: list[bytes] = [header]
    if font is not None:
        parts.append(font)
    parts.append(image_data)

    return b"".join(parts)


def parse(data: bytes) -> XbinFile:
    """Parse an XBIN file into components."""
    if len(data) < _HEADER_SIZE:
        msg = f"Data too short for XBIN header: {len(data)} bytes"
        raise ValueError(msg)

    magic, _eof_char, width, height, font_height, flags = struct.unpack_from("<4sBHHBB", data, 0)

    if magic != _MAGIC:
        msg = f"Invalid XBIN magic: expected {_MAGIC!r}, got {magic!r}"
        raise ValueError(msg)

    offset = _HEADER_SIZE

    if flags & _FLAG_PALETTE:
        offset += _PALETTE_SIZE

    font: bytes | None = None

    if flags & _FLAG_FONT:
        font_size = 256 * font_height
        font_end = offset + font_size
        if len(data) < font_end:
            msg = f"font data truncated: need {font_size} bytes, got {len(data) - offset}"
            raise ValueError(msg)
        font = data[offset:font_end]
        offset = font_end

    image_size = width * height * 2
    if len(data) - offset < image_size:
        msg = f"image_data truncated: need {image_size} bytes, got {len(data) - offset}"
        raise ValueError(msg)
    image_data = data[offset : offset + image_size]

    return XbinFile(
        width=width,
        height=height,
        font_height=font_height,
        font=font,
        image_data=image_data,
    )
