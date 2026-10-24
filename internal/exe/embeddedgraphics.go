package exe

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/refaim/gravedigger/internal/graphics"
	"github.com/refaim/gravedigger/internal/resource"
)

const (
	exeGraphicsHeader = 0x12290
	exeGraphicsTable  = exeGraphicsHeader + 0x40
	exeGraphicsBase   = 0x188e0
	exeGraphicsStride = 0x33b0
)

type exeGraphic struct {
	name                  string
	width, height, offset int
}

func exeGraphicsRecords(image []byte) ([]exeGraphic, error) {
	if len(image) < exeGraphicsBase+4*exeGraphicsStride {
		return nil, fmt.Errorf("truncated embedded EXE graphics")
	}
	header := image[exeGraphicsHeader:]
	if binary.LittleEndian.Uint32(header) != exeGraphicsStride ||
		binary.LittleEndian.Uint32(header[8:]) != 0x40 ||
		wordAt(header, 0x10) != 32 || wordAt(header, 0x28) != 41 ||
		binary.LittleEndian.Uint32(header[0x2a:]) != 0x100 {
		return nil, fmt.Errorf("unsupported embedded EXE graphics header")
	}
	out := make([]exeGraphic, 0, 73)
	// The first 256 bytes of every plane contain 32 individual 8x8 UI tiles.
	for i := range 32 {
		out = append(out, exeGraphic{fmt.Sprintf("exe/ui_%02d.png", i+1), 8, 8, i * 8})
	}
	groups := [...]string{"KNIFE", "SLIME", "ZOMBIE", "SPIDER", "WOLF", "GHOST", "SKULL", "SLICE"}
	for i := range 41 {
		widthBytes, height, offset := 26, 56, 0x2d00
		name, asset := "THEEND", "exe/the_end.png"
		if i < 40 {
			widthBytes, height, offset = 6, 48, i*0x120
			group, frame := groups[i/5], i%5+1
			name = fmt.Sprintf("%s%d", group, frame)
			asset = fmt.Sprintf("exe/%s_%d.png", strings.ToLower(group), frame)
		}
		record := image[exeGraphicsTable+i*16 : exeGraphicsTable+(i+1)*16]
		var expectedName [8]byte
		copy(expectedName[:], name)
		if wordAt(record, 0) != widthBytes || wordAt(record, 2) != height ||
			wordAt(record, 4) != offset || wordAt(record, 6) != 0 || !bytes.Equal(record[8:], expectedName[:]) {
			return nil, fmt.Errorf("invalid embedded EXE graphics record for %s", asset)
		}
		out = append(out, exeGraphic{asset, widthBytes * 8, height, 0x100 + offset})
	}
	return out, nil
}

func unpackEXEGraphics(image []byte) (resource.Assets, error) {
	records, err := exeGraphicsRecords(image)
	if err != nil {
		return nil, err
	}
	out := make(resource.Assets, len(records))
	for _, record := range records {
		chunk := record.width * record.height / 8
		planar := make([]byte, chunk*4)
		for plane := range 4 {
			at := exeGraphicsBase + plane*exeGraphicsStride + record.offset
			copy(planar[plane*chunk:], image[at:at+chunk])
		}
		// The known records have validated geometry and exact plane byte counts.
		out[record.name], _ = graphics.PlanarToPNG(planar, record.width, record.height)
	}
	return out, nil
}

func applyEXEGraphics(image []byte, assets resource.Assets) error {
	records, err := exeGraphicsRecords(image)
	if err != nil {
		return err
	}
	planars := make([][]byte, len(records))
	for i, record := range records {
		raw, ok := assets[record.name]
		if !ok {
			return fmt.Errorf("missing required EXE graphic %s", record.name)
		}
		planars[i], err = graphics.PNGToPlanar(raw, record.width, record.height)
		if err != nil {
			return fmt.Errorf("%s: %w", record.name, err)
		}
	}
	// Validate every asset before changing any original EXE byte.
	for i, record := range records {
		chunk := len(planars[i]) / 4
		for plane := range 4 {
			at := exeGraphicsBase + plane*exeGraphicsStride + record.offset
			copy(image[at:at+chunk], planars[i][plane*chunk:(plane+1)*chunk])
		}
	}
	return nil
}
