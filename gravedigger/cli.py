from __future__ import annotations

import argparse
import sys
from pathlib import Path

from gravedigger.core.handler import Manifest
from gravedigger.core.registry import Registry
from gravedigger.handlers.ctlpanel import CtlPanelHandler
from gravedigger.handlers.exe_death import ExeDeathHandler
from gravedigger.handlers.exe_text import ExeTextHandler
from gravedigger.handlers.intro import IntroHandler
from gravedigger.handlers.level import LevelHandler
from gravedigger.handlers.pic import PicHandler
from gravedigger.handlers.sprites import SpriteHandler
from gravedigger.handlers.tiles import TileHandler

_EXE_SUFFIXES = {".EXE"}


def _build_registry() -> Registry:
    reg = Registry()
    reg.register(PicHandler())
    reg.register(SpriteHandler())
    reg.register(TileHandler())
    reg.register(LevelHandler())
    reg.register(IntroHandler())
    reg.register(CtlPanelHandler())
    reg.register(ExeDeathHandler())
    reg.register(ExeTextHandler())
    return reg


def _cmd_unpack(args: argparse.Namespace) -> None:
    game_dir = Path(args.input_dir)
    output_dir = Path(args.output_dir)

    if not game_dir.is_dir():
        print(f"Error: input directory does not exist: {game_dir}", file=sys.stderr)
        sys.exit(1)

    game_files = sorted(
        p for p in game_dir.iterdir() if p.suffix.upper() in (".DD2", *_EXE_SUFFIXES)
    )
    if not game_files:
        print(f"Error: no game files found in {game_dir}", file=sys.stderr)
        sys.exit(1)

    output_dir.mkdir(parents=True, exist_ok=True)
    registry = _build_registry()

    for game_file in game_files:
        handlers = registry.get_handlers(game_file.name)
        if not handlers:
            print(f"Skipping {game_file.name}: no handler registered")
            continue

        if len(handlers) == 1:
            file_output_dir = output_dir / game_file.stem
            file_output_dir.mkdir(parents=True, exist_ok=True)
            handlers[0].unpack(game_file, file_output_dir)
        else:
            for handler in handlers:
                handler_name = type(handler).__name__
                file_output_dir = output_dir / game_file.stem / handler_name
                file_output_dir.mkdir(parents=True, exist_ok=True)
                handler.unpack(game_file, file_output_dir)

        print(f"Unpacked {game_file.name}")


def _cmd_repack(args: argparse.Namespace) -> None:
    input_dir = Path(args.input_dir)
    output_dir = Path(args.output_dir)

    if not input_dir.is_dir():
        print(f"Error: input directory does not exist: {input_dir}", file=sys.stderr)
        sys.exit(1)

    manifest_files = sorted(input_dir.rglob("manifest.json"))
    if not manifest_files:
        print(f"Error: no manifest.json files found in {input_dir}", file=sys.stderr)
        sys.exit(1)

    output_dir.mkdir(parents=True, exist_ok=True)
    registry = _build_registry()

    # Group manifests by source file for chained repack (e.g. multiple EXE handlers)
    by_source: dict[str, list[tuple[Manifest, Path]]] = {}
    for manifest_path in manifest_files:
        manifest = Manifest.from_json(manifest_path)
        by_source.setdefault(manifest.source_file, []).append((manifest, manifest_path.parent))

    for source_file, entries in sorted(by_source.items()):
        # Filter to entries with registered handlers
        valid_entries: list[tuple[Manifest, Path]] = []
        for manifest, manifest_dir in entries:
            try:
                registry.get_handler_by_name(manifest.handler)
                valid_entries.append((manifest, manifest_dir))
            except KeyError:
                print(f"Skipping {manifest.handler} for {source_file}: no handler registered")

        if not valid_entries:
            continue

        output_path = output_dir / source_file

        if len(valid_entries) == 1:
            manifest, manifest_dir = valid_entries[0]
            handler = registry.get_handler_by_name(manifest.handler)
            handler.repack(manifest, manifest_dir, output_path)
        else:
            _repack_chained(valid_entries, registry, output_path)

        print(f"Repacked {source_file}")


def _repack_chained(
    entries: list[tuple[Manifest, Path]],
    registry: Registry,
    output_path: Path,
) -> None:
    """Repack multiple handlers for the same source file sequentially.

    Each handler repacks to a temporary file, and subsequent handlers
    use the previous output as their original EXE template.
    """
    import base64
    import tempfile

    current_exe: bytes | None = None

    for manifest, manifest_dir in entries:
        handler = registry.get_handler_by_name(manifest.handler)

        if current_exe is not None:
            # Update the manifest's original_exe to use the previous handler's output
            manifest.metadata["original_exe"] = base64.b64encode(current_exe).decode()

        with tempfile.NamedTemporaryFile(delete=False, suffix=".exe") as tmp:
            tmp_path = Path(tmp.name)

        try:
            handler.repack(manifest, manifest_dir, tmp_path)
            current_exe = tmp_path.read_bytes()
        finally:
            tmp_path.unlink(missing_ok=True)

    if current_exe is not None:
        output_path.write_bytes(current_exe)


def main() -> None:
    parser = argparse.ArgumentParser(
        prog="gravedigger",
        description="Resource unpacker/repacker for Dangerous Dave in the Haunted Mansion",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    unpack_parser = subparsers.add_parser("unpack", help="Unpack game files to editable formats")
    unpack_parser.add_argument("input_dir", help="Directory containing game files (.DD2, .EXE)")
    unpack_parser.add_argument("output_dir", help="Output directory for unpacked files")

    repack_parser = subparsers.add_parser("repack", help="Repack edited files back to .DD2")
    repack_parser.add_argument("input_dir", help="Directory with unpacked files and manifests")
    repack_parser.add_argument("output_dir", help="Output directory for repacked .DD2 files")

    args = parser.parse_args()

    if args.command == "unpack":
        _cmd_unpack(args)
    elif args.command == "repack":
        _cmd_repack(args)


if __name__ == "__main__":
    main()
