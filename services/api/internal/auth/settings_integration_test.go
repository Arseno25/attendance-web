package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSchoolSettingsAndPrivatePhoto(t *testing.T) {
	f := newProfileFixture(t)
	var owner string
	if err := f.pool.QueryRow(context.Background(), `SELECT id::text FROM users WHERE role='super_admin'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	adminCookie := f.cookie(t, owner)
	teacher := f.user(t, "photo@example.test", "teacher", nil)
	cookie := f.cookie(t, teacher.ID)
	call := func(method, path string, body []byte, c *http.Cookie, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(body))
		r.Header.Set("Origin", f.s.origin)
		if method == "PATCH" {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("X-CSRF-Token", csrf)
		if c != nil {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		f.h.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
		}
	}
	settings := []byte(`{"name":"Sekolah Uji","app_name":"Presensi Sekolah","address":"Alamat uji","phone":"0211234567","email":"info@example.test"}`)
	expect(call("PATCH", "/school/settings", settings, cookie, csrfToken(cookie.Value)), 403)
	expect(call("PATCH", "/school/settings", settings, adminCookie, "wrong"), 403)
	expect(call("PATCH", "/school/settings", settings, adminCookie, csrfToken(adminCookie.Value)), 200)
	public := call("GET", "/school/branding", nil, nil, "")
	expect(public, 200)
	if !strings.Contains(public.Body.String(), "Sekolah Uji") || strings.Contains(public.Body.String(), "Alamat uji") || strings.Contains(public.Body.String(), "info@example.test") {
		t.Fatal("incorrect public school identity")
	}
	expect(call("PATCH", "/school/settings", []byte(`{"name":"","app_name":"X"}`), adminCookie, csrfToken(adminCookie.Value)), 400)
	pixels := image.NewRGBA(image.Rect(0, 0, 64, 64))
	pixels.Set(1, 1, color.White)
	var encoded bytes.Buffer
	_ = png.Encode(&encoded, pixels)
	photo := encoded.Bytes()
	expect(call("PUT", "/auth/profile/photo", photo, nil, ""), 401)
	expect(call("PUT", "/auth/profile/photo", photo, cookie, "wrong"), 403)
	expect(call("PUT", "/auth/profile/photo", []byte("<svg>private</svg>"), cookie, csrfToken(cookie.Value)), 422)
	expect(call("PUT", "/auth/profile/photo", photo, cookie, csrfToken(cookie.Value)), 200)
	saved := call("GET", "/auth/profile/photo", nil, cookie, "")
	expect(saved, 200)
	if saved.Header().Get("Content-Type") != "image/jpeg" || saved.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("photo leaked through cache or wrong content type")
	}
	expect(call("GET", "/auth/profile/photo", nil, adminCookie, ""), 404)
	expect(call("GET", "/auth/profile/photo", nil, nil, ""), 401)
	me := call("GET", "/auth/me", nil, cookie, "")
	expect(me, 200)
	var session struct {
		Data struct {
			User struct {
				AvatarURL string `json:"avatar_url"`
			}
		}
	}
	if err := json.Unmarshal(me.Body.Bytes(), &session); err != nil || !strings.HasPrefix(session.Data.User.AvatarURL, "/auth/profile/photo?v=") {
		t.Fatal("photo absent from session", err)
	}
	expect(call("PUT", "/school/logo", photo, cookie, csrfToken(cookie.Value)), 403)
	expect(call("PUT", "/school/logo", photo, adminCookie, csrfToken(adminCookie.Value)), 200)
	logo := call("GET", "/school/logo", nil, nil, "")
	expect(logo, 200)
	if logo.Header().Get("Content-Type") != "image/png" {
		t.Fatal("logo was not normalized")
	}
	expect(call("DELETE", "/school/logo", nil, adminCookie, csrfToken(adminCookie.Value)), 200)
	expect(call("GET", "/school/logo", nil, nil, ""), 404)
	expect(call("DELETE", "/auth/profile/photo", nil, cookie, csrfToken(cookie.Value)), 200)
	expect(call("GET", "/auth/profile/photo", nil, cookie, ""), 404)
	var leaks int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE metadata::text LIKE '%base64%' OR metadata::text LIKE '%info@example.test%' OR metadata::text LIKE '%Alamat uji%'`).Scan(&leaks); err != nil || leaks != 0 {
		t.Fatal("private values in audit", err)
	}
	f.exec(t, `UPDATE user_sessions SET created_at=now()-interval '2 hours',expires_at=now()-interval '1 hour' WHERE token_hash=$1`, tokenHash(cookie.Value))
	expect(call("PUT", "/auth/profile/photo", photo, cookie, csrfToken(cookie.Value)), 401)
}
