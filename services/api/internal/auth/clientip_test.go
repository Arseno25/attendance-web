package auth

import (
	"fmt"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestClientIPTrustsOnlyConfiguredProxyChain(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24"), netip.MustParsePrefix("fd00::/64")}
	for _, tc := range []struct{ peer, forwarded, want string }{
		{"198.51.100.1:1234", "203.0.113.9", "198.51.100.1"},
		{"10.0.0.2:1234", "198.51.100.8", "198.51.100.8"},
		{"10.0.0.2:1234", "203.0.113.99, 198.51.100.8, 10.0.0.3", "198.51.100.8"},
		{"[fd00::2]:1234", "2001:db8::8, fd00::3", "2001:db8::8"},
		{"[::ffff:10.0.0.2]:1234", "198.51.100.8", "198.51.100.8"},
		{"10.0.0.2:1234", "invalid", "10.0.0.2"},
		{"10.0.0.2:1234", "", "10.0.0.2"},
		{"10.0.0.2:1234", "198.51.100.8, unknown", "10.0.0.2"},
		{"10.0.0.2:1234", strings.Repeat("10.0.0.3,", 21), "10.0.0.2"},
	} {
		t.Run(tc.peer+"/"+tc.forwarded, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			s := Service{trustedProxies: trusted}
			if got := s.clientIP(r); got != tc.want {
				t.Fatalf("client IP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTrustedProxyClientsHaveSeparateLoginQuotas(t *testing.T) {
	s, err := NewWithLocalOrigins(nil, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "http://localhost:3000", false, netip.MustParsePrefix("10.0.0.2/32"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 40; i++ {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"email":"invalid","password":"x"}`))
		r.RemoteAddr = "10.0.0.2:4000"
		r.Header.Set("Origin", "http://localhost:3000")
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		w := httptest.NewRecorder()
		s.login(w, r)
		if w.Code != 400 {
			t.Fatalf("distinct client %d blocked: %d", i, w.Code)
		}
	}
	for i := 0; i < 30; i++ {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"email":"invalid","password":"x"}`))
		r.RemoteAddr = "198.51.100.1:4000"
		r.Header.Set("Origin", "http://localhost:3000")
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i))
		w := httptest.NewRecorder()
		s.login(w, r)
		want := 400
		if i == 29 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("untrusted client bypassed its quota: attempt %d status %d", i+1, w.Code)
		}
	}
}
