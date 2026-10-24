package graphics

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestPlanarPNGPublicRoundtripAndEdit(t *testing.T) {
	planar := []byte{0x80, 0x01, 0x40, 0x01, 0x20, 0x01, 0x10, 0x01}
	raw, err := PlanarToPNG(planar, 8, 2)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	for x, want := range []color.NRGBA{{0, 0, 168, 255}, {0, 168, 0, 255}, {168, 0, 0, 255}, {84, 84, 84, 255}} {
		if got := color.NRGBAModel.Convert(img.At(x, 0)).(color.NRGBA); got != want {
			t.Fatalf("pixel %d: %v, want %v", x, got, want)
		}
	}
	if got := color.NRGBAModel.Convert(img.At(7, 1)).(color.NRGBA); got != (color.NRGBA{255, 255, 255, 255}) {
		t.Fatal("last native bit did not decode white", got)
	}
	got, err := PNGToPlanar(raw, 8, 2)
	if err != nil || !bytes.Equal(got, planar) {
		t.Fatal("no-op planar roundtrip", got, err)
	}
	edited := image.NewNRGBA(image.Rect(0, 0, 8, 2))
	for y := range 2 {
		for x := range 8 {
			edited.Set(x, y, img.At(x, y))
		}
	}
	edited.SetNRGBA(0, 0, color.NRGBA{0, 168, 168, 255})
	var out bytes.Buffer
	if err := png.Encode(&out, edited); err != nil {
		t.Fatal(err)
	}
	got, err = PNGToPlanar(out.Bytes(), 8, 2)
	want := []byte{0x80, 0x01, 0xc0, 0x01, 0x20, 0x01, 0x10, 0x01}
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("edited color changed the wrong EGA plane", got, err)
	}
}

func TestPlanarPNGPublicValidation(t *testing.T) {
	if _, err := PlanarToPNG(nil, 8, 2); err == nil {
		t.Fatal("accepted truncated planes")
	}
	if _, err := PlanarToPNG(nil, 7, 2); err == nil {
		t.Fatal("accepted invalid planar geometry")
	}
	if _, err := PNGToPlanar(nil, 7, 2); err == nil {
		t.Fatal("accepted invalid PNG geometry")
	}
	if _, err := PNGToPlanar(nil, 8, 2); err == nil {
		t.Fatal("accepted invalid PNG")
	}
	badColor := image.NewNRGBA(image.Rect(0, 0, 8, 2))
	var out bytes.Buffer
	if err := png.Encode(&out, badColor); err != nil {
		t.Fatal(err)
	}
	if _, err := PNGToPlanar(out.Bytes(), 8, 2); err == nil {
		t.Fatal("accepted transparent pixels")
	}
}
