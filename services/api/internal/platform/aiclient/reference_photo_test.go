package aiclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReferencePhotoValidatesPrivateResponse(t *testing.T) {
	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, image.NewRGBA(image.Rect(0, 0, 192, 192)), nil); err != nil {
		t.Fatal(err)
	}
	photo := base64.StdEncoding.EncodeToString(jpegBytes.Bytes())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/reference-photo" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 48) {
			t.Error("unprotected reference photo request")
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request["school_id"] != "school" || request["student_id"] != "student" {
			t.Error("missing identity binding")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"image": photo})
	}))
	defer server.Close()
	client, err := New(server.URL, strings.Repeat("s", 48))
	if err != nil {
		t.Fatal(err)
	}
	input := ReferencePhoto{Ciphertext: "encrypted", Nonce: "nonce", KeyID: "key"}
	result, err := client.ReferencePhoto(context.Background(), "school", "student", input)
	if err != nil || result != photo {
		t.Fatalf("reference photo: %v", err)
	}
	for _, invalid := range []string{"data:image/svg+xml;base64,AAAA", base64.StdEncoding.EncodeToString([]byte("not jpeg")), strings.Repeat("a", 90000)} {
		photo = invalid
		if _, err = client.ReferencePhoto(context.Background(), "school", "student", input); err == nil {
			t.Error("accepted invalid reference image")
		}
	}
}
