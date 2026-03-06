from pathlib import Path

import pytest

_GAME_ROOT = Path(__file__).resolve().parent.parent / "game"
_VARIANTS = ["softdisk", "retail"]


def _available_variants() -> list[str]:
    return [v for v in _VARIANTS if (_GAME_ROOT / v).is_dir()]


@pytest.fixture(params=_available_variants())
def game_dir(request: pytest.FixtureRequest) -> Path:
    return _GAME_ROOT / str(request.param)


@pytest.fixture()
def tmp_output(tmp_path: Path) -> Path:
    out = tmp_path / "output"
    out.mkdir()
    return out


@pytest.fixture()
def tmp_translatable(tmp_path: Path) -> Path:
    out = tmp_path / "translatable"
    out.mkdir()
    return out


@pytest.fixture()
def tmp_meta(tmp_path: Path) -> Path:
    out = tmp_path / "meta"
    out.mkdir()
    return out
