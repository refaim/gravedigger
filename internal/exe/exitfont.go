package exe

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
)

//go:embed cp866_8x16.fnt
var bundledExitFont []byte

const exitCallOffset = 0x679

var exitModeInstruction = []byte{0xb8, 3, 0, 0xcd, 0x10}

const farFontStart = dsBase + 0x10000

// Load the font after a single mode-set. Saving and restoring all changed VGA
// sequencer/controller registers avoids the BIOS mode-set that reloads ROM font
// RAM. The font-segment operand stays at byte 16 for MZ relocation compatibility.
func buildFontStub(fontSegment int) []byte {
	stub := []byte{0xb8, 3, 0, 0xcd, 0x10, 0x1e, 0x06, 0x56, 0x57, 0x51, 0xb8, 0, 0xa0, 0x8e, 0xc0, 0xb8, byte(fontSegment), byte(fontSegment >> 8), 0x8e, 0xd8, 0x31, 0xf6, 0x52}
	for _, group := range []struct {
		port    uint16
		indices []byte
	}{{0x3c4, []byte{0, 2, 4}}, {0x3ce, []byte{4, 5, 6}}} {
		stub = append(stub, 0xba, byte(group.port), byte(group.port>>8))
		for _, index := range group.indices {
			stub = append(stub, 0xb0, index, 0xee, 0x42, 0xec, 0x88, 0xc4, 0xb0, index, 0x50, 0x4a)
		}
	}
	stub = append(stub,
		0xba, 0xc4, 3, 0xb8, 0, 1, 0xef, 0xb8, 2, 4, 0xef, 0xb8, 4, 6, 0xef, 0xb8, 0, 3, 0xef,
		0xba, 0xce, 3, 0xb8, 4, 2, 0xef, 0xb8, 5, 0, 0xef, 0xb8, 6, 0, 0xef,
		0xfc, 0x31, 0xff, 0xb9, 0, 1, 0x51, 0xb9, 8, 0, 0xf3, 0xa5, 0x83, 0xc7, 0x10, 0x59, 0xe2, 0xf4,
		0xba, 0xce, 3, 0x58, 0xef, 0x58, 0xef, 0x58, 0xef,
		0xba, 0xc4, 3, 0x58, 0xef, 0x58, 0xef, 0x58, 0xef,
		0x5a, 0x59, 0x5f, 0x5e, 0x07, 0x1f, 0xcb)
	return stub
}

func fontStubInfo(image []byte) (fontAt, stubAt int, err error) {
	call := image[exitCallOffset : exitCallOffset+5]
	if bytes.Equal(call, exitModeInstruction) {
		return -1, -1, nil
	}
	if call[0] != 0x9a {
		return 0, 0, fmt.Errorf("unsupported exit-screen call site")
	}
	stubAt = wordAt(call, 1) + 16*wordAt(call, 3)
	if stubAt+18 > len(image) {
		return 0, 0, fmt.Errorf("invalid VGA font stub address")
	}
	segment := wordAt(image, stubAt+16)
	fontAt = segment * 16
	current := buildFontStub(segment)
	if fontAt < farFontStart || stubAt+len(current) > len(image) || !bytes.Equal(image[stubAt:stubAt+len(current)], current) || fontAt+4096 > len(image) {
		return 0, 0, fmt.Errorf("unsupported VGA font stub")
	}
	return fontAt, stubAt, nil
}

func exitFont(image []byte) ([]byte, int, error) {
	at, _, err := fontStubInfo(image)
	if err != nil {
		return nil, 0, err
	}
	if at < 0 {
		return bytes.Clone(bundledExitFont), -1, nil
	}
	return bytes.Clone(image[at : at+4096]), at, nil
}

func patchExitFont(data []byte, font []byte) ([]byte, error) {
	start := wordAt(data, 8) * 16
	image := data[start:]
	at, _, err := fontStubInfo(image)
	if err != nil {
		return nil, err
	}
	if at >= 0 {
		copy(image[at:at+4096], font)
		return data, nil
	}
	relocEnd := wordAt(data, 24) + wordAt(data, 6)*4
	if start-relocEnd < 8 {
		return nil, fmt.Errorf("MZ header has no room for VGA font relocation entries")
	}
	fontOffset := max((len(image)+15)&^15, farFontStart)
	data = append(data, make([]byte, fontOffset-len(image))...)
	data = append(data, font...)
	stubOffset := fontOffset + len(font)
	data = append(data, buildFontStub(fontOffset/16)...)
	image = data[start:]
	image[exitCallOffset] = 0x9a
	putWordAt(image, exitCallOffset+1, stubOffset%16)
	putWordAt(image, exitCallOffset+3, stubOffset/16)
	for i, field := range []int{exitCallOffset + 3, stubOffset + 16} {
		binary.LittleEndian.PutUint16(data[relocEnd+i*4:], uint16(field%16))
		binary.LittleEndian.PutUint16(data[relocEnd+i*4+2:], uint16(field/16))
	}
	putWordAt(data, 6, wordAt(data, 6)+2)
	return data, nil
}
