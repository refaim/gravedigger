package graphics

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/refaim/gravedigger/internal/compression"
	"github.com/refaim/gravedigger/internal/resource"
)

type PicHandler struct{}

func (PicHandler) Name() string { return "PicHandler" }
func (PicHandler) Patterns() []string {
	return []string{"TITLE1.DD2", "TITLE2.DD2", "PROGPIC.DD2", "STARPIC.DD2"}
}

type picMetadata struct {
	Image  string `json:"image"`
	Tree   string `json:"tree"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Tail   []byte `json:"tail,omitempty"`
}

func (h PicHandler) Unpack(name string, data []byte) (resource.Unpacked, error) {
	decoded, tree, err := compression.HuffDecompress(data)
	if err != nil {
		return resource.Unpacked{}, err
	}
	if len(decoded) < 8 || !bytes.Equal(decoded[:4], []byte{'P', 'I', 'C', 0}) {
		return resource.Unpacked{}, fmt.Errorf("expected PIC signature and 8-byte header")
	}
	w, height := int(binary.LittleEndian.Uint16(decoded[4:]))*8, int(binary.LittleEndian.Uint16(decoded[6:]))
	pixels, err := decodePlanar(decoded[8:], w, height)
	if err != nil {
		return resource.Unpacked{}, err
	}
	m := picMetadata{Image: stem(name) + ".png", Tree: stem(name) + ".tree", Width: w, Height: height, Tail: bytes.Clone(decoded[8+w*height/2:])}
	return unpacked(h.Name(), name, m, resource.Assets{m.Image: pixelsPNG(pixels, w, height)}, resource.Assets{m.Tree: tree}), nil
}

func (PicHandler) Repack(manifest resource.Manifest, translatable, meta resource.Assets) ([]byte, error) {
	var m picMetadata
	if err := json.Unmarshal(manifest.Metadata, &m); err != nil {
		return nil, fmt.Errorf("PIC metadata: %w", err)
	}
	if _, err := pixelCount(m.Width, m.Height); err != nil {
		return nil, err
	}
	pixels, err := pngPixels(translatable[m.Image], m.Width, m.Height)
	if err != nil {
		return nil, err
	}
	planar, _ := encodePlanar(pixels, m.Width, m.Height)
	decoded := []byte{'P', 'I', 'C', 0}
	decoded = binary.LittleEndian.AppendUint16(decoded, uint16(m.Width/8))
	decoded = binary.LittleEndian.AppendUint16(decoded, uint16(m.Height))
	decoded = append(decoded, planar...)
	decoded = append(decoded, m.Tail...)
	return compression.HuffCompress(decoded, meta[m.Tree])
}
