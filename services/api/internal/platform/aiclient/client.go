package aiclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/jpeg"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("AI service unavailable")

type Client struct {
	baseURL, token string
	http           *http.Client
}

func New(baseURL, token string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || len(token) < 32 {
		return nil, errors.New("invalid internal AI configuration")
	}
	return &Client{baseURL: baseURL, token: token, http: &http.Client{Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type Status struct {
	PipelineReady      bool           `json:"pipeline_ready"`
	RecognitionEnabled bool           `json:"recognition_enabled"`
	BlockReason        string         `json:"block_reason"`
	ModelSHA           string         `json:"model_sha256"`
	Evaluation         map[string]any `json:"evaluation"`
}

func UnavailableStatus() Status {
	return Status{BlockReason: "PIPELINE_NOT_CONFIGURED", Evaluation: map[string]any{"status": "not_evaluated", "required": []string{"camera_hold_out", "liveness_spoof", "threshold_calibration"}, "automatic_attendance_approved": false}}
}
func (c *Client) Status(ctx context.Context) Status {
	var out Status
	if c == nil || c.call(ctx, "GET", "/internal/v1/status", nil, &out) != nil {
		return UnavailableStatus()
	}
	return out
}

type Template struct {
	StudentID  string `json:"student_id,omitempty"`
	ConsentID  string `json:"consent_id,omitempty"`
	Ciphertext string `json:"ciphertext"`
	Nonce      string `json:"nonce"`
	KeyID      string `json:"key_id"`
	ModelSHA   string `json:"model_sha256"`
	FrameCount int    `json:"frame_count,omitempty"`
	Status     string `json:"status,omitempty"`
}

func (c *Client) Enroll(ctx context.Context, school, student, consent string, frames []string, gallery ...Template) (Template, error) {
	var out Template
	var contextConsent any
	if consent != "" {
		contextConsent = consent
	}
	if gallery == nil {
		gallery = []Template{}
	}
	err := c.call(ctx, "POST", "/internal/v1/enroll", map[string]any{"school_id": school, "student_id": student, "consent_id": contextConsent, "frames": frames, "gallery": gallery}, &out)
	return out, err
}

type Box struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type Face struct {
	Box        Box      `json:"box"`
	StudentID  string   `json:"student_id,omitempty"`
	Label      string   `json:"label,omitempty"`
	Similarity *float64 `json:"similarity,omitempty"`
}
type PreviewFace struct {
	Box Box `json:"box"`
}
type FacePreview struct {
	Faces     []PreviewFace `json:"faces"`
	FaceCount int           `json:"face_count"`
	Quality   string        `json:"quality"`
	Message   string        `json:"message"`
}

func (p FacePreview) Valid() bool {
	if p.FaceCount != len(p.Faces) || p.FaceCount > 16 {
		return false
	}
	switch p.Quality {
	case "no_face":
		if p.FaceCount != 0 {
			return false
		}
	case "multiple_faces":
		if p.FaceCount < 2 {
			return false
		}
	case "ready", "low_quality":
		if p.FaceCount != 1 {
			return false
		}
	default:
		return false
	}
	for _, face := range p.Faces {
		b := face.Box
		for _, value := range []float64{b.X, b.Y, b.Width, b.Height} {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
				return false
			}
		}
		if b.Width == 0 || b.Height == 0 || b.X+b.Width > 1 || b.Y+b.Height > 1 {
			return false
		}
	}
	return true
}

func (c *Client) FacePreview(ctx context.Context, image string) (FacePreview, error) {
	var out FacePreview
	if err := c.call(ctx, "POST", "/internal/v1/face-preview", map[string]string{"image": image}, &out); err != nil {
		return FacePreview{}, err
	}
	if !out.Valid() {
		return FacePreview{}, ErrUnavailable
	}
	// Guidance comes from quality alone; upstream identity metadata must never reach enrollment previews.
	switch out.Quality {
	case "ready":
		out.Message = "Wajah siap diambil"
	case "no_face":
		out.Message = "Posisikan wajah di depan kamera"
	case "multiple_faces":
		out.Message = "Pastikan hanya satu wajah di dalam kamera"
	case "low_quality":
		out.Message = "Perbaiki pencahayaan dan hadapkan wajah ke kamera"
	}
	if out.Faces == nil {
		out.Faces = []PreviewFace{}
	}
	return out, nil
}

type Recognition struct {
	EngineStatus       string         `json:"engine_status"`
	Reason             string         `json:"reason"`
	LivenessPassed     bool           `json:"liveness_passed"`
	Confirmed          bool           `json:"is_confirmed"`
	Decision           string         `json:"decision"`
	Faces              []Face         `json:"faces"`
	RecognitionEnabled bool           `json:"recognition_enabled"`
	BlockReason        string         `json:"block_reason"`
	EventID            string         `json:"event_id"`
	GalleryRevision    int64          `json:"gallery_revision"`
	ModelSHA           string         `json:"model_sha256"`
	Evaluation         map[string]any `json:"evaluation"`
}

func (c *Client) Recognize(ctx context.Context, payload any) (Recognition, error) {
	var out Recognition
	err := c.call(ctx, "POST", "/internal/v1/recognize", payload, &out)
	return out, err
}

type ValidationError struct{ Code, Message string }

func (e *ValidationError) Error() string { return e.Code }
func (c *Client) call(ctx context.Context, method, path string, input, output any) error {
	if c == nil {
		return ErrUnavailable
	}
	body, err := json.Marshal(input)
	if err != nil {
		return ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return ErrUnavailable
	}
	if response.StatusCode == 422 {
		var result struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &result) == nil && result.Error.Code != "" {
			return &ValidationError{result.Error.Code, result.Error.Message}
		}
	}
	if response.StatusCode != 200 || json.Unmarshal(raw, output) != nil {
		return ErrUnavailable
	}
	return nil
}

func ValidateImage(value string) bool {
	if value == "" || len(value) > 524288 || strings.ContainsAny(value, "\r\n") {
		return false
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(raw) > 384<<10 || len(raw) < 4 || !bytes.Equal(raw[:2], []byte{255, 216}) || !bytes.Equal(raw[len(raw)-2:], []byte{255, 217}) {
		return false
	}
	dimensions, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	return err == nil && dimensions.Width >= 160 && dimensions.Width <= 1280 && dimensions.Height >= 160 && dimensions.Height <= 1280
}
