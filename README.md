# Gravedigger

Translation toolkit for **Dangerous Dave in the Haunted Mansion** (DOS, 1991).

Extracts translatable resources (text strings) from the game into editable formats and repacks them back into working game files, enabling fan translations into any language.

Supports arbitrary-length replacement strings — translated text is not limited to the original string length. When a string exceeds its original slot, all strings are relocated to an appended block with patched cross-references.

## Usage

```
gravedigger unpack <game_dir> <output_dir>
gravedigger repack <output_dir> <repacked_dir>
```

`unpack` reads original game files (`.DD2`, `.EXE`) and produces editable assets:

| Resource | Format | Notes |
|---|---|---|
| Text strings (menus, messages, win/lose screens) | XLSX | One row per string; translated text can exceed the original length |
| Sprites and tiles | PNG | Indexed-color images with original palette |
| Anti-piracy exit screen (80×25 VGA text mode) | [XBIN](https://web.archive.org/web/20120204063040/http://www.acid.org/info/xbin/x_spec.htm) | Can be edited in [Moebius](https://github.com/blocktronics/moebius), a free ANSI/XBIN art editor |

`repack` takes the edited translations and rebuilds game files ready to play.

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
- [UNLZEXE](https://github.com/mywave82/unlzexe) by Mitugu Kurizono / Stian Skjelstad — LZEXE decompression algorithm reference
- [unpacklzexe](https://github.com/samrussell/unpacklzexe) by Sam Russell — LZEXE decompression reference (Python)

## License

[GPL-3.0-or-later](LICENSE)
