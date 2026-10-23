# Gravedigger

Go translation toolkit for **Dangerous Dave in the Haunted Mansion** (DOS, 1991).

Extracts translatable resources from the game into editable formats and repacks them into working game files, enabling fan translations.

Supports longer replacement strings by relocating text and patching cross-references, subject to the game's buffer limits.

## Usage

```text
gravedigger unpack <game_dir> <output_dir>
gravedigger repack <output_dir> <repacked_dir>
```

`unpack` reads original game files (`.DD2`, `.EXE`) and produces editable assets:

| Resource | Format | Notes |
|---|---|---|
| Text strings (menus, messages, win/lose screens) | XLSX | Edit the `text` column and keep `id`; translated text can exceed the original length |
| Pictures and sprites | PNG | Keep the original dimensions and EGA palette |
| Game font | PNG + XLSX | Edit letters and outlines in `game_font.png`; assign characters on the `Шрифт` sheet in `strings.xlsx` |
| Anti-piracy exit screen (80×25 VGA text mode) | [XBIN](https://web.archive.org/web/20120204063040/http://www.acid.org/info/xbin/x_spec.htm) | Edit text and the VGA font in [Moebius](https://github.com/blocktronics/moebius), a free ANSI/XBIN art editor |

`repack` takes the edited assets and rebuilds game files ready to play. Keep the extracted `translatable/` and `meta/` directories together.

The font atlas is a 512×192 PNG with 16 columns and 6 rows of numbered 32×32 cells.
Draw in the top-left 16×10 area of each cell: white is the letter, black is its outline,
and transparency is the background. Keep the grey numbers unchanged; they match the
`slot` column on the `Шрифт` sheet. Change `symbol` only when assigning a different
character to a cell. Cell 127 is reserved and marked `служебный` in the sheet.

Edited letters get their width from the rightmost drawn pixel, including the outline;
left margins stay in place. The game allows widths up to 10 pixels and has a fixed
total font capacity. Unchanged and empty cells retain their original spacing.
Cyan preserves rare custom font pixels without an outline-mask bit. Keep exact
colours and transparency without smoothing. PNG and the font sheet are required;
the old JSON font format is no longer read.

Output directories must be empty and must not overlap their input directories.
For Cyrillic DOS startup and error messages, select code page 866 in DOS.

## Installation

Download a standalone binary from [GitHub Releases](../../releases).

Or run from source (Go 1.25+):

```sh
go run ./cmd/gravedigger --help
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
