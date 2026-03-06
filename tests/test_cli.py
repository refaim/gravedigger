from __future__ import annotations

import subprocess
import sys
from typing import TYPE_CHECKING
from unittest.mock import patch

import pytest

from gravedigger.cli import main

if TYPE_CHECKING:
    from pathlib import Path


def run_cli(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, "-m", "gravedigger.cli", *args],
        capture_output=True,
        text=True,
    )


class TestHelp:
    def test_help_flag(self) -> None:
        result = run_cli("--help")
        assert result.returncode == 0
        assert "unpack" in result.stdout
        assert "repack" in result.stdout

    def test_unpack_help(self) -> None:
        result = run_cli("unpack", "--help")
        assert result.returncode == 0

    def test_repack_help(self) -> None:
        result = run_cli("repack", "--help")
        assert result.returncode == 0

    def test_no_subcommand_exits(self) -> None:
        with patch("sys.argv", ["gravedigger"]), pytest.raises(SystemExit) as exc:
            main()
        assert exc.value.code != 0


class TestUnpack:
    def test_unpack_creates_output(self, game_dir: Path, tmp_output: Path) -> None:
        with patch("sys.argv", ["gravedigger", "unpack", str(game_dir), str(tmp_output)]):
            main()
        assert (tmp_output / "translatable").is_dir()
        assert (tmp_output / "meta").is_dir()

    def test_unpack_creates_manifests(self, game_dir: Path, tmp_output: Path) -> None:
        with patch("sys.argv", ["gravedigger", "unpack", str(game_dir), str(tmp_output)]):
            main()
        manifests = list((tmp_output / "meta").rglob("manifest.json"))
        assert len(manifests) >= 1

    def test_unpack_nonexistent_dir(self, tmp_path: Path) -> None:
        with (
            patch(
                "sys.argv",
                ["gravedigger", "unpack", str(tmp_path / "nonexistent"), str(tmp_path / "out")],
            ),
            pytest.raises(SystemExit) as exc,
        ):
            main()
        assert exc.value.code != 0

    def test_unpack_empty_dir(self, tmp_path: Path) -> None:
        empty = tmp_path / "empty"
        empty.mkdir()
        with (
            patch("sys.argv", ["gravedigger", "unpack", str(empty), str(tmp_path / "out")]),
            pytest.raises(SystemExit) as exc,
        ):
            main()
        assert exc.value.code != 0

    def test_unpack_skips_unknown_files(self, tmp_path: Path) -> None:
        """DD2 file with no matching handler is copied to originals, not errored."""
        game = tmp_path / "game"
        game.mkdir()
        (game / "UNKNOWN.DD2").write_bytes(b"\x00" * 64)
        out = tmp_path / "out"
        with patch("sys.argv", ["gravedigger", "unpack", str(game), str(out)]):
            main()
        # Unknown files go to meta/ directly
        assert (out / "meta" / "UNKNOWN.DD2").exists()


class TestRepack:
    def test_repack_creates_dd2_files(self, game_dir: Path, tmp_path: Path) -> None:
        unpack_dir = tmp_path / "unpacked"
        repack_dir = tmp_path / "repacked"

        with patch("sys.argv", ["gravedigger", "unpack", str(game_dir), str(unpack_dir)]):
            main()
        with patch("sys.argv", ["gravedigger", "repack", str(unpack_dir), str(repack_dir)]):
            main()

        dd2_files = list(repack_dir.glob("*.DD2"))
        assert len(dd2_files) > 0

    def test_repack_byte_exact(self, game_dir: Path, tmp_path: Path) -> None:
        unpack_dir = tmp_path / "unpacked"
        repack_dir = tmp_path / "repacked"

        with patch("sys.argv", ["gravedigger", "unpack", str(game_dir), str(unpack_dir)]):
            main()
        with patch("sys.argv", ["gravedigger", "repack", str(unpack_dir), str(repack_dir)]):
            main()

        for original in game_dir.glob("*.DD2"):
            repacked = repack_dir / original.name
            assert repacked.exists(), f"Missing repacked file: {original.name}"
            assert repacked.read_bytes() == original.read_bytes(), (
                f"Byte mismatch: {original.name}"
            )

    def test_repack_exe_byte_exact(self, game_dir: Path, tmp_path: Path) -> None:
        unpack_dir = tmp_path / "unpacked"
        repack_dir = tmp_path / "repacked"

        with patch("sys.argv", ["gravedigger", "unpack", str(game_dir), str(unpack_dir)]):
            main()
        with patch("sys.argv", ["gravedigger", "repack", str(unpack_dir), str(repack_dir)]):
            main()

        for original in game_dir.glob("*.EXE"):
            repacked = repack_dir / original.name
            assert repacked.exists(), f"Missing repacked EXE: {original.name}"
            assert repacked.read_bytes() == original.read_bytes(), (
                f"EXE byte mismatch: {original.name}"
            )

    def test_repack_nonexistent_dir(self, tmp_path: Path) -> None:
        with (
            patch(
                "sys.argv",
                ["gravedigger", "repack", str(tmp_path / "nonexistent"), str(tmp_path / "out")],
            ),
            pytest.raises(SystemExit) as exc,
        ):
            main()
        assert exc.value.code != 0

    def test_repack_empty_dir(self, tmp_path: Path) -> None:
        empty = tmp_path / "empty"
        empty.mkdir()
        with (
            patch("sys.argv", ["gravedigger", "repack", str(empty), str(tmp_path / "out")]),
            pytest.raises(SystemExit) as exc,
        ):
            main()
        assert exc.value.code != 0

    def test_repack_empty_meta_dir(self, tmp_path: Path) -> None:
        """meta/ exists but has no manifests or originals."""
        base = tmp_path / "unpacked"
        (base / "meta").mkdir(parents=True)
        with (
            patch("sys.argv", ["gravedigger", "repack", str(base), str(tmp_path / "out")]),
            pytest.raises(SystemExit) as exc,
        ):
            main()
        assert exc.value.code != 0

    def test_repack_skips_unknown_handler(self, tmp_path: Path) -> None:
        """Manifest referencing an unknown handler is skipped gracefully."""
        import json

        meta_dir = tmp_path / "unpacked" / "meta" / "UNKNOWN"
        meta_dir.mkdir(parents=True)
        manifest = {
            "handler": "FakeHandler",
            "source_file": "UNKNOWN.DD2",
            "metadata": {},
        }
        (meta_dir / "manifest.json").write_text(json.dumps(manifest))

        out = tmp_path / "out"
        with patch("sys.argv", ["gravedigger", "repack", str(tmp_path / "unpacked"), str(out)]):
            main()
        # Should not create the file, but should not error either
        assert not (out / "UNKNOWN.DD2").exists()


class TestEntryPoint:
    def test_module_runnable(self) -> None:
        result = run_cli("--help")
        assert result.returncode == 0

    def test_main_module(self) -> None:
        result = subprocess.run(
            [sys.executable, "-m", "gravedigger", "--help"],
            capture_output=True,
            text=True,
        )
        assert result.returncode == 0
        assert "unpack" in result.stdout

    def test_dunder_main(self) -> None:
        """Exercise __main__.py in-process via runpy."""
        import runpy

        with (
            patch("sys.argv", ["gravedigger", "--help"]),
            pytest.raises(SystemExit) as exc,
        ):
            runpy.run_module("gravedigger.__main__", run_name="__main__")
        assert exc.value.code == 0

    @pytest.mark.filterwarnings("ignore::RuntimeWarning")
    def test_cli_as_script(self) -> None:
        """Exercise cli.py __name__ == '__main__' guard."""
        import runpy

        with (
            patch("sys.argv", ["gravedigger.cli", "--help"]),
            pytest.raises(SystemExit) as exc,
        ):
            runpy.run_module("gravedigger.cli", run_name="__main__")
        assert exc.value.code == 0
