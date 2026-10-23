package exe

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"slices"
	"strconv"
)

const (
	gameFontCellSize     = 32
	gameFontAtlasColumns = 16
	gameFontGlyphCount   = 96
	gameFontAtlasWidth   = gameFontAtlasColumns * gameFontCellSize
	gameFontAtlasHeight  = gameFontGlyphCount / gameFontAtlasColumns * gameFontCellSize
)

var gameFontAtlasColors = [4]color.NRGBA{
	{},
	{0, 255, 255, 255}, // Foreground without a shadow-mask bit in custom fonts.
	{0, 0, 0, 255},
	{255, 255, 255, 255},
}

// Each native byte runs MSB first; the low byte contains the left eight pixels.
func gameFontPixelBit(x int) uint16 { return uint16(1) << ((x/8)*8 + 7 - x%8) }

func drawGameFontNumber(img *image.NRGBA, slot, left, top int) {
	text := strconv.Itoa(slot)
	left += (gameFontCellSize - len(text)*8) / 2
	top += gameFontCellSize - 16
	for digitIndex, digit := range text {
		for y := range 16 {
			row := bundledExitFont[int(digit)*16+y]
			for x := range 8 {
				if row&(0x80>>x) != 0 {
					img.SetNRGBA(left+digitIndex*8+x, top+y, color.NRGBA{128, 128, 128, 255})
				}
			}
		}
	}
}

func gameFontPNG(f GameFont) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, gameFontAtlasWidth, gameFontAtlasHeight))
	for _, g := range f.Glyphs {
		left, top := (g.Slot-32)%gameFontAtlasColumns*gameFontCellSize, (g.Slot-32)/gameFontAtlasColumns*gameFontCellSize
		drawGameFontNumber(img, g.Slot, left, top)
		for y := range 10 {
			for x := range 16 {
				mask := gameFontPixelBit(x)
				index := 0
				if g.Pixels[y]&mask != 0 {
					index |= 1
				}
				if g.Shadow[y]&mask != 0 {
					index |= 2
				}
				img.SetNRGBA(left+x, top+y, gameFontAtlasColors[index])
			}
		}
	}
	var out bytes.Buffer
	// The image dimensions are fixed and bytes.Buffer.Write cannot fail.
	_ = png.Encode(&out, img)
	return out.Bytes()
}

func readGameFontPNG(raw []byte, baseline GameFont) (GameFont, error) {
	config, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return GameFont{}, fmt.Errorf("game_font.png: invalid PNG config: %w", err)
	}
	if config.Width != gameFontAtlasWidth || config.Height != gameFontAtlasHeight {
		return GameFont{}, fmt.Errorf("game_font.png dimensions: expected %dx%d, got %dx%d", gameFontAtlasWidth, gameFontAtlasHeight, config.Width, config.Height)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return GameFont{}, fmt.Errorf("game_font.png: invalid PNG pixels: %w", err)
	}
	numbers := image.NewNRGBA(image.Rect(0, 0, gameFontAtlasWidth, gameFontAtlasHeight))
	result := GameFont{Version: baseline.Version, Height: baseline.Height, Glyphs: make([]GameGlyph, len(baseline.Glyphs))}
	for i, old := range baseline.Glyphs {
		g := GameGlyph{Slot: old.Slot, Unicode: old.Unicode, Width: old.Width, Pixels: make([]uint16, 10), Shadow: make([]uint16, 10)}
		left, top := (old.Slot-32)%gameFontAtlasColumns*gameFontCellSize, (old.Slot-32)/gameFontAtlasColumns*gameFontCellSize
		drawGameFontNumber(numbers, old.Slot, left, top)
		rightmost := 0
		for y := range gameFontCellSize {
			for x := range gameFontCellSize {
				r, green, b, a := img.At(left+x, top+y).RGBA()
				pixel := color.RGBA64{R: uint16(r), G: uint16(green), B: uint16(b), A: uint16(a)}
				if y >= gameFontCellSize-16 {
					er, eg, eb, ea := numbers.At(left+x, top+y).RGBA()
					expected := color.RGBA64{R: uint16(er), G: uint16(eg), B: uint16(eb), A: uint16(ea)}
					if pixel != expected {
						return GameFont{}, fmt.Errorf("game_font.png slot %d number label is read-only; keep the original label pixels", old.Slot)
					}
					continue
				}
				if a == 0 {
					continue
				}
				if x >= 16 || y >= 10 {
					return GameFont{}, fmt.Errorf("game_font.png slot %d pixel (%d,%d) is outside the 16x10 native canvas (drawing rows 0..9)", old.Slot, x, y)
				}
				flags := 0
				switch pixel {
				case color.RGBA64{R: 65535, G: 65535, B: 65535, A: 65535}:
					flags = 3
				case color.RGBA64{A: 65535}:
					flags = 2
				case color.RGBA64{G: 65535, B: 65535, A: 65535}:
					flags = 1
				default:
					return GameFont{}, fmt.Errorf("game_font.png slot %d pixel (%d,%d): unsupported color; use transparent, opaque white, black, or cyan", old.Slot, x, y)
				}
				mask := gameFontPixelBit(x)
				g.Pixels[y] |= uint16(flags&1) * mask
				g.Shadow[y] |= uint16(flags>>1) * mask
				rightmost = max(rightmost, x+1)
			}
		}
		if !slices.Equal(g.Pixels, old.Pixels) || !slices.Equal(g.Shadow, old.Shadow) {
			if old.Slot == 127 {
				return GameFont{}, fmt.Errorf("game_font.png slot %d is a color-control escape; keep its bitmap unchanged", old.Slot)
			}
			if rightmost > 10 {
				return GameFont{}, fmt.Errorf("game_font.png slot %d width %d exceeds the game's maximum of 10 pixels", old.Slot, rightmost)
			}
			if rightmost > 0 {
				g.Width = rightmost
			}
		}
		result.Glyphs[i] = g
	}
	return result, nil
}
