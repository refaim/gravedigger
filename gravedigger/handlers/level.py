"""Handler for LEVEL*.DD2 files — tile maps and object maps with RLEW compression."""

from __future__ import annotations

import json
import struct
from typing import TYPE_CHECKING, ClassVar

from gravedigger.compression.rlew import compress, decompress
from gravedigger.core.handler import FormatHandler, Manifest

if TYPE_CHECKING:
    from pathlib import Path

HEADER_SIZE = 32
MSDOS_TRAILER = b"MsDos"


class LevelHandler(FormatHandler):
    """Handler for LEVEL*.DD2 — RLEW-compressed level data."""

    file_patterns: ClassVar[list[str]] = [f"LEVEL{i:02d}.DD2" for i in range(1, 9)]

    def unpack(self, input_path: Path, translatable_dir: Path, meta_dir: Path) -> Manifest:
        raw = input_path.read_bytes()

        # File format: 4-byte decompressed size + RLEW data + "MsDos" trailer
        if raw[-len(MSDOS_TRAILER) :] != MSDOS_TRAILER:
            msg = f"Expected MsDos trailer, got {raw[-len(MSDOS_TRAILER) :]!r}"
            raise ValueError(msg)
        rlew_data = raw[4 : -len(MSDOS_TRAILER)]
        level = decompress(rlew_data)

        width, height = struct.unpack_from("<HH", level, 0)
        header = list(level[:HEADER_SIZE])

        num_tiles = width * height
        tile_offset = HEADER_SIZE
        obj_offset = tile_offset + num_tiles * 2

        tile_map = list(struct.unpack_from(f"<{num_tiles}H", level, tile_offset))
        object_map = list(struct.unpack_from(f"<{num_tiles}H", level, obj_offset))

        # Preserve any trailing bytes after the object map
        trailing_offset = obj_offset + num_tiles * 2
        trailing = list(level[trailing_offset:])

        data: dict[str, object] = {
            "width": width,
            "height": height,
            "header": header,
            "tile_map": tile_map,
            "object_map": object_map,
        }
        if trailing:
            data["trailing"] = trailing
        json_path = translatable_dir / "level.json"
        json_path.write_text(json.dumps(data, indent=2))

        manifest = Manifest(
            handler="LevelHandler",
            source_file=input_path.name,
            metadata={"width": width, "height": height},
        )
        manifest.to_json(meta_dir / "manifest.json")
        return manifest

    def repack(
        self, manifest: Manifest, translatable_dir: Path, meta_dir: Path, output_path: Path
    ) -> None:
        data = json.loads((translatable_dir / "level.json").read_text())

        header = bytes(data["header"])
        tile_map = struct.pack(f"<{len(data['tile_map'])}H", *data["tile_map"])
        object_map = struct.pack(f"<{len(data['object_map'])}H", *data["object_map"])
        trailing = bytes(data.get("trailing", []))

        level = header + tile_map + object_map + trailing

        rlew_data = compress(level)
        size_header = struct.pack("<I", len(level))
        output_path.write_bytes(size_header + rlew_data + MSDOS_TRAILER)
