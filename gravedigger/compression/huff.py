"""Huffman codec for Dangerous Dave 2 .DD2 files.

File format:
  - 4 bytes: "HUFF" signature
  - 4 bytes: uint32 LE decompressed size
  - 1020 bytes: Huffman tree (255 entries x 2 uint16 LE values)
  - remaining: compressed bitstream (may include trailing bytes after the stream)
"""

from __future__ import annotations

import struct

_SIGNATURE = b"HUFF"
_HEADER_SIZE = 4  # "HUFF"
_SIZE_FIELD = 4  # uint32 LE
_TREE_ENTRIES = 255
_TREE_SIZE = _TREE_ENTRIES * 2 * 2  # 255 * 2 * sizeof(uint16) = 1020
_DATA_OFFSET = _HEADER_SIZE + _SIZE_FIELD + _TREE_SIZE  # 1028 = 0x404
_ROOT = 254 * 2  # root node index in LUT
_LUT_TAG = 0x100  # values >= this are internal nodes


def decompress(data: bytes) -> tuple[bytes, bytes]:
    """Decompress HUFF-encoded data.

    Args:
        data: Raw .DD2 file bytes starting with "HUFF" signature.

    Returns:
        Tuple of (decompressed_data, tree_and_tail).
        tree_and_tail contains the 1020-byte Huffman tree followed by any trailing
        bytes from the compressed stream. Both are needed for byte-exact recompression.

    Raises:
        ValueError: If data does not start with "HUFF" signature.
    """
    if data[:4] != _SIGNATURE:
        msg = f"Expected HUFF signature, got {data[:4]!r}"
        raise ValueError(msg)

    unpacked_size: int = struct.unpack_from("<I", data, 4)[0]
    tree_bytes = data[_HEADER_SIZE + _SIZE_FIELD : _DATA_OFFSET]
    lut = struct.unpack_from(f"<{_TREE_ENTRIES * 2}H", data, _HEADER_SIZE + _SIZE_FIELD)

    cdata = data[_DATA_OFFSET:]

    if unpacked_size == 0:
        return b"", tree_bytes + cdata

    out = bytearray(unpacked_size)

    src = 0
    dst = 0
    leaf = _ROOT
    bit = 1

    value = cdata[src]
    src += 1

    while dst < unpacked_size:
        dx = lut[leaf] if (value & bit) == 0 else lut[leaf + 1]

        bit <<= 1
        if bit == 0x100:
            bit = 1
            if src >= len(cdata):
                msg = "Compressed data truncated before decompressed_size was reached"
                raise ValueError(msg)
            value = cdata[src]
            src += 1

        if dx >= _LUT_TAG:
            leaf = (dx - _LUT_TAG) * 2
        else:
            out[dst] = dx & 0xFF
            dst += 1
            leaf = _ROOT

    # Trailing bytes: the partially-consumed current byte (cdata[src-1]) plus
    # any unread bytes after it. Needed for byte-exact roundtrip.
    trailing = cdata[src - 1 :]

    return bytes(out), tree_bytes + trailing


def compress(data: bytes, tree: bytes) -> bytes:
    """Compress data using the provided Huffman tree.

    Args:
        data: Decompressed data bytes.
        tree: Tree-and-tail bytes as returned by decompress. First 1020 bytes are
              the Huffman tree; remaining bytes are trailing data to append after
              the compressed bitstream for byte-exact roundtrip.

    Returns:
        Complete .DD2 file bytes with HUFF signature, size, tree, and bitstream.
    """
    tree_bytes = tree[:_TREE_SIZE]
    tail = tree[_TREE_SIZE:]
    lut = struct.unpack(f"<{_TREE_ENTRIES * 2}H", tree_bytes)

    # Build encoding table: for each byte value, find the bit path from root.
    codes: dict[int, tuple[int, int]] = {}  # byte_value -> (bits, length)
    _build_codes(lut, _ROOT, 0, 0, codes)

    # Encode the data into a bitstream.
    out_bytes = bytearray()
    current_byte = 0
    bit_pos = 0  # which bit we're writing next (0-7)

    for byte_val in data:
        if byte_val not in codes:
            msg = f"Byte value {byte_val:#04x} not found in Huffman tree"
            raise ValueError(msg)
        bits, length = codes[byte_val]
        for i in range(length):
            if bits & (1 << i):
                current_byte |= 1 << bit_pos
            bit_pos += 1
            if bit_pos == 8:
                out_bytes.append(current_byte)
                current_byte = 0
                bit_pos = 0

    # Handle the partial last byte and trailing data.
    if bit_pos > 0:
        # There's a partial byte. The tail starts with the original byte that had
        # the same lower bits (from compression) plus original upper bits.
        if tail:
            # Merge: keep our compressed lower bits, take upper bits from tail[0].
            mask = (1 << bit_pos) - 1
            merged = (current_byte & mask) | (tail[0] & ~mask)
            out_bytes.append(merged & 0xFF)
            out_bytes.extend(tail[1:])
        else:
            out_bytes.append(current_byte)
    else:
        # bit_pos == 0 means we ended exactly at a byte boundary.
        # The tail contains the eagerly-read-but-unused byte + remaining.
        out_bytes.extend(tail)

    # Build the complete file
    header = _SIGNATURE + struct.pack("<I", len(data)) + tree_bytes
    return header + bytes(out_bytes)


def _build_codes(
    lut: tuple[int, ...],
    node: int,
    bits: int,
    depth: int,
    codes: dict[int, tuple[int, int]],
) -> None:
    """Recursively traverse the Huffman tree to build encoding table."""
    # Left child (bit 0)
    left = lut[node]
    if left >= _LUT_TAG:
        _build_codes(lut, (left - _LUT_TAG) * 2, bits, depth + 1, codes)
    else:
        codes[left & 0xFF] = (bits, depth + 1)

    # Right child (bit 1)
    right = lut[node + 1]
    if right >= _LUT_TAG:
        _build_codes(lut, (right - _LUT_TAG) * 2, bits | (1 << depth), depth + 1, codes)
    else:
        codes[right & 0xFF] = (bits | (1 << depth), depth + 1)
