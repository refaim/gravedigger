"""PicHandler — title/progress screen images (HUFF + PIC + EGA).

PIC file format (after HUFF decompression):
  - 4 bytes: "PIC\\x00" signature
  - 2 bytes: uint16 LE width_bytes (pixel width / 8)
  - 2 bytes: uint16 LE height
  - remaining: 4 EGA bitplanes of (width_bytes * height) bytes each
"""

from __future__ import annotations

import struct
from typing import TYPE_CHECKING, ClassVar

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

_PIC_SIGNATURE = b"PIC\x00"
_PIC_HEADER_SIZE = 8  # 4 bytes sig + 2 bytes width_bytes + 2 bytes height


class PicHandler(FormatHandler):
    file_patterns: ClassVar[list[str]] = ["TITLE1.DD2", "TITLE2.DD2", "PROGPIC.DD2", "STARPIC.DD2"]

    def unpack(self, input_path: Path, output_dir: Path) -> Manifest:
        raw = input_path.read_bytes()
        decompressed, tree = decompress(raw)

        sig = decompressed[:4]
        if sig != _PIC_SIGNATURE:
            msg = f"Expected PIC signature, got {sig!r}"
            raise ValueError(msg)

        width_bytes, height = struct.unpack_from("<HH", decompressed, 4)
        width = width_bytes * 8

        ega_data = decompressed[_PIC_HEADER_SIZE:]
        pixels = decode_planar(ega_data, width, height)
        img = pixels_to_image(pixels, width, height)

        image_name = input_path.stem.lower() + ".png"
        img.save(output_dir / image_name)

        tree_name = input_path.stem.lower() + ".tree"
        (output_dir / tree_name).write_bytes(tree)

        manifest = Manifest(
            handler="PicHandler",
            source_file=input_path.name,
            metadata={
                "image": image_name,
                "tree": tree_name,
                "width": width,
                "height": height,
            },
        )
        manifest.to_json(output_dir / "manifest.json")
        return manifest

    def repack(self, manifest: Manifest, input_dir: Path, output_path: Path) -> None:
        meta = manifest.metadata
        width: int = meta["width"]
        height: int = meta["height"]

        img_path = input_dir / meta["image"]
        from PIL import Image

        img = Image.open(img_path)
        pixels = image_to_pixels(img)

        ega_data = encode_planar(pixels, width, height)

        width_bytes = width // 8
        pic_header = _PIC_SIGNATURE + struct.pack("<HH", width_bytes, height)
        decompressed = pic_header + ega_data

        tree = (input_dir / meta["tree"]).read_bytes()
        compressed = compress(decompressed, tree)

        output_path.write_bytes(compressed)
