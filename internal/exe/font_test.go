package exe

import (
	"bytes"
	"slices"
	"testing"

	"github.com/xuri/excelize/v2"
)

func mapClone(m map[string][]byte) map[string][]byte {
	out := map[string][]byte{}
	for k, v := range m {
		out[k] = bytes.Clone(v)
	}
	return out
}

func cloneTestFont(f GameFont) GameFont {
	f.Glyphs = slices.Clone(f.Glyphs)
	for i := range f.Glyphs {
		f.Glyphs[i].Pixels = slices.Clone(f.Glyphs[i].Pixels)
		f.Glyphs[i].Shadow = slices.Clone(f.Glyphs[i].Shadow)
	}
	return f
}

func TestGameFontValidation(t *testing.T) {
	baseline := code(rawFixture(t))
	good, err := readGameFont(baseline)
	if err != nil {
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
		f := cloneTestFont(good)
		change(&f)
		if _, err := applyGameFont(bytes.Clone(baseline), f); err == nil {
			t.Fatal("accepted invalid game font")
		}
	}
	_, u := unpackFixture(t, "softdisk")
	atlasWorkbook(t, &u, func(f *excelize.File) {
		if err := f.SetCellStr("Шрифт", "B35", "Я"); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("ASCII A removed while English strings still use it")
	}
	_, u = unpackFixture(t, "softdisk")
	setText(t, &u, "win_line13", "Я")
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("missing Unicode mapping accepted")
	}
}

func TestDynamicDigitMappingCannotBeMoved(t *testing.T) {
	baseline := code(rawFixture(t))
	f, err := readGameFont(baseline)
	if err != nil {
		t.Fatal(err)
	}
	f.Glyphs['0'-32].Unicode = "Я"
	f.Glyphs['~'-32].Unicode = "0"
	if _, err := applyGameFont(baseline, f); err == nil {
		t.Fatal("moved dynamic digit mapping accepted")
	}
}

func TestFixedCursorMapping(t *testing.T) {
	baseline := code(rawFixture(t))
	f, err := readGameFont(baseline)
	if err != nil {
		t.Fatal(err)
	}
	f.Glyphs['_'-32].Unicode = "Я"
	if _, err := applyGameFont(baseline, f); err == nil {
		t.Fatal("hardcoded underscore cursor remapped")
	}
}

func TestBuildFontStub(t *testing.T) {
	stub := buildFontStub(0x1234)
	if len(stub) < 80 || !bytes.Equal(stub[:5], []byte{0xb8, 3, 0, 0xcd, 0x10}) || stub[len(stub)-1] != 0xcb || word(stub, 16) != 0x1234 {
		t.Fatal("VGA font stub changed")
	}
}

func TestGameFontOrderDoesNotChangeNativeImage(t *testing.T) {
	baseline := code(rawFixture(t))
	f, err := readGameFont(baseline)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(f.Glyphs)
	out := bytes.Clone(baseline)
	if _, err := applyGameFont(out, f); err != nil || !bytes.Equal(out, baseline) {
		t.Fatal("glyph order changed native font bytes", err)
	}
}

func TestDOSStringEncodingIndependentOfGameFont(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	atlasWorkbook(t, &u, func(f *excelize.File) {
		if err := f.SetCellStr("Шрифт", "B96", "🙂"); err != nil {
			t.Fatal(err)
		}
	})
	setText(t, &u, "startup_executing", "🙂")
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("DOS string accepted game-font-only Unicode")
	}
	setText(t, &u, "startup_executing", "~ASCII~")
	out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
	if err != nil {
		t.Fatal("ASCII DOS text should not use game font mapping", err)
	}
	if !bytes.HasPrefix(code(out)[0x276c2:], []byte("~ASCII~\x00")) {
		t.Fatal("DOS bytes changed by game font mapping")
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
