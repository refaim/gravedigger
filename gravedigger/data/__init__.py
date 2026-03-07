"""Bundled data files for Gravedigger."""

from importlib import resources


def load_cp866_font() -> bytes:
    """Load the CP866 VGA 8x16 font (4096 bytes)."""
    return (resources.files(__package__) / "cp866_8x16.fnt").read_bytes()
