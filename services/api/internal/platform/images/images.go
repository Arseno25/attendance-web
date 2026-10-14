package images

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
)

const MaxUpload = 2 << 20

var ErrTooLarge = errors.New("Pilih gambar JPG atau PNG berukuran maksimal 2 MB.")

// Decode dimensions before pixels, then rebuild to discard metadata and bound storage.
func Normalize(reader io.Reader, logo bool) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, MaxUpload+1))
	if err != nil {
		return nil, errors.New("Gambar tidak dapat dibaca. Pilih ulang file.")
	}
	if len(raw) > MaxUpload {
		return nil, ErrTooLarge
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, errors.New("Gunakan gambar JPG atau PNG yang dapat dibuka.")
	}
	if config.Width < 32 || config.Height < 32 || config.Width > 4096 || config.Height > 4096 || config.Width*config.Height > 12000000 {
		return nil, errors.New("Ukuran gambar belum sesuai. Gunakan gambar 32–4096 piksel, maksimal 12 megapiksel.")
	}
	source, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("Gambar rusak atau tidak lengkap. Pilih file lain.")
	}
	rotation := orientation(raw, format)
	orientedWidth, orientedHeight := config.Width, config.Height
	if rotation >= 5 {
		orientedWidth, orientedHeight = orientedHeight, orientedWidth
	}
	width, height := orientedWidth, orientedHeight
	if width > 512 || height > 512 {
		if width >= height {
			height = max(1, height*512/width)
			width = 512
		} else {
			width = max(1, width*512/height)
			height = 512
		}
	}
	resized := image.NewNRGBA(image.Rect(0, 0, width, height))
	bounds := source.Bounds()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sx, sy := x*orientedWidth/width, y*orientedHeight/height
			switch rotation {
			case 2:
				sx = config.Width - 1 - sx
			case 3:
				sx, sy = config.Width-1-sx, config.Height-1-sy
			case 4:
				sy = config.Height - 1 - sy
			case 5:
				sx, sy = sy, sx
			case 6:
				sx, sy = sy, config.Height-1-sx
			case 7:
				sx, sy = config.Width-1-sy, config.Height-1-sx
			case 8:
				sx, sy = config.Width-1-sy, sx
			}
			resized.Set(x, y, source.At(bounds.Min.X+sx, bounds.Min.Y+sy))
		}
	}
	var output bytes.Buffer
	if logo {
		err = png.Encode(&output, resized)
	} else {
		err = jpeg.Encode(&output, resized, &jpeg.Options{Quality: 85})
	}
	if err != nil {
		return nil, errors.New("Gambar tidak dapat diproses. Pilih file lain.")
	}
	if output.Len() > 512<<10 {
		return nil, errors.New("Gambar terlalu besar setelah diproses. Pilih gambar yang lebih sederhana.")
	}
	return output.Bytes(), nil
}
