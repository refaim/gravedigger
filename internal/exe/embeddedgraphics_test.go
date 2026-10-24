package exe

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/refaim/gravedigger/internal/resource"
)

type embeddedGraphicTestSpec struct {
	name                  string
	width, height, offset int
}

func embeddedGraphicsTestSpecs() []embeddedGraphicTestSpec {
	var specs []embeddedGraphicTestSpec
	for i := range 32 {
		specs = append(specs, embeddedGraphicTestSpec{fmt.Sprintf("exe/ui_%02d.png", i+1), 8, 8, i * 8})
	}
	for group, name := range []string{"knife", "slime", "zombie", "spider", "wolf", "ghost", "skull", "slice"} {
		for frame := range 5 {
			specs = append(specs, embeddedGraphicTestSpec{fmt.Sprintf("exe/%s_%d.png", name, frame+1), 48, 48, 0x100 + (group*5+frame)*0x120})
		}
	}
	return append(specs, embeddedGraphicTestSpec{"exe/the_end.png", 208, 56, 0x2e00})
}

func embeddedGraphicsTestImage(t *testing.T, variant string) []byte {
	t.Helper()
	data, start, err := decodeOriginal(fixture(t, variant))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Clone(data[start:])
}

func embeddedGraphicsPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestEXEEmbeddedGraphicsOriginalsRoundtripAndEdit(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			original := embeddedGraphicsTestImage(t, variant)
			assets, err := unpackEXEGraphics(original)
			if err != nil {
				t.Fatal(err)
			}
			if len(assets) != 73 {
				t.Fatalf("expected 40 death frames, THEEND and 32 UI tiles, got %d", len(assets))
			}
			unchanged := bytes.Clone(original)
			if err := applyEXEGraphics(unchanged, assets); err != nil || !bytes.Equal(unchanged, original) {
				t.Fatal("no-op changed native EXE bytes", err)
			}
			editedAssets := make(resource.Assets, len(assets))
			expected := bytes.Clone(original)
			for _, spec := range embeddedGraphicsTestSpecs() {
				decoded, err := png.Decode(bytes.NewReader(assets[spec.name]))
				if err != nil {
					t.Fatalf("%s: %v", spec.name, err)
				}
				img, ok := decoded.(*image.Paletted)
				if !ok || img.Bounds() != image.Rect(0, 0, spec.width, spec.height) {
					t.Fatalf("%s: wrong indexed PNG geometry: %T %v", spec.name, decoded, decoded.Bounds())
				}
				var want byte
				for plane := range 4 {
					at := 0x188e0 + plane*0x33b0 + spec.offset
					want |= ((original[at] >> 7) & 1) << plane
					expected[at] ^= 0x80
				}
				if got := img.ColorIndexAt(0, 0); got != want {
					t.Fatalf("%s: native pixel %d, PNG pixel %d", spec.name, want, got)
				}
				img.SetColorIndex(0, 0, want^15)
				editedAssets[spec.name] = embeddedGraphicsPNG(t, img)
			}
			edited := bytes.Clone(original)
			if err := applyEXEGraphics(edited, editedAssets); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(edited, expected) {
				t.Fatal("an edit changed the wrong plane, record, prefix, header or unrelated EXE bytes")
			}
			again, err := unpackEXEGraphics(edited)
			if err != nil {
				t.Fatal(err)
			}
			repeated := bytes.Clone(edited)
			if err := applyEXEGraphics(repeated, again); err != nil || !bytes.Equal(repeated, edited) {
				t.Fatal("reopening an edited raw image changed bytes", err)
			}
		})
	}
}

func TestEXEEmbeddedGraphicsMalformedRecords(t *testing.T) {
	original := embeddedGraphicsTestImage(t, "softdisk")
	for _, tc := range []struct {
		name   string
		change func([]byte) []byte
	}{
		{"empty image", func([]byte) []byte { return nil }},
		{"short table", func(b []byte) []byte { return b[:0x12550] }},
		{"short final plane", func(b []byte) []byte { return b[:0x2579f] }},
		{"plane stride", func(b []byte) []byte { putWord(b, 0x12290, 0x33b1); return b }},
		{"plane stride high word", func(b []byte) []byte { putWord(b, 0x12292, 1); return b }},
		{"record table offset", func(b []byte) []byte { putWord(b, 0x12298, 0x41); return b }},
		{"record table high word", func(b []byte) []byte { putWord(b, 0x1229a, 1); return b }},
		{"UI count", func(b []byte) []byte { putWord(b, 0x122a0, 31); return b }},
		{"picture count", func(b []byte) []byte { putWord(b, 0x122b8, 40); return b }},
		{"pixel bias", func(b []byte) []byte { putWord(b, 0x122ba, 0); return b }},
		{"pixel bias high word", func(b []byte) []byte { putWord(b, 0x122bc, 1); return b }},
		{"zero width", func(b []byte) []byte { putWord(b, 0x122d0, 0); return b }},
		{"wrong width", func(b []byte) []byte { putWord(b, 0x122d0, 5); return b }},
		{"zero height", func(b []byte) []byte { putWord(b, 0x122d2, 0); return b }},
		{"wrong height", func(b []byte) []byte { putWord(b, 0x122d2, 47); return b }},
		{"overlapping frame", func(b []byte) []byte { putWord(b, 0x122e4, 0); return b }},
		{"out of plane offset", func(b []byte) []byte { putWord(b, 0x12554, 0xffff); return b }},
		{"reserved record field", func(b []byte) []byte { putWord(b, 0x122d6, 1); return b }},
		{"wrong record name", func(b []byte) []byte { b[0x122d8] = '/'; return b }},
		{"nonzero name padding", func(b []byte) []byte { b[0x122df] = 1; return b }},
		{"THEEND geometry", func(b []byte) []byte { putWord(b, 0x12550, 25); return b }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := tc.change(bytes.Clone(original))
			if _, err := unpackEXEGraphics(bad); err == nil {
				t.Fatal("accepted malformed embedded graphics")
			}
			before := bytes.Clone(bad)
			if err := applyEXEGraphics(bad, nil); err == nil || !bytes.Equal(bad, before) {
				t.Fatal("accepted or changed malformed embedded graphics", err)
			}
		})
	}
}

func TestEXEEmbeddedGraphicsRejectsInvalidAssetsAtomically(t *testing.T) {
	original := embeddedGraphicsTestImage(t, "softdisk")
	valid, err := unpackEXEGraphics(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(resource.Assets)
	}{
		{"missing UI", func(a resource.Assets) { delete(a, "exe/ui_01.png") }},
		{"missing frame", func(a resource.Assets) { delete(a, "exe/knife_1.png") }},
		{"missing last picture", func(a resource.Assets) { delete(a, "exe/the_end.png") }},
		{"corrupt PNG", func(a resource.Assets) { a["exe/the_end.png"] = []byte("not PNG") }},
		{"nil PNG", func(a resource.Assets) { a["exe/the_end.png"] = nil }},
		{"wrong width", func(a resource.Assets) {
			a["exe/the_end.png"] = embeddedGraphicsPNG(t, image.NewNRGBA(image.Rect(0, 0, 216, 56)))
		}},
		{"wrong height", func(a resource.Assets) {
			a["exe/the_end.png"] = embeddedGraphicsPNG(t, image.NewNRGBA(image.Rect(0, 0, 208, 55)))
		}},
		{"transparent pixels", func(a resource.Assets) {
			a["exe/the_end.png"] = embeddedGraphicsPNG(t, image.NewNRGBA(image.Rect(0, 0, 208, 56)))
		}},
		{"wrong color", func(a resource.Assets) {
			img := image.NewNRGBA(image.Rect(0, 0, 208, 56))
			img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
			a["exe/the_end.png"] = embeddedGraphicsPNG(t, img)
		}},
		{"partial alpha", func(a resource.Assets) {
			img := image.NewNRGBA(image.Rect(0, 0, 208, 56))
			img.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 128})
			a["exe/the_end.png"] = embeddedGraphicsPNG(t, img)
		}},
		{"near EGA color", func(a resource.Assets) {
			img := image.NewNRGBA64(image.Rect(0, 0, 208, 56))
			img.SetNRGBA64(0, 0, color.NRGBA64{0, 0, 43175, 65535})
			a["exe/the_end.png"] = embeddedGraphicsPNG(t, img)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assets := resource.Assets(mapClone(valid))
			// An earlier valid edit makes a premature write visible when a later
			// PNG fails validation; unchanged earlier assets would hide that bug.
			decoded, err := png.Decode(bytes.NewReader(assets["exe/ui_01.png"]))
			if err != nil {
				t.Fatal(err)
			}
			ui := decoded.(*image.Paletted)
			ui.SetColorIndex(0, 0, ui.ColorIndexAt(0, 0)^15)
			assets["exe/ui_01.png"] = embeddedGraphicsPNG(t, ui)
			tc.change(assets)
			image := bytes.Clone(original)
			err = applyEXEGraphics(image, assets)
			if err == nil || !strings.Contains(err.Error(), "exe/") {
				t.Fatal("invalid asset needs a helpful path error", err)
			}
			if !bytes.Equal(image, original) {
				t.Fatal("invalid assets partially changed the EXE")
			}
		})
	}
}
