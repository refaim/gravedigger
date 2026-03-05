"""EXE roundtrip test: build artifact unpack -> repack -> byte-exact compare."""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

BIN_DIR = Path(__file__).resolve().parent.parent / "bin"
EXE_CANDIDATES = ["gravedigger", "gravedigger.exe"]


def _find_exe() -> Path | None:
    for name in EXE_CANDIDATES:
        path = BIN_DIR / name
        if path.is_file():
            return path
    return None


EXE_PATH = _find_exe()

pytestmark = pytest.mark.skipif(EXE_PATH is None, reason="EXE not built (run `make build` first)")


def _run_exe(*args: str) -> subprocess.CompletedProcess[str]:
    assert EXE_PATH is not None
    return subprocess.run(
        [str(EXE_PATH), *args],
        capture_output=True,
        text=True,
        timeout=120,
    )


class TestExeRoundtrip:
    """Run the built EXE on all game files and verify byte-exact roundtrip."""

    def test_help(self) -> None:
        result = _run_exe("--help")
        assert result.returncode == 0
        assert "gravedigger" in result.stdout.lower()

    def test_full_roundtrip(self, game_dir: Path, tmp_path: Path) -> None:
        unpack_dir = tmp_path / "unpacked"
        repack_dir = tmp_path / "repacked"

        result = _run_exe("unpack", str(game_dir), str(unpack_dir))
        assert result.returncode == 0, f"EXE unpack failed:\n{result.stderr}"

        result = _run_exe("repack", str(unpack_dir), str(repack_dir))
        assert result.returncode == 0, f"EXE repack failed:\n{result.stderr}"

        originals = sorted(game_dir.glob("*.DD2"))
        assert len(originals) > 0

        for original in originals:
            repacked = repack_dir / original.name
            assert repacked.exists(), f"Missing repacked file: {original.name}"
            assert repacked.read_bytes() == original.read_bytes(), (
                f"EXE roundtrip byte mismatch: {original.name}"
            )
