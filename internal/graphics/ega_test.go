package graphics

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func pngData(t *testing.T, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestPlanar(t *testing.T) {
	pixels := make([]byte, 32)
	for i := range pixels {
		pixels[i] = byte(i % 16)
	}
	encoded, err := encodePlanar(pixels, 16, 2)
	if err != nil {
		t.Fatal(err)
	}
	wantPlanar := []byte{0x55, 0x55, 0x55, 0x55, 0x33, 0x33, 0x33, 0x33, 0x0f, 0x0f, 0x0f, 0x0f, 0x00, 0xff, 0x00, 0xff}
	if !bytes.Equal(encoded, wantPlanar) {
		t.Fatalf("bitplanes %x, want %x", encoded, wantPlanar)
	}
	decoded, err := decodePlanar(encoded, 16, 2)
	if err != nil || !bytes.Equal(decoded, pixels) {
		t.Fatalf("pixels %v: %v", decoded, err)
	}
	for _, dims := range [][2]int{{0, 1}, {7, 1}, {8, 0}, {-8, 1}, {8, -1}, {1 << 30, 1 << 30}, {8, 65536}, {65528, 65535}} {
		if _, err := encodePlanar(nil, dims[0], dims[1]); err == nil {
			t.Errorf("encode accepted %v", dims)
		}
		if _, err := decodePlanar(nil, dims[0], dims[1]); err == nil {
			t.Errorf("decode accepted %v", dims)
		}
	}
	if _, err := decodePlanar(nil, 8, 1); err == nil {
		t.Fatal("short planar accepted")
	}
	if _, err := encodePlanar(nil, 8, 1); err == nil {
		t.Fatal("short pixels accepted")
	}
	if _, err := encodePlanar([]byte{16, 0, 0, 0, 0, 0, 0, 0}, 8, 1); err == nil {
		t.Fatal("color 16 accepted")
	}
}

func TestPNG(t *testing.T) {
	pixels := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	data := pixelsPNG(pixels, 8, 2)
	decoded, err := pngPixels(data, 8, 2)
	if err != nil || !bytes.Equal(decoded, pixels) {
		t.Fatalf("PNG pixels %v: %v", decoded, err)
	}
	rgb := image.NewNRGBA(image.Rect(0, 0, 8, 2))
	for i, p := range pixels {
		rgb.Set(i%8, i/8, egaPalette[p])
	}
	decoded, err = pngPixels(pngData(t, rgb), 8, 2)
	if err != nil || !bytes.Equal(decoded, pixels) {
		t.Fatalf("RGB pixels %v: %v", decoded, err)
	}
	if _, err := pngPixels(nil, 8, 2); err == nil {
		t.Fatal("invalid PNG accepted")
	}
	if _, err := pngPixels(data, 16, 2); err == nil {
		t.Fatal("wrong dimensions accepted")
	}
	if _, err := pngPixels(data, 8, 3); err == nil {
		t.Fatal("wrong image height accepted")
	}
	rgb.Set(0, 0, color.NRGBA{R: 1, A: 255})
	if _, err := pngPixels(pngData(t, rgb), 8, 2); err == nil {
		t.Fatal("non-EGA RGB accepted")
	}
	rgb.Set(0, 0, color.NRGBA{})
	if _, err := pngPixels(pngData(t, rgb), 8, 2); err == nil {
		t.Fatal("transparency accepted")
	}
	// Image editors may write 256 palette entries with unused black padding.
	palette := append(color.Palette(nil), egaPalette...)
	for len(palette) < 256 {
		palette = append(palette, color.Black)
	}
	paletted := image.NewPaletted(image.Rect(0, 0, 8, 2), palette)
	copy(paletted.Pix, pixels)
	decoded, err = pngPixels(pngData(t, paletted), 8, 2)
	if err != nil || !bytes.Equal(decoded, pixels) {
		t.Fatalf("padded palette %v: %v", decoded, err)
	}
}

func TestStem(t *testing.T) {
	if got := stem("Title1.dd2"); got != "title1" {
		t.Fatalf("lowercase extension: %s", got)
	}
}

func minimalPNG(width, height uint32) []byte {
	data := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	chunk := func(kind string, body []byte) {
		data = binary.BigEndian.AppendUint32(data, uint32(len(body)))
		start := len(data)
		data = append(data, kind...)
		data = append(data, body...)
		data = binary.BigEndian.AppendUint32(data, crc32.ChecksumIEEE(data[start:]))
	}
	header := binary.BigEndian.AppendUint32(nil, width)
	header = binary.BigEndian.AppendUint32(header, height)
	header = append(header, 8, 0, 0, 0, 0)
	chunk("IHDR", header)
	chunk("IDAT", []byte{0x78, 0x9c, 0x03, 0, 0, 0, 0, 1})
	chunk("IEND", nil)
	return data
}

func TestPNGRejectsHugeHeaderBeforeDecode(t *testing.T) {
	if _, err := pngPixels(minimalPNG(100000000, 100000000), 8, 2); err == nil {
		t.Fatal("huge PNG accepted")
	}
}

func TestPNGRejectsInvalidPixelsAfterValidConfig(t *testing.T) {
	if _, err := pngPixels(minimalPNG(8, 2), 8, 2); err == nil {
		t.Fatal("truncated PNG pixels accepted")
	}
}
