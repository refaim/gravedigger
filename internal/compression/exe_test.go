package compression

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func corpus(t *testing.T, variant, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "game", variant, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func put16(data []byte, at, n int) { binary.LittleEndian.PutUint16(data[at:], uint16(n)) }
func word(data []byte, at int) int { return int(binary.LittleEndian.Uint16(data[at:])) }

func rawMZ() []byte {
	data := make([]byte, 48)
	copy(data, "MZ")
	put16(data, 2, len(data))
	put16(data, 4, 1)
	put16(data, 8, 2)
	put16(data, 12, 0xffff)
	put16(data, 24, 28)
	copy(data[32:], "DOS test payload")
	return data
}

func TestRealExecutables(t *testing.T) {
	var previous []byte
	for _, tc := range []struct {
		variant, name string
		decode        func([]byte) ([]byte, error)
	}{
		{"softdisk", "1.EXE", DecompressPKLite}, {"retail", "MANSION.EXE", DecompressLZEXE},
	} {
		t.Run(tc.variant, func(t *testing.T) {
			input := corpus(t, tc.variant, tc.name)
			output, err := tc.decode(input)
			if err != nil {
				t.Fatal(err)
			}
			image := output[word(output, 8)*16:]
			// The 215024-byte image was independently reproduced by
			// hackerb9/depklite (Softdisk), commit
			// 83a11e57961cc95af0fdfebeb3759f82e0c11abe, and mywave82/unlzexe
			// (Retail), commit 066aac7be3b27813c221d3b03621ad6dfaecd285.
			// UNLZEXE also verified Retail's 22 relocations and header state.
			if got := fmt.Sprintf("%x", sha256.Sum256(image)); got != "8a96f9873c42d9feffffb7a29385643a86b0304e3c692508680a63ca4e2e3932" {
				t.Fatalf("code image hash %s", got)
			}
			if previous != nil && !bytes.Equal(previous, image) {
				t.Fatal("game variants differ")
			}
			previous = image
			if word(output, 10) > word(output, 12) {
				t.Fatal("invalid memory allocation header")
			}
			if word(output, 6) != 22 {
				t.Fatalf("relocations %d", word(output, 6))
			}
			got, err := DecompressEXE(input)
			if err != nil || !bytes.Equal(got, output) {
				t.Fatalf("dispatch: %v", err)
			}
			got, err = DecompressEXE(output)
			if err != nil || !bytes.Equal(got, output) {
				t.Fatalf("raw decoded EXE: %v", err)
			}
		})
	}
}

func TestRawMZ(t *testing.T) {
	for _, signature := range []string{"MZ", "ZM"} {
		data := rawMZ()
		copy(data, signature)
		got, err := DecompressEXE(data)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("raw %s: %v", signature, err)
		}
	}
	data := make([]byte, 512)
	copy(data, rawMZ())
	put16(data, 2, 0)
	if _, err := DecompressEXE(data); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidExecutables(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", nil}, {"short", []byte("MZ")}, {"not MZ", make([]byte, 64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecompressEXE(tc.data); err == nil {
				t.Fatal("accepted malformed EXE")
			}
		})
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { put16(b, 2, 512) }, func(b []byte) { put16(b, 4, 0) },
		func(b []byte) { put16(b, 8, 1) }, func(b []byte) { put16(b, 8, 4) },
		func(b []byte) { put16(b, 2, 47) }, func(b []byte) { put16(b, 6, 2) },
		func(b []byte) { put16(b, 24, 27) }, func(b []byte) { put16(b, 20, 0xffff) },
		func(b []byte) { put16(b, 10, 10); put16(b, 12, 1) },
		func(b []byte) { copy(b[28:], "LZ90") },
	} {
		data := rawMZ()
		mutate(data)
		if _, err := DecompressEXE(data); err == nil {
			t.Fatalf("accepted malformed header: %x", data[:32])
		}
	}
	for _, decode := range []func([]byte) ([]byte, error){DecompressPKLite, DecompressLZEXE} {
		for _, data := range [][]byte{nil, make([]byte, 100), rawMZ()} {
			if _, err := decode(data); err == nil {
				t.Fatal("accepted invalid compressed EXE")
			}
		}
	}
	for _, tc := range []struct {
		variant, name string
		decode        func([]byte) ([]byte, error)
	}{{"softdisk", "1.EXE", DecompressPKLite}, {"retail", "MANSION.EXE", DecompressLZEXE}} {
		data := corpus(t, tc.variant, tc.name)
		for _, n := range []int{30, 48, 112, 768, len(data) - 1} {
			if _, err := tc.decode(data[:n]); err == nil {
				t.Fatalf("accepted %s truncated at %d", tc.name, n)
			}
		}
	}
	data := corpus(t, "softdisk", "1.EXE")
	data[28] = 0xff
	if _, err := DecompressEXE(data); err == nil {
		t.Fatal("unsupported PKLITE version")
	}
	data = corpus(t, "softdisk", "1.EXE")
	data[29] = 1
	if _, err := DecompressPKLite(data); err == nil {
		t.Fatal("unsupported PKLITE mode")
	}
}
