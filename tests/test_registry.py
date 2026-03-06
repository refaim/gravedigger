from pathlib import Path
from typing import ClassVar

import pytest

from gravedigger.core.handler import FormatHandler, Manifest
from gravedigger.core.registry import Registry


class FakeHandler(FormatHandler):
    file_patterns: ClassVar[list[str]] = ["TITLE*.DD2", "PROGPIC.DD2", "STARPIC.DD2"]

    def unpack(self, input_path: Path, translatable_dir: Path, meta_dir: Path) -> Manifest:
        return Manifest(handler="FakeHandler", source_file=input_path.name)

    def repack(
        self, manifest: Manifest, translatable_dir: Path, meta_dir: Path, output_path: Path
    ) -> None:
        pass


class AnotherHandler(FormatHandler):
    file_patterns: ClassVar[list[str]] = ["LEVEL*.DD2"]

    def unpack(self, input_path: Path, translatable_dir: Path, meta_dir: Path) -> Manifest:
        return Manifest(handler="AnotherHandler", source_file=input_path.name)

    def repack(
        self, manifest: Manifest, translatable_dir: Path, meta_dir: Path, output_path: Path
    ) -> None:
        pass


class TestRegistry:
    def test_register_and_get(self) -> None:
        reg = Registry()
        reg.register(FakeHandler())
        handler = reg.get_handler("TITLE1.DD2")
        assert isinstance(handler, FakeHandler)

    def test_glob_pattern_match(self) -> None:
        reg = Registry()
        reg.register(FakeHandler())
        assert isinstance(reg.get_handler("TITLE2.DD2"), FakeHandler)
        assert isinstance(reg.get_handler("PROGPIC.DD2"), FakeHandler)
        assert isinstance(reg.get_handler("STARPIC.DD2"), FakeHandler)

    def test_unknown_file_raises(self) -> None:
        reg = Registry()
        reg.register(FakeHandler())
        with pytest.raises(KeyError):
            reg.get_handler("UNKNOWN.DD2")

    def test_multiple_handlers(self) -> None:
        reg = Registry()
        reg.register(FakeHandler())
        reg.register(AnotherHandler())
        assert isinstance(reg.get_handler("TITLE1.DD2"), FakeHandler)
        assert isinstance(reg.get_handler("LEVEL01.DD2"), AnotherHandler)

    def test_case_insensitive(self) -> None:
        reg = Registry()
        reg.register(FakeHandler())
        assert isinstance(reg.get_handler("title1.dd2"), FakeHandler)
