from __future__ import annotations

import json
import struct
from typing import TYPE_CHECKING, ClassVar

from gravedigger.core.handler import FormatHandler, Manifest

if TYPE_CHECKING:
    from pathlib import Path

NUM_KEYS = 10
EXTRA_SIZE = 4
EXPECTED_SIZE = NUM_KEYS + EXTRA_SIZE


class CtlPanelHandler(FormatHandler):
    """Handler for CTLPANEL.DD2 — keyboard scan code mappings and settings."""

    file_patterns: ClassVar[list[str]] = ["CTLPANEL.DD2"]

    def unpack(self, input_path: Path, translatable_dir: Path, meta_dir: Path) -> Manifest:
        raw = input_path.read_bytes()
        if len(raw) != EXPECTED_SIZE:
            msg = f"Expected {EXPECTED_SIZE} bytes, got {len(raw)}"
            raise ValueError(msg)

        keys = list(raw[:NUM_KEYS])
        (extra,) = struct.unpack_from("<I", raw, NUM_KEYS)

        data = {"keys": keys, "extra": extra}
        json_path = translatable_dir / "ctlpanel.json"
        json_path.write_text(json.dumps(data, indent=2))

        manifest = Manifest(
            handler="CtlPanelHandler",
            source_file=input_path.name,
        )
        manifest.to_json(meta_dir / "manifest.json")
        return manifest

    def repack(
        self, manifest: Manifest, translatable_dir: Path, meta_dir: Path, output_path: Path
    ) -> None:
        json_path = translatable_dir / "ctlpanel.json"
        data = json.loads(json_path.read_text())

        keys = bytes(data["keys"])
        extra = struct.pack("<I", data["extra"])
        output_path.write_bytes(keys + extra)
