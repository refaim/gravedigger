package compression

import (
	"bytes"
	"testing"
)

func TestLZEXEPackedStreamLocation(t *testing.T) {
	original := corpus(t, "retail", "MANSION.EXE")
	want, err := DecompressLZEXE(original)
	if err != nil {
		t.Fatal(err)
	}
	header := word(original, 8) * 16
	// UNLZEXE 066aac7 reproduces the original image and all 22 relocations
	// after inserting a paragraph before the packed stream. The stub's inf[4]
	// still counts the compressed paragraphs; outer CS moves with the stub.
	prefixed := append(bytes.Clone(original[:header]), make([]byte, 16)...)
	prefixed = append(prefixed, original[header:]...)
	put16(prefixed, 22, word(original, 22)+1)
	declared := len(prefixed) - len("MsDos")
	put16(prefixed, 2, declared%512)
	put16(prefixed, 4, (declared+511)/512)
	got, err := DecompressLZEXE(prefixed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("prefix changed the decoded executable")
	}
}

func TestLZEXEMemoryAllocation(t *testing.T) {
	// Independent upstream UNLZEXE 066aac7 outputs for Retail's inf[5]=2018h
	// and inf[6]=0181h: subtract 8250 paragraphs from packed allocations.
	for _, tc := range []struct {
		name                 string
		packedMin, packedMax int
		wantMin, wantMax     int
	}{
		{"finite", 8282, 8314, 32, 64},
		{"unlimited", 8282, 0xffff, 32, 0xffff},
		{"zero", 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := corpus(t, "retail", "MANSION.EXE")
			put16(data, 10, tc.packedMin)
			put16(data, 12, tc.packedMax)
			got, err := DecompressLZEXE(data)
			if err != nil {
				t.Fatal(err)
			}
			if word(got, 10) != tc.wantMin || word(got, 12) != tc.wantMax {
				t.Fatalf("min/max = %d/%d, want %d/%d", word(got, 10), word(got, 12), tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestLZEXEInvalidPackedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]byte, int)
	}{
		{"stream before image", func(data []byte, stub int) { put16(data, stub+8, word(data, 22)+1) }},
		{"allocation underflow", func(data []byte, _ int) { put16(data, 10, 8249) }},
		{"finite allocation underflow", func(data []byte, _ int) { put16(data, 12, 8249) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := corpus(t, "retail", "MANSION.EXE")
			stub := word(data, 8)*16 + word(data, 22)*16
			tc.mutate(data, stub)
			if _, err := DecompressLZEXE(data); err == nil {
				t.Fatal("accepted invalid packed metadata")
			}
		})
	}
}
