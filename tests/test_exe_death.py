"""Tests for ExeDeathHandler — death sequences from EXE."""

from __future__ import annotations

import struct
from pathlib import Path

import pytest
from PIL import Image

from gravedigger.compression.pklite import decompress
from gravedigger.handlers.exe_death import ExeDeathHandler

GAME_DIR = Path(__file__).resolve().parent.parent / "game"
EXE_PATH = GAME_DIR / "1.EXE"

SEQUENCE_COUNT = 8
FRAMES_PER_SEQUENCE = 5
TOTAL_FRAMES = SEQUENCE_COUNT * FRAMES_PER_SEQUENCE
FRAME_WIDTH = 48
FRAME_HEIGHT = 48


@pytest.fixture()
def handler() -> ExeDeathHandler:
    return ExeDeathHandler()


@pytest.fixture()
def exe_path() -> Path:
    if not EXE_PATH.exists():
        pytest.skip("game/1.EXE not found")
    return EXE_PATH


@pytest.fixture()
def unpacked_dir(handler: ExeDeathHandler, exe_path: Path, tmp_path: Path) -> Path:
    out = tmp_path / "unpacked"
    out.mkdir()
    handler.unpack(exe_path, out)
    return out


class TestUnpack:
    def test_produces_40_png_files(self, unpacked_dir: Path) -> None:
        """Unpack should produce 40 PNG files (8 sequences x 5 frames)."""
        pngs = sorted(unpacked_dir.glob("*.png"))
        assert len(pngs) == TOTAL_FRAMES

    def test_file_naming(self, unpacked_dir: Path) -> None:
        """Files should be named death_seq{N}_frame{M}.png."""
        for seq in range(1, SEQUENCE_COUNT + 1):
            for frame in range(1, FRAMES_PER_SEQUENCE + 1):
                expected = unpacked_dir / f"death_seq{seq}_frame{frame}.png"
                assert expected.exists(), f"Missing {expected.name}"

    def test_frame_dimensions(self, unpacked_dir: Path) -> None:
        """Each frame should be 48x48 pixels."""
        for png in sorted(unpacked_dir.glob("*.png")):
            img = Image.open(png)
            assert img.size == (FRAME_WIDTH, FRAME_HEIGHT), f"{png.name} has wrong size"

    def test_frames_are_paletted(self, unpacked_dir: Path) -> None:
        """Each frame should be a paletted (mode P) image."""
        first = Image.open(unpacked_dir / "death_seq1_frame1.png")
        assert first.mode == "P"

    def test_manifest_metadata(self, unpacked_dir: Path) -> None:
        """Manifest should contain correct metadata."""
        from gravedigger.core.handler import Manifest

        manifest = Manifest.from_json(unpacked_dir / "manifest.json")
        assert manifest.handler == "ExeDeathHandler"
        assert manifest.metadata["sequence_count"] == SEQUENCE_COUNT
        assert manifest.metadata["frames_per_sequence"] == FRAMES_PER_SEQUENCE
        assert manifest.metadata["frame_width"] == FRAME_WIDTH
        assert manifest.metadata["frame_height"] == FRAME_HEIGHT

    def test_first_frame_not_blank(self, unpacked_dir: Path) -> None:
        """First frame should have non-zero pixel data (not all black)."""
        img = Image.open(unpacked_dir / "death_seq1_frame1.png")
        pixels = list(img.tobytes())
        assert any(p != 0 for p in pixels), "First frame is all black"

    def test_first_frame_pixel_values(self, unpacked_dir: Path) -> None:
        """First frame should have correct pixel values at known positions."""
        img = Image.open(unpacked_dir / "death_seq1_frame1.png")
        pixels = list(img.tobytes())
        w = FRAME_WIDTH
        # Reference values extracted from the original EXE
        assert pixels[0] == 8  # (0,0) dark gray
        assert pixels[1] == 7  # (1,0) light gray
        assert pixels[4] == 15  # (4,0) white
        assert pixels[47] == 8  # (47,0) dark gray
        assert pixels[w] == 7  # (0,1) light gray
        assert pixels[24 * w + 23] == 3  # (23,24) dark cyan
        assert pixels[47 * w + 47] == 7  # (47,47) light gray

    def test_frames_have_varied_content(self, unpacked_dir: Path) -> None:
        """Different sequences should have different pixel data."""
        img1 = Image.open(unpacked_dir / "death_seq1_frame1.png")
        img2 = Image.open(unpacked_dir / "death_seq2_frame1.png")
        assert img1.tobytes() != img2.tobytes()


class TestRepack:
    def test_roundtrip_byte_exact(
        self, handler: ExeDeathHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        """Unpack then repack should produce byte-exact EXE."""
        unpack_dir = tmp_path / "unpack"
        unpack_dir.mkdir()
        manifest = handler.unpack(exe_path, unpack_dir)

        repack_path = tmp_path / "repacked.exe"
        handler.repack(manifest, unpack_dir, repack_path)

        # Compare the death sequence block in decompressed EXEs
        original_dec = decompress(exe_path.read_bytes())
        repacked_dec = decompress(repack_path.read_bytes())

        header_para = struct.unpack_from("<H", original_dec, 8)[0]
        code_start = header_para * 16
        death_offset = code_start + 0x18A60
        block_size = 0x33B0 * 4

        orig_block = original_dec[death_offset : death_offset + block_size]
        repacked_block = repacked_dec[death_offset : death_offset + block_size]
        assert orig_block == repacked_block

    def test_full_exe_roundtrip(
        self, handler: ExeDeathHandler, exe_path: Path, tmp_path: Path
    ) -> None:
        """Full EXE roundtrip: compressed output should match original."""
        unpack_dir = tmp_path / "unpack"
        unpack_dir.mkdir()
        manifest = handler.unpack(exe_path, unpack_dir)

        repack_path = tmp_path / "repacked.exe"
        handler.repack(manifest, unpack_dir, repack_path)

        assert repack_path.read_bytes() == exe_path.read_bytes()


class TestFilePatterns:
    def test_file_patterns(self, handler: ExeDeathHandler) -> None:
        """Handler should match known EXE file patterns."""
        assert "DAVE.EXE" in handler.file_patterns
        assert "1.EXE" in handler.file_patterns
