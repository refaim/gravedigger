"""ExeTextHandler — text strings from EXE.

Extracts and patches NUL-terminated ASCII text strings embedded in the
decompressed PKLITE EXE. Strings are identified by their code image offsets,
which are hardcoded based on analysis of the Dangerous Dave executable.

The string table covers: UI text, menus, prompts, game messages, win/lose
screens, copyright, configuration text, and error messages.
"""

from __future__ import annotations

import base64
import json
import struct
from typing import TYPE_CHECKING, Any, ClassVar

from gravedigger.compression.pklite import compress, decompress
from gravedigger.core.handler import FormatHandler, Manifest

if TYPE_CHECKING:
    from pathlib import Path


# Each entry: (code_offset, description)
# code_offset is relative to the start of the code image in the decompressed EXE.
_STRING_TABLE: list[tuple[int, str]] = [
    # Help / commands screen
    (0x27242, "help_title"),
    (0x27275, "help_f2"),
    (0x2728B, "help_f3"),
    (0x272A9, "help_f4"),
    (0x272BE, "help_f5"),
    (0x272D0, "help_esc"),
    (0x272DC, "help_tab"),
    (0x272F2, "help_ctrl"),
    (0x27309, "help_down_ctrl"),
    (0x27324, "help_alt"),
    (0x2733B, "help_up_down"),
    (0x27350, "help_left_right"),
    # Memory usage
    (0x27366, "memory_title"),
    # God mode
    (0x273AE, "god_mode_off"),
    (0x273BB, "god_mode_on"),
    # Prompts
    (0x273C7, "warp_prompt"),
    (0x273E1, "sound_prompt"),
    (0x273EE, "reset_game_prompt"),
    (0x27400, "quit_prompt"),
    # Status bar
    (0x2740D, "score_label"),
    (0x27417, "high_score_label"),
    (0x27426, "daves_label"),
    # Disk I/O error
    (0x2742D, "disk_error_line1"),
    (0x2744D, "disk_error_line2"),
    # Tile error
    (0x27473, "tile_error"),
    # Win screen
    (0x27510, "win_congrats"),
    (0x27522, "win_line1"),
    (0x2753F, "win_line2"),
    (0x27554, "win_line3"),
    (0x27564, "win_line4"),
    (0x2757B, "win_line5"),
    (0x27596, "win_line6"),
    (0x275AC, "win_line7"),
    (0x275C2, "win_line8"),
    (0x275DE, "win_line9"),
    (0x275FC, "win_line10"),
    (0x27618, "win_line11"),
    (0x27631, "win_line12"),
    (0x27646, "win_line13"),
    # Level loading
    (0x2765B, "level_1"),
    (0x27664, "level_2"),
    (0x2766D, "level_3"),
    (0x27676, "level_4"),
    (0x2767F, "level_5"),
    (0x27688, "level_6"),
    (0x27691, "level_7"),
    (0x2769A, "level_8"),
    (0x276A3, "loading"),
    # Startup messages
    (0x276C2, "startup_executing"),
    (0x276E1, "startup_joy_yes"),
    (0x276F3, "startup_joy_no"),
    (0x27709, "startup_ega"),
    (0x2771B, "startup_vga"),
    (0x2772D, "startup_no_card"),
    (0x27778, "startup_no_card_prompt"),
    # Debug / ted
    (0x277B5, "ted_exit"),
    (0x277C8, "bad_react"),
    (0x277D9, "object_overflow"),
    # Game over screen
    (0x277F5, "game_over"),
    (0x27807, "new_high_score"),
    (0x27829, "game_over_congrats"),
    (0x2783C, "continue_level"),
    # Copyright and info
    (0x29846, "copyright"),
    (0x29868, "info_hint"),
    # Gamer's Edge ad
    (0x298AE, "ad_line1"),
    (0x298C7, "ad_line2"),
    (0x298E1, "ad_line3"),
    (0x298F9, "ad_line4"),
    (0x29912, "ad_line5"),
    (0x2992A, "ad_line6"),
    # Joystick configuration
    (0x29A46, "joy_config_title"),
    (0x29A76, "joy_hold_line1"),
    (0x29A90, "joy_upper_left"),
    (0x29AA7, "joy_press_button"),
    (0x29AD3, "joy_lower_right"),
    (0x29AEB, "joy_press_button2"),
    (0x29AFB, "joy_button_choice"),
    # Keyboard configuration
    (0x29BA5, "kbd_config_title"),
    (0x29C0B, "kbd_modify_action"),
    (0x29C28, "kbd_press_key"),
    # Error messages
    (0x29C3E, "bload_error"),
    (0x29C5F, "huff_error"),
    (0x29C87, "rlew_error"),
]


def _read_nul_string(data: bytes, offset: int) -> str:
    """Read a NUL-terminated ASCII string from data at offset."""
    end = data.index(0, offset)
    return data[offset:end].decode("ascii")


class ExeTextHandler(FormatHandler):
    """Handler for text strings embedded in the game EXE."""

    file_patterns: ClassVar[list[str]] = ["DAVE.EXE", "1.EXE"]

    def unpack(self, input_path: Path, translatable_dir: Path, meta_dir: Path) -> Manifest:
        exe_data = input_path.read_bytes()
        decompressed = decompress(exe_data)

        header_para = struct.unpack_from("<H", decompressed, 8)[0]
        code_start = header_para * 16

        strings: list[dict[str, Any]] = []
        for code_offset, name in _STRING_TABLE:
            abs_offset = code_start + code_offset
            text = _read_nul_string(decompressed, abs_offset)
            strings.append(
                {
                    "id": name,
                    "offset": code_offset,
                    "text": text,
                    "max_length": len(text),
                }
            )

        strings_data = {"strings": strings}
        (translatable_dir / "strings.json").write_text(
            json.dumps(strings_data, indent=2, ensure_ascii=False)
        )

        metadata: dict[str, Any] = {
            "original_exe": base64.b64encode(exe_data).decode(),
        }

        manifest = Manifest(
            handler="ExeTextHandler",
            source_file=input_path.name,
            metadata=metadata,
        )
        manifest.to_json(meta_dir / "manifest.json")
        return manifest

    def repack(
        self, manifest: Manifest, translatable_dir: Path, meta_dir: Path, output_path: Path
    ) -> None:
        meta = manifest.metadata
        original_exe = base64.b64decode(meta["original_exe"])
        decompressed = bytearray(decompress(original_exe))

        header_para = struct.unpack_from("<H", decompressed, 8)[0]
        code_start = header_para * 16

        strings_data = json.loads((translatable_dir / "strings.json").read_text())

        for entry in strings_data["strings"]:
            code_offset: int = entry["offset"]
            new_text: str = entry["text"]
            max_length: int = entry["max_length"]

            encoded = new_text.encode("ascii")
            if len(encoded) > max_length:
                msg = (
                    f"String {entry['id']!r} ({len(encoded)} bytes) "
                    f"exceeds maximum length ({max_length} bytes)"
                )
                raise ValueError(msg)

            abs_offset = code_start + code_offset

            # Write new string + NUL padding up to original length + NUL terminator
            pad_len = max_length - len(encoded)
            decompressed[abs_offset : abs_offset + max_length + 1] = encoded + b"\x00" * (
                pad_len + 1
            )

        result = compress(bytes(decompressed), original_exe)
        output_path.write_bytes(result)
