import struct
from pathlib import Path

import pytest

from gravedigger.compression.huff import compress, decompress

HUFF_FILES = [
    "TITLE1.DD2",
    "TITLE2.DD2",
    "PROGPIC.DD2",
    "STARPIC.DD2",
    "S_DAVE.DD2",
    "S_CHUNK1.DD2",
    "S_CHUNK2.DD2",
    "S_FRANK.DD2",
    "S_MASTER.DD2",
]


class TestDecompress:
    def test_title1_size_and_magic(self, game_dir: Path) -> None:
        raw = (game_dir / "TITLE1.DD2").read_bytes()
        data, _tree = decompress(raw)
        expected_size = struct.unpack_from("<I", raw, 4)[0]
        assert len(data) == expected_size
        assert data[:4] == b"PIC\x00"

    def test_s_dave_size(self, game_dir: Path) -> None:
        raw = (game_dir / "S_DAVE.DD2").read_bytes()
        data, _tree = decompress(raw)
        expected_size = struct.unpack_from("<I", raw, 4)[0]
        assert len(data) == expected_size

    def test_no_huff_signature_raises(self) -> None:
        with pytest.raises(ValueError, match="HUFF"):
            decompress(b"NOTAHUFFFILE" + b"\x00" * 100)

    def test_tree_starts_with_1020_bytes(self, game_dir: Path) -> None:
        raw = (game_dir / "TITLE1.DD2").read_bytes()
        _data, tree_and_tail = decompress(raw)
        assert len(tree_and_tail) >= 1020
        assert tree_and_tail[:1020] == raw[8 : 8 + 1020]


class TestCompress:
    @pytest.mark.parametrize("filename", HUFF_FILES)
    def test_byte_exact_roundtrip(self, game_dir: Path, filename: str) -> None:
        """compress(decompress(file)) must reproduce the original file exactly."""
        path = game_dir / filename
        if not path.exists():
            pytest.skip(f"{filename} not found")
        raw = path.read_bytes()
        data, tree = decompress(raw)
        recompressed = compress(data, tree)
        assert recompressed == raw, f"Byte-exact roundtrip failed for {filename}"


class TestCompressDecompressRoundtrip:
    def test_roundtrip_via_real_tree(self, game_dir: Path) -> None:
        """Decompress a file, modify data slightly, compress, decompress again."""
        raw = (game_dir / "TITLE1.DD2").read_bytes()
        data, tree = decompress(raw)
        modified = bytearray(data)
        modified[10] = (modified[10] + 1) % 256
        modified[100] = (modified[100] + 3) % 256
        compressed = compress(bytes(modified), tree)
        result, _tree2 = decompress(compressed)
        assert result == bytes(modified)


class TestEdgeCases:
    def test_decompress_zero_size(self, game_dir: Path) -> None:
        """File with unpacked_size=0 returns empty data."""
        raw = (game_dir / "TITLE1.DD2").read_bytes()
        # Patch unpacked_size to 0
        patched = raw[:4] + struct.pack("<I", 0) + raw[8:]
        data, tree = decompress(patched)
        assert data == b""
        assert len(tree) >= 1020

    def test_compress_empty_data_no_tail(self, game_dir: Path) -> None:
        """Compress empty data with tree that has no tail."""
        raw = (game_dir / "TITLE1.DD2").read_bytes()
        _data, tree_and_tail = decompress(raw)
        # Use only the tree (1020 bytes), no tail
        tree_only = tree_and_tail[:1020]
        result = compress(b"", tree_only)
        # Should produce HUFF header + size(0) + tree + no bitstream
        assert result[:4] == b"HUFF"
        size = struct.unpack_from("<I", result, 4)[0]
        assert size == 0

    def test_decompress_truncated_data_raises(self, game_dir: Path) -> None:
        """Truncated compressed bitstream should raise ValueError."""
        raw = (game_dir / "TITLE1.DD2").read_bytes()
        # Keep header + tree but truncate compressed data to just 1 byte
        truncated = raw[:1028] + raw[1028:1029]
        with pytest.raises((ValueError, IndexError)):
            decompress(truncated)

    def test_compress_unknown_byte_raises(self, game_dir: Path) -> None:
        """Compressing a byte not in the Huffman tree should raise ValueError."""
        raw = (game_dir / "TITLE1.DD2").read_bytes()
        _data, tree_and_tail = decompress(raw)
        # Build a minimal tree with only 2 entries by zeroing out the tree
        # Actually, DD2 trees cover all 256 values so we need a synthetic tree.
        # Create a tree where only byte 0x00 is a leaf.
        # Simple approach: use a valid tree but corrupt it so some codes are missing.
        import struct as st

        tree_bytes = bytearray(tree_and_tail[:1020])
        # Zero out all entries to make most bytes unreachable
        for i in range(0, 1020, 2):
            st.pack_into("<H", tree_bytes, i, 0)
        # Set root (entry 254) left=0x00 (leaf), right=0x01 (leaf)
        root_offset = 254 * 2 * 2  # 254 * 4 = 1016
        st.pack_into("<H", tree_bytes, root_offset, 0x00)  # left = byte 0
        st.pack_into("<H", tree_bytes, root_offset + 2, 0x01)  # right = byte 1

        with pytest.raises(ValueError, match="not found in Huffman tree"):
            compress(b"\x42", bytes(tree_bytes))

    def test_compress_partial_byte_no_tail(self, game_dir: Path) -> None:
        """Compress data that ends on a partial byte with no trailing data."""
        raw = (game_dir / "TITLE1.DD2").read_bytes()
        _data, tree_and_tail = decompress(raw)
        tree_only = tree_and_tail[:1020]
        # Compress a single byte — likely won't fill a full byte of bitstream
        result = compress(b"\x00", tree_only)
        assert result[:4] == b"HUFF"
        recovered, _ = decompress(result)
        assert recovered == b"\x00"
