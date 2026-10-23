// Package graphics translates DD2 EGA pictures to editable PNG assets.
package graphics

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// Keep the original game's palette, including its unusual light-red entry.
var egaPalette = color.Palette{
	color.NRGBA{0, 0, 0, 255},
	color.NRGBA{0, 0, 168, 255},
	color.NRGBA{0, 168, 0, 255},
	color.NRGBA{0, 168, 168, 255},
	color.NRGBA{168, 0, 0, 255},
	color.NRGBA{168, 0, 168, 255},
	color.NRGBA{168, 84, 0, 255},
	color.NRGBA{168, 168, 168, 255},
	color.NRGBA{84, 84, 84, 255},
	color.NRGBA{84, 84, 255, 255},
	color.NRGBA{84, 255, 84, 255},
	color.NRGBA{84, 255, 255, 255},
	color.NRGBA{255, 84, 21, 255},
	color.NRGBA{255, 84, 255, 255},
	color.NRGBA{255, 255, 84, 255},
	color.NRGBA{255, 255, 255, 255},
}

func pixelCount(width, height int) (int, error) {
	if width <= 0 || width%8 != 0 || height <= 0 || width > 65535 || height > 65535 || width > (16<<20)/height {
		return 0, fmt.Errorf("invalid EGA dimensions %dx%d (positive width divisible by 8 required)", width, height)
	}
	return width * height, nil
}

func decodePlanar(data []byte, width, height int) ([]byte, error) {
	n, err := pixelCount(width, height)
	if err != nil {
		return nil, err
	}
	if len(data) < n/2 {
		return nil, fmt.Errorf("truncated EGA data: need %d bytes, got %d", n/2, len(data))
	}
	pixels := make([]byte, n)
	for i := range pixels {
		for p := 0; p < 4; p++ {
			pixels[i] |= ((data[p*(n/8)+i/8] >> uint(7-i%8)) & 1) << p
		}
	}
	return pixels, nil
}

func encodePlanar(pixels []byte, width, height int) ([]byte, error) {
	n, err := pixelCount(width, height)
	if err != nil {
		return nil, err
	}
	if len(pixels) != n {
		return nil, fmt.Errorf("pixel count: need %d, got %d", n, len(pixels))
	}
	data := make([]byte, n/2)
	for i, c := range pixels {
		if c > 15 {
			return nil, fmt.Errorf("EGA pixel %d has invalid color index %d", i, c)
		}
		for p := 0; p < 4; p++ {
			data[p*(n/8)+i/8] |= ((c >> p) & 1) << uint(7-i%8)
		}
	}
	return data, nil
}

// The buffer writer cannot fail and callers supply validated dimensions/pixels.
func pixelsPNG(pixels []byte, width, height int) []byte {
	img := image.NewPaletted(image.Rect(0, 0, width, height), egaPalette)
	copy(img.Pix, pixels)
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

func pngPixels(data []byte, width, height int) ([]byte, error) {
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid PNG: %w", err)
	}
	if config.Width != width || config.Height != height {
		return nil, fmt.Errorf("PNG dimensions: expected %dx%d, got %dx%d", width, height, config.Width, config.Height)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid PNG pixels: %w", err)
	}
	pixels := make([]byte, width*height)
	for i := range pixels {
		r, g, b, a := img.At(i%width, i/width).RGBA()
		found := false
		for c, entry := range egaPalette {
			er, eg, eb, ea := entry.RGBA()
			if r == er && g == eg && b == eb && a == ea {
				pixels[i] = byte(c)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("PNG pixel (%d,%d) must use an opaque EGA palette color", i%width, i/width)
		}
	}
	return pixels, nil
}
