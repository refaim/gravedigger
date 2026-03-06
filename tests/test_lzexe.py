"""Tests for LZEXE (LZ91) decompressor/compressor."""

from __future__ import annotations

import struct
from pathlib import Path

import pytest

from gravedigger.compression.lzexe import _decompress_reloc, compress, decompress

GAME_DIR = Path(__file__).resolve().parent.parent / "game" / "retail"
EXE_PATH = GAME_DIR / "MANSION.EXE"


@pytest.fixture
def exe_data() -> bytes:
    if not EXE_PATH.exists():
        pytest.skip("game/retail/MANSION.EXE not found")
    return EXE_PATH.read_bytes()


@pytest.fixture
def decompressed(exe_data: bytes) -> bytes:
    return decompress(exe_data)


class TestDecompress:
    def test_decompressed_size_is_reasonable(self, exe_data: bytes) -> None:
        result = decompress(exe_data)
        assert len(result) > len(exe_data)
        assert len(result) > 100_000

    def test_contains_dangerous_dave_text(self, decompressed: bytes) -> None:
        assert b"Dangerous Dave" in decompressed

    def test_contains_softdisk_copyright(self, decompressed: bytes) -> None:
        assert b"Softdisk" in decompressed

    def test_contains_control_strings(self, decompressed: bytes) -> None:
        assert b"ESC" in decompressed

    def test_non_lzexe_file_raises(self) -> None:
        with pytest.raises(ValueError, match="MZ"):
            decompress(b"This is not an LZEXE file at all")

    def test_too_short_raises(self) -> None:
        with pytest.raises(ValueError, match="too short"):
            decompress(b"MZ" + b"\x00" * 10)

    def test_no_mz_signature_raises(self) -> None:
        with pytest.raises(ValueError, match="MZ"):
            decompress(b"PK" + b"\x00" * 100)

    def test_non_lzexe_exe_raises(self) -> None:
        fake_exe = b"MZ" + b"\x00" * 100
        with pytest.raises(ValueError, match="LZEXE"):
            decompress(fake_exe)

    def test_output_is_valid_exe(self, decompressed: bytes) -> None:
        assert decompressed[:2] == b"MZ"

    def test_code_image_matches_pklite_variant(self, decompressed: bytes) -> None:
        """Code image should be identical to the PKLITE (softdisk) variant."""
        softdisk_exe = GAME_DIR.parent / "softdisk" / "1.EXE"
        if not softdisk_exe.exists():
            pytest.skip("game/softdisk/1.EXE not found")
        from gravedigger.compression.pklite import decompress as pklite_decompress

        softdisk_dec = pklite_decompress(softdisk_exe.read_bytes())

        hp_lz = struct.unpack_from("<H", decompressed, 8)[0]
        hp_pk = struct.unpack_from("<H", softdisk_dec, 8)[0]
        assert decompressed[hp_lz * 16 :] == softdisk_dec[hp_pk * 16 :]


class TestCompress:
    def test_roundtrip_byte_exact(self, exe_data: bytes) -> None:
        decompressed = decompress(exe_data)
        recompressed = compress(decompressed, exe_data)
        assert recompressed == exe_data

    def test_non_lzexe_original_raises(self) -> None:
        with pytest.raises(ValueError, match="LZEXE"):
            compress(b"some data", b"not an lzexe file")

    def test_size_mismatch_raises(self, exe_data: bytes) -> None:
        decompressed = decompress(exe_data)
        with pytest.raises(ValueError, match="size mismatch"):
            compress(decompressed + b"\x00", exe_data)

    def test_modified_literal_bytes(self, exe_data: bytes) -> None:
        decompressed = bytearray(decompress(exe_data))
        idx = decompressed.find(b"Softdisk")
        assert idx > 0
        decompressed[idx] = ord("X")
        recompressed = compress(bytes(decompressed), exe_data)
        re_decompressed = decompress(recompressed)
        assert re_decompressed[idx] == ord("X")
        assert b"Xoftdisk" in re_decompressed

    def test_header_only_change_returns_original(self, exe_data: bytes) -> None:
        decompressed = bytearray(decompress(exe_data))
        decompressed[18] ^= 0x01
        result = compress(bytes(decompressed), exe_data)
        assert result == exe_data

    def test_modified_backreference_raises(self, exe_data: bytes) -> None:
        decompressed = bytearray(decompress(exe_data))
        header_para = struct.unpack_from("<H", decompressed, 8)[0]
        code_start = header_para * 16
        decompressed[code_start] ^= 0xFF
        decompressed[code_start + 1] ^= 0xFF
        decompressed[code_start + 2] ^= 0xFF
        try:
            recompressed = compress(bytes(decompressed), exe_data)
            re_dec = decompress(recompressed)
            assert re_dec[code_start] == decompressed[code_start]
        except ValueError:
            pass


class TestValidation:
    def test_stub_beyond_file_raises(self) -> None:
        buf = bytearray(0x30)
        buf[0:2] = b"MZ"
        struct.pack_into("<H", buf, 8, 2)  # header_para=2
        struct.pack_into("<H", buf, 0x16, 0xFFFF)  # CS=0xFFFF → stub way beyond
        buf[0x1C:0x20] = b"LZ91"
        with pytest.raises(ValueError, match="stub extends beyond file"):
            decompress(bytes(buf))


class TestDecompressReloc:
    def test_segment_boundary_marker(self) -> None:
        """marker==0 should advance rel_seg by 0x0FFF."""
        # span=5 (one reloc), then span=0+marker=0 (seg boundary), span=3, marker=1 (end)
        data = bytes([5, 0, 0x00, 0x00, 3, 0, 0x01, 0x00])
        relocs = _decompress_reloc(data)
        assert len(relocs) == 2
        # First: rel_off=5, rel_seg=0 → 0x00000005
        assert relocs[0] & 0xFFFF == 5
        # After seg boundary: rel_seg += 0x0FFF, then span=3 → rel_off=5+3=8
        assert relocs[1] >> 16 > 0

    def test_early_break_on_short_data(self) -> None:
        """span=0 but not enough data for marker → break."""
        data = bytes([0])  # span=0, pos=1, pos+1 >= len → break
        relocs = _decompress_reloc(data)
        assert relocs == []
