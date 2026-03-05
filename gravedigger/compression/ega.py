from PIL import Image

EGA_PALETTE: list[tuple[int, int, int]] = [
    (0x00, 0x00, 0x00),  # 0  black
    (0x00, 0x00, 0xA8),  # 1  dark blue
    (0x00, 0xA8, 0x00),  # 2  dark green
    (0x00, 0xA8, 0xA8),  # 3  dark cyan
    (0xA8, 0x00, 0x00),  # 4  dark red
    (0xA8, 0x00, 0xA8),  # 5  dark magenta
    (0xA8, 0x54, 0x00),  # 6  brown
    (0xA8, 0xA8, 0xA8),  # 7  light gray
    (0x54, 0x54, 0x54),  # 8  dark gray
    (0x54, 0x54, 0xFF),  # 9  light blue
    (0x54, 0xFF, 0x54),  # 10 light green
    (0x54, 0xFF, 0xFF),  # 11 light cyan
    (0xFF, 0x54, 0x15),  # 12 light red
    (0xFF, 0x54, 0xFF),  # 13 light magenta
    (0xFF, 0xFF, 0x54),  # 14 yellow
    (0xFF, 0xFF, 0xFF),  # 15 white
]


def _build_pil_palette() -> list[int]:
    """Build a 768-entry flat RGB palette for PIL."""
    pal: list[int] = []
    for r, g, b in EGA_PALETTE:
        pal.extend((r, g, b))
    # Pad remaining 240 entries with zeros
    pal.extend([0] * (768 - len(pal)))
    return pal


_PIL_PALETTE = _build_pil_palette()


def decode_planar(data: bytes, width: int, height: int) -> list[int]:
    """Decode EGA 4-plane interleaved data into a flat list of pixel color indices (0-15)."""
    if width % 8 != 0:
        msg = f"Width must be a multiple of 8, got {width}"
        raise ValueError(msg)
    plane_size = (width * height) >> 3  # w*h/8 bytes per plane
    if len(data) < plane_size * 4:
        msg = f"Expected at least {plane_size * 4} bytes of EGA data, got {len(data)}"
        raise ValueError(msg)
    row_bytes = width >> 3  # bytes per row per plane
    pixels = [0] * (width * height)

    for y in range(height):
        for x in range(width):
            byte_offset = (x >> 3) + y * row_bytes
            bit = 7 - (x & 7)

            c0 = (data[byte_offset + plane_size * 0] >> bit) & 1
            c1 = (data[byte_offset + plane_size * 1] >> bit) & 1
            c2 = (data[byte_offset + plane_size * 2] >> bit) & 1
            c3 = (data[byte_offset + plane_size * 3] >> bit) & 1

            pixels[y * width + x] = c0 | (c1 << 1) | (c2 << 2) | (c3 << 3)

    return pixels


def encode_planar(pixels: list[int], width: int, height: int) -> bytes:
    """Encode a flat list of pixel color indices (0-15) into EGA 4-plane interleaved data."""
    if width % 8 != 0:
        msg = f"Width must be a multiple of 8, got {width}"
        raise ValueError(msg)
    plane_size = (width * height) >> 3
    row_bytes = width >> 3
    data = bytearray(plane_size * 4)

    for y in range(height):
        for x in range(width):
            color = pixels[y * width + x]
            byte_offset = (x >> 3) + y * row_bytes
            bit = 7 - (x & 7)

            for plane in range(4):
                if (color >> plane) & 1:
                    data[byte_offset + plane_size * plane] |= 1 << bit

    return bytes(data)


def pixels_to_image(pixels: list[int], width: int, height: int) -> Image.Image:
    """Convert pixel indices to a PIL paletted image with the EGA palette."""
    img = Image.new("P", (width, height))
    img.putpalette(_PIL_PALETTE)
    img.putdata(pixels)
    return img


def image_to_pixels(img: Image.Image) -> list[int]:
    """Convert a PIL paletted image back to a flat list of pixel indices."""
    return list(img.tobytes())
