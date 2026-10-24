package graphics

// PlanarToPNG exports four consecutive EGA planes with the game's fixed palette.
func PlanarToPNG(data []byte, width, height int) ([]byte, error) {
	pixels, err := decodePlanar(data, width, height)
	if err != nil {
		return nil, err
	}
	return pixelsPNG(pixels, width, height), nil
}

// PNGToPlanar validates fixed dimensions and opaque EGA colors before encoding.
func PNGToPlanar(data []byte, width, height int) ([]byte, error) {
	if _, err := pixelCount(width, height); err != nil {
		return nil, err
	}
	pixels, err := pngPixels(data, width, height)
	if err != nil {
		return nil, err
	}
	return encodePlanar(pixels, width, height)
}
