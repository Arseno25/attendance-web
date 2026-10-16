package aiclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/jpeg"
)

type ReferencePhoto struct {
	Ciphertext string `json:"ciphertext"`
	Nonce      string `json:"nonce"`
	KeyID      string `json:"key_id"`
}

func (c *Client) ReferencePhoto(ctx context.Context, school, student string, photo ReferencePhoto) (string, error) {
	var out struct {
		Image string `json:"image"`
	}
	if err := c.call(ctx, "POST", "/internal/v1/reference-photo", map[string]string{"school_id": school, "student_id": student, "ciphertext": photo.Ciphertext, "nonce": photo.Nonce, "key_id": photo.KeyID}, &out); err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(out.Image)
	if err != nil || len(raw) > 65536 || !ValidateImage(out.Image) {
		return "", ErrUnavailable
	}
	size, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil || size.Width != 192 || size.Height != 192 {
		return "", ErrUnavailable
	}
	return out.Image, nil
}
