package weborigin

import "testing"

func TestAllowed(t *testing.T) {
	for _, tc := range []struct {
		name       string
		origin     string
		configured string
		allowLocal bool
		want       bool
	}{
		{"configured exact origin", "https://attendance.example.com", "https://attendance.example.com", false, true},
		{"different production origin", "https://other.example.com", "https://attendance.example.com", true, false},
		{"local fallback start", "http://localhost:3001", "http://localhost:3000", true, true},
		{"local fallback end", "http://localhost:3010", "http://localhost:3000", true, true},
		{"default local origin", "http://localhost:3001", "http://localhost:3001", false, true},
		{"local fallback disabled", "http://localhost:3002", "http://localhost:3001", false, false},
		{"local port outside range", "http://localhost:3011", "http://localhost:3000", true, false},
		{"non-loopback hostname", "http://example.com:3002", "http://localhost:3000", true, false},
		{"secure scheme only", "https://localhost:3002", "http://localhost:3000", true, false},
		{"origin with path", "http://localhost:3002/path", "http://localhost:3000", true, false},
		{"lookalike hostname", "http://localhost.evil:3002", "http://localhost:3000", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Allowed(tc.origin, tc.configured, tc.allowLocal); got != tc.want {
				t.Fatalf("Allowed(%q, %q, %t) = %t, want %t", tc.origin, tc.configured, tc.allowLocal, got, tc.want)
			}
		})
	}
}
