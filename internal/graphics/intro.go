package graphics

import (
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/refaim/gravedigger/internal/resource"
)

type IntroHandler struct{}

func (IntroHandler) Name() string       { return "IntroHandler" }
func (IntroHandler) Patterns() []string { return []string{"INTRO.DD2"} }

type introMetadata struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	DecompSize int    `json:"decomp_size"`
	Trailing   []byte `json:"trailing,omitempty"`
}

func decompressRLE(data []byte, limit int) ([]byte, int, error) {
	out := make([]byte, 0, limit)
	i := 0
	for i < len(data) && len(out) < limit {
		value, count := data[i], 1
		i++
		if value == 254 {
			if i+2 > len(data) {
				return nil, i, fmt.Errorf("truncated RLE escape")
			}
			count, value = int(data[i]), data[i+1]
			i += 2
			if count == 0 || count > limit-len(out) {
				return nil, i, fmt.Errorf("invalid RLE run length %d", count)
			}
		}
		for range count {
			out = append(out, value)
		}
	}
	if len(out) != limit {
		return nil, i, fmt.Errorf("truncated RLE: need %d decoded bytes, got %d", limit, len(out))
	}
	return out, i, nil
}

func compressRLE(data []byte) []byte {
	out := []byte{}
	for i := 0; i < len(data); {
		count := 1
		for i+count < len(data) && data[i+count] == data[i] {
			count++
		}
		if data[i] == 254 || count >= 4 {
			for left := count; left > 0; {
				n := min(left, 255)
				out = append(out, 254, byte(n), data[i])
				left -= n
			}
		} else {
			out = append(out, data[i:i+count]...)
		}
		i += count
	}
	return out
}

func (h IntroHandler) Unpack(name string, data []byte) (resource.Unpacked, error) {
	if len(data) < 4 {
		return resource.Unpacked{}, fmt.Errorf("truncated INTRO header")
	}
	size := int(binary.LittleEndian.Uint32(data))
	if size != 8192 {
		return resource.Unpacked{}, fmt.Errorf("INTRO decoded size: expected 8192, got %d", size)
	}
	decoded, end, err := decompressRLE(data[4:], size)
	if err != nil {
		return resource.Unpacked{}, err
	}
	pixels, _ := decodePlanar(decoded, 256, 64)
	m := introMetadata{Width: 256, Height: 64, DecompSize: size, Trailing: data[4+end:]}
	return unpacked(h.Name(), name, m, resource.Assets{"intro.png": pixelsPNG(pixels, 256, 64)}, nil), nil
}

func (IntroHandler) Repack(manifest resource.Manifest, translatable, meta resource.Assets) ([]byte, error) {
	var m introMetadata
	if err := json.Unmarshal(manifest.Metadata, &m); err != nil {
		return nil, fmt.Errorf("INTRO metadata: %w", err)
	}
	if m.Width != 256 || m.Height != 64 || m.DecompSize != 8192 {
		return nil, fmt.Errorf("invalid INTRO dimensions or decoded size")
	}
	pixels, err := pngPixels(translatable["intro.png"], m.Width, m.Height)
	if err != nil {
		return nil, err
	}
	planar, _ := encodePlanar(pixels, m.Width, m.Height)
	out := binary.LittleEndian.AppendUint32(nil, uint32(m.DecompSize))
	out = append(out, compressRLE(planar)...)
	return append(out, m.Trailing...), nil
}
