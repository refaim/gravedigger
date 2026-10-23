package compression

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// EXE streams interleave 16-bit control words with literal/offset bytes. The
// decoder consumes the next control word eagerly after the sixteenth bit.
type streamWriter struct {
	data     []byte
	at, used int
}

func newStream() *streamWriter { return &streamWriter{data: make([]byte, 2)} }
func (w *streamWriter) bits(value string) {
	for _, bit := range value {
		if bit == '1' {
			w.data[w.at+w.used/8] |= 1 << uint(w.used%8)
		}
		w.used++
		if w.used == 16 {
			w.at = len(w.data)
			w.data = append(w.data, 0, 0)
			w.used = 0
		}
	}
}
func (w *streamWriter) byte(value ...byte) { w.data = append(w.data, value...) }
func sized(data []byte) []byte {
	set16(data, 2, len(data)%512)
	set16(data, 4, (len(data)+511)/512)
	return data
}

func pkFile(stream []byte) []byte {
	data := make([]byte, 0x300)
	copy(data, "MZ")
	put16(data, 8, 7)
	put16(data, 24, 64)
	put16(data, 28, 0x210d)
	copy(data[30:], "PKLITE")
	put16(data, 64+10, 0xffff)
	put16(data, 64+22, 28)
	return sized(append(data, stream...))
}
func pkEnd(w *streamWriter) { w.bits("1011100"); w.byte(0xff) }
func lzFile(stream, relocs []byte) []byte {
	stub := (32 + len(stream) + 15) &^ 15
	data := make([]byte, stub+0x158)
	copy(data, "MZ")
	put16(data, 8, 2)
	put16(data, 22, (stub-32)/16)
	put16(data, stub+8, (stub-32)/16)
	copy(data[28:], "LZ91")
	copy(data[32:], stream)
	return sized(append(data, relocs...))
}

func TestPKLiteCorruption(t *testing.T) {
	w := newStream()
	w.bits("0")
	w.byte('A')
	pkEnd(w)
	valid := append(append([]byte{}, w.data...), make([]byte, 9)...)
	got, err := DecompressPKLite(pkFile(valid))
	if err != nil || !bytes.Equal(got[word(got, 8)*16:], []byte{'A'}) {
		t.Fatalf("minimal valid PKLITE: %v", err)
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { put16(b, 8, 2) }, func(b []byte) { put16(b, 8, 0xffff) },
		func(b []byte) { put16(b, 24, 0) }, func(b []byte) { put16(b, 24, 100) },
		func(b []byte) { put16(b, 64+22, 0) }, func(b []byte) { put16(b, 64+22, 100) },
	} {
		bad := pkFile(valid)
		mutate(bad)
		if _, err := DecompressPKLite(bad); err == nil {
			t.Fatal("accepted invalid PKLITE metadata")
		}
	}
	for _, stream := range [][]byte{
		{0},
		{0, 0},
		{0x1d, 0},
		{0x1d, 0, 0xff},
		append(append([]byte{}, w.data...), 1),
		append(append([]byte{}, w.data...), 0, 0),
		{1, 0},    // A duplicate whose distance byte is missing.
		{1, 0, 0}, // Zero-distance duplicate.
	} {
		if _, err := DecompressPKLite(pkFile(stream)); err == nil {
			t.Fatalf("accepted corrupt stream %x", stream)
		}
	}
}

func TestLZEXECorruption(t *testing.T) {
	w := newStream()
	w.bits("1")
	w.byte('A')
	w.bits("01")
	w.byte(0, 0, 1)
	w.bits("01")
	w.byte(0, 0, 0)
	valid := lzFile(w.data, []byte{0, 1, 0})
	if _, err := DecompressLZEXE(valid); err != nil {
		t.Fatalf("segment marker: %v", err)
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { put16(b, 8, 1) }, func(b []byte) { put16(b, 22, 0xffff) },
	} {
		bad := bytes.Clone(valid)
		mutate(bad)
		if _, err := DecompressLZEXE(bad); err == nil {
			t.Fatal("accepted invalid LZEXE metadata")
		}
	}
	for _, stream := range [][]byte{
		{0xff, 0xff, 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A'},
		{0xff, 0x3f, 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A', 'A'},
		{0, 0, 0},
	} {
		if _, err := DecompressLZEXE(lzFile(stream, []byte{0, 1, 0})); err == nil {
			t.Fatalf("accepted invalid LZEXE stream %x", stream)
		}
	}
	if _, err := DecompressLZEXE(lzFile(w.data, []byte{0})); err == nil {
		t.Fatal("accepted truncated relocations")
	}
}

func TestRelocationAndHistoryBounds(t *testing.T) {
	got, err := lzRelocations([]byte{5, 0, 0, 0, 3, 0, 0x34, 0x12, 0, 1, 0})
	if err != nil || len(got) != 3 || got[0] != 5 || got[1]>>16 != 0xfff {
		t.Fatalf("relocation markers: %x %v", got, err)
	}
	for _, data := range [][]byte{nil, {0}, {0, 0}, {5, 3}, append(bytes.Repeat([]byte{0, 0, 0}, 17), 1)} {
		if _, err := lzRelocations(data); err == nil {
			t.Fatalf("accepted invalid relocations %x", data)
		}
	}
	for _, distance := range []int{-1, 0, 2} {
		if _, err := history([]byte{'A'}, distance, 2); err == nil {
			t.Fatal("accepted invalid history")
		}
	}
	if _, err := history(make([]byte, maxEXEImage), 1, 1); err == nil {
		t.Fatal("unbounded back-reference output")
	}
	if _, err := literal(make([]byte, maxEXEImage), 'A'); err == nil {
		t.Fatal("unbounded literal output")
	}
	if got, err := history([]byte{'A'}, 1, 4); err != nil || string(got) != "AAAAA" {
		t.Fatalf("overlapping history: %q %v", got, err)
	}
	if _, err := buildMZ([]byte{0, 0}, make([]uint32, 65536), nil, exeState{}); err == nil {
		t.Fatal("too many relocations")
	}
	if _, err := buildMZ([]byte{0, 0}, []uint32{1}, nil, exeState{}); err == nil {
		t.Fatal("relocation out of image")
	}
	data := rawMZ()
	put16(data, 6, 1)
	put16(data, 28, 15)
	if _, err := DecompressEXE(data); err == nil {
		t.Fatal("accepted raw relocation outside image")
	}
	put16(data, 28, 0)
	if _, err := DecompressEXE(data); err != nil {
		t.Fatalf("valid raw relocation: %v", err)
	}
	r := &cursor{}
	bits := newBits16(r)
	if bits.code(pkCounts) != 0 || !errors.Is(r.err, io.ErrUnexpectedEOF) {
		t.Fatal("truncated control word")
	}
}

func TestHUFFOutputLimit(t *testing.T) {
	if _, err := HuffCompress(make([]byte, maxHuffOutput+1), simpleTree()); err == nil {
		t.Fatal("unbounded Huffman output")
	}
}

func TestDecodedEXEHeaderValidation(t *testing.T) {
	w := newStream()
	w.bits("0")
	w.byte('A')
	pkEnd(w)
	stream := append(bytes.Clone(w.data), make([]byte, 9)...)
	for _, mutate := range []func([]byte){
		func(b []byte) { put16(b, 64+8, 2); put16(b, 64+10, 1) },
		func(b []byte) { put16(b, len(b)-4, 0xffff) },
	} {
		data := pkFile(stream)
		mutate(data)
		if _, err := DecompressPKLite(data); err == nil {
			t.Fatal("accepted invalid decoded DOS header")
		}
	}
	w = newStream()
	pkEnd(w)
	if _, err := DecompressPKLite(pkFile(append(w.data, make([]byte, 9)...))); err == nil {
		t.Fatal("accepted empty decoded executable")
	}
	if _, err := buildMZ(nil, nil, nil, exeState{}); err == nil {
		t.Fatal("accepted empty executable")
	}
}
