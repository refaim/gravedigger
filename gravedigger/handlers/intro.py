from __future__ import annotations

import base64
import struct
from typing import TYPE_CHECKING, ClassVar

from gravedigger.compression.ega import (
    decode_planar,
    encode_planar,
    image_to_pixels,
    pixels_to_image,
)
from gravedigger.core.handler import FormatHandler, Manifest

if TYPE_CHECKING:
    from pathlib import Path

INTRO_W = 256
INTRO_H = 64
ESCAPE = 0xFE
MIN_RLE_RUN = 4


def decompress_rle(data: bytes, offset: int, limit: int) -> tuple[bytes, int]:
    """Decompress byte-level RLE with 0xFE escape byte.

    Returns decompressed data and the offset where reading stopped.
    """
    out = bytearray()
    i = offset
    while i < len(data) and len(out) < limit:
        if data[i] == ESCAPE:
            if i + 2 >= len(data):
                msg = f"Truncated RLE escape at offset {i}"
                raise ValueError(msg)
            count = data[i + 1]
            value = data[i + 2]
            remaining = limit - len(out)
            actual = min(count, remaining)
            out.extend([value] * actual)
            i += 3
        else:
            out.append(data[i])
            i += 1
    return bytes(out), i


def compress_rle(decompressed: bytes) -> bytes:
    """Compress using byte-level RLE with 0xFE escape byte.

    Rules: 0xFE bytes always escaped as FE 01 FE.
    Non-0xFE runs of 4+ use RLE. Runs of 1-3 are literal.
    """
    out = bytearray()
    i = 0
    while i < len(decompressed):
        b = decompressed[i]
        count = 1
        while i + count < len(decompressed) and decompressed[i + count] == b:
            count += 1

        if b == ESCAPE:
            total = count
            while total > 0:
                chunk = min(total, 255)
                out.extend([ESCAPE, chunk, ESCAPE])
                total -= chunk
        elif count >= MIN_RLE_RUN:
            total = count
            while total > 0:
                chunk = min(total, 255)
                out.extend([ESCAPE, chunk, b])
                total -= chunk
        else:
            out.extend([b] * count)

        i += count
    return bytes(out)


class IntroHandler(FormatHandler):
    file_patterns: ClassVar[list[str]] = ["INTRO.DD2"]

    def unpack(self, input_path: Path, output_dir: Path) -> Manifest:
        data = input_path.read_bytes()
        (decomp_size,) = struct.unpack_from("<I", data, 0)
        decompressed, end_offset = decompress_rle(data, 4, decomp_size)
        trailing = data[end_offset:]

        pixels = decode_planar(decompressed, INTRO_W, INTRO_H)
        img = pixels_to_image(pixels, INTRO_W, INTRO_H)
        img.save(output_dir / "intro.png")

        metadata: dict[str, object] = {
            "width": INTRO_W,
            "height": INTRO_H,
            "decomp_size": decomp_size,
        }
        if trailing:
            metadata["trailing"] = base64.b64encode(trailing).decode()

        manifest = Manifest(
            handler="IntroHandler",
            source_file=input_path.name,
            metadata=metadata,
        )
        manifest.to_json(output_dir / "manifest.json")
        return manifest

    def repack(self, manifest: Manifest, input_dir: Path, output_path: Path) -> None:
        from PIL import Image

        img = Image.open(input_dir / "intro.png")
        pixels = image_to_pixels(img)
        width: int = manifest.metadata["width"]
        height: int = manifest.metadata["height"]
        planar = encode_planar(pixels, width, height)

        compressed = compress_rle(planar)
        decomp_size: int = manifest.metadata["decomp_size"]
        header = struct.pack("<I", decomp_size)

        trailing_b64: str = manifest.metadata.get("trailing", "")
        trailing = base64.b64decode(trailing_b64) if trailing_b64 else b""

        output_path.write_bytes(header + compressed + trailing)
