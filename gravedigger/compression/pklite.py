"""PKLITE (LZ91) codec for DOS EXE files.

Decompresses and recompresses DOS executables compressed with PKLITE.
The decompressor handles the LZ77+Huffman code image compression,
relocation table, and reconstructs a valid MZ EXE.

The compressor uses a byte-patching strategy: it stores the original
compressed file and patches in modified code image bytes, enabling
byte-exact roundtrip without reimplementing the full LZ77+Huffman encoder.

Algorithm reference:
  - camoto-project/gamecompjs (GPLv3, Adam Nielsen)
  - OpenTESArena ExeUnpacker (MIT, afritz1)
  - depklite (MIT, hackerb9 / NY00123)

File format:
  - Standard MZ EXE header
  - PKLITE signature at offset 0x1E ("PKLITE")
  - Version byte at 0x1C (minor) and 0x1D (major + flags)
  - Decompression stub starting at header_paragraphs * 16
  - Compressed code image
  - Compressed relocation table
  - 8-byte footer: original SS, SP, CS, IP
"""

from __future__ import annotations

import struct
from collections.abc import Callable
from typing import NamedTuple

# Type alias for the bit-reader closure used in Huffman tree walking.
_BitReader = Callable[[], int]

# ---------------------------------------------------------------------------
# Huffman trees for LZ77 decoding
# ---------------------------------------------------------------------------
# Trees are nested lists: [left_subtree, right_subtree]
# Leaf nodes are plain integers (-1 = special marker).

# "Large" mode count tree (used when 0x20 flag is set in version byte)
_BT_COUNT_LARGE: list[object] = [
    [  # 0
        [4, [5, 6]],  # 00x, 001x
        [  # 01
            [7, [8, 9]],  # 010x, 0101x
            [  # 011
                [10, [11, 12]],  # 0110x, 01101x
                [  # 0111
                    [-1, [13, 14]],  # 01110x, 011101x
                    [  # 01111
                        [15, [16, 17]],  # 011110x, 0111101x
                        [  # 011111
                            [18, [19, 20]],  # 0111110x, 01111101x
                            [  # 0111111
                                [21, 22],  # 01111110x
                                [23, 24],  # 01111111x
                            ],
                        ],
                    ],
                ],
            ],
        ],
    ],
    [2, 3],  # 1x
]

# Offset tree (same for both large and small modes)
_BT_OFFSET: list[object] = [
    [  # 0
        [  # 00
            [1, 2],  # 000x
            [[3, 4], [5, 6]],  # 001xx
        ],
        [  # 01
            [  # 010
                [[7, 8], [9, 10]],  # 0100xx
                [[11, 12], [13, [14, 15]]],  # 0101xx
            ],
            [  # 011
                [[[16, 17], [18, 19]], [[20, 21], [22, 23]]],  # 011xxxx
                [[[24, 25], [26, 27]], [[28, 29], [30, 31]]],  # 0111xxxx
            ],
        ],
    ],
    0,  # 1
]

# ---------------------------------------------------------------------------
# Decompressor stub size lookup (version code -> stub length in bytes)
# ---------------------------------------------------------------------------
_STUB_SIZES: dict[int, int] = {
    0x0100: 0x1D0,
    0x0103: 0x1D0,
    0x0105: 0x1D0,
    0x010C: 0x1D0,
    0x010D: 0x1D0,
    0x010E: 0x1D0,
    0x010F: 0x1D0,
    0x1103: 0x1E0,
    0x110C: 0x1E0,
    0x110D: 0x1E0,
    0x110E: 0x200,
    0x110F: 0x200,
    0x2100: 0x290,
    0x2103: 0x290,
    0x2105: 0x290,
    0x210A: 0x290,
    0x210C: 0x290,
    0x210D: 0x290,
    0x210E: 0x290,
    0x210F: 0x290,
    0x3103: 0x2A0,
    0x310C: 0x290,
    0x310D: 0x290,
    0x310E: 0x2C0,
    0x310F: 0x2C0,
}


class _PkliteParams(NamedTuple):
    """Parameters extracted from a PKLITE-compressed EXE."""

    ver_code: int
    flag_large: bool
    header_size: int  # in bytes (pgLenHeader * 16)
    stub_size: int  # decompressor code size
    comp_start: int  # absolute file offset where compressed data begins


class _ExeHeader(NamedTuple):
    """Parsed MZ EXE header fields."""

    last_page: int
    pages: int
    reloc_count: int
    header_para: int
    min_extra: int
    max_extra: int
    ss: int
    sp: int
    checksum: int
    ip: int
    cs: int
    reloc_offset: int
    overlay: int


def _parse_exe_header(data: bytes) -> _ExeHeader:
    """Parse the 28-byte MZ EXE header (after 'MZ' signature)."""
    fields = struct.unpack_from("<13H", data, 2)
    return _ExeHeader(*fields)


def _validate_pklite(data: bytes) -> _PkliteParams:
    """Validate that data is a PKLITE-compressed EXE and extract parameters."""
    if len(data) < 0x30:
        msg = "File too short to be a PKLITE executable"
        raise ValueError(msg)

    if data[:2] != b"MZ":
        msg = "Not an MZ executable (missing MZ signature)"
        raise ValueError(msg)

    # Check for PKLITE signature at offset 0x1E
    if data[0x1E : 0x1E + 6] != b"PKLITE":
        msg = "Not compressed with PKLITE (missing PKLITE signature at 0x1E)"
        raise ValueError(msg)

    ver_minor = data[0x1C]
    ver_major_raw = data[0x1D]
    flag_large = bool(ver_major_raw & 0x20)

    header = _parse_exe_header(data)
    header_size = header.header_para * 16

    ver_code = (ver_major_raw << 8) | ver_minor

    stub_size = _STUB_SIZES.get(ver_code)
    if stub_size is None:
        ver_major = ver_major_raw & 0x0F
        msg = f"Unsupported PKLITE version {ver_major}.{ver_minor:02d} (code 0x{ver_code:04x})"
        raise ValueError(msg)

    comp_start = header_size + stub_size

    return _PkliteParams(
        ver_code=ver_code,
        flag_large=flag_large,
        header_size=header_size,
        stub_size=stub_size,
        comp_start=comp_start,
    )


def _bt_read(tree: list[object], next_bit: _BitReader) -> int:
    """Walk a Huffman tree using bits from the stream."""
    b = next_bit()
    node = tree[b]
    if isinstance(node, list):
        return _bt_read(node, next_bit)
    if not isinstance(node, int):
        msg = f"Invalid Huffman tree node: expected int, got {type(node).__name__}"
        raise ValueError(msg)
    return node


def decompress(data: bytes) -> bytes:
    """Decompress a PKLITE-compressed DOS EXE.

    Args:
        data: Raw bytes of the PKLITE-compressed EXE file.

    Returns:
        Complete decompressed MZ EXE file bytes.

    Raises:
        ValueError: If data is not a valid PKLITE-compressed EXE.
    """
    params = _validate_pklite(data)
    header = _parse_exe_header(data)

    if not params.flag_large:
        msg = "Only large-mode PKLITE files are supported"
        raise ValueError(msg)

    bt_count = _BT_COUNT_LARGE

    # Set up bit reader state
    pos = params.comp_start
    bit_index = 15
    bit_cache = 0

    def get_byte() -> int:
        nonlocal pos
        if pos >= len(data):
            msg = "Unexpected end of compressed data"
            raise ValueError(msg)
        b = data[pos]
        pos += 1
        return b

    def next_bit() -> int:
        nonlocal bit_index, bit_cache
        bit = (bit_cache >> bit_index) & 1
        bit_index += 1
        if bit_index == 16:
            bit_cache = get_byte() | (get_byte() << 8)
            bit_index = 0
        return bit

    # Prime the bit cache
    next_bit()

    # Decompress code image
    code_image = bytearray()

    while pos < len(data):
        if next_bit():  # duplication mode
            count = _bt_read(bt_count, next_bit)

            if count == -1:  # special marker
                code = get_byte()
                if code == 0xFE:
                    continue
                if code == 0xFF:
                    break  # end of compressed data
                count = code + 25

            # Read offset
            offset = 0
            if count != 2:
                offset_code = _bt_read(_BT_OFFSET, next_bit)
                offset = offset_code << 8
            lsb = get_byte()
            offset |= lsb

            # Copy from already-decompressed data
            src = len(code_image) - offset
            for i in range(count):
                code_image.append(code_image[src + i])
        else:
            # Literal byte (no XOR decryption - extra mode not supported)
            code_image.append(get_byte())
    else:
        msg = "Compressed code image ended without end-of-stream marker"
        raise ValueError(msg)

    # Decompress relocation table (standard non-large mode)
    reloc_table: list[int] = []

    while pos < len(data) - 8:
        count = data[pos]
        pos += 1
        if count == 0:
            break
        rel_msb = struct.unpack_from("<H", data, pos)[0]
        pos += 2

        for _ in range(count):
            rel_lsb = struct.unpack_from("<H", data, pos)[0]
            pos += 2
            reloc_table.append((rel_msb << 16) | rel_lsb)
    else:
        msg = "Relocation table ended without terminator"
        raise ValueError(msg)

    # Read footer (original SS, SP, CS, IP)
    footer_ss, footer_sp, footer_cs, footer_ip = struct.unpack_from("<4H", data, pos)

    # Reconstruct the original EXE header
    # Read original header from the PKLITE file (stored at reloc_offset)
    off_orig_header = header.reloc_offset + header.reloc_count * 4

    orig_header = _ExeHeader(*struct.unpack_from("<13H", data, off_orig_header + 2))
    # Extra header data between standard header and relocation table
    orig_reloc_off = orig_header.reloc_offset
    extra_len = min(orig_reloc_off - 2, params.header_size - off_orig_header) - 26
    orig_header_extra = (
        data[off_orig_header + 28 : off_orig_header + 28 + extra_len] if extra_len > 0 else b""
    )

    # Use footer values for the original entry point state (SS/SP/CS/IP).
    # The 8-byte footer contains the real original values per PKLITE format.
    final_ss = footer_ss
    final_sp = footer_sp
    final_cs = footer_cs
    final_ip = footer_ip

    # Build output EXE
    out_reloc_offset = 0x1C + len(orig_header_extra)
    out_reloc_count = len(reloc_table)
    out_header_para = (out_reloc_offset + out_reloc_count * 4 + 0x0F) >> 4
    out_header_size = out_header_para * 16
    total_size = out_header_size + len(code_image)
    out_pages = (total_size + 0x1FF) >> 9
    out_last_page = total_size & 0x1FF

    out_min_extra = orig_header.min_extra
    out_max_extra = orig_header.max_extra
    out_checksum = orig_header.checksum

    # Write MZ header
    out = bytearray()
    out += b"MZ"
    out += struct.pack(
        "<13H",
        out_last_page,
        out_pages,
        out_reloc_count,
        out_header_para,
        out_min_extra,
        out_max_extra,
        final_ss,
        final_sp,
        out_checksum,
        final_ip,
        final_cs,
        out_reloc_offset,
        0,  # overlay
    )
    out += orig_header_extra

    # Write relocation table
    for r in reloc_table:
        out += struct.pack("<I", r)

    # Pad to header boundary
    out += b"\x00" * (out_header_size - len(out))

    # Write code image
    out += code_image

    return bytes(out)


def compress(data: bytes, original: bytes) -> bytes:
    """Compress (repack) a decompressed EXE back to PKLITE format.

    Uses byte-patching strategy: takes the original compressed file as a
    template and patches in the modified code image. This ensures byte-exact
    roundtrip when the code image hasn't been modified.

    Args:
        data: Decompressed EXE bytes (as produced by decompress()).
        original: Original PKLITE-compressed EXE bytes (used as template).

    Returns:
        PKLITE-compressed EXE file bytes.

    Raises:
        ValueError: If original is not a valid PKLITE file, or if the
            decompressed data cannot be patched back.
    """
    params = _validate_pklite(original)

    # Decompress the original to get the mapping
    orig_decompressed = decompress(original)

    if len(data) != len(orig_decompressed):
        msg = (
            f"Decompressed data size mismatch: got {len(data)}, expected {len(orig_decompressed)}"
        )
        raise ValueError(msg)

    if data == orig_decompressed:
        # No changes - return original as-is
        return original

    # Find the code image region in the decompressed EXE
    header_para = struct.unpack_from("<H", data, 8)[0]
    code_start = header_para * 16

    new_code = data[code_start:]
    orig_code = orig_decompressed[code_start:]

    if new_code == orig_code:
        # Code image unchanged, only header differs
        return original

    # Re-decompress original to build a mapping from decompressed offsets
    # to compressed stream positions, then patch the compressed data.
    return _patch_compressed(original, params, orig_code, new_code)


def _patch_compressed(
    original: bytes,
    params: _PkliteParams,
    orig_code: bytes,
    new_code: bytes,
) -> bytes:
    """Patch the compressed data stream with modified code image bytes.

    This re-runs the decompression and for each literal byte that differs,
    patches the corresponding byte in the compressed stream.
    For LZ77 back-references, we verify the referenced data hasn't changed
    in a way that's incompatible.
    """
    bt_count = _BT_COUNT_LARGE

    out = bytearray(original)

    pos = params.comp_start
    bit_index = 15
    bit_cache = 0
    code_offset = 0

    def get_byte() -> int:
        nonlocal pos
        b = original[pos]
        pos += 1
        return b

    def next_bit() -> int:
        nonlocal bit_index, bit_cache
        bit = (bit_cache >> bit_index) & 1
        bit_index += 1
        if bit_index == 16:
            bit_cache = get_byte() | (get_byte() << 8)
            bit_index = 0
        return bit

    next_bit()  # prime

    while True:
        if next_bit():  # duplication
            count = _bt_read(bt_count, next_bit)

            if count == -1:
                code = get_byte()
                if code == 0xFE:
                    continue
                if code == 0xFF:
                    break
                count = code + 25

            offset = 0
            if count != 2:
                offset_code = _bt_read(_BT_OFFSET, next_bit)
                offset = offset_code << 8
            lsb = get_byte()
            offset |= lsb

            # Back-reference: verify referenced data hasn't changed
            src = code_offset - offset
            for i in range(count):
                if src + i >= 0 and orig_code[src + i] != new_code[src + i]:
                    msg = (
                        f"Cannot patch: back-reference at code offset "
                        f"{code_offset + i} references modified byte at "
                        f"{src + i}"
                    )
                    raise ValueError(msg)
            code_offset += count
        else:
            # Literal byte - this CAN be patched
            literal_pos = pos
            get_byte()  # consume the byte (advance pos)
            out[literal_pos] = new_code[code_offset]
            code_offset += 1

    return bytes(out)
