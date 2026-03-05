"""Tests for the RLEW codec (Run-Length Encoding on Words)."""

import struct
from pathlib import Path

import pytest

from gravedigger.compression.rlew import compress, decompress

MARKER = 0xFEFE


class TestDecompressManualVector:
    """Test decompress with hand-crafted RLEW data."""

    def test_single_run(self) -> None:
        # marker(0xFEFE) + count(3) + value(0x0042) -> 0x0042 0x0042 0x0042
        data = struct.pack("<HHH", MARKER, 3, 0x0042)
        result = decompress(data)
        expected = struct.pack("<HHH", 0x0042, 0x0042, 0x0042)
        assert result == expected

    def test_literal_words(self) -> None:
        data = struct.pack("<HH", 0x0001, 0x0002)
        result = decompress(data)
        assert result == data

    def test_mixed_literal_and_run(self) -> None:
        # literal 0x0001, then run of 2x 0x00FF, then literal 0x0003
        data = struct.pack("<H HHH H", 0x0001, MARKER, 2, 0x00FF, 0x0003)
        result = decompress(data)
        expected = struct.pack("<HHHH", 0x0001, 0x00FF, 0x00FF, 0x0003)
        assert result == expected

    def test_run_of_marker_value(self) -> None:
        # Encoding a run of the marker value itself: marker + count + marker
        data = struct.pack("<HHH", MARKER, 2, MARKER)
        result = decompress(data)
        expected = struct.pack("<HH", MARKER, MARKER)
        assert result == expected

    def test_empty_input(self) -> None:
        assert decompress(b"") == b""

    def test_run_count_zero(self) -> None:
        # A run with count=0 produces nothing
        data = struct.pack("<HHH H", MARKER, 0, 0x0042, 0x0001)
        result = decompress(data)
        expected = struct.pack("<H", 0x0001)
        assert result == expected


class TestDecompressLevel01:
    """Test decompress on real LEVEL01.DD2 game file."""

    @pytest.fixture()
    def level01_raw(self, game_dir: Path) -> bytes:
        return (game_dir / "LEVEL01.DD2").read_bytes()

    @pytest.fixture()
    def level01_rlew(self, level01_raw: bytes) -> bytes:
        # LEVEL*.DD2 file format wraps RLEW data with a 4-byte decompressed size
        # header (uint32 LE) and a 5-byte "MsDos" signature trailer. These are
        # part of the level file format, not RLEW itself. The LevelHandler is
        # responsible for stripping/restoring them; here we do it manually to
        # test the pure RLEW codec in isolation.
        return level01_raw[4:-5]

    def test_decompressed_size(self, level01_raw: bytes, level01_rlew: bytes) -> None:
        # First 4 bytes = expected decompressed size (uint32 LE)
        expected_size = struct.unpack_from("<I", level01_raw, 0)[0]
        result = decompress(level01_rlew)
        assert len(result) == expected_size

    def test_width_height(self, level01_rlew: bytes) -> None:
        result = decompress(level01_rlew)
        # Level data starts with width and height as uint16 LE
        width, height = struct.unpack_from("<HH", result, 0)
        assert width == 64
        assert height == 57


class TestCompressDecompressRoundtrip:
    """Test that compress -> decompress is identity."""

    def test_roundtrip_simple(self) -> None:
        original = struct.pack("<HHHH", 0x0001, 0x0001, 0x0001, 0x0002)
        assert decompress(compress(original)) == original

    def test_roundtrip_with_marker_value(self) -> None:
        # Data that contains the marker value as literal data
        original = struct.pack("<HHH", MARKER, MARKER, MARKER)
        assert decompress(compress(original)) == original

    def test_roundtrip_random_words(self) -> None:
        import random

        rng = random.Random(42)
        words = [rng.randint(0, 0xFFFF) for _ in range(200)]
        original = struct.pack(f"<{len(words)}H", *words)
        assert decompress(compress(original)) == original


class TestByteExactRoundtrip:
    """Test compress(decompress(file)) reproduces the original RLEW data."""

    @pytest.fixture()
    def level01_raw(self, game_dir: Path) -> bytes:
        return (game_dir / "LEVEL01.DD2").read_bytes()

    def test_level01_byte_exact(self, level01_raw: bytes) -> None:
        # Strip level file wrapper (see TestDecompressLevel01.level01_rlew comment)
        rlew_data = level01_raw[4:-5]
        decompressed = decompress(rlew_data)
        recompressed = compress(decompressed)
        assert recompressed == rlew_data


class TestValidation:
    def test_decompress_odd_length_raises(self) -> None:
        with pytest.raises(ValueError, match="even length"):
            decompress(b"\x01\x02\x03")

    def test_compress_odd_length_raises(self) -> None:
        with pytest.raises(ValueError, match="even length"):
            compress(b"\x01\x02\x03")


class TestPassthrough:
    """Data without 0xFEFE markers passes through unchanged."""

    def test_no_markers(self) -> None:
        # Words that are not the marker
        data = struct.pack("<HHHH", 0x0001, 0x0002, 0x0003, 0x0004)
        result = decompress(data)
        assert result == data

    def test_compress_no_runs(self) -> None:
        # All unique words — compress should not insert markers
        data = struct.pack("<HHHH", 0x0001, 0x0002, 0x0003, 0x0004)
        compressed = compress(data)
        # No runs to encode, but marker value must still be handled
        assert decompress(compressed) == data
