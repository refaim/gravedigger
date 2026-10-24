package exe

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/refaim/gravedigger/internal/compression"
	"github.com/refaim/gravedigger/internal/resource"
)

func rawFixture(t *testing.T) []byte {
	t.Helper()
	b, err := compression.DecompressEXE(fixture(t, "softdisk"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func replaceOriginal(t *testing.T, u resource.Unpacked, data []byte) resource.Unpacked {
	t.Helper()
	var m exeMetadata
	if err := json.Unmarshal(u.Manifest.Metadata, &m); err != nil {
		t.Fatal(err)
	}
	m.OriginalEXE = data
	u.Manifest.Metadata, _ = json.Marshal(m)
	return u
}

func TestMalformedOriginal(t *testing.T) {
	h := ExeTextHandler{}
	if _, err := h.Unpack("DAVE.EXE", nil); err == nil {
		t.Fatal("accepted empty EXE")
	}
	small := make([]byte, 64)
	copy(small, "MZ")
	putWord(small, 2, 64)
	putWord(small, 4, 1)
	putWord(small, 8, 2)
	putWord(small, 24, 28)
	if _, err := h.Unpack("DAVE.EXE", small); err == nil {
		t.Fatal("accepted unrelated MZ")
	}
	_, u := unpackFixture(t, "softdisk")
	for _, mutate := range []func([]byte){
		func(c []byte) { c[0x247] = 0 },
		func(c []byte) { putWord(c, 0x10fa, 0xffff) },
		func(c []byte) { c[exitCallOffset] = 0xcc },
		func(c []byte) { putWord(c, fontBase, 8) },
		func(c []byte) { c[fontBase+0x202+32] = 0 },
		func(c []byte) { c[0x27242] = 1 },
		func(c []byte) { putWord(c, 0x248, 0xffff) },
	} {
		raw := rawFixture(t)
		mutate(code(raw))
		if _, err := h.Unpack("DAVE.EXE", raw); err == nil {
			t.Fatal("accepted unsupported original")
		}
		bad := replaceOriginal(t, u, raw)
		if _, err := h.Repack(bad.Manifest, bad.Translatable, bad.Meta); err == nil {
			t.Fatal("repacked unsupported original")
		}
	}
	if _, _, err := inspectStrings(nil); err == nil {
		t.Fatal("short image accepted")
	}
	c := bytes.Clone(code(rawFixture(t)))
	for i := 0x27242; i < len(c); i++ {
		c[i] = 'x'
	}
	if _, _, err := inspectStrings(c); err == nil {
		t.Fatal("unterminated string accepted")
	}
	if _, err := readGameFont(nil); err == nil {
		t.Fatal("short font accepted")
	}
	c = bytes.Clone(code(rawFixture(t)))
	putWord(c, fontBase+2+32*2, 0)
	if _, err := readGameFont(c); err == nil {
		t.Fatal("invalid font pointer accepted")
	}
}

func TestManifestValidation(t *testing.T) {
	h := ExeTextHandler{}
	_, u := unpackFixture(t, "softdisk")
	for _, mutate := range []func(*resource.Manifest){
		func(m *resource.Manifest) { m.Handler = "other" }, func(m *resource.Manifest) { m.Metadata = []byte("bad") },
		func(m *resource.Manifest) { m.Metadata = []byte(`{"original_exe":"bad base64"}`) },
		func(m *resource.Manifest) { m.Metadata = []byte(`{"original_exe":""}`) },
	} {
		m := u.Manifest
		mutate(&m)
		if _, err := h.Repack(m, u.Translatable, u.Meta); err == nil {
			t.Fatal("accepted malformed manifest")
		}
	}
	var m exeMetadata
	if err := json.Unmarshal(u.Manifest.Metadata, &m); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*exeMetadata){
		func(m *exeMetadata) { m.Strings = nil }, func(m *exeMetadata) { m.Strings[0].ID = "unknown" },
		func(m *exeMetadata) { m.Strings = append(m.Strings, m.Strings[0]) }, func(m *exeMetadata) { m.Strings[0].Offset++ }, func(m *exeMetadata) { m.Strings[0].MaxLength++ },
	} {
		var m exeMetadata
		if err := json.Unmarshal(u.Manifest.Metadata, &m); err != nil {
			t.Fatal(err)
		}
		change(&m)
		manifest := u.Manifest
		manifest.Metadata, _ = json.Marshal(m)
		if _, err := h.Repack(manifest, u.Translatable, u.Meta); err == nil {
			t.Fatal("accepted edited offset table")
		}
	}
}

func TestEditableAssetValidation(t *testing.T) {
	h := ExeTextHandler{}
	_, u := unpackFixture(t, "softdisk")
	for _, change := range []func(resource.Assets){
		func(a resource.Assets) { delete(a, "strings.xlsx") },
		func(a resource.Assets) {
			a["strings.xlsx"] = badWorkbook(t, [][]any{{"id", "text"}, {"typo", "a"}}, "strings")
		},
		func(a resource.Assets) { a["strings.xlsx"] = badWorkbook(t, [][]any{{"id", "text"}}, "strings") },
		func(a resource.Assets) { delete(a, "exit_screen.xb") },
		func(a resource.Assets) {
			x, _ := ParseXBIN(a["exit_screen.xb"])
			x.Width = 40
			x.Height = 50
			a["exit_screen.xb"], _ = BuildXBIN(x)
		},
		func(a resource.Assets) {
			x, _ := ParseXBIN(a["exit_screen.xb"])
			x.FontHeight = 8
			x.Font = x.Font[:2048]
			a["exit_screen.xb"], _ = BuildXBIN(x)
		},
	} {
		a := mapClone(u.Translatable)
		change(a)
		if _, err := h.Repack(u.Manifest, a, u.Meta); err == nil {
			t.Fatal("accepted invalid editable asset")
		}
	}
	// Blank spreadsheet rows are harmless, but a missing required id is not.
	b := badWorkbook(t, [][]any{{"id", "text"}, nil, {"x", "value"}}, "strings")
	rows, err := readStrings(b, []StringRow{{ID: "x", Text: "original"}})
	if err != nil || rows["x"] != "value" {
		t.Fatal("blank row", err)
	}
}

func TestLocalBufferStringLimit(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	setText(t, &u, "bload_error", string(bytes.Repeat([]byte{'A'}, 400)))
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("Bload strcpy overflow accepted")
	}
	for _, pair := range [][2]string{{string(bytes.Repeat([]byte{'A'}, 400)), " ?"}, {"Continue from level ", string(bytes.Repeat([]byte{'B'}, 80))}, {string(bytes.Repeat([]byte{'A'}, 40)), string(bytes.Repeat([]byte{'B'}, 39))}} {
		_, u := unpackFixture(t, "softdisk")
		setText(t, &u, "continue_level", pair[0])
		setText(t, &u, "continue_suffix", pair[1])
		if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
			t.Fatal("Continue strcpy/strcat buffer overflow accepted")
		}
	}
}

func TestPartialManifestAndNoExitFont(t *testing.T) {
	original, u := unpackFixture(t, "softdisk")
	var m exeMetadata
	if err := json.Unmarshal(u.Manifest.Metadata, &m); err != nil {
		t.Fatal(err)
	}
	rows, err := readUnpackedStrings(t, u)
	if err != nil {
		t.Fatal(err)
	}
	m.Strings = m.Strings[:83]
	u.Manifest.Metadata, _ = json.Marshal(m)
	var selected []StringRow
	for _, e := range m.Strings {
		selected = append(selected, StringRow{ID: e.ID, Text: rows[e.ID]})
	}
	u.Translatable["strings.xlsx"], _ = writeStrings(selected)
	f, err := readGameFont(code(rawFixture(t)))
	if err != nil {
		t.Fatal(err)
	}
	u.Translatable["strings.xlsx"], _ = addFontSheet(u.Translatable["strings.xlsx"], f)
	x, _ := ParseXBIN(u.Translatable["exit_screen.xb"])
	x.Font = nil
	u.Translatable["exit_screen.xb"], _ = BuildXBIN(x)
	out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
	if err != nil || !bytes.Equal(out, original) {
		t.Fatal("legacy manifest roundtrip", err)
	}
	x.Image[0] = 'X'
	u.Translatable["exit_screen.xb"], _ = BuildXBIN(x)
	out, err = (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
	if err != nil || code(out)[exitCallOffset] != 0xb8 {
		t.Fatal("screen without embedded font", err)
	}
}

func TestExistingVGAFontStubCanBeEdited(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	x, _ := ParseXBIN(u.Translatable["exit_screen.xb"])
	x.Font[0] ^= 1
	u.Translatable["exit_screen.xb"], _ = BuildXBIN(x)
	out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := (ExeTextHandler{}).Unpack("DAVE.EXE", out)
	if err != nil {
		t.Fatal(err)
	}
	same, err := (ExeTextHandler{}).Repack(u2.Manifest, u2.Translatable, u2.Meta)
	if err != nil || !bytes.Equal(same, out) {
		t.Fatal("existing stub roundtrip", err)
	}
	x2, _ := ParseXBIN(u2.Translatable["exit_screen.xb"])
	if !bytes.Equal(x.Font, x2.Font) {
		t.Fatal("embedded font not exported")
	}
	x2.Font[1] ^= 1
	u2.Translatable["exit_screen.xb"], _ = BuildXBIN(x2)
	out2, err := (ExeTextHandler{}).Repack(u2.Manifest, u2.Translatable, u2.Meta)
	if err != nil || len(out2) != len(out) || word(out2, 6) != word(out, 6) {
		t.Fatal("existing font edit appended redundant stub", err)
	}
}

func TestVGAStubAndResizeValidation(t *testing.T) {
	raw := rawFixture(t)
	patched, err := patchExitFont(bytes.Clone(raw), bundledExitFont)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func([]byte){
		func(c []byte) { c[exitCallOffset] = 0xcc },
		func(c []byte) { putWord(c, exitCallOffset+3, 0xffff) },
		func(c []byte) { stub := word(c, exitCallOffset+3)*16 + word(c, exitCallOffset+1); c[stub] ^= 1 },
		func(c []byte) {
			stub := word(c, exitCallOffset+3)*16 + word(c, exitCallOffset+1)
			copy(c[stub:], buildFontStub(0xffff))
		},
	} {
		b := bytes.Clone(patched)
		change(code(b))
		if _, _, err := exitFont(code(b)); err == nil {
			t.Fatal("accepted bad VGA stub")
		}
		if _, err := patchExitFont(b, bundledExitFont); err == nil {
			t.Fatal("patched bad VGA stub")
		}
	}
	raw = rawFixture(t)
	putWord(raw, 6, 25)
	if _, err := patchExitFont(raw, bundledExitFont); err == nil {
		t.Fatal("overwrote full relocation table")
	}
	for _, change := range []func([]byte){
		func(c []byte) { c[0x6b] = 0 }, func(c []byte) { c[0x8c] = 0 }, func(c []byte) { putWord(c, dsBase+0x4998, 0xffff) },
	} {
		b := bytes.Clone(patched)
		change(code(b))
		if err := finishMZ(b, word(b, 8)*16, word(code(b), 0x6d)); err == nil {
			t.Fatal("accepted unsafe startup/stack")
		}
	}
	_, u := unpackFixture(t, "softdisk")
	bad := rawFixture(t)
	putWord(bad, 6, 25)
	u = replaceOriginal(t, u, bad)
	x, _ := ParseXBIN(u.Translatable["exit_screen.xb"])
	x.Font[0] ^= 1
	u.Translatable["exit_screen.xb"], _ = BuildXBIN(x)
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("Repack overwrote full relocation table")
	}
	_, u = unpackFixture(t, "softdisk")
	bad = rawFixture(t)
	code(bad)[0x6b] = 0
	u = replaceOriginal(t, u, bad)
	setText(t, &u, "win_line13", "this must relocate a string")
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("Repack accepted unsafe resize")
	}
}

func TestUnsafeOriginalResizeOnShortEdit(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	raw := rawFixture(t)
	putWord(code(raw), 0x6d, 0xfff0)
	u = replaceOriginal(t, u, raw)
	setText(t, &u, "win_line13", "win")
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("unsafe original data-size plus stack accepted")
	}
}
