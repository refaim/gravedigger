from __future__ import annotations

import base64
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

TILE_W = 16
TILE_H = 16
TILE_BYTES = 128  # 4 planes * (16*16/8)


class TileHandler(FormatHandler):
    file_patterns: ClassVar[list[str]] = ["EGATILES.DD2"]

    def unpack(self, input_path: Path, output_dir: Path) -> Manifest:
        data = input_path.read_bytes()
        total_tiles = len(data) // TILE_BYTES
        trailing = data[total_tiles * TILE_BYTES :]

        for i in range(total_tiles):
            tile_data = data[i * TILE_BYTES : (i + 1) * TILE_BYTES]
            pixels = decode_planar(tile_data, TILE_W, TILE_H)
            img = pixels_to_image(pixels, TILE_W, TILE_H)
            img.save(output_dir / f"tile_{i:04d}.png")

        metadata: dict[str, object] = {"total_tiles": total_tiles}
        if trailing:
            metadata["trailing"] = base64.b64encode(trailing).decode()

        manifest = Manifest(
            handler="TileHandler",
            source_file=input_path.name,
            metadata=metadata,
        )
        manifest.to_json(output_dir / "manifest.json")
        return manifest

    def repack(self, manifest: Manifest, input_dir: Path, output_path: Path) -> None:
        from PIL import Image

        total_tiles: int = manifest.metadata["total_tiles"]
        parts: list[bytes] = []

        for i in range(total_tiles):
            img = Image.open(input_dir / f"tile_{i:04d}.png")
            pixels = image_to_pixels(img)
            parts.append(encode_planar(pixels, TILE_W, TILE_H))

        trailing_b64: str = manifest.metadata.get("trailing", "")
        if trailing_b64:
            parts.append(base64.b64decode(trailing_b64))

        output_path.write_bytes(b"".join(parts))
