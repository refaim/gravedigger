import json
from pathlib import Path
from typing import Any, ClassVar

import pytest

from gravedigger.core.handler import FormatHandler, Manifest


class TestFormatHandlerABC:
    def test_cannot_instantiate(self) -> None:
        with pytest.raises(TypeError):
            FormatHandler()  # type: ignore[abstract]

    def test_concrete_subclass(self, tmp_path: Path) -> None:
        class FakeHandler(FormatHandler):
            file_patterns: ClassVar[list[str]] = ["FAKE*.DD2"]

            def unpack(self, input_path: Path, output_dir: Path) -> Manifest:
                return Manifest(
                    handler="FakeHandler",
                    source_file=input_path.name,
                )

            def repack(self, manifest: Manifest, input_dir: Path, output_path: Path) -> None:
                pass

        handler = FakeHandler()
        manifest = handler.unpack(tmp_path / "FAKE1.DD2", tmp_path / "out")
        assert manifest.handler == "FakeHandler"
        assert manifest.source_file == "FAKE1.DD2"


class TestManifest:
    def test_create(self) -> None:
        m = Manifest(handler="PicHandler", source_file="TITLE1.DD2")
        assert m.handler == "PicHandler"
        assert m.source_file == "TITLE1.DD2"
        assert m.metadata == {}

    def test_with_metadata(self) -> None:
        m = Manifest(
            handler="TileHandler",
            source_file="EGATILES.DD2",
            metadata={"total_tiles": 858},
        )
        assert m.metadata["total_tiles"] == 858

    def test_to_json(self, tmp_path: Path) -> None:
        m = Manifest(
            handler="PicHandler",
            source_file="TITLE1.DD2",
            metadata={"width": 320, "height": 200},
        )
        path = tmp_path / "manifest.json"
        m.to_json(path)
        assert path.exists()
        data: dict[str, Any] = json.loads(path.read_text())
        assert data["handler"] == "PicHandler"
        assert data["source_file"] == "TITLE1.DD2"
        assert data["metadata"]["width"] == 320

    def test_from_json(self, tmp_path: Path) -> None:
        path = tmp_path / "manifest.json"
        path.write_text(
            json.dumps(
                {
                    "handler": "LevelHandler",
                    "source_file": "LEVEL01.DD2",
                    "metadata": {"width": 64, "height": 57},
                }
            )
        )
        m = Manifest.from_json(path)
        assert m.handler == "LevelHandler"
        assert m.source_file == "LEVEL01.DD2"
        assert m.metadata["width"] == 64

    def test_from_json_missing_key(self, tmp_path: Path) -> None:
        path = tmp_path / "bad.json"
        path.write_text('{"handler": "test"}')
        with pytest.raises(ValueError, match="Missing required key"):
            Manifest.from_json(path)

    def test_roundtrip(self, tmp_path: Path) -> None:
        original = Manifest(
            handler="SpriteHandler",
            source_file="S_DAVE.DD2",
            metadata={"sprites": [{"w": 24, "h": 32}], "huff_tree": "AQID"},
        )
        path = tmp_path / "manifest.json"
        original.to_json(path)
        loaded = Manifest.from_json(path)
        assert loaded.handler == original.handler
        assert loaded.source_file == original.source_file
        assert loaded.metadata == original.metadata
