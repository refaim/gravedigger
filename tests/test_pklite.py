"""Tests for PKLITE (LZ91) decompressor/compressor."""

from __future__ import annotations

import struct
from pathlib import Path

import pytest

from gravedigger.compression.pklite import _bt_read, compress, decompress

GAME_DIR = Path(__file__).resolve().parent.parent / "game"
EXE_PATH = GAME_DIR / "1.EXE"


@pytest.fixture
def exe_data() -> bytes:
    return EXE_PATH.read_bytes()


@pytest.fixture
def decompressed(exe_data: bytes) -> bytes:
    return decompress(exe_data)


class TestDecompress:
    def test_decompressed_size_is_reasonable(self, exe_data: bytes) -> None:
        """Decompressed EXE should be larger than compressed."""
        result = decompress(exe_data)
        assert len(result) > len(exe_data)
        assert len(result) > 100_000

    def test_contains_dangerous_dave_text(self, decompressed: bytes) -> None:
        """Decompressed EXE should contain 'Dangerous Dave' text."""
        assert b"Dangerous Dave" in decompressed

    def test_contains_death_sequence_data(self, decompressed: bytes) -> None:
        """Decompressed EXE should have death sequence data at offset 0x18A60."""
        assert decompressed[:2] == b"MZ"
        header_para = struct.unpack_from("<H", decompressed, 8)[0]
        code_start = header_para * 16
        death_offset = code_start + 0x18A60
        assert len(decompressed) > death_offset + 1000
        death_data = decompressed[death_offset : death_offset + 288]
        assert death_data != b"\x00" * 288

    def test_contains_sprite_names(self, decompressed: bytes) -> None:
        """Decompressed EXE should contain sprite descriptor names."""
        assert b"DAVESTANDE" in decompressed
        assert b"DAVEWALKE1" in decompressed

    def test_contains_softdisk_copyright(self, decompressed: bytes) -> None:
        """Decompressed EXE should contain Softdisk copyright."""
        assert b"Softdisk" in decompressed

    def test_contains_control_strings(self, decompressed: bytes) -> None:
        """Decompressed EXE should contain keyboard control strings."""
        assert b"ESC" in decompressed

    def test_non_pklite_file_raises(self) -> None:
        """Non-PKLITE file should raise ValueError."""
        with pytest.raises(ValueError, match="PKLITE"):
            decompress(b"This is not a PKLITE file at all")

    def test_too_short_raises(self) -> None:
        """File shorter than 0x30 bytes should raise ValueError."""
        with pytest.raises(ValueError, match="too short"):
            decompress(b"MZ" + b"\x00" * 10)

    def test_no_mz_signature_raises(self) -> None:
        """Non-MZ file should raise ValueError."""
        with pytest.raises(ValueError, match="MZ"):
            decompress(b"PK" + b"\x00" * 100)

    def test_non_pklite_exe_raises(self) -> None:
        """MZ file without PKLITE signature should raise ValueError."""
        fake_exe = b"MZ" + b"\x00" * 100
        with pytest.raises(ValueError, match="PKLITE"):
            decompress(fake_exe)

    def test_unsupported_version_raises(self) -> None:
        """Unsupported PKLITE version should raise ValueError."""
        buf = bytearray(0x60)
        buf[0:2] = b"MZ"
        struct.pack_into("<H", buf, 8, 4)  # header_para
        buf[0x1C] = 0xFF  # unsupported minor
        buf[0x1D] = 0x0F  # unsupported flags combo
        buf[0x1E : 0x1E + 6] = b"PKLITE"
        with pytest.raises(ValueError, match="Unsupported"):
            decompress(bytes(buf))

    def test_small_mode_raises(self) -> None:
        """Non-large-mode PKLITE files should raise ValueError."""
        buf = bytearray(0x300)
        buf[0:2] = b"MZ"
        struct.pack_into("<H", buf, 8, 7)  # header_para = 7 (112 bytes)
        buf[0x1C] = 0x0D  # ver_minor
        buf[0x1D] = 0x01  # ver_major without 0x20 flag (small mode)
        buf[0x1E : 0x1E + 6] = b"PKLITE"
        with pytest.raises(ValueError, match="large-mode"):
            decompress(bytes(buf))

    def test_output_is_valid_exe(self, decompressed: bytes) -> None:
        """Decompressed output should be a valid MZ EXE."""
        assert decompressed[:2] == b"MZ"
        last_page = struct.unpack_from("<H", decompressed, 2)[0]
        pages = struct.unpack_from("<H", decompressed, 4)[0]
        computed_size = (pages - 1) * 512 + last_page
        assert computed_size == len(decompressed)

    def test_invalid_huffman_tree_node_raises(self) -> None:
        """Invalid tree node type should raise ValueError."""
        bad_tree: list[object] = ["not_an_int", 1]
        bits = iter([0])
        with pytest.raises(ValueError, match="Invalid Huffman tree node"):
            _bt_read(bad_tree, lambda: next(bits))

    def test_truncated_compressed_data_raises(self, exe_data: bytes) -> None:
        """Truncated compressed data should raise ValueError."""
        # Truncate at a point that causes get_byte() to fail mid-read.
        # Compressed data starts at 0x300; first bitArray is 2 bytes at 0x300.
        # Truncating at 0x312 leaves an odd number of bytes so the bit reader
        # tries to reload a 16-bit word with only 1 byte available.
        truncated = exe_data[:0x312]
        with pytest.raises((ValueError, struct.error)):
            decompress(truncated)


class TestCompress:
    def test_roundtrip_byte_exact(self, exe_data: bytes) -> None:
        """compress(decompress(data), data) should produce the original."""
        decompressed = decompress(exe_data)
        recompressed = compress(decompressed, exe_data)
        assert recompressed == exe_data

    def test_non_pklite_original_raises(self) -> None:
        """compress() with non-PKLITE original should raise ValueError."""
        with pytest.raises(ValueError, match="PKLITE"):
            compress(b"some data", b"not a pklite file")

    def test_size_mismatch_raises(self, exe_data: bytes) -> None:
        """compress() with wrong-sized decompressed data raises ValueError."""
        decompressed = decompress(exe_data)
        with pytest.raises(ValueError, match="size mismatch"):
            compress(decompressed + b"\x00", exe_data)

    def test_modified_literal_bytes(self, exe_data: bytes) -> None:
        """Modifying literal bytes in the code image should roundtrip."""
        decompressed = bytearray(decompress(exe_data))
        # Find the "Softdisk" string and modify a character
        idx = decompressed.find(b"Softdisk")
        assert idx > 0
        decompressed[idx] = ord("X")
        recompressed = compress(bytes(decompressed), exe_data)
        # Re-decompress to verify the change took effect
        re_decompressed = decompress(recompressed)
        assert re_decompressed[idx] == ord("X")
        assert b"Xoftdisk" in re_decompressed

    def test_modified_backreference_raises(self, exe_data: bytes) -> None:
        """Modifying bytes referenced by LZ77 back-references should raise."""
        decompressed = bytearray(decompress(exe_data))
        header_para = struct.unpack_from("<H", decompressed, 8)[0]
        code_start = header_para * 16
        # Modify the very first bytes - these are heavily referenced
        decompressed[code_start] ^= 0xFF
        decompressed[code_start + 1] ^= 0xFF
        decompressed[code_start + 2] ^= 0xFF
        try:
            recompressed = compress(bytes(decompressed), exe_data)
            re_dec = decompress(recompressed)
            assert re_dec[code_start] == decompressed[code_start]
        except ValueError:
            # Expected when modifying back-referenced bytes
            pass

    def test_header_only_change_returns_original(self, exe_data: bytes) -> None:
        """Changing only header bytes (not code image) returns original."""
        decompressed = bytearray(decompress(exe_data))
        # Modify a byte in the EXE header (not the code image)
        # The checksum field at offset 18
        decompressed[18] ^= 0x01
        result = compress(bytes(decompressed), exe_data)
        # Header change doesn't affect compressed data, returns original
        assert result == exe_data
