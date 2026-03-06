"""SpriteHandler — sprite files (HUFF + EGA).

Sprite .DD2 files are HUFF-compressed. After decompression the layout is:
  - 2 bytes: uint16 LE plane_stride (size of one bitplane block for all sprites)
  - 5 * plane_stride bytes: five bitplane blocks (4 EGA color planes + 1 mask plane)

Within each plane block, the first 8 bytes are a header/unused area.
Sprite pixel data starts at offset 8 within each plane block.
Sprites are packed sequentially, each contributing (width * height / 8) bytes per plane.
After all sprites there may be trailing padding to fill the plane_stride.
"""

from __future__ import annotations

import base64
import struct
from typing import TYPE_CHECKING, Any, ClassVar

from gravedigger.compression.ega import (
    decode_planar,
    encode_planar,
    image_to_pixels,
    pixels_to_image,
)
from gravedigger.compression.huff import compress, decompress
from gravedigger.core.handler import FormatHandler, Manifest

if TYPE_CHECKING:
    from pathlib import Path

# Hardcoded sprite sizes per file, from the reference implementation.
SPRITE_SIZES: dict[str, list[tuple[int, int]]] = {
    "S_DAVE.DD2": [
        (24, 32),  # standing right
        (24, 32),
        (24, 32),
        (24, 32),
        (24, 32),  # running right
        (24, 32),
        (24, 32),
        (24, 32),  # jumping right
        (24, 32),  # standing left
        (24, 32),
        (24, 32),
        (24, 32),
        (24, 32),  # running left
        (24, 32),
        (24, 32),
        (24, 32),  # jumping left
        (24, 32),
        (24, 32),  # reload
        (40, 16),
        (40, 16),
        (40, 16),
        (40, 16),
        (40, 16),
        (40, 16),
        (40, 16),
        (40, 16),
        (40, 16),  # ammo
        (32, 32),
        (32, 32),
        (24, 32),
        (24, 32),
        (24, 32),
        (24, 32),  # aim right
        (32, 32),
        (32, 32),
        (24, 32),
        (24, 32),
        (24, 32),
        (24, 32),  # aim left
        (24, 32),
        (24, 32),
        (24, 32),
        (24, 32),  # leaving
        (16, 8),
        (16, 8),
        (16, 8),
        (16, 8),
        (16, 8),
        (16, 8),
        (16, 8),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 8),
        (16, 8),
        (16, 8),
        (16, 8),
        (16, 8),
        (24, 16),  # 1UP
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),  # chunks
        (16, 16),
        (16, 16),
        (16, 16),
        (16, 16),  # chunks
    ],
    "S_CHUNK1.DD2": [
        (24, 40),
        (24, 40),
        (24, 40),
        (24, 40),  # walk
        (32, 40),
        (24, 40),
        (40, 40),  # hit
        (24, 40),
        (24, 40),
        (24, 40),
        (24, 40),  # walk
        (32, 40),
        (24, 40),
        (40, 40),  # hit
        (24, 40),
        (24, 40),  # climb
        (8, 16),  # alignment
        (24, 24),
        (24, 24),
        (24, 24),
        (24, 24),  # walk
        (24, 24),
        (24, 24),
        (24, 24),
        (24, 24),  # walk
        (32, 24),
        (32, 24),  # throw
        (40, 24),
        (32, 24),
        (32, 24),
        (32, 24),  # attack
    ],
    "S_CHUNK2.DD2": [
        (24, 32),
        (24, 32),
        (24, 32),
        (24, 32),
        (24, 32),
        (24, 32),
        (24, 32),
        (8, 16),  # alignment
        (24, 8),
        (24, 8),
        (24, 8),
        (24, 8),
        (24, 8),
        (24, 8),
        (24, 8),
        (24, 8),
        (24, 16),
        (24, 16),
        (24, 16),
        (24, 16),
        (24, 16),
        (24, 16),
        (24, 16),
        (24, 16),
        (8, 16),  # alignment
        (32, 8),
        (32, 8),
        (32, 8),
        (32, 8),
        (16, 24),
        (8, 8),
        (8, 16),  # alignment
        (24, 16),
        (24, 16),
        (24, 16),
        (24, 16),  # flame
        (8, 16),  # alignment
        (32, 32),
        (32, 32),
        (32, 32),
        (32, 32),
        (40, 24),
        (32, 32),
        (32, 32),
        (32, 32),
        (32, 32),
    ],
    "S_FRANK.DD2": [
        (56, 80),
        (32, 80),
        (56, 80),
        (32, 80),
        (56, 80),
        (32, 80),
        (56, 80),
        (32, 80),
        (16, 16),
        (16, 16),
    ],
    "S_MASTER.DD2": [
        (72, 80),
        (72, 80),
        (24, 24),
        (24, 24),
        (24, 24),
        (24, 24),
        (64, 56),
        (64, 56),
        (64, 56),
        (104, 48),
    ],
}

_STRIDE_FIELD = 2  # uint16 LE at start
_PLANE_HEADER = 8  # first 8 bytes of plane 0 are unused header
_NUM_PLANES = 5  # 4 EGA color planes + 1 mask plane


class SpriteHandler(FormatHandler):
    file_patterns: ClassVar[list[str]] = [
        "S_DAVE.DD2",
        "S_CHUNK1.DD2",
        "S_CHUNK2.DD2",
        "S_FRANK.DD2",
        "S_MASTER.DD2",
    ]

    def unpack(self, input_path: Path, translatable_dir: Path, meta_dir: Path) -> Manifest:
        filename = input_path.name
        if filename not in SPRITE_SIZES:
            msg = f"Unknown sprite file: {filename!r}"
            raise ValueError(msg)

        raw = input_path.read_bytes()
        decompressed, huff_tree = decompress(raw)

        plane_stride = struct.unpack_from("<H", decompressed, 0)[0]

        # Save the entire decompressed blob for byte-exact roundtrip.
        # We only modify the sprite pixel data within the 4 color planes.
        blob = bytearray(decompressed[_STRIDE_FIELD:])

        sizes = SPRITE_SIZES[filename]

        # Derive prefix from source filename: S_DAVE.DD2 -> dave, S_CHUNK1.DD2 -> chunk1
        prefix = input_path.stem.lower().removeprefix("s_")

        sprites_dir = translatable_dir / "sprites"
        sprites_dir.mkdir(parents=True, exist_ok=True)

        sprites_meta: list[dict[str, Any]] = []
        offset = _PLANE_HEADER  # sprite data starts 8 bytes into each plane

        for i, (w, h) in enumerate(sizes):
            plane_chunk = (w * h) >> 3
            # Gather 4 color planes
            planes = b""
            for p in range(4):
                start = plane_stride * p + offset
                planes += blob[start : start + plane_chunk]

            pixels = decode_planar(planes, w, h)
            img = pixels_to_image(pixels, w, h)
            img.save(sprites_dir / f"{prefix}_{i:04d}.png")

            sprites_meta.append({"width": w, "height": h})
            offset += plane_chunk

        metadata: dict[str, Any] = {
            "prefix": prefix,
            "sprites": sprites_meta,
            "huff_tree": base64.b64encode(huff_tree).decode(),
            "blob": base64.b64encode(bytes(blob)).decode(),
        }

        manifest = Manifest(
            handler="SpriteHandler",
            source_file=filename,
            metadata=metadata,
        )
        manifest.to_json(meta_dir / "manifest.json")
        return manifest

    def repack(
        self, manifest: Manifest, translatable_dir: Path, meta_dir: Path, output_path: Path
    ) -> None:
        from PIL import Image

        sprites_meta: list[dict[str, int]] = manifest.metadata["sprites"]
        prefix: str = manifest.metadata["prefix"]
        huff_tree = base64.b64decode(manifest.metadata["huff_tree"])
        blob = bytearray(base64.b64decode(manifest.metadata["blob"]))

        plane_stride = len(blob) // _NUM_PLANES

        sprites_dir = translatable_dir / "sprites"

        # Write modified sprite pixels back into the blob
        offset = _PLANE_HEADER
        for i, sp in enumerate(sprites_meta):
            w, h = sp["width"], sp["height"]
            img = Image.open(sprites_dir / f"{prefix}_{i:04d}.png")
            pixels = image_to_pixels(img)
            planar = encode_planar(pixels, w, h)
            plane_chunk = len(planar) // 4

            for p in range(4):
                start = plane_stride * p + offset
                blob[start : start + plane_chunk] = planar[p * plane_chunk : (p + 1) * plane_chunk]
            offset += plane_chunk

        decompressed = struct.pack("<H", plane_stride) + bytes(blob)
        output_path.write_bytes(compress(decompressed, huff_tree))
