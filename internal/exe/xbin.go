package exe

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// XBIN is the uncompressed, 256-character subset supported by the game's VGA
// exit screen. A palette is retained for editors; the game uses its VGA palette.
type XBIN struct {
	Width, Height, FontHeight int
	Font, Palette, Image      []byte
}

func validDimensions(width, height int) bool {
	return width > 0 && width <= 65535 && height > 0 && height <= 65535
}

func BuildXBIN(x XBIN) ([]byte, error) {
	if !validDimensions(x.Width, x.Height) {
		return nil, fmt.Errorf("invalid XBIN dimensions")
	}
	if len(x.Image) != x.Width*x.Height*2 {
		return nil, fmt.Errorf("invalid XBIN image size")
	}
	flags := byte(0)
	if x.Font != nil {
		if x.FontHeight < 1 || x.FontHeight > 32 || len(x.Font) != 256*x.FontHeight {
			return nil, fmt.Errorf("invalid XBIN font size or height")
		}
		flags |= 2
	}
	if x.Palette != nil {
		if len(x.Palette) != 48 {
			return nil, fmt.Errorf("invalid XBIN palette size")
		}
		flags |= 1
	}
	out := make([]byte, 11, 11+len(x.Palette)+len(x.Font)+len(x.Image))
	copy(out, "XBIN\x1a")
	binary.LittleEndian.PutUint16(out[5:], uint16(x.Width))
	binary.LittleEndian.PutUint16(out[7:], uint16(x.Height))
	if x.Font != nil {
		out[9] = byte(x.FontHeight)
	}
	out[10] = flags
	out = append(out, x.Palette...)
	out = append(out, x.Font...)
	out = append(out, x.Image...)
	return out, nil
}

func ParseXBIN(data []byte) (XBIN, error) {
	var x XBIN
	if len(data) < 11 || !bytes.Equal(data[:5], []byte("XBIN\x1a")) {
		return x, fmt.Errorf("invalid XBIN header")
	}
	x.Width = int(binary.LittleEndian.Uint16(data[5:]))
	x.Height = int(binary.LittleEndian.Uint16(data[7:]))
	if !validDimensions(x.Width, x.Height) {
		return x, fmt.Errorf("invalid XBIN dimensions")
	}
	flags := data[10]
	if flags & ^byte(0x0b) != 0 {
		return x, fmt.Errorf("unsupported XBIN flags: 0x%02x (compressed and 512-character fonts are unsupported)", flags)
	}
	offset := 11
	if flags&1 != 0 {
		if len(data)-offset < 48 {
			return x, fmt.Errorf("truncated XBIN palette")
		}
		x.Palette = bytes.Clone(data[offset : offset+48])
		offset += 48
	}
	if flags&2 != 0 {
		x.FontHeight = int(data[9])
		if x.FontHeight < 1 || x.FontHeight > 32 {
			return x, fmt.Errorf("invalid XBIN font height")
		}
		size := 256 * x.FontHeight
		if len(data)-offset < size {
			return x, fmt.Errorf("truncated XBIN font")
		}
		x.Font = bytes.Clone(data[offset : offset+size])
		offset += size
	}
	size := x.Width * x.Height * 2
	if len(data)-offset < size {
		return x, fmt.Errorf("truncated XBIN image")
	}
	x.Image = bytes.Clone(data[offset : offset+size])
	return x, nil
}
