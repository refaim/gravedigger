# Gravedigger

Go translation toolkit for Dangerous Dave in the Haunted Mansion.

## Development

- Source requires Go 1.25+; validation uses Go 1.27.3 and PowerShell 7.
  Dependencies and the pinned gobco tool are in go.mod.
- `.golangci.yml` uses the pre2 set of ten linters plus gofumpt/goimports,
  pinned to golangci-lint 2.14.0. `scripts/check.ps1` enforces lint and format
  checks; it accepts `-LintPath` or uses `.tools/golangci-lint/2.14.0/` or PATH.
- Run `pwsh -NoProfile -File scripts/check.ps1` for formatting, vet, tests, exact
  100% statement and branch coverage gates, a standalone build, and byte-exact
  unpack/repack smoke tests of both game variants.
- CI checks and packages Windows AMD64 (ZIP), Linux AMD64 (tar.gz), and macOS
  ARM64 (tar.gz). Branch/PR checks do not publish releases. A `vX.Y.Z` tag must
  match the built executable's version and belong to master history before
  the three checked archives can be released.
- The external corpus is required in `game/softdisk` and `game/retail`; CI downloads
  the same two archives used by the Python predecessor.
- Run `go run ./cmd/gravedigger --help` during development.
- Build with `go build -o bin/gravedigger ./cmd/gravedigger` (use `.exe` on Windows).
- `internal/app.Version` is the application version; current app version: 3.1.0.
- Excelize handles XLSX; Afero provides filesystem access and failure testing.
  The bundled VGA font is embedded in the Go binary.
- Preserve the `translatable/` and `meta/` layout. Unchanged EXEs roundtrip
  byte-exact; edited EXEs are rebuilt as decompressed MZ files.
- Embedded EXE artwork exports 73 fixed EGA PNGs under `translatable/exe/`:
  32 UI tiles (8x8), 40 death frames (48x48), and THE END (208x56).
  The four-plane bank at code-image 0x188e0 has stride 0x33b0 and is fully
  covered by these images. Reuse the graphics package's planar PNG codec.
  New manifests mark `embedded_graphics` and require every PNG; older projects
  without the marker or artwork preserve the embedded bank unchanged.
  External-sprite descriptors and the 24 PC Speaker cues remain untouched.
- String xrefs must be verified as pointers in their actual call context.
  0xF28 is a memset count (1024), not the win_congrats pointer at 0x10FA.
  Keyboard binding separators are exported so font remapping also covers them.
- Fonts require `game_font.png` and the `Шрифт` sheet in `strings.xlsx`.
  JSON font compatibility was explicitly removed at Roma's request.
  The PNG is 320x120: 16x6 bordered 20x20 cells numbered 1..96. Each has a
  16x10 drawing area inset one pixel and tiny 3x5 grey numbers below.
  Sheet columns are number/symbol with the same 1..96 cell numbers; convert
  to native slots only internally. White is foreground+mask,
  black is mask-only, cyan is foreground-only, and transparency clears both.
  Changed glyphs use automatic width (at most 10); unchanged or empty glyphs
  retain their advance. Cell 96 is reserved and must remain unchanged.
- DOS startup/error strings use CP866 through x/text/charmap, independently of
  the game font. Select code page 866 in DOS when testing Cyrillic DOS messages.
- Exit-screen XBIN supports custom 8×16 glyphs and text cells. Custom palettes
  and the non-blinking background flag are currently not applied by the game.
- Write a failing test before each feature/fix; keep 100% statements and branches.
- On Windows, also run `scripts/test-exit-screen.ps1` with DOSBox Staging and
  DOS-debug to verify the live exit screen, VGA font, and relocated text.
  A separate reviewer must return zero comments before completing changes.
- Temporary check artifacts are removed in `finally`; reusable Go caches remain
  under `.cache/go`. Do not publish releases or push changes without authorization.
