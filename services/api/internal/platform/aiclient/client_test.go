package aiclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInternalClientAuthorizationBoundedResponseAndNoRedirect(t *testing.T) {
	token := strings.Repeat("s", 48)
	mode := "status"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing internal token")
		}
		switch mode {
		case "status":
			_ = json.NewEncoder(w).Encode(Status{PipelineReady: true, BlockReason: "PIPELINE_NOT_CONFIGURED"})
		case "redirect":
			http.Redirect(w, r, "https://example.invalid", 302)
		case "oversized":
			_, _ = w.Write(bytes.Repeat([]byte("x"), (1<<20)+1))
		case "validation":
			w.WriteHeader(422)
			_, _ = w.Write([]byte(`{"error":{"code":"FACE_COUNT","message":"Satu wajah wajib"}}`))
		}
	}))
	defer server.Close()
	client, err := New(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	if state := client.Status(context.Background()); !state.PipelineReady || state.RecognitionEnabled {
		t.Fatal(state)
	}
	for _, value := range []string{"redirect", "oversized"} {
		mode = value
		if _, err := client.Enroll(context.Background(), "s", "st", "co", nil); !errors.Is(err, ErrUnavailable) {
			t.Fatal(value, err)
		}
	}
	mode = "validation"
	_, err = client.Enroll(context.Background(), "s", "st", "co", nil)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Code != "FACE_COUNT" {
		t.Fatal(err)
	}
	if state := (*Client)(nil).Status(context.Background()); state.PipelineReady || state.RecognitionEnabled {
		t.Fatal(state)
	}
	for _, value := range []string{"https://user:password@example.test", "file:///tmp/ai", "https://example.test/client/path"} {
		if _, err := New(value, token); err == nil {
			t.Fatal("unsafe base URL accepted")
		}
	}
}

func TestFacePreviewValidatesGeometryAndDropsPrivateMetadata(t *testing.T) {
	response := `{"faces":[{"box":{"x":0.1,"y":0.2,"width":0.3,"height":0.4},"student_id":"PRIVATE-ID","label":"PRIVATE-NAME","embedding":[1]}],"face_count":1,"quality":"ready","message":"PRIVATE-NAME","similarity":0.99}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/face-preview" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 48) {
			t.Error("wrong preview request")
		}
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input) != 1 || input["image"] != "synthetic" {
			t.Error("wrong preview payload", input, err)
		}
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	client, err := New(server.URL, strings.Repeat("s", 48))
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.FacePreview(context.Background(), "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "embedding") || strings.Contains(string(raw), "similarity") {
		t.Fatal("private preview metadata leaked", string(raw))
	}
	for _, quality := range []string{"no_face", "multiple_faces", "low_quality", "ready"} {
		result.Quality = quality
		result.Faces = []PreviewFace{{Box: Box{X: .1, Y: .2, Width: .3, Height: .4}}}
		if quality == "no_face" {
			result.Faces = []PreviewFace{}
		}
		if quality == "multiple_faces" {
			result.Faces = append(result.Faces, result.Faces[0])
		}
		result.FaceCount = len(result.Faces)
		if !result.Valid() {
			t.Fatal("valid quality rejected", quality)
		}
	}
	result.Quality = "ready"
	result.Faces = result.Faces[:1]
	result.FaceCount = 1
	for _, box := range []Box{{Width: .2, Height: 0}, {X: -.1, Width: .2, Height: .2}, {X: .9, Width: .2, Height: .2}, {Y: .9, Width: .2, Height: .2}, {X: math.NaN(), Width: .2, Height: .2}, {Width: math.Inf(1), Height: .2}} {
		result.Faces[0].Box = box
		if result.Valid() {
			t.Fatal("invalid box accepted", box)
		}
	}
	for _, bad := range []string{
		`{"faces":[],"face_count":1,"quality":"ready"}`,
		`{"faces":[],"face_count":0,"quality":"ready"}`,
		`{"faces":[],"face_count":0,"quality":"unexpected"}`,
		`{"faces":[{"box":{"x":0,"y":0,"width":0,"height":1}}],"face_count":1,"quality":"ready"}`,
		`{"faces":[],"face_count":-1,"quality":"no_face"}`,
		`{"faces":[],"face_count":0,"quality":"no_face"} {}`,
	} {
		response = bad
		if _, err := client.FacePreview(context.Background(), "synthetic"); !errors.Is(err, ErrUnavailable) {
			t.Fatal("invalid preview accepted", bad, err)
		}
	}
}
func TestImageBoundariesBeforeNetwork(t *testing.T) {
	var raw bytes.Buffer
	if err := jpeg.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 240, 240)), nil); err != nil {
		t.Fatal(err)
	}
	value := base64.StdEncoding.EncodeToString(raw.Bytes())
	if !ValidateImage(value) {
		t.Fatal("valid synthetic JPEG rejected")
	}
	for _, bad := range []string{"", value + "\n", "data:image/jpeg;base64," + value, strings.Repeat("A", 524292), base64.StdEncoding.EncodeToString([]byte("not JPEG"))} {
		if ValidateImage(bad) {
			t.Fatal("invalid JPEG accepted")
		}
	}
	raw.Reset()
	if err := jpeg.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 1281, 160)), nil); err != nil {
		t.Fatal(err)
	}
	if ValidateImage(base64.StdEncoding.EncodeToString(raw.Bytes())) {
		t.Fatal("oversized dimensions accepted")
	}
}
