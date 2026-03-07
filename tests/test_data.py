from gravedigger.data import load_cp866_font


def test_load_cp866_font_size() -> None:
    font = load_cp866_font()
    assert len(font) == 4096


def test_load_cp866_font_cyrillic_glyphs() -> None:
    font = load_cp866_font()
    # Cyrillic A (CP866 index 0x80): should have non-zero pixels
    cyr_a = font[0x80 * 16 : 0x80 * 16 + 16]
    assert any(b != 0 for b in cyr_a)
