package exe

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/refaim/gravedigger/internal/compression"
	"github.com/refaim/gravedigger/internal/resource"
)

func fixture(t *testing.T, variant string) []byte {
	t.Helper()
	name := "1.EXE"
	if variant == "retail" {
		name = "MANSION.EXE"
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "game", variant, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func unpackFixture(t *testing.T, variant string) ([]byte, resource.Unpacked) {
	t.Helper()
	original := fixture(t, variant)
	u, err := (ExeTextHandler{}).Unpack("DAVE.EXE", original)
	if err != nil {
		t.Fatal(err)
	}
	return original, u
}

func setText(t *testing.T, u *resource.Unpacked, id, text string) {
	t.Helper()
	rows, err := readStrings(u.Translatable["strings.xlsx"])
	if err != nil {
		t.Fatal(err)
	}
	rows[id] = text
	var out []StringRow
	for id, text := range rows {
		out = append(out, StringRow{ID: id, Text: text})
	}
	b, err := writeStrings(out)
	if err != nil {
		t.Fatal(err)
	}
	u.Translatable["strings.xlsx"] = b
}
func word(b []byte, p int) int   { return int(binary.LittleEndian.Uint16(b[p:])) }
func putWord(b []byte, p, v int) { binary.LittleEndian.PutUint16(b[p:], uint16(v)) }
func code(b []byte) []byte       { return b[word(b, 8)*16:] }

func TestEXEOriginalsAndTable(t *testing.T) {
	h := ExeTextHandler{}
	if h.Name() != "ExeTextHandler" || len(h.Patterns()) != 3 {
		t.Fatal("handler registration")
	}
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			original, u := unpackFixture(t, variant)
			rows, err := readStrings(u.Translatable["strings.xlsx"])
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"help_title", "game_over", "game_over_score", "joy_hold_line2", "kbd_north", "key_escape", "key_left"} {
				if rows[id] == "" {
					t.Errorf("missing %s", id)
				}
			}
			if len(rows) <= 83 {
				t.Fatal("incomplete string catalog")
			}
			out, err := h.Repack(u.Manifest, u.Translatable, u.Meta)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out, original) {
				t.Fatal("unchanged EXE is not byte exact")
			}
			var f GameFont
			if err := json.Unmarshal(u.Translatable["game_font.json"], &f); err != nil {
				t.Fatal(err)
			}
			if f.Height != 10 || len(f.Glyphs) != 96 || len(f.Glyphs[0].Pixels) != 10 || len(f.Glyphs[0].Shadow) != 10 {
				t.Fatal("incorrect real font geometry")
			}
			if f.Glyphs[1].Pixels[1] != 0x60 || f.Glyphs[1].Shadow[0] != 0xf0 {
				t.Fatalf("! glyph offset/order wrong: %#v", f.Glyphs[1])
			}
		})
	}
}

func TestEveryShortStringPreservesAllOtherBytes(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		original, u := unpackFixture(t, variant)
		raw, err := compression.DecompressEXE(original)
		if err != nil {
			t.Fatal(err)
		}
		baseline := code(raw)
		var m exeMetadata
		if err := json.Unmarshal(u.Manifest.Metadata, &m); err != nil {
			t.Fatal(err)
		}
		for _, entry := range m.Strings {
			t.Run(variant+"/"+entry.ID, func(t *testing.T) {
				assets := resource.Assets{}
				for k, v := range u.Translatable {
					assets[k] = v
				}
				edited := u
				edited.Translatable = assets
				setText(t, &edited, entry.ID, "!")
				out, err := (ExeTextHandler{}).Repack(edited.Manifest, edited.Translatable, edited.Meta)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Equal(out, original) || len(out) < len(raw) {
					t.Fatal("edit did not produce full decompressed MZ")
				}
				got := code(out)
				begin, end := entry.Offset, entry.Offset+entry.MaxLength+1
				if !bytes.Equal(got[:begin], baseline[:begin]) || !bytes.Equal(got[end:], baseline[end:]) {
					t.Fatal("unrelated bytes changed")
				}
				if got[begin] != '!' {
					t.Fatal("replacement missing")
				}
			})
		}
	}
}

func TestLongStringAndMemory(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		original, u := unpackFixture(t, variant)
		raw, err := compression.DecompressEXE(original)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Repeat("LONG", 100)
		setText(t, &u, "win_line13", text)
		out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
		if err != nil {
			t.Fatal(err)
		}
		c := code(out)
		at := word(c, 0x11ce) + dsBase
		if string(c[at:at+len(text)]) != text || c[at+len(text)] != 0 {
			t.Fatal("relocated string/xref missing")
		}
		if !bytes.Equal(c[0x27646:0x2764f], code(raw)[0x27646:0x2764f]) {
			t.Fatal("old slot changed")
		}
		u2, err := (ExeTextHandler{}).Unpack("DAVE.EXE", out)
		if err != nil {
			t.Fatal("cannot unpack a relocated executable", err)
		}
		unchanged, err := (ExeTextHandler{}).Repack(u2.Manifest, u2.Translatable, u2.Meta)
		if err != nil || !bytes.Equal(unchanged, out) {
			t.Fatal("relocated raw EXE no-edit roundtrip", err)
		}
		if c[0x8c] != 0x75 || word(c, 0x6d) < len(c)-dsBase {
			t.Fatal("startup resize not extended")
		}
		declared := (word(out, 4)-1)*512 + word(out, 2)
		if word(out, 2) == 0 {
			declared = word(out, 4) * 512
		}
		if declared != len(out) {
			t.Fatal("MZ file size incorrect")
		}
		need := dsBase + ((word(c, 0x6d)+word(c, dsBase+0x4998)+word(c, dsBase+0x4990))/16+1)*16
		if ((len(c)+15)/16+word(out, 10))*16 < need {
			t.Fatal("MZ minimum memory does not cover relocated stack")
		}
		setText(t, &u, "win_line13", strings.Repeat("A", 20000))
		if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
			t.Fatal("accepted stack/segment overflow")
		}
	}
}

func TestCyrillicAndGameFontRoundtrip(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		original, u := unpackFixture(t, variant)
		raw, err := compression.DecompressEXE(original)
		if err != nil {
			t.Fatal(err)
		}
		baseline := code(raw)
		var f GameFont
		if err := json.Unmarshal(u.Translatable["game_font.json"], &f); err != nil {
			t.Fatal(err)
		}
		f.Glyphs[94].Unicode = "Я"
		f.Glyphs[94].Pixels[2] = 0xff00
		b, _ := json.Marshal(f)
		u.Translatable["game_font.json"] = b
		setText(t, &u, "win_line13", "ЯЯ")
		out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
		if err != nil {
			t.Fatal(err)
		}
		c := code(out)
		if !bytes.Equal(c[0x27646:0x27649], []byte{0x7e, 0x7e, 0}) {
			t.Fatal("Unicode mapping not applied")
		}
		if !bytes.Equal(c[exitScreenOffset:exitScreenOffset+4000], baseline[exitScreenOffset:exitScreenOffset+4000]) {
			t.Fatal("font damaged exit screen")
		}
		if word(c, fontBase+2+0x7e*2) != 0xbea || word(c, fontBase+0xbea+20+4) != 0xff00 {
			t.Fatal("glyph mapping/record layout wrong")
		}
		if _, err := (ExeTextHandler{}).Unpack("DAVE.EXE", out); err != nil {
			t.Fatal("edited raw MZ cannot be unpacked", err)
		}
	}
}

func TestExitScreenAndFontOnly(t *testing.T) {
	for _, fontOnly := range []bool{false, true} {
		original, u := unpackFixture(t, "softdisk")
		x, err := ParseXBIN(u.Translatable["exit_screen.xb"])
		if err != nil {
			t.Fatal(err)
		}
		if fontOnly {
			x.Font[0] ^= 1
		} else {
			x.Image[0] = 'X'
		}
		b, err := BuildXBIN(x)
		if err != nil {
			t.Fatal(err)
		}
		u.Translatable["exit_screen.xb"] = b
		out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(out, original) || code(out)[0x679] != 0x9a || word(out, 6) != 24 {
			t.Fatal("font-only/screen edit failed to inject relocated VGA stub")
		}
		if !bytes.Equal(code(out)[exitScreenOffset:exitScreenOffset+4000], x.Image) {
			t.Fatal("exit screen not patched")
		}
	}
}
