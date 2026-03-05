"""Integration roundtrip tests: unpack -> repack -> byte-exact compare for every game file."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

import pytest

from gravedigger.cli import _build_registry
from gravedigger.core.handler import Manifest


def _all_dd2_files() -> list[str]:
    game = Path(__file__).resolve().parent.parent / "game"
    if not game.is_dir():
        return []
    return sorted(p.name for p in game.glob("*.DD2"))


ALL_DD2 = _all_dd2_files()


# ---------------------------------------------------------------------------
# Per-file handler-level roundtrip
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("filename", ALL_DD2)
class TestHandlerRoundtrip:
    """Unpack then repack each DD2 file via its handler and compare bytes."""

    def test_byte_exact_roundtrip(self, game_dir: Path, tmp_path: Path, filename: str) -> None:
        original = game_dir / filename
        registry = _build_registry()
        handler = registry.get_handler(filename)

        # Unpack
        unpack_dir = tmp_path / "unpacked"
        unpack_dir.mkdir()
        manifest = handler.unpack(original, unpack_dir)

        # Repack
        repacked = tmp_path / "repacked.DD2"
        handler.repack(manifest, unpack_dir, repacked)

        assert repacked.read_bytes() == original.read_bytes(), (
            f"Byte-exact roundtrip failed for {filename}"
        )

    def test_manifest_is_valid(self, game_dir: Path, tmp_path: Path, filename: str) -> None:
        original = game_dir / filename
        registry = _build_registry()
        handler = registry.get_handler(filename)

        unpack_dir = tmp_path / "unpacked"
        unpack_dir.mkdir()
        handler.unpack(original, unpack_dir)

        manifest_path = unpack_dir / "manifest.json"
        assert manifest_path.exists(), "manifest.json not created"

        manifest = Manifest.from_json(manifest_path)
        assert manifest.source_file == filename
        assert manifest.handler != ""


# ---------------------------------------------------------------------------
# Full CLI cycle: unpack all -> repack all -> compare
# ---------------------------------------------------------------------------


def _run_cli(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, "-m", "gravedigger.cli", *args],
        capture_output=True,
        text=True,
    )


class TestCLIFullCycle:
    """Run the CLI unpack + repack on all game files and verify byte-exact output."""

    def test_full_cycle_byte_exact(self, game_dir: Path, tmp_path: Path) -> None:
        unpack_dir = tmp_path / "unpacked"
        repack_dir = tmp_path / "repacked"

        result = _run_cli("unpack", str(game_dir), str(unpack_dir))
        assert result.returncode == 0, f"unpack failed:\n{result.stderr}"

        result = _run_cli("repack", str(unpack_dir), str(repack_dir))
        assert result.returncode == 0, f"repack failed:\n{result.stderr}"

        # Check DD2 files
        originals = sorted(game_dir.glob("*.DD2"))
        assert len(originals) > 0

        for original in originals:
            repacked = repack_dir / original.name
            assert repacked.exists(), f"Missing repacked file: {original.name}"
            assert repacked.read_bytes() == original.read_bytes(), (
                f"CLI roundtrip byte mismatch: {original.name}"
            )

        # Check EXE files
        for original in sorted(game_dir.glob("*.EXE")):
            repacked = repack_dir / original.name
            assert repacked.exists(), f"Missing repacked EXE: {original.name}"
            assert repacked.read_bytes() == original.read_bytes(), (
                f"CLI EXE roundtrip byte mismatch: {original.name}"
            )

    def test_unpack_creates_all_subdirs(self, game_dir: Path, tmp_path: Path) -> None:
        unpack_dir = tmp_path / "unpacked"

        result = _run_cli("unpack", str(game_dir), str(unpack_dir))
        assert result.returncode == 0, f"unpack failed:\n{result.stderr}"

        originals = sorted(game_dir.glob("*.DD2"))
        for original in originals:
            subdir = unpack_dir / original.stem
            assert subdir.is_dir(), f"Missing subdir for {original.name}"
            assert (subdir / "manifest.json").exists(), f"Missing manifest.json in {subdir.name}"

    def test_exe_unpack_creates_handler_subdirs(self, game_dir: Path, tmp_path: Path) -> None:
        unpack_dir = tmp_path / "unpacked"

        result = _run_cli("unpack", str(game_dir), str(unpack_dir))
        assert result.returncode == 0, f"unpack failed:\n{result.stderr}"

        # EXE files with multiple handlers get per-handler subdirectories
        for exe_file in sorted(game_dir.glob("*.EXE")):
            exe_dir = unpack_dir / exe_file.stem
            assert exe_dir.is_dir(), f"Missing dir for {exe_file.name}"
            death_dir = exe_dir / "ExeDeathHandler"
            text_dir = exe_dir / "ExeTextHandler"
            assert death_dir.is_dir(), "Missing ExeDeathHandler subdir"
            assert text_dir.is_dir(), "Missing ExeTextHandler subdir"
            assert (death_dir / "manifest.json").exists()
            assert (text_dir / "manifest.json").exists()
