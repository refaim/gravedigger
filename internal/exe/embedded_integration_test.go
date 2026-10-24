package exe

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/refaim/gravedigger/internal/compression"
	"github.com/refaim/gravedigger/internal/resource"
)

func embeddedTestImage(t *testing.T, raw []byte) *image.NRGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	result := image.NewNRGBA(img.Bounds())
	draw.Draw(result, result.Bounds(), img, image.Point{}, draw.Src)
	return result
}

func TestEXEEmbeddedArtworkRoundtrip(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			original, editable := unpackFixture(t, variant)
			count := 0
			for name := range editable.Translatable {
				if strings.HasPrefix(name, "exe/") {
					count++
				}
			}
			if count != 73 {
				t.Fatalf("need 40 death frames, THE END, and 32 UI tiles; got %d embedded PNGs", count)
			}
			raw, err := compression.DecompressEXE(original)
			if err != nil {
				t.Fatal(err)
			}
			expected := bytes.Clone(code(raw))
			// Independent native addresses, including the UI prefix before frames.
			for _, edit := range []struct {
				name                      string
				base, width, height, x, y int
			}{
				{"exe/ui_01.png", 0x188e0, 8, 8, 3, 3},
				{"exe/knife_1.png", 0x189e0, 48, 48, 12, 14},
				{"exe/the_end.png", 0x1b6e0, 208, 56, 12, 12},
			} {
				img := embeddedTestImage(t, editable.Translatable[edit.name])
				if img.Bounds() != image.Rect(0, 0, edit.width, edit.height) {
					t.Fatalf("%s geometry: %v", edit.name, img.Bounds())
				}
				white := img.NRGBAAt(edit.x, edit.y) != (color.NRGBA{R: 255, G: 255, B: 255, A: 255})
				newColor := color.NRGBA{A: 255}
				if white {
					newColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				}
				img.SetNRGBA(edit.x, edit.y, newColor)
				var encoded bytes.Buffer
				if err := png.Encode(&encoded, img); err != nil {
					t.Fatal(err)
				}
				editable.Translatable[edit.name] = encoded.Bytes()
				for plane := range 4 {
					at := edit.base + plane*0x33b0 + edit.y*(edit.width/8) + edit.x/8
					mask := byte(1 << (7 - edit.x%8))
					expected[at] &^= mask
					if white {
						expected[at] |= mask
					}
				}
			}
			out, err := (ExeTextHandler{}).Repack(editable.Manifest, editable.Translatable, editable.Meta)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(out, original) || !bytes.Equal(code(out), expected) {
				t.Fatal("embedded edits did not change exactly their native pixel bits")
			}
			reopened, err := (ExeTextHandler{}).Unpack("DAVE.EXE", out)
			if err != nil {
				t.Fatal(err)
			}
			for name, before := range editable.Translatable {
				if strings.HasPrefix(name, "exe/") && !bytes.Equal(embeddedTestImage(t, before).Pix, embeddedTestImage(t, reopened.Translatable[name]).Pix) {
					t.Fatal("reopened embedded PNG differs", name)
				}
			}
			again, err := (ExeTextHandler{}).Repack(reopened.Manifest, reopened.Translatable, reopened.Meta)
			if err != nil || !bytes.Equal(again, out) {
				t.Fatal("edited embedded graphics do not roundtrip byte-exact", err)
			}
		})
	}
}

func TestEXEEmbeddedArtworkValidation(t *testing.T) {
	_, editable := unpackFixture(t, "softdisk")
	for _, name := range []string{"exe/the_end.png", "exe/ui_01.png"} {
		t.Run("missing "+name, func(t *testing.T) {
			assets := mapClone(editable.Translatable)
			delete(assets, name)
			if _, err := (ExeTextHandler{}).Repack(editable.Manifest, assets, editable.Meta); err == nil {
				t.Fatal("accepted a missing required embedded PNG", name)
			}
		})
	}
	t.Run("missing all new artwork", func(t *testing.T) {
		assets := mapClone(editable.Translatable)
		for name := range assets {
			if strings.HasPrefix(name, "exe/") {
				delete(assets, name)
			}
		}
		if _, err := (ExeTextHandler{}).Repack(editable.Manifest, assets, editable.Meta); err == nil {
			t.Fatal("new manifests must detect removal of the entire embedded artwork directory")
		}
	})
	bad := rawFixture(t)
	putWord(code(bad), 0x12290, 0xffff)
	t.Run("unsupported header unpack", func(t *testing.T) {
		if _, err := (ExeTextHandler{}).Unpack("DAVE.EXE", bad); err == nil {
			t.Fatal("unpack accepted an unsupported embedded graphics header")
		}
	})
	t.Run("unsupported header repack", func(t *testing.T) {
		changed := replaceOriginal(t, editable, bad)
		if _, err := (ExeTextHandler{}).Repack(changed.Manifest, changed.Translatable, changed.Meta); err == nil {
			t.Fatal("repack accepted an unsupported embedded graphics header")
		}
	})
}

func legacyEmbeddedProject(t *testing.T, editable resource.Unpacked) resource.Unpacked {
	t.Helper()
	originalTexts, err := readUnpackedStrings(t, editable)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(editable.Manifest.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	delete(metadata, "embedded_graphics")
	var entries []stringMetadata
	if err := json.Unmarshal(metadata["strings"], &entries); err != nil {
		t.Fatal(err)
	}
	var originalEntries []stringMetadata
	for _, entry := range entries {
		if !strings.HasSuffix(entry.ID, "_binding_separator") {
			originalEntries = append(originalEntries, entry)
		}
	}
	metadata["strings"], _ = json.Marshal(originalEntries)
	editable.Manifest.Metadata, _ = json.Marshal(metadata)
	for name := range editable.Translatable {
		if strings.HasPrefix(name, "exe/") {
			delete(editable.Translatable, name)
		}
	}
	f, err := excelize.OpenReader(bytes.NewReader(editable.Translatable["strings.xlsx"]))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetSheetName(sheet, "strings"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetSheetRow("strings", "A1", &[]any{"id", "text"}); err != nil {
		t.Fatal(err)
	}
	for i, entry := range originalEntries {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		if err := f.SetSheetRow("strings", cell, &[]any{entry.ID, originalTexts[entry.ID]}); err != nil {
			t.Fatal(err)
		}
	}
	for row := len(rows); row > len(originalEntries)+1; row-- {
		if err := f.RemoveRow("strings", row); err != nil {
			t.Fatal(err)
		}
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	editable.Translatable["strings.xlsx"] = b.Bytes()
	return editable
}

func TestEXEEmbeddedArtworkKeepsExistingProjects(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			original, editable := unpackFixture(t, variant)
			fullArtwork := mapClone(editable.Translatable)
			artwork := editable.Translatable["exe/the_end.png"]
			legacy := legacyEmbeddedProject(t, editable)
			out, err := (ExeTextHandler{}).Repack(legacy.Manifest, legacy.Translatable, legacy.Meta)
			if err != nil || !bytes.Equal(out, original) {
				t.Fatal("existing 3.0.1 projects without embedded PNGs must remain byte-exact", err)
			}
			atlasWorkbook(t, &legacy, func(f *excelize.File) {
				rows, err := f.GetRows("strings")
				if err != nil {
					t.Fatal(err)
				}
				for i, row := range rows {
					if row[0] == "win_congrats" {
						cell, _ := excelize.CoordinatesToCellName(2, i+1)
						if err := f.SetCellStr("strings", cell, "A LONGER TRANSLATION OF CONGRATULATIONS!"); err != nil {
							t.Fatal(err)
						}
					}
				}
			})
			out, err = (ExeTextHandler{}).Repack(legacy.Manifest, legacy.Translatable, legacy.Meta)
			if err != nil || word(code(out), 0xf28) != 1024 {
				t.Fatal("legacy text edits must retain the corrected native instruction", err)
			}
			legacy.Translatable["exe/the_end.png"] = artwork
			if _, err := (ExeTextHandler{}).Repack(legacy.Manifest, legacy.Translatable, legacy.Meta); err == nil {
				t.Fatal("adding partial artwork to an old project must not silently ignore it")
			}
			for name, data := range fullArtwork {
				if strings.HasPrefix(name, "exe/") {
					legacy.Translatable[name] = data
				}
			}
			withArtwork, err := (ExeTextHandler{}).Repack(legacy.Manifest, legacy.Translatable, legacy.Meta)
			if err != nil || !bytes.Equal(withArtwork, out) {
				t.Fatal("old projects with a complete unchanged artwork bank must remain byte-exact", err)
			}
		})
	}
}
