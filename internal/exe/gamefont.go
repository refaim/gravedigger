package exe

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"
)

const (
	fontBase     = 0x106b0
	fontRecords  = fontBase + 0x302
	fontCapacity = 2360
)

// GameFont describes the game's proportional EGA font, not the VGA CP866 font.
// Each record contains ten shadow rows followed by ten foreground rows. Rows
// are native bit masks: eight bits for widths <=8, sixteen bits for wider glyphs.
type GameFont struct {
	Version int
	Height  int
	Glyphs  []GameGlyph
}

type GameGlyph struct {
	Slot    int
	Unicode string
	Width   int
	Pixels  []uint16
	Shadow  []uint16
}

func readGameFont(image []byte) (GameFont, error) {
	f := GameFont{Version: 1, Height: 10}
	if len(image) < fontRecords+fontCapacity || wordAt(image, fontBase) != 10 {
		return f, fmt.Errorf("unsupported game font header")
	}
	expectedOffset := fontRecords
	for slot := 32; slot < 128; slot++ {
		width := int(image[fontBase+0x202+slot])
		rowBytes := (width + 7) / 8
		offset := fontBase + wordAt(image, fontBase+2+slot*2)
		if width < 1 || width > 10 || offset != expectedOffset || offset+20*rowBytes > fontRecords+fontCapacity {
			return f, fmt.Errorf("invalid game font record for slot %d", slot)
		}
		glyph := GameGlyph{Slot: slot, Unicode: string(rune(slot)), Width: width, Pixels: make([]uint16, 10), Shadow: make([]uint16, 10)}
		for plane, rows := range [][]uint16{glyph.Shadow, glyph.Pixels} {
			for i := range rows {
				at := offset + (plane*10+i)*rowBytes
				rows[i] = uint16(image[at])
				if rowBytes == 2 {
					rows[i] = binary.LittleEndian.Uint16(image[at:])
				}
			}
		}
		f.Glyphs = append(f.Glyphs, glyph)
		expectedOffset += 20 * rowBytes
	}
	return f, nil
}

func applyGameFont(image []byte, f GameFont) (map[rune]byte, error) {
	if f.Version != 1 || f.Height != 10 || len(f.Glyphs) != 96 {
		return nil, fmt.Errorf("game font requires version 1, height 10, and 96 glyphs")
	}
	mapping := make(map[rune]byte, 96)
	slots := make(map[int]bool, 96)
	ordered := make([]GameGlyph, 96)
	for _, g := range f.Glyphs {
		if g.Slot < 32 || g.Slot > 127 || slots[g.Slot] {
			return nil, fmt.Errorf("invalid or duplicate game glyph slot %d", g.Slot)
		}
		slots[g.Slot] = true
		ordered[g.Slot-32] = g
	}
	offset := fontRecords
	for _, g := range ordered {
		if (g.Slot == 32 || g.Slot == '_' || (g.Slot >= '0' && g.Slot <= '9')) && g.Unicode != string(rune(g.Slot)) {
			return nil, fmt.Errorf("slot 0x%02X is reserved for the game's space/dynamic ASCII digits/underscore cursor; edit its bitmap without changing its Unicode mapping", g.Slot)
		}
		if g.Slot == 127 && g.Unicode != "\x7f" {
			return nil, fmt.Errorf("game slot 0x7F is a color-control escape and cannot be remapped")
		}
		r, size := utf8.DecodeRuneInString(g.Unicode)
		if size != len(g.Unicode) || size == 0 || r < 32 || r == utf8.RuneError {
			return nil, fmt.Errorf("slot %d requires one printable Unicode character", g.Slot)
		}
		if _, exists := mapping[r]; exists {
			return nil, fmt.Errorf("duplicate Unicode mapping %q", r)
		}
		mapping[r] = byte(g.Slot)
		if g.Width < 1 || g.Width > 10 || len(g.Pixels) != 10 || len(g.Shadow) != 10 {
			return nil, fmt.Errorf("invalid geometry for game glyph %d", g.Slot)
		}
		rowBytes := (g.Width + 7) / 8
		if offset+20*rowBytes > fontRecords+fontCapacity {
			return nil, fmt.Errorf("game font exceeds its %d-byte capacity", fontCapacity)
		}
		binary.LittleEndian.PutUint16(image[fontBase+2+g.Slot*2:], uint16(offset-fontBase))
		image[fontBase+0x202+g.Slot] = byte(g.Width)
		for _, rows := range [][]uint16{g.Shadow, g.Pixels} {
			for _, value := range rows {
				if rowBytes == 1 {
					if value > 255 {
						return nil, fmt.Errorf("8-bit row overflow for slot %d", g.Slot)
					}
					image[offset] = byte(value)
				} else {
					binary.LittleEndian.PutUint16(image[offset:], value)
				}
				offset += rowBytes
			}
		}
	}
	return mapping, nil
}

func encodeGameText(text string, mapping map[rune]byte) ([]byte, error) {
	out := make([]byte, 0, len(text))
	for _, r := range text {
		if r == 127 {
			return nil, fmt.Errorf("U+007F is a game color-control escape, not a printable glyph")
		}
		if r == '\n' {
			out = append(out, '\n')
			continue
		}
		b, ok := mapping[r]
		if !ok {
			return nil, fmt.Errorf("no game-font glyph for %q (U+%04X); keep its mapping or translate all strings using it", r, r)
		}
		out = append(out, b)
	}
	return out, nil
}
