from pathlib import Path

import pytest


@pytest.fixture()
def game_dir() -> Path:
    path = Path(__file__).resolve().parent.parent / "game"
    if not path.is_dir():
        pytest.skip("game/ directory not found")
    return path


@pytest.fixture()
def tmp_output(tmp_path: Path) -> Path:
    out = tmp_path / "output"
    out.mkdir()
    return out
