package graphics

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/refaim/gravedigger/internal/compression"
	"github.com/refaim/gravedigger/internal/resource"
)

func mustRead(t *testing.T, variant, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "game", variant, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCorpus(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		for _, h := range []resource.Handler{PicHandler{}, SpriteHandler{}, IntroHandler{}} {
			if h.Name() == "" {
				t.Fatal("empty handler name")
			}
			for _, name := range h.Patterns() {
				t.Run(variant+"/"+name, func(t *testing.T) {
					data := mustRead(t, variant, name)
					u, err := h.Unpack(name, data)
					if err != nil {
						t.Fatal(err)
					}
					if u.Manifest.Handler != h.Name() || u.Manifest.SourceFile != name {
						t.Fatalf("manifest %+v", u.Manifest)
					}
					got, err := h.Repack(u.Manifest, u.Translatable, u.Meta)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(got, data) {
						t.Fatalf("byte-exact roundtrip: sizes %d/%d", len(got), len(data))
					}
					for asset, pngBytes := range u.Translatable {
						img, err := pngPixelsAny(pngBytes)
						if err != nil {
							t.Fatal(err)
						}
						p, err := pngPixels(pngBytes, img.Bounds().Dx(), img.Bounds().Dy())
						if err != nil {
							t.Fatal(err)
						}
						p[0] = (p[0] + 1) % 16
						u.Translatable[asset] = pixelsPNG(p, img.Bounds().Dx(), img.Bounds().Dy())
						modified, err := h.Repack(u.Manifest, u.Translatable, u.Meta)
						if err != nil {
							t.Fatal(err)
						}
						v, err := h.Unpack(name, modified)
						if err != nil {
							t.Fatal(err)
						}
						q, err := pngPixels(v.Translatable[asset], img.Bounds().Dx(), img.Bounds().Dy())
						if err != nil || !bytes.Equal(p, q) {
							t.Fatalf("edited pixels %s: %v", asset, err)
						}
						u.Translatable[asset] = pngBytes
						break
					}
				})
			}
		}
	}
}

func TestHandlerErrors(t *testing.T) {
	for _, h := range []resource.Handler{PicHandler{}, SpriteHandler{}, IntroHandler{}} {
		t.Run(h.Name(), func(t *testing.T) {
			if _, err := h.Unpack(h.Patterns()[0], nil); err == nil {
				t.Fatal("empty source accepted")
			}
			if _, err := h.Repack(resource.Manifest{Metadata: json.RawMessage("{")}, nil, nil); err == nil {
				t.Fatal("invalid metadata accepted")
			}
			if _, err := h.Repack(resource.Manifest{Metadata: json.RawMessage("{}")}, nil, nil); err == nil {
				t.Fatal("empty metadata accepted")
			}
			u, err := h.Unpack(h.Patterns()[0], mustRead(t, "softdisk", h.Patterns()[0]))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.Repack(u.Manifest, nil, u.Meta); err == nil {
				t.Fatal("missing PNG accepted")
			}
			for name := range u.Translatable {
				u.Translatable[name] = []byte("bad png")
				break
			}
			if _, err := h.Repack(u.Manifest, u.Translatable, u.Meta); err == nil {
				t.Fatal("invalid PNG accepted")
			}
		})
	}
	if _, err := (SpriteHandler{}).Unpack("UNKNOWN.DD2", nil); err == nil {
		t.Fatal("unknown sprites accepted")
	}
}

func TestMetadataErrors(t *testing.T) {
	for _, h := range []resource.Handler{PicHandler{}, SpriteHandler{}, IntroHandler{}} {
		u, err := h.Unpack(h.Patterns()[0], mustRead(t, "softdisk", h.Patterns()[0]))
		if err != nil {
			t.Fatal(err)
		}
		var meta map[string]any
		if err := json.Unmarshal(u.Manifest.Metadata, &meta); err != nil {
			t.Fatal(err)
		}
		cases := map[string]any{}
		switch h.Name() {
		case "PicHandler":
			cases["tree"] = "missing.tree"
			cases["width"] = 7
		case "SpriteHandler":
			cases["huff_tree"] = "@"
			cases["blob"] = "@"
			cases["sprites"] = []any{map[string]any{"width": 8, "height": 99999}}
			cases["prefix"] = ""
		case "IntroHandler":
			cases["trailing"] = "@"
			cases["width"] = 7
			cases["decomp_size"] = 3
		}
		for field, value := range cases {
			t.Run(h.Name()+"/"+field, func(t *testing.T) {
				original := meta[field]
				meta[field] = value
				changed, _ := json.Marshal(meta)
				meta[field] = original
				manifest := u.Manifest
				manifest.Metadata = changed
				if _, err := h.Repack(manifest, u.Translatable, u.Meta); err == nil {
					t.Fatal("invalid metadata accepted")
				}
			})
		}
	}
}

func TestRLE(t *testing.T) {
	cases := [][]byte{nil, {1, 2, 3}, {254}, bytes.Repeat([]byte{4}, 300), bytes.Repeat([]byte{254}, 300), {1, 1, 1, 2, 2, 2, 2}}
	for _, data := range cases {
		decoded, end, err := decompressRLE(compressRLE(data), len(data))
		if err != nil || !bytes.Equal(decoded, data) || end != len(compressRLE(data)) {
			t.Fatalf("RLE %v: %v", data, err)
		}
	}
	for _, data := range [][]byte{{254}, {254, 2}, nil, {254, 0, 1}, {254, 5, 1}} {
		if _, _, err := decompressRLE(data, 4); err == nil {
			t.Fatalf("invalid RLE accepted: %v", data)
		}
	}
}

func TestIntroSynthetic(t *testing.T) {
	planar := make([]byte, 256*64/2)
	data := binary.LittleEndian.AppendUint32(nil, uint32(len(planar)))
	data = append(data, compressRLE(planar)...)
	for _, tail := range [][]byte{nil, {1, 2, 3}} {
		original := append(bytes.Clone(data), tail...)
		u, err := (IntroHandler{}).Unpack("INTRO.DD2", original)
		if err != nil {
			t.Fatal(err)
		}
		got, err := (IntroHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
		if err != nil || !bytes.Equal(got, original) {
			t.Fatalf("synthetic intro: %v", err)
		}
	}
	for _, raw := range [][]byte{{0, 0, 0, 0}, binary.LittleEndian.AppendUint32(nil, 5), append(binary.LittleEndian.AppendUint32(nil, 8192), 254)} {
		if _, err := (IntroHandler{}).Unpack("INTRO.DD2", raw); err == nil {
			t.Fatalf("invalid intro accepted: %v", raw)
		}
	}
}

func TestManifestShape(t *testing.T) {
	for _, h := range []resource.Handler{PicHandler{}, SpriteHandler{}, IntroHandler{}} {
		u, err := h.Unpack(h.Patterns()[0], mustRead(t, "softdisk", h.Patterns()[0]))
		if err != nil {
			t.Fatal(err)
		}
		var meta map[string]any
		if err := json.Unmarshal(u.Manifest.Metadata, &meta); err != nil {
			t.Fatal(err)
		}
		switch h.Name() {
		case "PicHandler":
			if meta["image"] != "title1.png" || meta["tree"] != "title1.tree" || len(u.Meta["title1.tree"]) == 0 {
				t.Fatal(meta)
			}
		case "SpriteHandler":
			if meta["prefix"] != "dave" || len(u.Translatable) != 77 || meta["huff_tree"] == "" || meta["blob"] == "" {
				t.Fatalf("sprites %d, %v", len(u.Translatable), meta["prefix"])
			}
			if _, ok := u.Translatable["sprites/dave_0000.png"]; !ok {
				t.Fatal("sprite name missing")
			}
		case "IntroHandler":
			if meta["width"] != float64(256) || meta["height"] != float64(64) || meta["decomp_size"] != float64(8192) {
				t.Fatal(meta)
			}
		}
	}
}

// Decode without depending on production helpers for expected dimensions.
func pngPixelsAny(data []byte) (image.Image, error) { return png.Decode(bytes.NewReader(data)) }

func TestSpriteSizeTable(t *testing.T) {
	for _, name := range (SpriteHandler{}).Patterns() {
		if !strings.HasPrefix(name, "S_") {
			t.Fatal(name)
		}
		u, err := (SpriteHandler{}).Unpack(name, mustRead(t, "softdisk", name))
		if err != nil {
			t.Fatal(err)
		}
		for asset := range u.Translatable {
			if !strings.HasPrefix(asset, fmt.Sprintf("sprites/%s_", strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(name, "S_"), ".DD2")))) {
				t.Fatal(asset)
			}
		}
	}
}

func huffData(t *testing.T, decoded []byte) []byte {
	t.Helper()
	_, tree, err := compression.HuffDecompress(mustRead(t, "softdisk", "TITLE1.DD2"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := compression.HuffCompress(decoded, tree)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMalformedPictures(t *testing.T) {
	for _, decoded := range [][]byte{nil, []byte("BAD\x00\x01\x00\x01\x00"), []byte("PIC\x00\x00\x00\x01\x00"), []byte("PIC\x00\x01\x00\x01\x00")} {
		if _, err := (PicHandler{}).Unpack("TITLE1.DD2", huffData(t, decoded)); err == nil {
			t.Fatalf("invalid PIC accepted: %v", decoded)
		}
	}
	decoded := append([]byte("PIC\x00\x01\x00\x01\x00"), 0, 0, 0, 0, 99)
	original := huffData(t, decoded)
	u, err := (PicHandler{}).Unpack("TITLE1.dd2", original)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := u.Translatable["title1.png"]; !ok {
		t.Fatal("lowercase source extension changed asset name")
	}
	got, err := (PicHandler{}).Repack(u.Manifest, u.Translatable, u.Meta)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("PIC opaque tail: %v", err)
	}
}

func TestMalformedSprites(t *testing.T) {
	for _, decoded := range [][]byte{nil, {0}, {8, 0}, append([]byte{8, 0}, make([]byte, 40)...)} {
		if _, err := (SpriteHandler{}).Unpack("S_DAVE.DD2", huffData(t, decoded)); err == nil {
			t.Fatal("short sprites accepted")
		}
	}
	data, _, err := compression.HuffDecompress(mustRead(t, "softdisk", "S_DAVE.DD2"))
	if err != nil {
		t.Fatal(err)
	}
	data[0] ^= 1
	if _, err := (SpriteHandler{}).Unpack("S_DAVE.DD2", huffData(t, data)); err == nil {
		t.Fatal("stride mismatch accepted")
	}
	if _, err := (SpriteHandler{}).Unpack("s_dave.dd2", mustRead(t, "softdisk", "S_DAVE.DD2")); err != nil {
		t.Fatalf("lowercase sprite source: %v", err)
	}
}

func TestSpriteValidation(t *testing.T) {
	valid := spriteMetadata{Prefix: "test", Sprites: []spriteSize{{8, 1}}, Blob: make([]byte, 45)}
	if stride, err := validateSprites(valid); err != nil || stride != 9 {
		t.Fatalf("valid synthetic sprites: %d %v", stride, err)
	}
	for _, m := range []spriteMetadata{
		{},
		{Prefix: "test"},
		{Prefix: "test", Sprites: valid.Sprites},
		{Prefix: "test", Sprites: valid.Sprites, Blob: make([]byte, 41)},
		{Prefix: "test", Sprites: valid.Sprites, Blob: make([]byte, 5*65536)},
		{Prefix: "test", Sprites: []spriteSize{{7, 1}}, Blob: valid.Blob},
		{Prefix: "test", Sprites: []spriteSize{{8, 2}}, Blob: valid.Blob},
	} {
		if _, err := validateSprites(m); err == nil {
			t.Fatal("invalid sprite metadata accepted")
		}
	}
}

func TestPictureReferencePixels(t *testing.T) {
	refs := map[string][3]byte{"TITLE1.DD2": {8, 15, 2}, "TITLE2.DD2": {8, 6, 8}, "PROGPIC.DD2": {3, 6, 6}, "STARPIC.DD2": {0, 0, 0}}
	for _, variant := range []string{"softdisk", "retail"} {
		for name, want := range refs {
			u, err := (PicHandler{}).Unpack(name, mustRead(t, variant, name))
			if err != nil {
				t.Fatal(err)
			}
			pixels, err := pngPixels(u.Translatable[stem(name)+".png"], 320, 200)
			if err != nil {
				t.Fatal(err)
			}
			got := [3]byte{pixels[0], pixels[100*320+160], pixels[199*320+319]}
			if got != want {
				t.Errorf("%s/%s pixels %v, want %v", variant, name, got, want)
			}
		}
	}
}
