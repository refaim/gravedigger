// Package exe edits the two known Dangerous Dave executables in memory.
package exe

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/text/encoding/charmap"

	"github.com/refaim/gravedigger/internal/compression"
	"github.com/refaim/gravedigger/internal/resource"
)

const (
	dsBase           = 0x27110
	exitScreenOffset = 0x112f0
	exitScreenSize   = 80 * 25 * 2
)

type stringMetadata struct {
	ID               string `json:"id"`
	Offset           int    `json:"offset"`
	MaxLength        int    `json:"max_length"`
	TrailingNewlines int    `json:"trailing_newlines"`
	Encoding         string `json:"encoding,omitempty"`
}
type exeMetadata struct {
	OriginalEXE []byte           `json:"original_exe"`
	Strings     []stringMetadata `json:"strings"`
}
type ExeTextHandler struct{}

func (ExeTextHandler) Name() string       { return "ExeTextHandler" }
func (ExeTextHandler) Patterns() []string { return []string{"DAVE.EXE", "1.EXE", "MANSION.EXE"} }

func wordAt(b []byte, at int) int       { return int(binary.LittleEndian.Uint16(b[at:])) }
func putWordAt(b []byte, at, value int) { binary.LittleEndian.PutUint16(b[at:], uint16(value)) }

func inspectStrings(image []byte) ([]stringMetadata, map[string]string, error) {
	entries := make([]stringMetadata, 0, len(stringTable))
	texts := make(map[string]string, len(stringTable))
	for _, spec := range stringTable {
		if spec.Offset >= len(image) {
			return nil, nil, fmt.Errorf("unsupported game code image: missing %s", spec.ID)
		}
		offset := dsBase + wordAt(image, spec.Xrefs[0])
		if offset >= len(image) {
			return nil, nil, fmt.Errorf("string %s points outside the code image", spec.ID)
		}
		length := bytes.IndexByte(image[offset:], 0)
		if length < 0 {
			return nil, nil, fmt.Errorf("unterminated game string %s", spec.ID)
		}
		for _, xref := range spec.Xrefs {
			if image[xref-1] != 0xb8 || wordAt(image, xref) != offset-dsBase {
				return nil, nil, fmt.Errorf("unsupported xref for %s at 0x%x", spec.ID, xref)
			}
		}
		text := string(image[offset : offset+length])
		dos := isDOSString(spec.ID)
		for _, b := range []byte(text) {
			if (b < 32 && b != '\n') || (b >= 128 && !dos) {
				return nil, nil, fmt.Errorf("unsupported byte in game string %s", spec.ID)
			}
		}
		encoding := "game-font"
		if dos {
			var decoded strings.Builder
			for _, b := range []byte(text) {
				decoded.WriteRune(charmap.CodePage866.DecodeByte(b))
			}
			text = decoded.String()
			encoding = "cp866-dos"
		}
		trailing := len(text) - len(strings.TrimRight(text, "\n"))
		entries = append(entries, stringMetadata{ID: spec.ID, Offset: offset, MaxLength: length, TrailingNewlines: trailing, Encoding: encoding})
		texts[spec.ID] = text
	}
	return entries, texts, nil
}

func decodeOriginal(original []byte) ([]byte, int, error) {
	data, err := compression.DecompressEXE(original)
	if err != nil {
		return nil, 0, err
	}
	start := wordAt(data, 8) * 16
	if len(data)-start < 0x29c98 {
		return nil, 0, fmt.Errorf("EXE is not a supported Dangerous Dave code image")
	}
	return data, start, nil
}

func (h ExeTextHandler) Unpack(name string, original []byte) (resource.Unpacked, error) {
	var out resource.Unpacked
	data, start, err := decodeOriginal(original)
	if err != nil {
		return out, err
	}
	image := data[start:]
	entries, texts, err := inspectStrings(image)
	if err != nil {
		return out, err
	}
	rows := make([]StringRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, StringRow{ID: entry.ID, Text: strings.TrimRight(texts[entry.ID], "\n")})
	}
	xlsx, _ := writeStrings(rows)
	font, _, err := exitFont(image)
	if err != nil {
		return out, err
	}
	// All dimensions and byte counts above are fixed and validated. The JSON
	// values contain only concrete integers, strings, and byte slices.
	xbin, _ := BuildXBIN(XBIN{Width: 80, Height: 25, FontHeight: 16, Font: font, Image: image[exitScreenOffset : exitScreenOffset+exitScreenSize]})
	gameFont, err := readGameFont(image)
	if err != nil {
		return out, err
	}
	// This workbook was just generated with fixed sheet/cell names; adding the
	// validated font mapping has no user-controlled serialization failure path.
	xlsx, _ = addFontSheet(xlsx, gameFont)
	metadata, _ := json.Marshal(exeMetadata{OriginalEXE: original, Strings: entries})
	out.Manifest = resource.Manifest{Handler: h.Name(), SourceFile: name, Metadata: metadata}
	out.Translatable = resource.Assets{"strings.xlsx": xlsx, "exit_screen.xb": xbin, "game_font.png": gameFontPNG(gameFont)}
	out.Meta = resource.Assets{}
	return out, nil
}

func validateManifest(entries []stringMetadata, expected []stringMetadata) (map[string]bool, error) {
	known := make(map[string]stringMetadata, len(expected))
	for _, e := range expected {
		known[e.ID] = e
	}
	seen := make(map[string]bool, len(entries))
	if len(entries) == 0 {
		return nil, fmt.Errorf("manifest has no strings")
	}
	for _, e := range entries {
		actual, ok := known[e.ID]
		if !ok || seen[e.ID] || e.Offset != actual.Offset || e.MaxLength != actual.MaxLength {
			return nil, fmt.Errorf("invalid manifest string %q", e.ID)
		}
		seen[e.ID] = true
	}
	return seen, nil
}

func (h ExeTextHandler) Repack(manifest resource.Manifest, assets, meta resource.Assets) ([]byte, error) {
	if manifest.Handler != h.Name() {
		return nil, fmt.Errorf("wrong EXE handler %q", manifest.Handler)
	}
	var m exeMetadata
	if err := json.Unmarshal(manifest.Metadata, &m); err != nil {
		return nil, fmt.Errorf("EXE manifest: %w", err)
	}
	data, start, err := decodeOriginal(m.OriginalEXE)
	if err != nil {
		return nil, err
	}
	image := data[start:]
	baseline := bytes.Clone(data)
	entries, texts, err := inspectStrings(image)
	if err != nil {
		return nil, err
	}
	editable, err := validateManifest(m.Strings, entries)
	if err != nil {
		return nil, err
	}
	rows, err := readStrings(assets["strings.xlsx"])
	if err != nil {
		return nil, err
	}
	for id := range rows {
		if !editable[id] {
			return nil, fmt.Errorf("unknown strings.xlsx id %q", id)
		}
	}
	fontAt, _, legacy, err := fontStubInfo(image)
	if err != nil {
		return nil, err
	}
	originalFont := bundledExitFont
	if fontAt >= 0 {
		originalFont = bytes.Clone(image[fontAt : fontAt+4096])
	}
	nearEnd := wordAt(image, 0x6d)
	initialStackEnd := wordAt(data, 14)*16 + wordAt(data, 16)
	if legacy {
		// The historical size constant included a near font/stub. On an edit,
		// compact all live appended strings below the new far font reservation.
		nearEnd = wordAt(image, 0xc8)
	}
	nearCursor := max(initialStackEnd, dsBase+nearEnd)
	f, err := readGameFont(image)
	if err != nil {
		return nil, err
	}
	f, err = readGameFontPNG(assets["game_font.png"], f)
	if err != nil {
		return nil, err
	}
	if err := readFontSheet(assets["strings.xlsx"], &f); err != nil {
		return nil, err
	}
	mapping, err := applyGameFont(image, f)
	if err != nil {
		return nil, err
	}
	encodedByID := make(map[string][]byte, len(entries))
	stringChanged := false
	for _, entry := range entries {
		text := texts[entry.ID]
		if editable[entry.ID] {
			var ok bool
			text, ok = rows[entry.ID]
			if !ok {
				return nil, fmt.Errorf("missing string %q", entry.ID)
			}
			if entry.TrailingNewlines > 0 && !strings.HasSuffix(text, "\n") {
				text += strings.Repeat("\n", entry.TrailingNewlines)
			}
		}
		var encoded []byte
		if isDOSString(entry.ID) {
			encoded, err = encodeDOSText(text)
		} else {
			encoded, err = encodeGameText(text, mapping)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.ID, err)
		}
		if entry.ID == "bload_error" && len(encoded) > 27 {
			return nil, fmt.Errorf("bload_error is limited to 27 bytes by its stack-buffer caller")
		}
		encodedByID[entry.ID] = encoded
		stringChanged = stringChanged || !bytes.Equal(encoded, image[entry.Offset:entry.Offset+entry.MaxLength])
	}
	if len(encodedByID["continue_level"])+1+len(encodedByID["continue_suffix"])+1 > 80 {
		return nil, fmt.Errorf("continue_level + level digit + continue_suffix + NUL exceeds its 80-byte game buffer")
	}
	x, err := ParseXBIN(assets["exit_screen.xb"])
	if err != nil {
		return nil, err
	}
	if x.Width != 80 || x.Height != 25 || (x.Font != nil && x.FontHeight != 16) {
		return nil, fmt.Errorf("exit_screen.xb requires 80x25 cells and a 256-character 8x16 font")
	}
	screenChanged := !bytes.Equal(image[exitScreenOffset:exitScreenOffset+exitScreenSize], x.Image)
	copy(image[exitScreenOffset:exitScreenOffset+exitScreenSize], x.Image)
	fontChanged := x.Font != nil && !bytes.Equal(x.Font, originalFont)
	anythingChanged := stringChanged || fontChanged || !bytes.Equal(data, baseline)
	if (x.Font != nil && (screenChanged || fontChanged)) || (fontAt >= 0 && anythingChanged) {
		font := x.Font
		if font == nil {
			font = originalFont
		}
		data, err = patchExitFont(data, font)
		if err != nil {
			return nil, err
		}
		image = data[start:]
	}
	for i, entry := range entries {
		encoded := encodedByID[entry.ID]
		moveLegacyString := legacy && anythingChanged && entry.Offset >= initialStackEnd
		if len(encoded) <= entry.MaxLength && !moveLegacyString {
			if !bytes.Equal(encoded, image[entry.Offset:entry.Offset+entry.MaxLength]) {
				clear(image[entry.Offset : entry.Offset+entry.MaxLength+1])
				copy(image[entry.Offset:], encoded)
			}
		} else {
			dsOffset := nearCursor - dsBase
			if dsOffset+len(encoded) >= 65536 {
				return nil, fmt.Errorf("string %s exceeds the 16-bit data segment", entry.ID)
			}
			end := nearCursor + len(encoded) + 1
			if end > len(image) {
				data = append(data, make([]byte, end-len(image))...)
				image = data[start:]
			}
			copy(image[nearCursor:], encoded)
			image[end-1] = 0
			for _, xref := range stringTable[i].Xrefs {
				putWordAt(image, xref, dsOffset)
			}
			nearCursor = end
			nearEnd = max(nearEnd, end-dsBase)
		}
	}
	if bytes.Equal(data, baseline) {
		return bytes.Clone(m.OriginalEXE), nil
	}
	if err := finishMZ(data, start, nearEnd); err != nil {
		return nil, err
	}
	return data, nil
}

func isDOSString(id string) bool {
	if strings.HasPrefix(id, "startup_") {
		return true
	}
	switch id {
	case "tile_error", "ted_exit", "bad_react", "object_overflow", "bload_error", "huff_error", "rlew_error",
		"memory_block_missing", "memory_too_many_blocks", "memory_out_of_memory":
		return true
	}
	return false
}

func encodeDOSText(text string) ([]byte, error) {
	out := make([]byte, 0, len(text))
	for _, r := range text {
		b, ok := charmap.CodePage866.EncodeRune(r)
		if (r < 32 && r != '\n') || r == 127 || !ok {
			return nil, fmt.Errorf("DOS/startup text requires printable CP866; the proportional game font is not used by this caller")
		}
		out = append(out, b)
	}
	return out, nil
}

var (
	originalResizeBlock = []byte{0x8b, 0xdf, 0x03, 0xda, 0x89, 0x1e, 0xa0, 0, 0x89, 0x1e, 0xa4, 0}
	farResizeTail       = []byte{0x03, 0xc2, 0xa3, 0xa0, 0, 0xa3, 0xa4, 0, 0x93}
)

func finishMZ(data []byte, start, nearEnd int) error {
	image := data[start:]
	if !bytes.Equal(image[0x6b:0x6d], []byte{0x81, 0xc7}) || (image[0x8c] != 0x75 && image[0x8c] != 0xeb) || image[0xc7] != 0xb9 || wordAt(image, 0xc8) != 0xd65a {
		return fmt.Errorf("unsupported C startup resize/BSS code")
	}
	block := image[0x9c:0xa8]
	if !bytes.Equal(block, originalResizeBlock) && (block[0] != 0xb8 || !bytes.Equal(block[3:], farResizeTail)) {
		return fmt.Errorf("unsupported C startup far-heap initialization")
	}
	nearEnd = max(nearEnd, wordAt(image, 0xc8))
	stack := max(wordAt(image, dsBase+0x4998), 0x1000)
	heap := wordAt(image, dsBase+0x4990)
	if heap != 0 {
		return fmt.Errorf("unsupported C runtime explicit-heap configuration")
	}
	// MM_Startup mallocs coreleft, aligns it and subtracts a 4096-byte reserve.
	// Retain the declared stack budget, that reserve and a full alignment unit;
	// otherwise the unsigned pool-size calculation can underflow at runtime.
	if nearEnd+stack+heap+0x1010 >= 0xfff0 {
		return fmt.Errorf("translated near data exceeds the game's stack and memory-manager pool budget")
	}
	putWordAt(image, 0x6d, nearEnd)
	putWordAt(image, dsBase+0x9a, nearEnd)
	putWordAt(image, dsBase+0x9c, nearEnd)
	image[0x8c] = 0x75
	farReserved := false
	if image[exitCallOffset] == 0x9a {
		stub := wordAt(image, exitCallOffset+1) + wordAt(image, exitCallOffset+3)*16
		farReserved = wordAt(image, stub+16)*16 >= farFontStart
	}
	if farReserved {
		endParagraphs := (len(image) - dsBase + 15) / 16
		if endParagraphs > 0xffff {
			return fmt.Errorf("far resources exceed the real-mode address space")
		}
		block[0] = 0xb8
		putWordAt(block, 1, endParagraphs)
		copy(block[3:], farResizeTail)
	} else {
		copy(block, originalResizeBlock)
	}
	moduleParagraphs := (len(image) + 15) / 16
	initialStackParagraphs := (wordAt(data, 14)*16 + wordAt(data, 16) + 15) / 16
	neededParagraphs := max(dsBase/16+0x1000, initialStackParagraphs)
	putWordAt(data, 10, max(0, neededParagraphs-moduleParagraphs))
	putWordAt(data, 12, 0xffff)
	putWordAt(data, 2, len(data)%512)
	putWordAt(data, 4, (len(data)+511)/512)
	return nil
}
