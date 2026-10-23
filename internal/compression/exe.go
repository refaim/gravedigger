// Package compression implements the DOS resource codecs used by Dangerous Dave 2.
package compression

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

const maxEXEImage = 1 << 20 // Real-mode DOS addresses at most one MiB.

type cursor struct {
	data []byte
	pos  int
	err  error
}

func (r *cursor) byte() byte {
	if r.pos >= len(r.data) {
		r.err = io.ErrUnexpectedEOF
		return 0
	}
	b := r.data[r.pos]
	r.pos++
	return b
}
func (r *cursor) word() int { return int(r.byte()) | int(r.byte())<<8 }

type bits16 struct {
	r                *cursor
	cache, remaining int
}

func newBits16(r *cursor) *bits16 { return &bits16{r: r, cache: r.word(), remaining: 16} }
func (b *bits16) bit() int {
	v := b.cache & 1
	b.cache >>= 1
	b.remaining--
	if b.remaining == 0 {
		b.cache = b.r.word()
		b.remaining = 16
	}
	return v
}

func (b *bits16) code(table map[int]int) int {
	prefix := 1
	for b.r.err == nil {
		prefix = prefix*2 + b.bit()
		if value, ok := table[prefix]; ok {
			return value
		}
	}
	return 0
}
func u16(data []byte, at int) int      { return int(binary.LittleEndian.Uint16(data[at:])) }
func set16(data []byte, at, value int) { binary.LittleEndian.PutUint16(data[at:], uint16(value)) }

func mzHeader(data []byte) error {
	if len(data) < 28 {
		return errors.New("MZ header truncated")
	}
	if !bytes.Equal(data[:2], []byte("MZ")) && !bytes.Equal(data[:2], []byte("ZM")) {
		return errors.New("not an MZ executable")
	}
	last, pages := u16(data, 2), u16(data, 4)
	if last >= 512 || pages == 0 {
		return errors.New("invalid MZ file size")
	}
	size := pages * 512
	if last != 0 {
		size -= 512 - last
	}
	if size > len(data) {
		return errors.New("MZ file size does not match header")
	}
	// Retail Dave carries this five-byte distribution marker outside the MZ image.
	if size < len(data) && !bytes.Equal(data[size:], []byte("MsDos")) {
		return errors.New("unrecognized data following MZ image")
	}
	return nil
}

// DecompressEXE accepts PKLITE, LZEXE 0.91, or a structurally valid raw DOS MZ.
func DecompressEXE(data []byte) ([]byte, error) {
	if err := mzHeader(data); err != nil {
		return nil, err
	}
	if len(data) >= 36 && bytes.Equal(data[30:36], []byte("PKLITE")) {
		return DecompressPKLite(data)
	}
	if len(data) >= 32 && bytes.Equal(data[28:30], []byte("LZ")) {
		return DecompressLZEXE(data)
	}
	header := u16(data, 8) * 16
	if header < 32 || header >= len(data) {
		return nil, errors.New("invalid MZ header or file size")
	}
	if u16(data, 24) < 28 || u16(data, 24)+u16(data, 6)*4 > header {
		return nil, errors.New("invalid MZ relocation table")
	}
	image := data[header:]
	if u16(data, 20)+u16(data, 22)*16 >= len(image) {
		return nil, errors.New("MZ entry point outside image")
	}
	if u16(data, 10) > u16(data, 12) {
		return nil, errors.New("invalid MZ memory allocation")
	}
	for at := u16(data, 24); at < u16(data, 24)+u16(data, 6)*4; at += 4 {
		if u16(data, at)+u16(data, at+2)*16+1 >= len(image) {
			return nil, errors.New("MZ relocation outside image")
		}
	}
	return bytes.Clone(data), nil
}

func history(out []byte, distance, count int) ([]byte, error) {
	if distance <= 0 || distance > len(out) {
		return nil, errors.New("invalid EXE back-reference")
	}
	if len(out)+count > maxEXEImage {
		return nil, errors.New("EXE image exceeds real-mode address space")
	}
	for range count {
		out = append(out, out[len(out)-distance])
	}
	return out, nil
}

func literal(out []byte, value byte) ([]byte, error) {
	if len(out) == maxEXEImage {
		return nil, errors.New("EXE image exceeds real-mode address space")
	}
	return append(out, value), nil
}

type exeState struct{ ss, sp, cs, ip, min, max, checksum int }

func buildMZ(image []byte, relocs []uint32, extra []byte, state exeState) ([]byte, error) {
	if len(image) == 0 || state.ip+state.cs*16 >= len(image) {
		return nil, errors.New("decoded MZ entry point outside image")
	}
	if state.min > state.max {
		return nil, errors.New("invalid decoded MZ memory allocation")
	}
	if len(relocs) > 0xffff {
		return nil, errors.New("too many MZ relocations")
	}
	relocAt := 28 + len(extra)
	header := (relocAt + len(relocs)*4 + 15) &^ 15
	out := make([]byte, header+len(image))
	copy(out, "MZ")
	set16(out, 2, len(out)%512)
	set16(out, 4, (len(out)+511)/512)
	set16(out, 6, len(relocs))
	set16(out, 8, header/16)
	set16(out, 10, state.min)
	set16(out, 12, state.max)
	set16(out, 14, state.ss)
	set16(out, 16, state.sp)
	set16(out, 18, state.checksum)
	set16(out, 20, state.ip)
	set16(out, 22, state.cs)
	set16(out, 24, relocAt)
	copy(out[28:], extra)
	for i, r := range relocs {
		if int(r&0xffff)+int(r>>16)*16+1 >= len(image) {
			return nil, errors.New("EXE relocation outside decoded image")
		}
		binary.LittleEndian.PutUint32(out[relocAt+i*4:], r)
	}
	copy(out[header:], image)
	return out, nil
}
