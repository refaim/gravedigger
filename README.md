# Gravedigger

Resource unpacker/repacker for **Dangerous Dave in the Haunted Mansion** (DOS, 1991).

Extracts game assets (graphics, text, sprites, tiles) into editable formats and repacks them back, enabling fan translations and modding.

## Usage

```
gravedigger unpack <game_dir> <output_dir>
gravedigger repack <output_dir> <repacked_dir>
```

`unpack` reads original game files (`.DD2`, `.EXE`) and produces editable PNGs + metadata.
`repack` takes the edited assets and rebuilds game files.

## Installation

Download a standalone binary from [GitHub Releases](../../releases).

Or run from source (Python 3.13+):

```
uv sync
uv run gravedigger --help
```

## Development

```
make lint    # ruff + mypy
make test    # pytest (100% coverage required)
make build   # standalone binary via nuitka
```

## Acknowledgements

- [dangerous-dave-re](https://github.com/gmegidish/dangerous-dave-re) by Gil Megidish — reverse-engineered game format documentation and reference implementation
- [gamecompjs](https://github.com/camoto-project/gamecompjs) by Adam Nielsen — PKLITE decompression algorithm reference
- [OpenTESArena ExeUnpacker](https://github.com/afritz1/OpenTESArena) by afritz1 — PKLITE decompression reference
- [depklite](https://github.com/hackerb9/depklite) by hackerb9 / NY00123 — PKLITE decompression reference

## License

[GPL-3.0-or-later](LICENSE)
