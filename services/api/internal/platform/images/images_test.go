package images

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestNormalizeRejectsInvalidAndOversizedImages(t *testing.T) {
	for _, data := range [][]byte{[]byte("<svg>private</svg>"), bytes.Repeat([]byte("x"), MaxUpload+1)} {
		if _, err := Normalize(bytes.NewReader(data), false); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 5000, 32)))
	if _, err := Normalize(&b, false); err == nil {
		t.Fatal("oversized dimensions accepted")
	}
}
func TestNormalizeResizesAndReencodes(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 800, 400))
	source.Set(1, 1, color.White)
	var input bytes.Buffer
	_ = png.Encode(&input, source)
	raw := input.Bytes()
	for _, logo := range []bool{false, true} {
		result, err := Normalize(bytes.NewReader(raw), logo)
		if err != nil {
			t.Fatal(err)
		}
		c, format, err := image.DecodeConfig(bytes.NewReader(result))
		if err != nil || c.Width != 512 || c.Height != 256 {
			t.Fatal("invalid normalized dimensions", c, err)
		}
		if (logo && format != "png") || (!logo && format != "jpeg") {
			t.Fatal("incorrect normalized format", format)
		}
	}
}

func TestNormalizeAppliesPhonePhotoOrientation(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			source.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var encoded bytes.Buffer
	_ = jpeg.Encode(&encoded, source, nil)
	for value := 1; value <= 8; value++ {
		metadata := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, byte(value), 0, 0, 0, 0, 0, 0, 0}
		segment := append([]byte{0xff, 0xe1, 0, byte(len(metadata) + 2)}, metadata...)
		raw := append(append(append([]byte{}, encoded.Bytes()[:2]...), segment...), encoded.Bytes()[2:]...)
		normalized, err := Normalize(bytes.NewReader(raw), false)
		if err != nil {
			t.Fatal(err)
		}
		result, _, err := image.Decode(bytes.NewReader(normalized))
		if err != nil {
			t.Fatal(err)
		}
		wantWidth, wantHeight := 80, 40
		if value >= 5 {
			wantWidth, wantHeight = 40, 80
		}
		if result.Bounds().Dx() != wantWidth || result.Bounds().Dy() != wantHeight {
			t.Fatalf("orientation %d dimensions incorrect", value)
		}
		positions := [][2]int{{5, 5}, {74, 5}, {74, 34}, {5, 34}, {5, 5}, {34, 5}, {34, 74}, {5, 74}}
		x, y := positions[value-1][0], positions[value-1][1]
		red, green, blue, _ := result.At(x, y).RGBA()
		if red < 40000 || green > 10000 || blue > 10000 {
			t.Fatalf("orientation %d: red corner not rotated correctly", value)
		}
		if bytes.Contains(normalized, []byte("Exif")) {
			t.Fatal("metadata persisted")
		}
	}
}

func TestPNGOrientationAndMalformedMetadata(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 80, 40))
	var encoded bytes.Buffer
	_ = png.Encode(&encoded, source)
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	chunk := make([]byte, 12+len(tiff))
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(tiff)))
	copy(chunk[4:8], "eXIf")
	copy(chunk[8:], tiff)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	raw := append(append(append([]byte{}, encoded.Bytes()[:33]...), chunk...), encoded.Bytes()[33:]...)
	normalized, err := Normalize(bytes.NewReader(raw), true)
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(normalized))
	if err != nil || config.Width != 40 || config.Height != 80 {
		t.Fatal("PNG orientation not applied", err)
	}
	for size := 0; size < 22; size++ {
		if got := tiffOrientation(tiff[:size]); got != 1 {
			t.Fatal("truncated metadata should be ignored", size, got)
		}
	}
	malformed := append([]byte{}, tiff...)
	binary.LittleEndian.PutUint32(malformed[4:8], 0xffffffff)
	if tiffOrientation(malformed) != 1 {
		t.Fatal("out-of-bounds offset accepted")
	}
	big := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1, 1, 0x12, 0, 3, 0, 0, 0, 1, 0, 8, 0, 0, 0, 0, 0, 0}
	if tiffOrientation(big) != 8 {
		t.Fatal("big-endian orientation ignored")
	}
}
