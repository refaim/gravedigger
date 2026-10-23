package exe

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/refaim/gravedigger/internal/resource"
)

func atlasWorkbook(t *testing.T, u *resource.Unpacked, edit func(*excelize.File)) {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(u.Translatable["strings.xlsx"]))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	edit(f)
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	u.Translatable["strings.xlsx"] = b.Bytes()
}

func atlasPicture(t *testing.T, u resource.Unpacked) *image.NRGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(u.Translatable["game_font.png"]))
	if err != nil {
		t.Fatal(err)
	}
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, image.Point{}, draw.Src)
	return out
}

func storeAtlas(t *testing.T, u *resource.Unpacked, img image.Image) {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	u.Translatable["game_font.png"] = b.Bytes()
}

func TestFontAtlasExportRoundtrip(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			original, u := unpackFixture(t, variant)
			if _, ok := u.Translatable["game_font.json"]; ok {
				t.Fatal("new export must not duplicate the PNG font in JSON")
			}
			img := atlasPicture(t, u)
			if img.Bounds() != image.Rect(0, 0, 320, 120) {
				t.Fatal("incorrect atlas dimensions", img.Bounds())
			}
			if img.NRGBAAt(0, 0) != (color.NRGBA{R: 96, G: 96, B: 96, A: 255}) {
				t.Fatal("each cell requires an explicit visible border")
			}
			left, top := atlasCell(33)
			for point, want := range map[image.Point]color.NRGBA{
				{X: left + 1, Y: top + 1}: {R: 255, G: 255, B: 255, A: 255},
				{X: left, Y: top}:         {A: 255},
				{X: left + 8, Y: top + 1}: {},
			} {
				if got := img.NRGBAAt(point.X, point.Y); got != want {
					t.Fatalf("pixel %v = %v, want %v", point, got, want)
				}
			}
			atlasWorkbook(t, &u, func(f *excelize.File) {
				if f.GetSheetName(0) != "strings" || f.GetSheetName(1) != "Шрифт" {
					t.Fatal("strings must remain the first sheet", f.GetSheetList())
				}
				rows, err := f.GetRows("Шрифт")
				if err != nil || len(rows) != 97 {
					t.Fatal("font sheet requires all 96 numbered cells", err, len(rows))
				}
				space, _ := f.GetCellValue("Шрифт", "B2")
				if space != " " || rows[0][0] != "number" || rows[0][1] != "symbol" {
					t.Fatal("font sheet schema/space", rows[0], space)
				}
				for i, row := range rows[1:] {
					if row[0] != strconv.Itoa(i+1) {
						t.Fatal("font sheet must show user cell numbers 1..96", row)
					}
					if i < 95 && row[1] != string(rune(i+32)) {
						t.Fatal("original game export must retain its original Latin character mapping", row)
					}
				}
			})
			out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
			if err != nil || !bytes.Equal(original, out) {
				t.Fatal("PNG/XLSX no-op must preserve the original EXE exactly", err)
			}
		})
	}
}

func TestFontAtlasRequiresPNGWithAnyManifest(t *testing.T) {
	for _, name := range []string{"JSON only", "no font assets"} {
		t.Run(name, func(t *testing.T) {
			_, u := unpackFixture(t, "softdisk")
			delete(u.Translatable, "game_font.png")
			if name == "JSON only" {
				f, err := readGameFont(code(rawFixture(t)))
				if err != nil {
					t.Fatal(err)
				}
				u.Translatable["game_font.json"], _ = json.Marshal(f)
			}
			if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
				t.Fatal("PNG is required even for older manifests; JSON must not be loaded")
			}
		})
	}
}

func TestFontAtlasEditsGlyphAndUnicode(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			_, u := unpackFixture(t, variant)
			img := atlasPicture(t, u)
			// Slot 126 is the last printable cell. Extend it from nine to ten
			// pixels, across the native two-byte row boundary.
			left, top := atlasCell(126)
			for y := 0; y < 10; y++ {
				for x := 0; x < 16; x++ {
					img.SetNRGBA(left+x, top+y, color.NRGBA{})
				}
			}
			img.SetNRGBA(left+1, top+1, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			img.SetNRGBA(left+9, top+8, color.NRGBA{A: 255})
			storeAtlas(t, &u, img)
			atlasWorkbook(t, &u, func(f *excelize.File) {
				if err := f.SetCellStr("Шрифт", "B96", "Я"); err != nil {
					t.Fatal(err)
				}
			})
			setText(t, &u, "win_line13", "ЯЯ")
			out, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
			if err != nil {
				t.Fatal(err)
			}
			f, err := readGameFont(code(out))
			if err != nil {
				t.Fatal(err)
			}
			g := f.Glyphs[94]
			if g.Width != 10 || g.Pixels[1] != 0x40 || g.Shadow[1] != 0x40 || g.Pixels[8] != 0 || g.Shadow[8] != 0x4000 {
				t.Fatal("PNG pixels, outline, or automatic width lost", g)
			}
			at := dsBase + word(code(out), 0x11ce)
			if !bytes.Equal(code(out)[at:at+3], []byte{126, 126, 0}) {
				t.Fatal("font sheet Unicode mapping not used")
			}
			again, err := (ExeTextHandler{}).Unpack("DAVE.EXE", out)
			if err != nil {
				t.Fatal(err)
			}
			got := atlasPicture(t, again)
			if !bytes.Equal(got.Pix, img.Pix) {
				t.Fatal("reopened EXE has different PNG pixels")
			}
			same, err := (ExeTextHandler{}).Repack(again.Manifest, again.Translatable, again.Meta)
			if err != nil || !bytes.Equal(same, out) {
				t.Fatal("edited raw EXE must also roundtrip exactly", err)
			}
		})
	}
}

func TestFontAtlasRejectsMissingAssets(t *testing.T) {
	for _, name := range []string{"missing PNG", "empty PNG", "missing sheet", "missing text"} {
		t.Run(name, func(t *testing.T) {
			_, u := unpackFixture(t, "softdisk")
			switch name {
			case "missing PNG":
				delete(u.Translatable, "game_font.png")
			case "empty PNG":
				u.Translatable["game_font.png"] = nil
			case "missing sheet":
				atlasWorkbook(t, &u, func(f *excelize.File) {
					if err := f.DeleteSheet("Шрифт"); err != nil {
						t.Fatal(err)
					}
				})
			case "missing text":
				atlasWorkbook(t, &u, func(f *excelize.File) {
					if err := f.RemoveRow("strings", 2); err != nil {
						t.Fatal(err)
					}
				})
			}
			if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
				t.Fatal("accepted incomplete/ambiguous font assets")
			}
		})
	}
}

func TestFontAtlasChecksCombinedRecordCapacity(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	img := atlasPicture(t, u)
	for i := 0; i < 95; i++ {
		left, top := atlasCell(i + 32)
		img.SetNRGBA(left+9, top, color.NRGBA{A: 255})
	}
	storeAtlas(t, &u, img)
	if _, err := (ExeTextHandler{}).Repack(u.Manifest, u.Translatable, u.Meta); err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatal("oversized font records must not overwrite adjacent game resources", err)
	}
}

func TestFontAtlasCustomNativeMaskRoundtrip(t *testing.T) {
	raw := rawFixture(t)
	f, err := readGameFont(code(raw))
	if err != nil {
		t.Fatal(err)
	}
	// Existing custom executables can contain independent masks and stored
	// pixels beyond the advance. Export must preserve them without loss.
	f.Glyphs[94].Width = 9
	f.Glyphs[94].Pixels[0] = 0x0180
	f.Glyphs[94].Shadow[0] = 0
	if _, err := applyGameFont(code(raw), f); err != nil {
		t.Fatal(err)
	}
	again, err := (ExeTextHandler{}).Unpack("DAVE.EXE", raw)
	if err != nil {
		t.Fatal(err)
	}
	img := atlasPicture(t, again)
	left, top := atlasCell(126)
	if got := img.NRGBAAt(left, top); got != (color.NRGBA{G: 255, B: 255, A: 255}) {
		t.Fatal("independent foreground needs its own lossless color", got)
	}
	same, err := (ExeTextHandler{}).Repack(again.Manifest, again.Translatable, again.Meta)
	if err != nil || !bytes.Equal(same, raw) {
		t.Fatal("custom native PNG no-op corrupted the font", err)
	}
}
