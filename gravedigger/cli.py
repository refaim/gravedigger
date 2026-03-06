from __future__ import annotations

import argparse
import shutil
import sys
from pathlib import Path

from gravedigger import __version__
from gravedigger.core.handler import Manifest
from gravedigger.core.registry import Registry
from gravedigger.handlers.exe_text import ExeTextHandler
from gravedigger.handlers.intro import IntroHandler
from gravedigger.handlers.pic import PicHandler
from gravedigger.handlers.sprites import SpriteHandler

_EXE_SUFFIXES = {".EXE"}
_GAME_SUFFIXES = {".DD2", *_EXE_SUFFIXES}
_TRANSLATABLE = "translatable"
_META = "meta"


def _build_registry() -> Registry:
    reg = Registry()
    reg.register(PicHandler())
    reg.register(SpriteHandler())
    reg.register(IntroHandler())
    reg.register(ExeTextHandler())
    return reg


def _cmd_unpack(args: argparse.Namespace) -> None:
    game_dir = Path(args.input_dir)
    output_dir = Path(args.output_dir)

    if not game_dir.is_dir():
        print(f"Error: input directory does not exist: {game_dir}", file=sys.stderr)
        sys.exit(1)

    game_files = sorted(p for p in game_dir.iterdir() if p.suffix.upper() in _GAME_SUFFIXES)
    if not game_files:
        print(f"Error: no game files found in {game_dir}", file=sys.stderr)
        sys.exit(1)

    translatable_root = output_dir / _TRANSLATABLE
    meta_root = output_dir / _META
    translatable_root.mkdir(parents=True, exist_ok=True)
    meta_root.mkdir(parents=True, exist_ok=True)

    registry = _build_registry()

    for game_file in game_files:
        handlers = registry.get_handlers(game_file.name)

        if not handlers:
            # No handler — copy original to meta/ for full game restoration
            shutil.copy2(game_file, meta_root / game_file.name)
            print(f"Copied {game_file.name} (no handler)")
            continue

        handler = handlers[0]
        meta_dir = meta_root / game_file.stem
        meta_dir.mkdir(parents=True, exist_ok=True)
        handler.unpack(game_file, translatable_root, meta_dir)

        print(f"Unpacked {game_file.name}")


def _cmd_repack(args: argparse.Namespace) -> None:
    input_dir = Path(args.input_dir)
    output_dir = Path(args.output_dir)

    if not input_dir.is_dir():
        print(f"Error: input directory does not exist: {input_dir}", file=sys.stderr)
        sys.exit(1)

    meta_root = input_dir / _META
    translatable_root = input_dir / _TRANSLATABLE

    if not meta_root.is_dir():
        print(f"Error: meta directory not found in {input_dir}", file=sys.stderr)
        sys.exit(1)

    manifest_files = sorted(meta_root.rglob("manifest.json"))

    # Original files (no handler) are stored directly in meta/ as regular files
    originals = sorted(p for p in meta_root.iterdir() if p.is_file())

    if not manifest_files and not originals:
        print(f"Error: no manifest.json files or originals found in {meta_root}", file=sys.stderr)
        sys.exit(1)

    output_dir.mkdir(parents=True, exist_ok=True)
    registry = _build_registry()

    # Copy original files (levels, unhandled files) directly
    for original in originals:
        shutil.copy2(original, output_dir / original.name)
        print(f"Copied {original.name}")

    for manifest_path in manifest_files:
        manifest = Manifest.from_json(manifest_path)
        try:
            handler = registry.get_handler_by_name(manifest.handler)
        except KeyError:
            print(f"Skipping {manifest.handler} for {manifest.source_file}: no handler registered")
            continue

        output_path = output_dir / manifest.source_file
        handler.repack(manifest, translatable_root, manifest_path.parent, output_path)
        print(f"Repacked {manifest.source_file}")


def main() -> None:
    parser = argparse.ArgumentParser(
        prog="gravedigger",
        description="Resource unpacker/repacker for Dangerous Dave in the Haunted Mansion",
    )
    parser.add_argument("-V", "--version", action="version", version=f"%(prog)s {__version__}")
    subparsers = parser.add_subparsers(dest="command", required=True)

    unpack_parser = subparsers.add_parser("unpack", help="Unpack game files to editable formats")
    unpack_parser.add_argument("input_dir", help="Directory containing game files (.DD2, .EXE)")
    unpack_parser.add_argument("output_dir", help="Output directory for unpacked files")

    repack_parser = subparsers.add_parser("repack", help="Repack edited files back to .DD2")
    repack_parser.add_argument("input_dir", help="Directory with translatable/ and meta/")
    repack_parser.add_argument("output_dir", help="Output directory for repacked .DD2 files")

    args = parser.parse_args()

    commands = {"unpack": _cmd_unpack, "repack": _cmd_repack}
    commands[args.command](args)


if __name__ == "__main__":
    main()
