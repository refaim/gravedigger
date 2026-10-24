package exe

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// The C runtime's sbrk starts at DS:009C, independently of the DOS block-size
// constant at CS:006D. Native DOS snapshots showed stdout allocations replacing
// the appended font and long strings when this initializer stayed at D65Ah.
func TestAppendedAssetsAreOutsideInitialNearHeap(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		for _, fontEdit := range []bool{false, true} {
			_, u := unpackFixture(t, variant)
			if fontEdit {
				x, _ := ParseXBIN(u.Translatable["exit_screen.xb"])
				for i := range x.Font {
					x.Font[i] ^= 0x5a
				}
				u.Translatable["exit_screen.xb"], _ = BuildXBIN(x)
			} else {
				setText(t, &u, "win_line13", "GRAVEDIGGER_NATIVE_LONG_TEXT_"+string(bytes.Repeat([]byte{'A'}, 1024))+"_END")
			}
			out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
			if err != nil {
				t.Fatal(err)
			}
			image := code(out)
			nearEnd := word(image, 0x6d)
			if word(image, dsBase+0x9c) < nearEnd || word(image, dsBase+0x9a) < nearEnd {
				t.Fatalf("%s font=%v: near-heap initializers %04X/%04X overlap static near bytes ending at %04X", variant, fontEdit, word(image, dsBase+0x9a), word(image, dsBase+0x9c), nearEnd)
			}
			if fontEdit {
				stubAt := word(image, exitCallOffset+3)*16 + word(image, exitCallOffset+1)
				fontAt := word(image, stubAt+16) * 16
				if fontAt < dsBase+0x10000 {
					t.Fatal("VGA font still consumes the game's near heap")
				}
				if nearEnd != 0xd65a {
					t.Fatal("font-only edit changed the original near heap budget")
				}
				if image[0x9c] != 0xb8 || word(image, 0x9d) < (len(image)-dsBase+15)/16 {
					t.Fatal("startup DOS/far-heap reserve does not retain far font/stub")
				}
			} else {
				end := word(image, 0x11ce) + 1058
				if nearEnd < end {
					t.Fatal("long-only data overlaps near heap")
				}
			}
			if word(image, 0xc8) != 0xd65a {
				t.Fatal("reserving appended resources changed BSS clear end and would erase the resources")
			}
			if image[0x8c] != 0x75 {
				t.Fatal("startup no longer retains its normal 64KiB DGROUP allocation")
			}
		}
	}
}

const legacyStubHex = "b80300cd101e06565751b800a08ec0b800008ed831f6bac403b80001efb80204efb80406efb80003efbace03b80402efb80500efb80600effc31ffb9000151b90800f3a583c71059e2f4595f5e071f1e56b80300cd105e1fcb"

func obsoleteFontEXE(t *testing.T, kind string) []byte {
	t.Helper()
	raw := rawFixture(t)
	start := word(raw, 8) * 16
	image := code(raw)
	fontAt := (len(image) + 15) &^ 15
	if kind == "legacy-far" {
		fontAt = farFontStart
	}
	raw = append(raw, make([]byte, fontAt-len(image))...)
	raw = append(raw, bundledExitFont...)
	stubAt := fontAt + 4096
	stub, err := hex.DecodeString(legacyStubHex)
	if err != nil {
		t.Fatal(err)
	}
	putWord(stub, 16, fontAt/16)
	if kind == "current-near" {
		stub = buildFontStub(fontAt / 16)
	}
	raw = append(raw, stub...)
	image = raw[start:]
	image[exitCallOffset] = 0x9a
	putWord(image, exitCallOffset+1, 0)
	putWord(image, exitCallOffset+3, stubAt/16)
	relocEnd := word(raw, 24) + word(raw, 6)*4
	for i, field := range []int{exitCallOffset + 3, stubAt + 16} {
		putWord(raw, relocEnd+i*4, field%16)
		putWord(raw, relocEnd+i*4+2, field/16)
	}
	putWord(raw, 6, word(raw, 6)+2)
	putWord(raw, 2, len(raw)%512)
	putWord(raw, 4, (len(raw)+511)/512)
	putWord(image, 0x6d, len(image)-dsBase)
	image[0x8c] = 0xeb
	return raw
}

func TestRejectsObsoleteFontStubs(t *testing.T) {
	for _, kind := range []string{"legacy-near", "legacy-far", "current-near"} {
		t.Run(kind, func(t *testing.T) {
			raw := obsoleteFontEXE(t, kind)
			h := ExeTextHandler{}
			if _, err := h.Unpack("DAVE.EXE", raw); err == nil {
				t.Error("unpack accepted an obsolete VGA font stub")
			}
			if _, err := patchExitFont(bytes.Clone(raw), bundledExitFont); err == nil {
				t.Error("patch accepted an obsolete VGA font stub")
			}
			_, u := unpackFixture(t, "softdisk")
			u = replaceOriginal(t, u, raw)
			if _, err := h.Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
				t.Error("repack accepted an obsolete VGA font stub")
			}
		})
	}
}

func TestRejectsObsoleteDGROUPBypass(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			original, u := unpackFixture(t, variant)
			raw, _, err := decodeOriginal(original)
			if err != nil {
				t.Fatal(err)
			}
			code(raw)[0x8c] = 0xeb
			u = replaceOriginal(t, u, raw)
			setText(t, &u, "win_line13", "win")
			if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
				t.Fatal("edited an executable with the obsolete DGROUP bypass")
			}
		})
	}
}

func TestVGAFontStubDoesNotReloadROMFont(t *testing.T) {
	stub := buildFontStub(0x1234)
	if got := bytes.Count(stub, exitModeInstruction); got != 1 {
		t.Fatalf("mode 3 is called %d times: a second BIOS mode-set replaces the freshly loaded font with ROM glyphs", got)
	}
	// Font RAM is enabled through these registers. Their original mode-3 values
	// must be read and restored, rather than invoking BIOS mode-set a second time.
	if !bytes.Contains(stub, []byte{0xec}) {
		t.Fatal("stub does not read VGA state for restoration")
	}
}

func TestReopenFARFontAndGrowNearText(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	x, _ := ParseXBIN(u.Translatable["exit_screen.xb"])
	x.Font[0] ^= 0x5a
	u.Translatable["exit_screen.xb"], _ = BuildXBIN(x)
	h := ExeTextHandler{}
	out, err := h.Repack(u.Manifest, u.Translatable, u.Meta)
	if err != nil {
		t.Fatal(err)
	}
	u, err = h.Unpack("DAVE.EXE", out)
	if err != nil {
		t.Fatal(err)
	}
	marker := "REOPEN_FAR_FONT_" + string(bytes.Repeat([]byte{'A'}, 1024))
	setText(t, &u, "win_line13", marker)
	out2, err := h.Repack(u.Manifest, u.Translatable, u.Meta)
	if err != nil {
		t.Fatal(err)
	}
	c := code(out2)
	at := dsBase + word(c, 0x11ce)
	if !bytes.Equal(c[at:at+len(marker)+1], append([]byte(marker), 0)) {
		t.Fatal("near text not preserved when reopening FAR font")
	}
	font, _, err := exitFont(c)
	if err != nil || !bytes.Equal(font, x.Font) {
		t.Fatal("reopened FAR font changed", err)
	}
}

func TestNearMemoryManagerBudget(t *testing.T) {
	for _, smallStack := range []bool{false, true} {
		_, u := unpackFixture(t, "softdisk")
		if smallStack {
			raw := rawFixture(t)
			putWord(code(raw), dsBase+0x4998, 0x200)
			u = replaceOriginal(t, u, raw)
		}
		setText(t, &u, "win_line13", string(bytes.Repeat([]byte{'A'}, 3000)))
		if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
			t.Fatalf("MM_Startup pool underflow budget accepted, smallStack=%v", smallStack)
		}
	}
	_, u := unpackFixture(t, "softdisk")
	raw := rawFixture(t)
	putWord(code(raw), dsBase+0x4990, 1)
	u = replaceOriginal(t, u, raw)
	setText(t, &u, "win_line13", "win")
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
		t.Fatal("unsupported CRT explicit heap accepted")
	}
}

func TestFARLayoutValidation(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	raw := rawFixture(t)
	code(raw)[0x9c] = 0x90
	bad := replaceOriginal(t, u, raw)
	setText(t, &bad, "win_line13", "win")
	if _, err := (ExeTextHandler{}).Repack(bad.Manifest, bad.Translatable, bad.Meta); err == nil {
		t.Fatal("unknown far-heap initializer accepted")
	}
	far, err := patchExitFont(rawFixture(t), bundledExitFont)
	if err != nil {
		t.Fatal(err)
	}
	far = append(far, make([]byte, dsBase+0x100000)...)
	if err := finishMZ(far, word(far, 8)*16, 0xd65a); err == nil {
		t.Fatal("far resources outside real-mode space accepted")
	}
}
