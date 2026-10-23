package exe

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestXBINRoundtrip(t *testing.T) {
	for _, withFont := range []bool{false, true} {
		for _, withPalette := range []bool{false, true} {
			x := XBIN{Width: 80, Height: 25, Image: bytes.Repeat([]byte{'A', 7}, 2000)}
			if withFont {
				x.FontHeight = 16
				x.Font = make([]byte, 4096)
			}
			if withPalette {
				x.Palette = make([]byte, 48)
			}
			raw, err := BuildXBIN(x)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseXBIN(append(raw, append([]byte{26}, append([]byte("SAUCE00"), make([]byte, 121)...)...)...))
			if err != nil {
				t.Fatal(err)
			}
			if got.Width != 80 || got.Height != 25 || !bytes.Equal(got.Font, x.Font) || !bytes.Equal(got.Image, x.Image) || !bytes.Equal(got.Palette, x.Palette) {
				t.Fatal("XBIN changed in roundtrip")
			}
		}
	}
}

func TestXBINRejectsInvalid(t *testing.T) {
	good, err := BuildXBIN(XBIN{Width: 80, Height: 25, FontHeight: 16, Font: make([]byte, 4096), Image: make([]byte, 4000)})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func([]byte) []byte{
		func(b []byte) []byte { return b[:10] },
		func(b []byte) []byte { b[0] = 'Y'; return b },
		func(b []byte) []byte { b[4] = 0; return b },
		func(b []byte) []byte { binary.LittleEndian.PutUint16(b[5:], 0); return b },
		func(b []byte) []byte { b[9] = 0; return b },
		func(b []byte) []byte { b[9] = 33; return b },
		func(b []byte) []byte { b[10] |= 4; return b },
		func(b []byte) []byte { b[10] |= 16; return b },
		func(b []byte) []byte { b[10] |= 128; return b },
		func(b []byte) []byte { return b[:12] },
		func(b []byte) []byte { return b[:len(b)-1] },
		func(b []byte) []byte { b[10] |= 1; return b[:20] },
	} {
		if _, err := ParseXBIN(mutate(bytes.Clone(good))); err == nil {
			t.Fatal("accepted invalid XBIN")
		}
	}
	for _, x := range []XBIN{
		{Width: 0, Height: 25},
		{Width: 65536, Height: 1},
		{Width: 1, Height: -1},
		{Width: 1, Height: 1, Image: []byte{0}},
		{Width: 1, Height: 1, Image: []byte{0, 0}, Font: []byte{0}, FontHeight: 16},
		{Width: 1, Height: 1, Image: []byte{0, 0}, Font: []byte{}, FontHeight: 0},
		{Width: 1, Height: 1, Image: []byte{0, 0}, Palette: []byte{0}},
	} {
		if _, err := BuildXBIN(x); err == nil {
			t.Fatal("accepted invalid components")
		}
	}
}
