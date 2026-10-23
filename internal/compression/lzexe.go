package compression

import (
	"bytes"
	"errors"
)

// DecompressLZEXE decodes LZEXE 0.91 DOS executables.
func DecompressLZEXE(data []byte) ([]byte, error) {
	if err := mzHeader(data); err != nil {
		return nil, err
	}
	if len(data) < 32 || !bytes.Equal(data[28:32], []byte("LZ91")) {
		return nil, errors.New("unsupported LZEXE signature/version")
	}
	header := u16(data, 8) * 16
	stub := header + u16(data, 22)*16
	if header < 32 || stub+0x158 >= len(data) {
		return nil, errors.New("LZEXE stub or relocation table outside file")
	}
	start := stub - u16(data, stub+8)*16
	if start < header {
		return nil, errors.New("LZEXE compressed stream starts before load image")
	}
	state := exeState{ip: u16(data, stub), cs: u16(data, stub+2), sp: u16(data, stub+4), ss: u16(data, stub+6), min: u16(data, 10), max: u16(data, 12)}
	if state.max != 0 {
		// LZEXE reserves its expanded image, decoder, and nine extra paragraphs.
		overhead := u16(data, stub+10) + (u16(data, stub+12)+15)/16 + 9
		state.min -= overhead
		if state.max != 0xffff {
			state.max -= overhead
		}
		if state.min < 0 || state.max < 0 {
			return nil, errors.New("LZEXE memory allocation underflow")
		}
	}
	r := &cursor{data: data[:stub], pos: start}
	bits := newBits16(r)
	var image []byte
	for r.err == nil {
		if bits.bit() != 0 {
			value := r.byte()
			if r.err != nil {
				break
			}
			image, r.err = literal(image, value)
			continue
		}
		var count, distance int
		if bits.bit() == 0 {
			count = (bits.bit()<<1 | bits.bit()) + 2
			distance = 256 - int(r.byte())
		} else {
			lo, hi := int(r.byte()), int(r.byte())
			distance = 8192 - (lo | (hi&0xf8)<<5)
			count = (hi & 7) + 2
			if count == 2 {
				count = int(r.byte())
				if count == 0 {
					break
				}
				if count == 1 {
					continue
				}
				count++
			}
		}
		if r.err != nil {
			break
		}
		image, r.err = history(image, distance, count)
	}
	if r.err != nil {
		return nil, r.err
	}
	relocs, err := lzRelocations(data[stub+0x158:])
	if err != nil {
		return nil, err
	}
	return buildMZ(image, relocs, nil, state)
}

func lzRelocations(data []byte) ([]uint32, error) {
	r := &cursor{data: data}
	var relocs []uint32
	offset, segment := 0, 0
	for {
		span := int(r.byte())
		if span == 0 {
			span = r.word()
			if r.err != nil {
				return nil, r.err
			}
			if span == 1 {
				return relocs, nil
			}
			if span == 0 {
				segment += 0xfff
				continue
			}
		}
		offset += span
		segment += offset >> 4
		offset &= 15
		if segment > 0xffff {
			return nil, errors.New("LZEXE relocation exceeds real-mode address space")
		}
		relocs = append(relocs, uint32(segment)<<16|uint32(offset))
	}
}
