from __future__ import annotations

import json
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any, ClassVar

if TYPE_CHECKING:
    from pathlib import Path


@dataclass
class Manifest:
    handler: str
    source_file: str
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_json(self, path: Path) -> None:
        data = {
            "handler": self.handler,
            "source_file": self.source_file,
            "metadata": self.metadata,
        }
        path.write_text(json.dumps(data, indent=2))

    @classmethod
    def from_json(cls, path: Path) -> Manifest:
        data: dict[str, Any] = json.loads(path.read_text())
        for key in ("handler", "source_file"):
            if key not in data:
                msg = f"Missing required key {key!r} in {path}"
                raise ValueError(msg)
        return cls(
            handler=data["handler"],
            source_file=data["source_file"],
            metadata=data.get("metadata", {}),
        )


class FormatHandler(ABC):
    """Abstract base class for .DD2 format handlers."""

    file_patterns: ClassVar[list[str]]

    @abstractmethod
    def unpack(self, input_path: Path, translatable_dir: Path, meta_dir: Path) -> Manifest:
        """Unpack a game file into editable formats.

        Translatable files (PNGs, JSON for editing) go to translatable_dir.
        Metadata files (manifests, binary blobs) go to meta_dir.
        """
        ...

    @abstractmethod
    def repack(
        self, manifest: Manifest, translatable_dir: Path, meta_dir: Path, output_path: Path
    ) -> None:
        """Repack edited files back into a game file."""
        ...
