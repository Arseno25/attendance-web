package auth_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/platform/httpserver"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuthDatabaseLifecycleAndPermissions(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run real database integration")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "auth_test_" + strings.ToLower(rand.Text()[:12])
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); root.Close() })
	conf, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	conf.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.Split(string(body), "-- +goose Down")[0]
		lines := []string{}
		for _, line := range strings.Split(up, "\n") {
			if !strings.HasPrefix(line, "-- +goose") {
				lines = append(lines, line)
			}
		}
		if _, err = pool.Exec(ctx, strings.Join(lines, "\n")); err != nil {
			t.Fatal(err)
		}
	}
	schoolID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	svc, err := auth.New(pool, schoolID, "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	password := strings.Repeat("a", 16)
	if err = svc.Bootstrap(ctx, "owner@example.test", "Pemilik Uji", password, "Sekolah Uji"); err != nil {
		t.Fatal(err)
	}
	if err = svc.Bootstrap(ctx, "other@example.test", "Lain", password, "Sekolah Uji"); err == nil {
		t.Fatal("bootstrap reused")
	}
	h := httpserver.NewHandler(pool.Ping, "http://localhost:3000", svc.Register)
	call := func(method, path string, body any, cookie *http.Cookie, csrf string, origin bool) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(payload))
		r.Header.Set("Content-Type", "application/json")
		if origin {
			r.Header.Set("Origin", "http://localhost:3000")
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body.String())
		}
	}
	login := func(email string, cookie *http.Cookie) (*http.Cookie, string) {
		w := call("POST", "/auth/login", map[string]string{"email": email, "password": password}, cookie, "", true)
		expect(w, 200)
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].HttpOnly {
			t.Fatal("HttpOnly session missing")
		}
		var result struct {
			Data struct {
				CSRF string `json:"csrf_token"`
			}
		}
		if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Data.CSRF == "" {
			t.Fatal("CSRF missing")
		}
		if strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), cookies[0].Value) {
			t.Fatal("private credentials returned")
		}
		return cookies[0], result.Data.CSRF
	}
	expect(call("POST", "/auth/login", map[string]string{"email": "owner@example.test", "password": password}, nil, "", false), 403)
	expect(call("POST", "/auth/login", map[string]string{"email": "owner@example.test", "password": "wrong"}, nil, "", true), 401)
	cookie, csrf := login("owner@example.test", nil)
	var storedHash []byte
	if err = pool.QueryRow(ctx, "SELECT token_hash FROM user_sessions").Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256([]byte(cookie.Value))
	if !bytes.Equal(storedHash, wantHash[:]) {
		t.Fatal("session token must be stored as hash")
	}
	expect(call("POST", "/users", map[string]any{"unexpected": "field"}, cookie, csrf, true), 400)
	expect(call("GET", "/users?page_size=101", nil, cookie, "", true), 400)

	expect(call("GET", "/auth/me", nil, cookie, "", true), 200)
	upperSvc, err := auth.New(pool, strings.ToUpper(schoolID), "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	upperHandler := httpserver.NewHandler(pool.Ping, "http://localhost:3000", upperSvc.Register)
	r := httptest.NewRequest("GET", "/api/v1/users", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	upperHandler.ServeHTTP(w, r)
	t.Run("uppercase school UUID", func(t *testing.T) {
		if w.Code != 200 {
			t.Errorf("status=%d want=200", w.Code)
		}
	})
	expect(call("POST", "/auth/logout", nil, cookie, "wrong", true), 403)
	expect(call("POST", "/users", map[string]string{}, cookie, "", true), 403)
	created := call("POST", "/users", map[string]any{"email": "teacher@example.test", "full_name": "Guru Uji", "role": "teacher", "password": password, "grants": []string{}}, cookie, csrf, true)
	expect(created, 201)
	var user struct{ Data auth.User }
	if err = json.Unmarshal(created.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	expect(call("POST", "/users", map[string]any{"email": "teacher@example.test", "full_name": "Guru Uji", "role": "teacher", "password": password}, cookie, csrf, true), 409)
	teacher, teacherCSRF := login("teacher@example.test", nil)
	expect(call("GET", "/users", nil, teacher, "", true), 403)
	expect(call("GET", "/audit-logs", nil, teacher, "", true), 403)
	expect(call("PATCH", "/users/"+user.Data.ID, map[string]any{"role": "super_admin"}, teacher, teacherCSRF, true), 403)
	expect(call("PATCH", "/users/"+user.Data.ID, map[string]any{"is_active": false}, cookie, csrf, true), 200)
	expect(call("GET", "/auth/me", nil, teacher, "", true), 401)
	var ownerID string
	if err = pool.QueryRow(ctx, "SELECT id::text FROM users WHERE email='owner@example.test'").Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	expect(call("PATCH", "/users/"+ownerID, map[string]any{"is_active": false}, cookie, csrf, true), 409)
	t.Run("uppercase self UUID", func(t *testing.T) {
		w := call("PATCH", "/users/"+strings.ToUpper(ownerID), map[string]any{"email": "changed@example.test"}, cookie, csrf, true)
		if w.Code != 409 {
			t.Errorf("status=%d want=409", w.Code)
		}
	})
	otherSchool := "00000000-0000-4000-8000-000000000002"
	if _, err = pool.Exec(ctx, "INSERT INTO schools(id,name) VALUES($1,'Other');", otherSchool); err != nil {
		t.Fatal(err)
	}
	otherSvc, err := auth.New(pool, otherSchool, "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	if err = otherSvc.Bootstrap(ctx, "other@example.test", "Other", password, "Other"); err != nil {
		t.Fatal(err)
	}
	var otherID string
	_ = pool.QueryRow(ctx, "SELECT id::text FROM users WHERE school_id=$1", otherSchool).Scan(&otherID)
	expect(call("PATCH", "/users/"+otherID, map[string]any{"is_active": false}, cookie, csrf, true), 404)
	rotated, rotatedCSRF := login("owner@example.test", cookie)
	if rotated.Value == cookie.Value {
		t.Fatal("session not rotated")
	}
	expect(call("GET", "/auth/me", nil, cookie, "", true), 401)
	expect(call("POST", "/auth/logout", nil, rotated, rotatedCSRF, true), 200)
	expect(call("GET", "/auth/me", nil, rotated, "", true), 401)
	expired, _ := login("owner@example.test", nil)
	if _, err = pool.Exec(ctx, "UPDATE user_sessions SET last_seen_at=now()-interval '31 minutes'"); err != nil {
		t.Fatal(err)
	}
	expect(call("GET", "/auth/me", nil, expired, "", true), 401)
	absolute, _ := login("owner@example.test", nil)
	if _, err = pool.Exec(ctx, "UPDATE user_sessions SET created_at=now()-interval '9 hours',expires_at=now()-interval '1 hour',last_seen_at=now()"); err != nil {
		t.Fatal(err)
	}
	expect(call("GET", "/auth/me", nil, absolute, "", true), 401)
	schoolDisabled, _ := login("owner@example.test", nil)
	if _, err = pool.Exec(ctx, "UPDATE schools SET is_active=false WHERE id=$1", schoolID); err != nil {
		t.Fatal(err)
	}
	expect(call("GET", "/auth/me", nil, schoolDisabled, "", true), 401)

	var auditCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE school_id=$1", schoolID).Scan(&auditCount)
	if auditCount < 5 {
		t.Fatal("audit events missing")
	}
	if _, err = pool.Exec(ctx, "UPDATE audit_logs SET action='changed'"); err == nil {
		t.Fatal("audit mutable")
	}
}
