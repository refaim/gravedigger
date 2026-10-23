package exe

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestGameFontValidation(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	var good GameFont
	if err := json.Unmarshal(u.Translatable["game_font.json"], &good); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*GameFont){
		func(f *GameFont) { f.Version = 2 }, func(f *GameFont) { f.Height = 8 }, func(f *GameFont) { f.Glyphs = f.Glyphs[:95] },
		func(f *GameFont) { f.Glyphs[0].Slot = 31 }, func(f *GameFont) { f.Glyphs[0].Slot = 128 }, func(f *GameFont) { f.Glyphs[0].Slot = 33 },
		func(f *GameFont) { f.Glyphs[1].Unicode = "" }, func(f *GameFont) { f.Glyphs[1].Unicode = "ab" }, func(f *GameFont) { f.Glyphs[1].Unicode = "\"" }, func(f *GameFont) { f.Glyphs[1].Unicode = "\n" },
		func(f *GameFont) { f.Glyphs[0].Width = 0 }, func(f *GameFont) { f.Glyphs[0].Width = 11 }, func(f *GameFont) { f.Glyphs[0].Pixels = nil }, func(f *GameFont) { f.Glyphs[0].Shadow = nil },
		func(f *GameFont) { f.Glyphs[95].Unicode = "Я" },
		func(f *GameFont) { f.Glyphs['0'-32].Unicode = "Я" },
		func(f *GameFont) { f.Glyphs[' '-32].Unicode = "Я" },
		func(f *GameFont) { f.Glyphs[0].Pixels[0] = 256 }, func(f *GameFont) { f.Glyphs[0].Shadow[0] = 256 },
		func(f *GameFont) {
			for i := range f.Glyphs {
				f.Glyphs[i].Width = 10
			}
		},
	} {
		var f GameFont
		b, _ := json.Marshal(good)
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatal(err)
		}
		change(&f)
		b, _ = json.Marshal(f)
		assets := mapClone(u.Translatable)
		assets["game_font.json"] = b
		if _, err := (ExeTextHandler{}).Repack(u.Manifest, assets, u.Meta); err == nil {
			t.Fatal("accepted invalid gamefont")
		}
	}
	assets := mapClone(u.Translatable)
	assets["game_font.json"] = []byte("bad")
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, assets, u.Meta); err == nil {
		t.Fatal("accepted malformed JSON")
	}
	good.Glyphs['A'-32].Unicode = "Я"
	b, _ := json.Marshal(good)
	assets["game_font.json"] = b
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, assets, u.Meta); err == nil {
		t.Fatal("ASCII glyph removed while English strings still use it")
	}
	setText(t, &u, "win_line13", "Я")
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("missing Unicode mapping accepted")
	}
}

func TestDynamicDigitMappingCannotBeMoved(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	var f GameFont
	if err := json.Unmarshal(u.Translatable["game_font.json"], &f); err != nil {
		t.Fatal(err)
	}
	f.Glyphs['0'-32].Unicode = "Я"
	f.Glyphs['~'-32].Unicode = "0"
	b, _ := json.Marshal(f)
	u.Translatable["game_font.json"] = b
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("moved dynamic digit mapping accepted")
	}
}

func TestFixedCursorMapping(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	var f GameFont
	if err := json.Unmarshal(u.Translatable["game_font.json"], &f); err != nil {
		t.Fatal(err)
	}
	f.Glyphs['_'-32].Unicode = "Я"
	b, _ := json.Marshal(f)
	u.Translatable["game_font.json"] = b
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("hardcoded underscore cursor remapped")
	}
}

func TestIncompleteFontJSON(t *testing.T) {
	for _, body := range []string{"{}", "null"} {
		_, u := unpackFixture(t, "softdisk")
		u.Translatable["game_font.json"] = []byte(body)
		if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
			t.Fatal("missing font properties accepted")
		}
	}
}

func mapClone(m map[string][]byte) map[string][]byte {
	out := map[string][]byte{}
	for k, v := range m {
		out[k] = bytes.Clone(v)
	}
	return out
}

func TestBuildFontStub(t *testing.T) {
	stub := buildFontStub(0x1234)
	if len(stub) < 80 || !bytes.Equal(stub[:5], []byte{0xb8, 3, 0, 0xcd, 0x10}) || stub[len(stub)-1] != 0xcb || word(stub, 16) != 0x1234 {
		t.Fatal("VGA font stub changed")
	}
}

func TestGameFontOrderDoesNotChangeExecutable(t *testing.T) {
	original, u := unpackFixture(t, "softdisk")
	var f GameFont
	if err := json.Unmarshal(u.Translatable["game_font.json"], &f); err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(f.Glyphs)-1; i < j; i, j = i+1, j-1 {
		f.Glyphs[i], f.Glyphs[j] = f.Glyphs[j], f.Glyphs[i]
	}
	b, _ := json.Marshal(f)
	u.Translatable["game_font.json"] = b
	out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
	if err != nil || !bytes.Equal(original, out) {
		t.Fatal("JSON glyph order changed executable", err)
	}
}

func TestDOSStringEncodingIndependentOfGameFont(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	var f GameFont
	if err := json.Unmarshal(u.Translatable["game_font.json"], &f); err != nil {
		t.Fatal(err)
	}
	f.Glyphs[94].Unicode = "🙂"
	b, _ := json.Marshal(f)
	u.Translatable["game_font.json"] = b
	setText(t, &u, "startup_executing", "🙂")
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("DOS string accepted game-font-only Unicode")
	}
	setText(t, &u, "startup_executing", "~ASCII~")
	out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
	if err != nil {
		t.Fatal("ASCII DOS text should not use gamefont remapping", err)
	}
	if !bytes.HasPrefix(code(out)[0x276c2:], []byte("~ASCII~\x00")) {
		t.Fatal("DOS bytes changed by gamefont mapping")
	}
	for _, id := range []string{"startup_executing", "tile_error", "ted_exit", "bad_react", "object_overflow", "huff_error", "rlew_error", "bload_error"} {
		if !isDOSString(id) {
			t.Errorf("missing DOS context: %s", id)
		}
	}
	if isDOSString("win_line13") {
		t.Fatal("game string marked as DOS text")
	}
	if _, err := encodeGameText("\x7f", map[rune]byte{127: 127}); err == nil {
		t.Fatal("accepted game color escape as a printable glyph")
	}
}
