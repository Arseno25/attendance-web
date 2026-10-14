package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteOriginUsesLocalFallbackOnlyWhenEnabled(t *testing.T) {
	for _, tc := range []struct {
		name      string
		origin    string
		enabled   bool
		wantAllow bool
	}{
		{"configured origin", "http://localhost:3000", false, true},
		{"alternate local origin disabled", "http://localhost:3002", false, false},
		{"alternate local origin enabled", "http://localhost:3002", true, true},
		{"non-loopback rejected", "http://attacker.example:3002", true, false},
		{"missing origin rejected", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{origin: "http://localhost:3000", allowLocalWebPortRange: tc.enabled}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			if got := s.writeOrigin(w, r); got != tc.wantAllow {
				t.Fatalf("writeOrigin allowed=%t, want %t", got, tc.wantAllow)
			}
			if !tc.wantAllow && w.Code != http.StatusForbidden {
				t.Fatalf("rejected origin status=%d, want 403", w.Code)
			}
		})
	}
}
