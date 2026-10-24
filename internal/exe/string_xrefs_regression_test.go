package exe

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

var keyboardSeparatorReferences = []struct {
	id           string
	offset, xref int
}{
	{"kbd_direction_binding_separator", 0x29c22, 0x6fec},
	{"kbd_button1_binding_separator", 0x29c24, 0x701d},
	{"kbd_button2_binding_separator", 0x29c26, 0x7043},
	{"kbd_modified_binding_separator", 0x29c3c, 0x7161},
}

func TestWinCongratsRelocationPreservesLevelTileFlagClear(t *testing.T) {
	// This is memset(DS:4E1E, 0, 0400h), not a reference to the string at
	// DS:0400. The unrelated size constant happens to equal its old address.
	clearTiles := []byte{0xb8, 0x00, 0x04, 0x50, 0x33, 0xc0, 0x50, 0xb8, 0x1e, 0x4e, 0x50, 0xe8, 0xb2, 0xb6, 0x83, 0xc4, 0x06}
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			original, u := unpackFixture(t, variant)
			raw, start, err := decodeOriginal(original)
			if err != nil {
				t.Fatal(err)
			}
			before := raw[start:]
			if !bytes.Equal(before[0xf27:0xf38], clearTiles) || word(before, 0x10fa) != 0x0400 {
				t.Fatal("fixture must contain the distinct memset size and real string pointer")
			}
			text := strings.Repeat("CONGRATULATIONS ", 8) + "WIN_CONGRATS_END"
			setText(t, &u, "win_congrats", text)
			out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
			if err != nil {
				t.Fatal(err)
			}
			after := code(out)
			if !bytes.Equal(after[0xf27:0xf38], clearTiles) {
				t.Errorf("relocating win_congrats changed the level's 1024-byte memset instruction: got %x, want %x", after[0xf27:0xf38], clearTiles)
			}
			pointer := word(after, 0x10fa)
			if pointer == 0x0400 {
				t.Fatal("the actual congrats string pointer was not relocated")
			}
			at := dsBase + pointer
			wantText := []byte(text + "\n\x00")
			if !bytes.Equal(after[at:at+len(wantText)], wantText) {
				t.Fatal("the actual print pointer does not target the complete translated string")
			}
			if !bytes.Equal(after[0x10f9:0x10fa], before[0x10f9:0x10fa]) || !bytes.Equal(after[0x10fc:0x1102], before[0x10fc:0x1102]) {
				t.Fatal("relocation changed the actual print instructions outside their pointer operand")
			}
			reopened, err := (ExeTextHandler{}).Unpack("DAVE.EXE", out)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := readStrings(reopened.Translatable["strings.xlsx"])
			if err != nil || rows["win_congrats"] != text {
				t.Fatal("reopening lost the relocated translation", err)
			}
			again, err := (ExeTextHandler{}).Repack(reopened.Manifest, reopened.Translatable, reopened.Meta)
			if err != nil || !bytes.Equal(again, out) {
				t.Fatal("reopened translation must repack byte-exact", err)
			}
		})
	}
}

func TestKeyboardBindingSeparatorsUseRemappedGlyph(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			original, u := unpackFixture(t, variant)
			raw, start, err := decodeOriginal(original)
			if err != nil {
				t.Fatal(err)
			}
			before := raw[start:]
			rows, err := readStrings(u.Translatable["strings.xlsx"])
			if err != nil {
				t.Fatal(err)
			}
			for _, separator := range keyboardSeparatorReferences {
				if rows[separator.id] != ":" {
					t.Errorf("missing keyboard binding separator %s", separator.id)
				}
			}
			atlasWorkbook(t, &u, func(f *excelize.File) {
				// Native ':' (cell 27) becomes a Cyrillic slot, while the last
				// printable cell now supplies ':' for every printed separator.
				for _, cell := range []struct{ at, symbol string }{{"B28", "Я"}, {"B96", ":"}} {
					if err := f.SetCellStr("Шрифт", cell.at, cell.symbol); err != nil {
						t.Fatal(err)
					}
				}
			})
			out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
			if err != nil {
				t.Fatal(err)
			}
			after := code(out)
			for _, separator := range keyboardSeparatorReferences {
				at := dsBase + word(after, separator.xref)
				if !bytes.Equal(after[at:at+2], []byte{126, 0}) {
					t.Errorf("%s still prints native glyph %02X instead of remapped glyph 7E", separator.id, after[at])
				}
				if !bytes.Equal(after[separator.xref-1:separator.xref+8], before[separator.xref-1:separator.xref+8]) {
					t.Errorf("same-length %s edit changed its MOV/push/print instructions", separator.id)
				}
			}
		})
	}
}

func TestKeyboardBindingSeparatorsRelocateIndependently(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		for _, separator := range keyboardSeparatorReferences {
			t.Run(variant+"/"+separator.id, func(t *testing.T) {
				original, u := unpackFixture(t, variant)
				raw, start, err := decodeOriginal(original)
				if err != nil {
					t.Fatal(err)
				}
				before := raw[start:]
				text := "KEYBOARD " + strings.Repeat("SEPARATOR ", 12)
				setText(t, &u, separator.id, text)
				out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
				if err != nil {
					t.Fatal(err)
				}
				after := code(out)
				pointer := word(after, separator.xref)
				if pointer == word(before, separator.xref) {
					t.Fatal("long separator did not relocate")
				}
				at := dsBase + pointer
				if !bytes.Equal(after[at:at+len(text)+1], []byte(text+"\x00")) {
					t.Fatal("print pointer does not target the complete translated separator")
				}
				if !bytes.Equal(after[separator.xref-1:separator.xref], before[separator.xref-1:separator.xref]) || !bytes.Equal(after[separator.xref+2:separator.xref+8], before[separator.xref+2:separator.xref+8]) {
					t.Fatal("relocation changed the caller outside its pointer operand")
				}
				for _, other := range keyboardSeparatorReferences {
					if other.id != separator.id && (!bytes.Equal(after[other.xref-1:other.xref+8], before[other.xref-1:other.xref+8]) || !bytes.Equal(after[other.offset:other.offset+2], []byte(":\x00"))) {
						t.Errorf("editing %s changed independent separator %s", separator.id, other.id)
					}
				}
				reopened, err := (ExeTextHandler{}).Unpack("DAVE.EXE", out)
				if err != nil {
					t.Fatal(err)
				}
				rows, err := readStrings(reopened.Translatable["strings.xlsx"])
				if err != nil || rows[separator.id] != text {
					t.Fatal("reopening lost the translated separator", err)
				}
				again, err := (ExeTextHandler{}).Repack(reopened.Manifest, reopened.Translatable, reopened.Meta)
				if err != nil || !bytes.Equal(again, out) {
					t.Fatal("reopened separator must repack byte-exact", err)
				}
			})
		}
	}
}
