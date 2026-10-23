package graphics

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/refaim/gravedigger/internal/compression"
	"github.com/refaim/gravedigger/internal/resource"
)

type SpriteHandler struct{}

func (SpriteHandler) Name() string { return "SpriteHandler" }
func (SpriteHandler) Patterns() []string {
	return []string{"S_DAVE.DD2", "S_CHUNK1.DD2", "S_CHUNK2.DD2", "S_FRANK.DD2", "S_MASTER.DD2"}
}

type spriteSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type spriteMetadata struct {
	Prefix   string       `json:"prefix"`
	Sprites  []spriteSize `json:"sprites"`
	HuffTree []byte       `json:"huff_tree"`
	Blob     []byte       `json:"blob"`
}

func validateSprites(m spriteMetadata) (int, error) {
	if m.Prefix == "" || len(m.Sprites) == 0 || len(m.Blob) < 40 || len(m.Blob)%5 != 0 || len(m.Blob)/5 > 65535 {
		return 0, fmt.Errorf("invalid sprite metadata or plane blob")
	}
	stride := len(m.Blob) / 5
	offset := 8
	for _, sp := range m.Sprites {
		n, err := pixelCount(sp.Width, sp.Height)
		if err != nil {
			return 0, err
		}
		offset += n / 8
		if offset > stride {
			return 0, fmt.Errorf("sprite pixels exceed plane stride %d", stride)
		}
	}
	return stride, nil
}

func (h SpriteHandler) Unpack(name string, data []byte) (resource.Unpacked, error) {
	sizes, ok := spriteSizes[strings.ToUpper(name)]
	if !ok {
		return resource.Unpacked{}, fmt.Errorf("unknown sprite file %q", name)
	}
	decoded, tree, err := compression.HuffDecompress(data)
	if err != nil {
		return resource.Unpacked{}, err
	}
	if len(decoded) < 2 {
		return resource.Unpacked{}, fmt.Errorf("truncated sprite stride")
	}
	m := spriteMetadata{Prefix: strings.TrimPrefix(stem(strings.ToUpper(name)), "s_"), Sprites: sizes, HuffTree: tree, Blob: decoded[2:]}
	stride, err := validateSprites(m)
	if err != nil {
		return resource.Unpacked{}, err
	}
	if stride != int(binary.LittleEndian.Uint16(decoded)) {
		return resource.Unpacked{}, fmt.Errorf("sprite stride does not match plane blob")
	}
	assets := resource.Assets{}
	offset := 8
	for i, sp := range sizes {
		chunk := sp.Width * sp.Height / 8
		planar := make([]byte, 0, chunk*4)
		for p := 0; p < 4; p++ {
			planar = append(planar, m.Blob[p*stride+offset:p*stride+offset+chunk]...)
		}
		pixels, _ := decodePlanar(planar, sp.Width, sp.Height)
		assets[fmt.Sprintf("sprites/%s_%04d.png", m.Prefix, i)] = pixelsPNG(pixels, sp.Width, sp.Height)
		offset += chunk
	}
	return unpacked(h.Name(), name, m, assets, nil), nil
}

func (SpriteHandler) Repack(manifest resource.Manifest, translatable, meta resource.Assets) ([]byte, error) {
	var m spriteMetadata
	if err := json.Unmarshal(manifest.Metadata, &m); err != nil {
		return nil, fmt.Errorf("sprite metadata: %w", err)
	}
	stride, err := validateSprites(m)
	if err != nil {
		return nil, err
	}
	blob := bytes.Clone(m.Blob)
	offset := 8
	for i, sp := range m.Sprites {
		name := fmt.Sprintf("sprites/%s_%04d.png", m.Prefix, i)
		pixels, err := pngPixels(translatable[name], sp.Width, sp.Height)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		planar, _ := encodePlanar(pixels, sp.Width, sp.Height)
		chunk := len(planar) / 4
		for p := 0; p < 4; p++ {
			copy(blob[p*stride+offset:], planar[p*chunk:(p+1)*chunk])
		}
		offset += chunk
	}
	decoded := binary.LittleEndian.AppendUint16(nil, uint16(stride))
	return compression.HuffCompress(append(decoded, blob...), m.HuffTree)
}
