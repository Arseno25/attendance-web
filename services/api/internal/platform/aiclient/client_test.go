package aiclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
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
			_ = json.NewEncoder(w).Encode(Status{PipelineReady: true, BlockReason: BlockReason})
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
