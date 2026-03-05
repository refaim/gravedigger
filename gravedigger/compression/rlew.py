"""RLEW (Run-Length Encoding on Words) codec for Dangerous Dave 2 level data.

Operates on 16-bit little-endian words. Marker word 0xFEFE followed by
count (uint16) and value (uint16) means "repeat value count times".
"""

import struct

MARKER = 0xFEFE


def decompress(data: bytes) -> bytes:
    """Decompress RLEW-encoded data."""
    if len(data) % 2 != 0:
        msg = f"RLEW data must have even length, got {len(data)}"
        raise ValueError(msg)
    out = bytearray()
    offset = 0
    length = len(data)

    while offset < length:
        word = struct.unpack_from("<H", data, offset)[0]
        if word == MARKER:
            count = struct.unpack_from("<H", data, offset + 2)[0]
            value = data[offset + 4 : offset + 6]
            out.extend(value * count)
            offset += 6
        else:
            out.extend(data[offset : offset + 2])
            offset += 2

    return bytes(out)


def compress(data: bytes) -> bytes:
    """Compress data using RLEW encoding."""
    if len(data) % 2 != 0:
        msg = f"RLEW data must have even length, got {len(data)}"
        raise ValueError(msg)
    out = bytearray()
    num_words = len(data) // 2
    words = struct.unpack(f"<{num_words}H", data)

    i = 0
    while i < num_words:
        word = words[i]
        # Count consecutive identical words
        run_len = 1
        while i + run_len < num_words and words[i + run_len] == word:
            run_len += 1

        # Encode as a run if it saves space (run >= 4 words, since a run of 3
        # costs the same 6 bytes as 3 literals) or if the word is the marker
        # value itself (must be encoded as a run to avoid ambiguity).
        if run_len >= 4 or word == MARKER:
            out.extend(struct.pack("<HHH", MARKER, run_len, word))
        else:
            for _ in range(run_len):
                out.extend(struct.pack("<H", word))

        i += run_len

    return bytes(out)
