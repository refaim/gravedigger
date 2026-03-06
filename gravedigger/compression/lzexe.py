"""LZEXE (LZ91) codec for DOS EXE files.

Decompresses and recompresses DOS executables compressed with LZEXE 0.91.
The decompressor handles the LZ77 code image compression, relocation table,
and reconstructs a valid MZ EXE.

The compressor uses a byte-patching strategy identical to the PKLITE codec:
it stores the original compressed file and patches in modified code image
bytes, enabling byte-exact roundtrip without reimplementing the LZ77 encoder.

Algorithm reference:
  - UNLZEXE v0.9 (Mitugu Kurizono / Stian Skjelstad)
  - unpacklzexe (MIT, Sam Russell)
"""

from __future__ import annotations

import struct
from typing import NamedTuple

_LZ91_SIG = b"LZ91"
_RELOC_TABLE_STUB_OFFSET = 0x158


class _LzexeParams(NamedTuple):
    header_size: int
    comp_start: int
    stub_offset: int
    orig_ip: int
    orig_cs: int
    orig_sp: int
    orig_ss: int


def _validate_lzexe(data: bytes) -> _LzexeParams:
    if len(data) < 0x20:
        msg = "File too short to be an LZEXE executable"
        raise ValueError(msg)

    if data[:2] not in (b"MZ", b"ZM"):
        msg = "Not an MZ executable"
        raise ValueError(msg)

    if data[0x1C:0x20] != _LZ91_SIG:
        msg = "Not compressed with LZEXE 0.91 (missing LZ91 signature at 0x1C)"
        raise ValueError(msg)

    header_para = struct.unpack_from("<H", data, 8)[0]
    cs = struct.unpack_from("<H", data, 0x16)[0]

    header_size = header_para * 16
    stub_offset = header_size + cs * 16

    if stub_offset + 14 > len(data):
        msg = "LZEXE stub extends beyond file"
        raise ValueError(msg)

    orig_ip, orig_cs, orig_sp, orig_ss = struct.unpack_from("<4H", data, stub_offset)

    return _LzexeParams(
        header_size=header_size,
        comp_start=header_size,
        stub_offset=stub_offset,
        orig_ip=orig_ip,
        orig_cs=orig_cs,
        orig_sp=orig_sp,
        orig_ss=orig_ss,
    )


class _BitReader:
    """LSB-first bit reader over a byte buffer."""

    __slots__ = ("_cache", "_count", "_data", "_pos")

    def __init__(self, data: bytes, start: int = 0) -> None:
        self._data = data
        self._pos = start
        self._cache = data[start] | (data[start + 1] << 8)
        self._pos = start + 2
        self._count = 16

    @property
    def pos(self) -> int:
        return self._pos

    def get_byte(self) -> int:
        b = self._data[self._pos]
        self._pos += 1
        return b

    def get_bit(self) -> int:
        b = self._cache & 1
        self._cache >>= 1
        self._count -= 1
        if self._count == 0:
            self._cache = self._data[self._pos] | (self._data[self._pos + 1] << 8)
            self._pos += 2
            self._count = 16
        return b


def _run_decompression(
    bits: _BitReader,
) -> tuple[bytearray, list[tuple[int, int | None]]]:
    """Run LZ91 decompression, returning code image and operation log.

    The operation log records each operation:
      - ('L', file_pos): literal byte read from file_pos
      - ('R', span, length, code_offset): back-reference
    This log is used by the byte-patcher.

    Returns (code_image, ops) where ops is a list of
    (code_offset, literal_file_pos | None) tuples.
    One entry per code byte: literal_file_pos for literals, None for
    back-reference copies.
    """
    code = bytearray()
    ops: list[tuple[int, int | None]] = []

    while True:
        if bits.get_bit():
            # Literal byte
            literal_pos = bits.pos
            code.append(bits.get_byte())
            ops.append((len(code) - 1, literal_pos))
            continue

        if not bits.get_bit():
            # 00: short back-reference
            length = (bits.get_bit() << 1) | bits.get_bit()
            length += 2
            span = bits.get_byte() | 0xFF00
            span -= 0x10000  # sign-extend to negative
        else:
            # 01: long back-reference
            lo = bits.get_byte()
            hi = bits.get_byte()
            span = lo | ((hi & 0xF8) << 5) | 0xE000
            span -= 0x10000  # sign-extend to negative
            length = (hi & 0x07) + 2

            if length == 2:
                length = bits.get_byte()
                if length == 0:
                    break  # end of compressed data
                if length == 1:
                    continue  # segment boundary
                length += 1

        src = len(code) + span
        for i in range(length):
            code.append(code[src + i])
            ops.append((len(code) - 1, None))

    return code, ops


def _decompress_reloc(data: bytes) -> list[int]:
    """Decompress LZ91 relocation table.

    Returns list of 32-bit values: (segment << 16) | offset.
    """
    relocs: list[int] = []
    pos = 0
    rel_off = 0
    rel_seg = 0

    while pos < len(data):
        span = data[pos]
        pos += 1
        if span == 0:
            if pos + 1 >= len(data):
                break
            marker = data[pos] | (data[pos + 1] << 8)
            pos += 2
            if marker == 0:
                rel_seg += 0x0FFF
                continue
            if marker == 1:
                break
            span = marker

        rel_off += span
        rel_seg += (rel_off & ~0x0F) >> 4
        rel_off &= 0x0F
        relocs.append(rel_off | (rel_seg << 16))

    return relocs


def _build_exe(params: _LzexeParams, code_image: bytearray, reloc_table: list[int]) -> bytes:
    reloc_offset = 0x1C
    reloc_count = len(reloc_table)
    header_para = (reloc_offset + reloc_count * 4 + 0x0F) >> 4
    header_size = header_para * 16
    total_size = header_size + len(code_image)
    pages = (total_size + 0x1FF) >> 9
    last_page = total_size & 0x1FF

    out = bytearray()
    out += b"MZ"
    out += struct.pack(
        "<13H",
        last_page,
        pages,
        reloc_count,
        header_para,
        0,  # min_extra
        0xFFFF,  # max_extra
        params.orig_ss,
        params.orig_sp,
        0,  # checksum
        params.orig_ip,
        params.orig_cs,
        reloc_offset,
        0,  # overlay
    )

    for r in reloc_table:
        out += struct.pack("<I", r)

    out += b"\x00" * (header_size - len(out))
    out += code_image

    return bytes(out)


def decompress(data: bytes) -> bytes:
    """Decompress an LZEXE 0.91 compressed DOS EXE."""
    params = _validate_lzexe(data)
    compressed = data[params.comp_start : params.stub_offset]
    bits = _BitReader(compressed)
    code_image, _ = _run_decompression(bits)
    reloc_data = data[params.stub_offset + _RELOC_TABLE_STUB_OFFSET :]
    reloc_table = _decompress_reloc(reloc_data)
    return _build_exe(params, code_image, reloc_table)


def compress(data: bytes, original: bytes) -> bytes:
    """Compress (repack) a decompressed EXE back to LZEXE format.

    Uses byte-patching: takes the original compressed file as a template
    and patches in modified literal bytes in the compressed stream.
    """
    params = _validate_lzexe(original)
    orig_decompressed = decompress(original)

    if len(data) != len(orig_decompressed):
        msg = (
            f"Decompressed data size mismatch: got {len(data)}, expected {len(orig_decompressed)}"
        )
        raise ValueError(msg)

    if data == orig_decompressed:
        return original

    header_para = struct.unpack_from("<H", data, 8)[0]
    code_start = header_para * 16

    new_code = data[code_start:]
    orig_code = orig_decompressed[code_start:]

    if new_code == orig_code:
        return original

    return _patch_compressed(original, params, orig_code, new_code)


def _patch_compressed(
    original: bytes,
    params: _LzexeParams,
    orig_code: bytes,
    new_code: bytes,
) -> bytes:
    """Patch literal bytes in the compressed stream."""
    compressed = original[params.comp_start : params.stub_offset]
    bits = _BitReader(compressed)
    _, ops = _run_decompression(bits)

    out = bytearray(original)

    for code_offset, literal_pos in ops:
        if code_offset >= len(new_code):
            break
        if orig_code[code_offset] == new_code[code_offset]:
            continue
        if literal_pos is not None:
            out[params.comp_start + literal_pos] = new_code[code_offset]
        else:
            msg = (
                f"Cannot patch: back-reference at code offset "
                f"{code_offset} references modified byte"
            )
            raise ValueError(msg)

    return bytes(out)
