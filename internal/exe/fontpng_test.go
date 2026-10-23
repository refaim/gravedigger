package exe

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"reflect"
	"strings"
	"testing"
)

func emptyAtlasFont() GameFont {
	f := GameFont{Version: 1, Height: 10}
	for slot := 32; slot < 128; slot++ {
		f.Glyphs = append(f.Glyphs, GameGlyph{Slot: slot, Unicode: string(rune(slot)), Width: 5, Pixels: make([]uint16, 10), Shadow: make([]uint16, 10)})
	}
	return f
}

func atlasPNGBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func editAtlasPNG(t *testing.T, raw []byte, edit func(*image.NRGBA)) []byte {
	t.Helper()
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(decoded.Bounds())
	draw.Draw(img, img.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	edit(img)
	return atlasPNGBytes(t, img)
}

func atlasCell(slot int) (int, int) {
	return (slot - 32) % 16 * gameFontCellSize, (slot - 32) / 16 * gameFontCellSize
}

func clearAtlasCell(img *image.NRGBA, slot int) {
	left, top := atlasCell(slot)
	for y := 0; y < gameFontCellSize/2; y++ {
		for x := 0; x < gameFontCellSize; x++ {
			img.SetNRGBA(left+x, top+y, color.NRGBA{})
		}
	}
}

func TestGameFontPNGNativeBitsAndIndependentRoundtrip(t *testing.T) {
	baseline := emptyAtlasFont()
	baseline.Glyphs[0].Width = 9
	baseline.Glyphs[0].Pixels[0] = 0x8009
	baseline.Glyphs[0].Shadow[0] = 0x0188
	baseline.Glyphs[1].Pixels[4] = 0xa0
	baseline.Glyphs[1].Shadow[4] = 0xf0
	baseline.Glyphs[2].Width = 1
	baseline.Glyphs[2].Pixels[0] = 1 // Preserve native padding beyond the advance.
	baseline.Glyphs[2].Shadow[0] = 1
	baseline.Glyphs[64].Unicode = "Я"
	baseline.Glyphs[95].Width = 10
	baseline.Glyphs[95].Pixels[9] = 0x8001
	baseline.Glyphs[95].Shadow[9] = 0x8001
	raw := gameFontPNG(baseline)
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, gameFontAtlasWidth, gameFontAtlasHeight) {
		t.Fatal("wrong atlas dimensions", img.Bounds())
	}
	white, black, cyan := color.NRGBA{255, 255, 255, 255}, color.NRGBA{0, 0, 0, 255}, color.NRGBA{0, 255, 255, 255}
	for _, check := range []struct {
		x, y int
		want color.NRGBA
	}{
		{0, 0, black},
		{15, 0, black},
		{4, 0, white},
		{7, 0, cyan},
		{8, 0, cyan},
		{gameFontCellSize, 4, white},
		{gameFontCellSize + 1, 4, black},
		{gameFontCellSize + 2, 4, white},
		{gameFontCellSize + 3, 4, black},
		{gameFontCellSize + 4, 4, color.NRGBA{}},
		{2*gameFontCellSize + 7, 0, white},
		{15*gameFontCellSize + 7, 5*gameFontCellSize + 9, white},
		{15*gameFontCellSize + 8, 5*gameFontCellSize + 9, white},
		{gameFontAtlasWidth - 1, gameFontAtlasHeight - 1, color.NRGBA{}},
	} {
		if got := color.NRGBAModel.Convert(img.At(check.x, check.y)).(color.NRGBA); got != check.want {
			t.Fatalf("pixel (%d,%d): got %v, want %v", check.x, check.y, got, check.want)
		}
	}
	got, err := readGameFontPNG(raw, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, baseline) {
		t.Fatal("native masks, mapping, or original widths changed")
	}
	got.Glyphs[0].Pixels[0] = 0
	got.Glyphs[1].Shadow[4] = 0
	got.Glyphs[64].Unicode = "Б"
	if baseline.Glyphs[0].Pixels[0] != 0x8009 || baseline.Glyphs[1].Shadow[4] != 0xf0 || baseline.Glyphs[64].Unicode != "Я" {
		t.Fatal("imported font aliases its baseline")
	}
}

func TestGameFontPNGOriginalNativeBytes(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			original, _ := unpackFixture(t, variant)
			mz, start, err := decodeOriginal(original)
			if err != nil {
				t.Fatal(err)
			}
			image := mz[start:]
			before := bytes.Clone(image)
			baseline, err := readGameFont(image)
			if err != nil {
				t.Fatal(err)
			}
			got, err := readGameFontPNG(gameFontPNG(baseline), baseline)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := applyGameFont(image, got); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(image, before) {
				t.Fatal("PNG no-op changed original native bytes")
			}
		})
	}
}

func TestGameFontPNGAutomaticAdvance(t *testing.T) {
	baseline := emptyAtlasFont()
	slot := int('A')
	baseline.Glyphs[slot-32].Pixels[1] = 0x20
	baseline.Glyphs[slot-32].Shadow[1] = 0x70
	left, top := atlasCell(slot)
	white, black, cyan := color.NRGBA{255, 255, 255, 255}, color.NRGBA{0, 0, 0, 255}, color.NRGBA{0, 255, 255, 255}
	for _, check := range []struct {
		name        string
		edit        func(*image.NRGBA)
		width       int
		ink, shadow uint16
	}{
		{"foreground only grows", func(img *image.NRGBA) { img.SetNRGBA(left+5, top+1, cyan) }, 6, 0x24, 0x70},
		{"shadow only crosses byte boundary", func(img *image.NRGBA) { img.SetNRGBA(left+8, top+1, black) }, 9, 0x20, 0x8070},
		{"both planes reach maximum", func(img *image.NRGBA) { img.SetNRGBA(left+9, top+1, white) }, 10, 0x4020, 0x4070},
		{"narrower byte boundary", func(img *image.NRGBA) { clearAtlasCell(img, slot); img.SetNRGBA(left+7, top+1, white) }, 8, 1, 1},
		{"shrinks", func(img *image.NRGBA) { clearAtlasCell(img, slot); img.SetNRGBA(left+1, top+1, white) }, 2, 0x40, 0x40},
		{"empty preserves advance", func(img *image.NRGBA) { clearAtlasCell(img, slot) }, 5, 0, 0},
	} {
		t.Run(check.name, func(t *testing.T) {
			raw := editAtlasPNG(t, gameFontPNG(baseline), check.edit)
			got, err := readGameFontPNG(raw, baseline)
			if err != nil {
				t.Fatal(err)
			}
			g := got.Glyphs[slot-32]
			if g.Width != check.width || g.Pixels[1] != check.ink || g.Shadow[1] != check.shadow {
				t.Fatalf("wrong imported glyph: %#v", g)
			}
			if g.Unicode != "A" || g.Slot != slot {
				t.Fatal("bitmap edit changed character mapping")
			}
		})
	}
}

func TestGameFontPNGRejectsOutOfCellGeometry(t *testing.T) {
	baseline := emptyAtlasFont()
	for _, check := range []struct {
		name       string
		slot, x, y int
		color      color.NRGBA
		contains   string
	}{
		{"eleven columns", 'A', 10, 0, color.NRGBA{255, 255, 255, 255}, "width"},
		{"sixteen columns", 'A', 15, 0, color.NRGBA{0, 255, 255, 255}, "width"},
		{"first spare row", 'A', 0, 10, color.NRGBA{0, 0, 0, 255}, "row"},
		{"last spare row", 127, 15, 15, color.NRGBA{255, 255, 255, 255}, "row"},
		{"first spare column", 'A', 16, 0, color.NRGBA{0, 0, 0, 255}, "canvas"},
		{"last spare column", 'A', 31, 15, color.NRGBA{0, 255, 255, 255}, "canvas"},
	} {
		t.Run(check.name, func(t *testing.T) {
			left, top := atlasCell(check.slot)
			raw := editAtlasPNG(t, gameFontPNG(baseline), func(img *image.NRGBA) { img.SetNRGBA(left+check.x, top+check.y, check.color) })
			_, err := readGameFontPNG(raw, baseline)
			if err == nil || !strings.Contains(err.Error(), check.contains) {
				t.Fatalf("expected %s error, got %v", check.contains, err)
			}
			wantSlot := "slot 65"
			if check.slot == 127 {
				wantSlot = "slot 127"
			}
			if !strings.Contains(err.Error(), wantSlot) {
				t.Fatal("error does not identify slot", err)
			}
		})
	}
	baseline.Glyphs[0].Width = 9
	baseline.Glyphs[0].Pixels[0] = 0x0100 // Native x=15 padding is valid while unchanged.
	raw := editAtlasPNG(t, gameFontPNG(baseline), func(img *image.NRGBA) { img.SetNRGBA(0, 0, color.NRGBA{255, 255, 255, 255}) })
	if _, err := readGameFontPNG(raw, baseline); err == nil || !strings.Contains(err.Error(), "slot 32") {
		t.Fatal("changed padding must not silently truncate", err)
	}
}

func TestGameFontPNGInvalidInput(t *testing.T) {
	baseline := emptyAtlasFont()
	full := gameFontPNG(baseline)
	truncated := full[:33]
	if _, err := png.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Fatal("truncated-body fixture must retain a valid config", err)
	}
	wrongWidth := atlasPNGBytes(t, image.NewNRGBA(image.Rect(0, 0, gameFontAtlasWidth-1, gameFontAtlasHeight)))
	wrongHeight := atlasPNGBytes(t, image.NewNRGBA(image.Rect(0, 0, gameFontAtlasWidth, gameFontAtlasHeight-1)))
	for _, check := range []struct {
		name     string
		raw      []byte
		contains string
	}{
		{"bad config", []byte("not PNG"), "PNG"},
		{"truncated body", truncated, "pixels"},
		{"wrong width before body decode", wrongWidth[:33], "dimensions"},
		{"wrong height", wrongHeight, "dimensions"},
	} {
		t.Run(check.name, func(t *testing.T) {
			_, err := readGameFontPNG(check.raw, baseline)
			if err == nil || !strings.Contains(err.Error(), check.contains) {
				t.Fatalf("expected %s error, got %v", check.contains, err)
			}
		})
	}
	for _, check := range []struct {
		name string
		c    color.NRGBA64
	}{
		{"red", color.NRGBA64{65535, 0, 0, 65535}},
		{"gray", color.NRGBA64{32768, 32768, 32768, 65535}},
		{"partial alpha", color.NRGBA64{65535, 65535, 65535, 32768}},
		{"near opaque alpha16", color.NRGBA64{65535, 65535, 65535, 65534}},
		{"near white16", color.NRGBA64{65534, 65535, 65535, 65535}},
		{"near cyan16", color.NRGBA64{1, 65535, 65535, 65535}},
	} {
		t.Run(check.name, func(t *testing.T) {
			decoded, err := png.Decode(bytes.NewReader(full))
			if err != nil {
				t.Fatal(err)
			}
			img := image.NewNRGBA64(decoded.Bounds())
			draw.Draw(img, img.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
			img.SetNRGBA64(gameFontCellSize, 0, check.c)
			_, err = readGameFontPNG(atlasPNGBytes(t, img), baseline)
			if err == nil || !strings.Contains(err.Error(), "color") || !strings.Contains(err.Error(), "slot 33") {
				t.Fatal("unsupported pixel accepted or not identified", err)
			}
		})
	}
}

func TestGameFontPNGPalettedAndTransparentPixels(t *testing.T) {
	baseline := emptyAtlasFont()
	decoded, err := png.Decode(bytes.NewReader(gameFontPNG(baseline)))
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewPaletted(decoded.Bounds(), color.Palette{color.NRGBA{}, color.NRGBA{255, 255, 255, 255}, color.NRGBA{0, 0, 0, 255}, color.NRGBA{0, 255, 255, 255}, color.NRGBA{128, 128, 128, 255}})
	draw.Draw(img, img.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	img.SetColorIndex(0, 0, 1)
	img.SetColorIndex(1, 0, 2)
	img.SetColorIndex(2, 0, 3)
	got, err := readGameFontPNG(atlasPNGBytes(t, img), baseline)
	if err != nil {
		t.Fatal(err)
	}
	if g := got.Glyphs[0]; g.Pixels[0] != 0xa0 || g.Shadow[0] != 0xc0 || g.Width != 3 {
		t.Fatalf("paletted masks/advance wrong: %#v", g)
	}
	raw := editAtlasPNG(t, gameFontPNG(baseline), func(img *image.NRGBA) { img.SetNRGBA(15, 15, color.NRGBA{255, 0, 0, 0}) })
	got, err = readGameFontPNG(raw, baseline)
	if err != nil || !reflect.DeepEqual(got, baseline) {
		t.Fatal("fully transparent RGB should remain empty", err)
	}
}

func TestGameFontPNGReservedColorControlCannotChange(t *testing.T) {
	baseline := emptyAtlasFont()
	left, top := atlasCell(127)
	for _, c := range []color.NRGBA{{255, 255, 255, 255}, {0, 0, 0, 255}, {0, 255, 255, 255}} {
		raw := editAtlasPNG(t, gameFontPNG(baseline), func(img *image.NRGBA) { img.SetNRGBA(left, top, c) })
		_, err := readGameFontPNG(raw, baseline)
		if err == nil || !strings.Contains(err.Error(), "slot 127") || !strings.Contains(err.Error(), "color-control") {
			t.Fatal("reserved slot bitmap change was accepted or not identified", err)
		}
	}
}

func TestGameFontPNGCellNumbers(t *testing.T) {
	baseline := emptyAtlasFont()
	img, err := png.Decode(bytes.NewReader(gameFontPNG(baseline)))
	if err != nil {
		t.Fatal(err)
	}
	if gameFontCellSize != 32 || img.Bounds() != image.Rect(0, 0, 512, 192) {
		t.Fatal("numbered atlas must use 32x32 cells in a 512x192 image", img.Bounds())
	}
	gray := color.NRGBA{128, 128, 128, 255}
	for _, check := range []struct {
		slot int
		text string
		left int
	}{{32, "32", 8}, {99, "99", 8}, {100, "100", 4}, {127, "127", 4}} {
		left, top := atlasCell(check.slot)
		for digitIndex, digit := range check.text {
			for row := range 16 {
				var got byte
				for col := range 8 {
					pixel := color.NRGBAModel.Convert(img.At(left+check.left+digitIndex*8+col, top+16+row)).(color.NRGBA)
					if pixel == gray {
						got |= 0x80 >> col
					} else if pixel != (color.NRGBA{}) {
						t.Fatalf("slot %d label has a non-gray pixel: %v", check.slot, pixel)
					}
				}
				if want := bundledExitFont[int(digit)*16+row]; got != want {
					t.Fatalf("slot %d digit %c row %d: got %02X, want fixed VGA %02X", check.slot, digit, row, got, want)
				}
			}
		}
		for row := range 16 {
			for col := range gameFontCellSize {
				if col >= check.left && col < check.left+len(check.text)*8 {
					continue
				}
				if _, _, _, alpha := img.At(left+col, top+16+row).RGBA(); alpha != 0 {
					t.Fatalf("slot %d number is not horizontally centered", check.slot)
				}
			}
		}
	}
	for digit := '0'; digit <= '9'; digit++ {
		for row := range 10 {
			baseline.Glyphs[digit-32].Pixels[row] = 0xffff
		}
	}
	changed, err := png.Decode(bytes.NewReader(gameFontPNG(baseline)))
	if err != nil {
		t.Fatal(err)
	}
	for slot := 32; slot < 128; slot++ {
		left, top := atlasCell(slot)
		nonempty := false
		for y := 16; y < gameFontCellSize; y++ {
			for x := range gameFontCellSize {
				a := color.NRGBAModel.Convert(img.At(left+x, top+y)).(color.NRGBA)
				b := color.NRGBAModel.Convert(changed.At(left+x, top+y)).(color.NRGBA)
				if a != b {
					t.Fatalf("editing game digits changed fixed number of slot %d", slot)
				}
				nonempty = nonempty || a == gray
			}
		}
		if !nonempty {
			t.Fatalf("slot %d has no visible cell number", slot)
		}
	}
}

func TestGameFontPNGCellNumbersAreReadOnly(t *testing.T) {
	baseline := emptyAtlasFont()
	raw := gameFontPNG(baseline)
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	gray := color.NRGBA{128, 128, 128, 255}
	left, top := atlasCell(32)
	x, y := -1, -1
	for row := 16; row < gameFontCellSize; row++ {
		for col := range gameFontCellSize {
			if color.NRGBAModel.Convert(img.At(left+col, top+row)).(color.NRGBA) == gray {
				x, y = left+col, top+row
			}
		}
	}
	if x < 0 {
		t.Fatal("slot 32 number is missing")
	}
	for _, check := range []struct {
		name string
		x, y int
		c    color.NRGBA
	}{
		{"erase number pixel", x, y, color.NRGBA{}},
		{"recolor number pixel", x, y, color.NRGBA{255, 255, 255, 255}},
		{"partial label alpha", x, y, color.NRGBA{128, 128, 128, 254}},
		{"add number pixel", left, top + 16, gray},
	} {
		t.Run(check.name, func(t *testing.T) {
			changed := editAtlasPNG(t, raw, func(img *image.NRGBA) { img.SetNRGBA(check.x, check.y, check.c) })
			_, err := readGameFontPNG(changed, baseline)
			if err == nil || !strings.Contains(err.Error(), "label") || !strings.Contains(err.Error(), "slot 32") {
				t.Fatal("cell number edit must identify the read-only label and slot", err)
			}
		})
	}
	precise := image.NewNRGBA64(img.Bounds())
	draw.Draw(precise, precise.Bounds(), img, img.Bounds().Min, draw.Src)
	precise.SetNRGBA64(x, y, color.NRGBA64{32895, 32896, 32896, 65535})
	if _, err := readGameFontPNG(atlasPNGBytes(t, precise), baseline); err == nil || !strings.Contains(err.Error(), "label") {
		t.Fatal("16-bit near-gray must not pass label validation", err)
	}
}
