from gravedigger.core.models import Level, Picture, Sprite, Tile


class TestTile:
    def test_create(self) -> None:
        tile = Tile(index=0, width=16, height=16, data=b"\x00" * 128)
        assert tile.index == 0
        assert tile.width == 16
        assert tile.height == 16
        assert tile.data == b"\x00" * 128

    def test_defaults(self) -> None:
        tile = Tile(index=5, data=b"\xff")
        assert tile.width == 16
        assert tile.height == 16


class TestSprite:
    def test_create(self) -> None:
        sprite = Sprite(index=0, width=24, height=32, data=b"\x00" * 96)
        assert sprite.index == 0
        assert sprite.width == 24
        assert sprite.height == 32
        assert sprite.data == b"\x00" * 96

    def test_no_defaults(self) -> None:
        sprite = Sprite(index=1, width=40, height=16, data=b"\xab")
        assert sprite.width == 40
        assert sprite.height == 16


class TestLevel:
    def test_create(self) -> None:
        level = Level(
            width=64,
            height=57,
            tile_map=list(range(10)),
            object_map=list(range(5)),
        )
        assert level.width == 64
        assert level.height == 57
        assert len(level.tile_map) == 10
        assert len(level.object_map) == 5


class TestPicture:
    def test_create(self) -> None:
        pic = Picture(width=320, height=200, data=b"\x00" * 32000)
        assert pic.width == 320
        assert pic.height == 200
        assert len(pic.data) == 32000

    def test_defaults(self) -> None:
        pic = Picture(data=b"\x00")
        assert pic.width == 320
        assert pic.height == 200
