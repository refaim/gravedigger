package compression

import (
	"bytes"
	"errors"
)

// Prefix keys include a leading sentinel bit, preserving leading zeroes.
var pkCounts = map[int]int{
	0b1000: 4, 0b10010: 5, 0b10011: 6, 0b10100: 7, 0b101010: 8, 0b101011: 9,
	0b101100: 10, 0b1011010: 11, 0b1011011: 12, 0b1011100: -1, 0b10111010: 13,
	0b10111011: 14, 0b10111100: 15, 0b101111010: 16, 0b101111011: 17, 0b101111100: 18,
	0b1011111010: 19, 0b1011111011: 20, 0b1011111100: 21, 0b1011111101: 22,
	0b1011111110: 23, 0b1011111111: 24, 0b110: 2, 0b111: 3,
}

var pkOffsets = map[int]int{
	0b10000: 1, 0b10001: 2, 0b100100: 3, 0b100101: 4, 0b100110: 5, 0b100111: 6,
	0b1010000: 7, 0b1010001: 8, 0b1010010: 9, 0b1010011: 10, 0b1010100: 11,
	0b1010101: 12, 0b1010110: 13, 0b10101110: 14, 0b10101111: 15, 0b10110000: 16,
	0b10110001: 17, 0b10110010: 18, 0b10110011: 19, 0b10110100: 20, 0b10110101: 21,
	0b10110110: 22, 0b10110111: 23, 0b10111000: 24, 0b10111001: 25, 0b10111010: 26,
	0b10111011: 27, 0b10111100: 28, 0b10111101: 29, 0b10111110: 30, 0b10111111: 31, 0b11: 0,
}

var pkStubs = map[int]int{
	0x2100: 0x290, 0x2103: 0x290, 0x2105: 0x290, 0x210a: 0x290, 0x210c: 0x290,
	0x210d: 0x290, 0x210e: 0x290, 0x210f: 0x290,
}

// DecompressPKLite decodes unencrypted, large-mode PKLITE 1.x DOS executables.
func DecompressPKLite(data []byte) ([]byte, error) {
	if err := mzHeader(data); err != nil {
		return nil, err
	}
	if len(data) < 48 || !bytes.Equal(data[30:36], []byte("PKLITE")) {
		return nil, errors.New("missing PKLITE signature")
	}
	stub, ok := pkStubs[u16(data, 28)]
	if !ok {
		return nil, errors.New("unsupported PKLITE version or mode")
	}
	header := u16(data, 8) * 16
	start := header + stub
	if header < 48 || start >= len(data) {
		return nil, errors.New("PKLITE stub outside file")
	}
	origAt := u16(data, 24) + u16(data, 6)*4
	// PKLITE stores the 13 original header words without the MZ signature.
	if origAt < 36 || origAt+26 > header {
		return nil, errors.New("PKLITE original header outside file header")
	}
	orig := data[origAt : origAt+26]
	extraEnd := origAt + u16(orig, 22) - 2
	if extraEnd < origAt+26 || extraEnd > header {
		return nil, errors.New("invalid PKLITE original header extension")
	}
	r := &cursor{data: data, pos: start}
	bits := newBits16(r)
	var image []byte
	for r.err == nil {
		if bits.bit() == 0 {
			value := r.byte()
			if r.err != nil {
				break
			}
			image, r.err = literal(image, value)
			continue
		}
		count := bits.code(pkCounts)
		if count == -1 {
			marker := r.byte()
			if marker == 0xfe {
				continue
			}
			if marker == 0xff {
				break
			}
			count = int(marker) + 25
		}
		distance := 0
		if count != 2 {
			distance = bits.code(pkOffsets) << 8
		}
		distance |= int(r.byte())
		if r.err != nil {
			break
		}
		image, r.err = history(image, distance, count)
	}
	if r.err != nil {
		return nil, r.err
	}
	var relocs []uint32
	for {
		count := int(r.byte())
		if count == 0 {
			break
		}
		segment := r.word()
		for range count {
			relocs = append(relocs, uint32(segment)<<16|uint32(r.word()))
		}
		if r.err != nil {
			return nil, r.err
		}
	}
	state := exeState{ss: r.word(), sp: r.word(), cs: r.word(), ip: r.word(), min: u16(orig, 8), max: u16(orig, 10), checksum: u16(orig, 16)}
	if r.err != nil {
		return nil, r.err
	}
	return buildMZ(image, relocs, data[origAt+26:extraEnd], state)
}
