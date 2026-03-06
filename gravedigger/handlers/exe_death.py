"""ExeDeathHandler — death sequences from EXE.

Death sequence data in the decompressed EXE at code offset 0x18A60:
  - 8 death animations x 5 frames = 40 frames total
  - Each frame is 48x48 pixels, EGA 4-plane interleaved
  - Planes are spaced by MAGIC (0x33B0) bytes apart
  - Each frame occupies row_bytes * height = 6 * 48 = 288 bytes per plane
  - Frames advance sequentially within each plane
  - Total block size: MAGIC * 4 = 52928 bytes
"""

from __future__ import annotations

import base64
import struct
from typing import TYPE_CHECKING, Any, ClassVar

from gravedigger.compression.ega import (
    image_to_pixels,
    pixels_to_image,
)
from gravedigger.compression.pklite import compress, decompress
from gravedigger.core.handler import FormatHandler, Manifest

if TYPE_CHECKING:
    from pathlib import Path

_DEATH_CODE_OFFSET = 0x18A60
_MAGIC = 0x33B0  # plane stride (distance between bitplanes)
_SEQUENCE_COUNT = 8
_FRAMES_PER_SEQUENCE = 5
_TOTAL_FRAMES = _SEQUENCE_COUNT * _FRAMES_PER_SEQUENCE
_FRAME_WIDTH = 48
_FRAME_HEIGHT = 48
_ROW_BYTES = _FRAME_WIDTH // 8  # 6
_FRAME_PLANE_SIZE = _ROW_BYTES * _FRAME_HEIGHT  # 288 bytes per frame per plane
_BLOCK_SIZE = _MAGIC * 4  # total death sequence block


def _decode_frame(block: bytes, frame_index: int) -> list[int]:
    """Decode a single 48x48 EGA frame from the death sequence block."""
    offset = frame_index * _FRAME_PLANE_SIZE
    pixels = [0] * (_FRAME_WIDTH * _FRAME_HEIGHT)

    for y in range(_FRAME_HEIGHT):
        for x in range(_FRAME_WIDTH):
            byte_offset = (x >> 3) + y * _ROW_BYTES + offset
            bit = 7 - (x & 7)

            c0 = (block[byte_offset + _MAGIC * 0] >> bit) & 1
            c1 = (block[byte_offset + _MAGIC * 1] >> bit) & 1
            c2 = (block[byte_offset + _MAGIC * 2] >> bit) & 1
            c3 = (block[byte_offset + _MAGIC * 3] >> bit) & 1

            pixels[y * _FRAME_WIDTH + x] = c0 | (c1 << 1) | (c2 << 2) | (c3 << 3)

    return pixels


def _encode_frame(pixels: list[int], block: bytearray, frame_index: int) -> None:
    """Encode a single 48x48 EGA frame into the death sequence block."""
    offset = frame_index * _FRAME_PLANE_SIZE

    for y in range(_FRAME_HEIGHT):
        for x in range(_FRAME_WIDTH):
            color = pixels[y * _FRAME_WIDTH + x]
            byte_offset = (x >> 3) + y * _ROW_BYTES + offset
            bit = 7 - (x & 7)

            for plane in range(4):
                if (color >> plane) & 1:
                    block[byte_offset + _MAGIC * plane] |= 1 << bit


class ExeDeathHandler(FormatHandler):
    """Handler for death sequence animations embedded in the game EXE."""

    file_patterns: ClassVar[list[str]] = ["DAVE.EXE", "1.EXE"]

    def unpack(self, input_path: Path, translatable_dir: Path, meta_dir: Path) -> Manifest:
        exe_data = input_path.read_bytes()
        decompressed = decompress(exe_data)

        header_para = struct.unpack_from("<H", decompressed, 8)[0]
        code_start = header_para * 16
        death_offset = code_start + _DEATH_CODE_OFFSET

        block = decompressed[death_offset : death_offset + _BLOCK_SIZE]

        death_dir = translatable_dir / "death"
        death_dir.mkdir(parents=True, exist_ok=True)

        for seq in range(_SEQUENCE_COUNT):
            for frame in range(_FRAMES_PER_SEQUENCE):
                frame_index = seq * _FRAMES_PER_SEQUENCE + frame
                pixels = _decode_frame(block, frame_index)
                img = pixels_to_image(pixels, _FRAME_WIDTH, _FRAME_HEIGHT)
                img.save(death_dir / f"seq{seq + 1}_frame{frame + 1}.png")

        metadata: dict[str, Any] = {
            "sequence_count": _SEQUENCE_COUNT,
            "frames_per_sequence": _FRAMES_PER_SEQUENCE,
            "frame_width": _FRAME_WIDTH,
            "frame_height": _FRAME_HEIGHT,
            "original_exe": base64.b64encode(exe_data).decode(),
        }

        manifest = Manifest(
            handler="ExeDeathHandler",
            source_file=input_path.name,
            metadata=metadata,
        )
        manifest.to_json(meta_dir / "manifest.json")
        return manifest

    def repack(
        self, manifest: Manifest, translatable_dir: Path, meta_dir: Path, output_path: Path
    ) -> None:
        from PIL import Image

        meta = manifest.metadata
        original_exe = base64.b64decode(meta["original_exe"])
        decompressed = decompress(original_exe)

        header_para = struct.unpack_from("<H", decompressed, 8)[0]
        code_start = header_para * 16
        death_offset = code_start + _DEATH_CODE_OFFSET

        # Start from original block to preserve any padding/extra data
        block = bytearray(decompressed[death_offset : death_offset + _BLOCK_SIZE])

        # Zero out the frame data regions in each plane, then re-encode
        frame_data_total = _TOTAL_FRAMES * _FRAME_PLANE_SIZE
        for plane in range(4):
            plane_start = _MAGIC * plane
            block[plane_start : plane_start + frame_data_total] = b"\x00" * frame_data_total

        death_dir = translatable_dir / "death"
        for seq in range(_SEQUENCE_COUNT):
            for frame in range(_FRAMES_PER_SEQUENCE):
                frame_index = seq * _FRAMES_PER_SEQUENCE + frame
                img = Image.open(death_dir / f"seq{seq + 1}_frame{frame + 1}.png")
                pixels = image_to_pixels(img)
                _encode_frame(pixels, block, frame_index)

        # Patch the death sequence block into the decompressed EXE
        patched = bytearray(decompressed)
        patched[death_offset : death_offset + _BLOCK_SIZE] = block

        # Recompress with PKLITE
        result = compress(bytes(patched), original_exe)
        output_path.write_bytes(result)
