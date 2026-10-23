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
	gameFontCellSize     = 20
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

// Fixed 3x5 UI digits; cell numbers stay smaller than the editable game glyphs.
var gameFontNumberDigits = [10][5]byte{
	{7, 5, 5, 5, 7},
	{2, 6, 2, 2, 7},
	{7, 1, 7, 4, 7},
	{7, 1, 7, 1, 7},
	{5, 5, 7, 1, 1},
	{7, 4, 7, 1, 7},
	{7, 4, 7, 5, 7},
	{7, 1, 2, 2, 2},
	{7, 5, 7, 5, 7},
	{7, 5, 7, 1, 7},
}

// Each native byte runs MSB first; the low byte contains the left eight pixels.
func gameFontPixelBit(x int) uint16 { return uint16(1) << ((x/8)*8 + 7 - x%8) }

func drawGameFontDecorations(img *image.NRGBA, slot, left, top int) {
	frame := color.NRGBA{96, 96, 96, 255}
	for i := range gameFontCellSize {
		img.SetNRGBA(left+i, top, frame)
		img.SetNRGBA(left+i, top+gameFontCellSize-1, frame)
		img.SetNRGBA(left, top+i, frame)
		img.SetNRGBA(left+gameFontCellSize-1, top+i, frame)
	}
	text := strconv.Itoa(slot - 31)
	left += (gameFontCellSize - (len(text)*4 - 1)) / 2
	top += 13
	for digitIndex, digit := range text {
		for y, row := range gameFontNumberDigits[digit-'0'] {
			for x := range 3 {
				if row&(4>>x) != 0 {
					img.SetNRGBA(left+digitIndex*4+x, top+y, color.NRGBA{160, 160, 160, 255})
				}
			}
		}
	}
}

func gameFontPNG(f GameFont) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, gameFontAtlasWidth, gameFontAtlasHeight))
	for _, g := range f.Glyphs {
		left, top := (g.Slot-32)%gameFontAtlasColumns*gameFontCellSize, (g.Slot-32)/gameFontAtlasColumns*gameFontCellSize
		drawGameFontDecorations(img, g.Slot, left, top)
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
				img.SetNRGBA(left+x+1, top+y+1, gameFontAtlasColors[index])
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
	decorations := image.NewNRGBA(image.Rect(0, 0, gameFontAtlasWidth, gameFontAtlasHeight))
	result := GameFont{Version: baseline.Version, Height: baseline.Height, Glyphs: make([]GameGlyph, len(baseline.Glyphs))}
	for i, old := range baseline.Glyphs {
		g := GameGlyph{Slot: old.Slot, Unicode: old.Unicode, Width: old.Width, Pixels: make([]uint16, 10), Shadow: make([]uint16, 10)}
		left, top := (old.Slot-32)%gameFontAtlasColumns*gameFontCellSize, (old.Slot-32)/gameFontAtlasColumns*gameFontCellSize
		drawGameFontDecorations(decorations, old.Slot, left, top)
		rightmost := 0
		for y := range gameFontCellSize {
			for x := range gameFontCellSize {
				r, green, b, a := img.At(left+x, top+y).RGBA()
				pixel := color.RGBA64{R: uint16(r), G: uint16(green), B: uint16(b), A: uint16(a)}
				if x < 1 || x > 16 || y < 1 || y > 10 {
					er, eg, eb, ea := decorations.At(left+x, top+y).RGBA()
					expected := color.RGBA64{R: uint16(er), G: uint16(eg), B: uint16(eb), A: uint16(ea)}
					if pixel != expected {
						return GameFont{}, fmt.Errorf("game_font.png cell %d frame/number area is read-only; keep the original decoration pixels", old.Slot-31)
					}
					continue
				}
				if a == 0 {
					continue
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
					return GameFont{}, fmt.Errorf("game_font.png cell %d pixel (%d,%d): unsupported color; use transparent, opaque white, black, or cyan", old.Slot-31, x-1, y-1)
				}
				mask := gameFontPixelBit(x - 1)
				g.Pixels[y-1] |= uint16(flags&1) * mask
				g.Shadow[y-1] |= uint16(flags>>1) * mask
				rightmost = max(rightmost, x)
			}
		}
		if !slices.Equal(g.Pixels, old.Pixels) || !slices.Equal(g.Shadow, old.Shadow) {
			if old.Slot == 127 {
				return GameFont{}, fmt.Errorf("game_font.png cell %d is a color-control escape; keep its bitmap unchanged", old.Slot-31)
			}
			if rightmost > 10 {
				return GameFont{}, fmt.Errorf("game_font.png cell %d width %d exceeds the game's maximum of 10 pixels", old.Slot-31, rightmost)
			}
			if rightmost > 0 {
				g.Width = rightmost
			}
		}
		result.Glyphs[i] = g
	}
	return result, nil
}
