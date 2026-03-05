from __future__ import annotations

from dataclasses import dataclass


@dataclass
class Tile:
    """A 16x16 EGA tile from EGATILES.DD2."""

    index: int
    data: bytes
    width: int = 16
    height: int = 16


@dataclass
class Sprite:
    """A variable-size EGA sprite from S_*.DD2 files."""

    index: int
    width: int
    height: int
    data: bytes


@dataclass
class Level:
    """Tile and object maps from LEVEL*.DD2 files."""

    width: int
    height: int
    tile_map: list[int]
    object_map: list[int]


@dataclass
class Picture:
    """A 320x200 EGA title/progress screen from PIC .DD2 files."""

    data: bytes
    width: int = 320
    height: int = 200
