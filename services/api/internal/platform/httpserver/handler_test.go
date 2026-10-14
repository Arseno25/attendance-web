package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthDoesNotClaimReadinessWithoutDatabase(t *testing.T) {
	for _, tc := range []struct {
		path    string
		pingErr error
		want    int
	}{
		{"/api/v1/health/live", errors.New("private connection string"), 200},
		{"/api/v1/health/ready", errors.New("private connection string"), 503},
		{"/api/v1/health/ready", nil, 200},
	} {
		t.Run(tc.path+http.StatusText(tc.want), func(t *testing.T) {
			h := NewHandler(func(context.Context) error { return tc.pingErr }, "http://localhost:3000", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != tc.want {
				t.Fatalf("status=%d, want=%d", w.Code, tc.want)
			}
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			id, ok := body["request_id"].(string)
			if !ok || id == "" || id != w.Header().Get("X-Request-ID") {
				t.Fatal("request ID missing or inconsistent")
			}
			if strings.Contains(w.Body.String(), "private connection string") {
				t.Fatal("database error exposed")
			}
		})
	}
}

func TestCORSOnlyAllowsConfiguredOrigin(t *testing.T) {
	h := NewHandler(func(context.Context) error { return nil }, "http://localhost:3000", nil)
	for _, tc := range []struct {
		origin string
		want   int
	}{
		{"http://localhost:3000", 204},
		{"https://untrusted.example", 403},
	} {
		r := httptest.NewRequest("OPTIONS", "/api/v1/health/live", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Access-Control-Request-Method", "GET")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("origin %s: status=%d want=%d", tc.origin, w.Code, tc.want)
		}
		if tc.want == 403 && w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted CORS origin allowed")
		}
	}
}

func TestCORSAllowsDevelopmentFallbackPortsOnlyWhenEnabled(t *testing.T) {
	h := NewHandlerWithLocalOrigins(func(context.Context) error { return nil }, "http://localhost:3000", true, nil)
	for _, tc := range []struct {
		origin string
		want   int
	}{
		{"http://localhost:3001", 204},
		{"http://localhost:3010", 204},
		{"http://localhost:3011", 403},
		{"http://127.0.0.1:3002", 403},
		{"http://localhost.evil:3002", 403},
		{"http://evil.example:3002", 403},
	} {
		r := httptest.NewRequest("OPTIONS", "/api/v1/health/live", nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("origin %s: status=%d want=%d", tc.origin, w.Code, tc.want)
		}
		if tc.want == 204 && w.Header().Get("Access-Control-Allow-Origin") != tc.origin {
			t.Errorf("origin %s was not echoed", tc.origin)
		}
	}

	strict := NewHandler(func(context.Context) error { return nil }, "http://localhost:3000", nil)
	r := httptest.NewRequest("OPTIONS", "/api/v1/health/live", nil)
	r.Header.Set("Origin", "http://localhost:3002")
	w := httptest.NewRecorder()
	strict.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("strict handler accepted local alternate origin: %d", w.Code)
	}
}

func TestUnknownRoutesReturnJSONWithoutEchoingClientRequestID(t *testing.T) {
	h := NewHandler(func(context.Context) error { return nil }, "http://localhost:3000", nil)
	r := httptest.NewRequest("GET", "/api/v1/students", nil)
	r.Header.Set("X-Request-ID", "attacker-controlled")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 404 || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Request-ID") == "attacker-controlled" {
		t.Fatal("untrusted request ID reflected")
	}
}

func TestRequestHasBoundedDatabaseContext(t *testing.T) {
	h := NewHandler(func(context.Context) error { return nil }, "http://localhost:3000", func(m *http.ServeMux) {
		m.HandleFunc("/api/v1/probe", func(w http.ResponseWriter, r *http.Request) {
			deadline, ok := r.Context().Deadline()
			remaining := time.Until(deadline)
			if !ok || remaining <= 7*time.Second || remaining > 8*time.Second {
				t.Error("request database context not bounded")
			}
			w.WriteHeader(204)
		})
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/probe", nil))
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
}

func TestCalendarPUTPreflightAllowsCredentialedWrite(t *testing.T) {
	h := NewHandler(func(context.Context) error { return nil }, "http://localhost:3000", nil)
	r := httptest.NewRequest("OPTIONS", "/api/v1/school/calendar/2026-10-13", nil)
	r.Header.Set("Origin", "http://localhost:3000")
	r.Header.Set("Access-Control-Request-Method", "PUT")
	r.Header.Set("Access-Control-Request-Headers", "Content-Type, X-CSRF-Token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" || w.Header().Get("Access-Control-Allow-Credentials") != "true" || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "PUT") {
		t.Fatal("calendar write blocked by preflight", w.Code, w.Header())
	}
}
