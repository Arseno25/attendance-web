package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"attendance.local/api/internal/platform/httpserver"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const profileSchool = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
const profilePassword = "Original test password 1!"

type profileFixture struct {
	s      *Service
	h      http.Handler
	pool   *pgxpool.Pool
	schema string
	hash   string
}

func newProfileFixture(t *testing.T) profileFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run real database integration")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "profile_test_" + strings.ToLower(rand.Text()[:12])
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		root.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); root.Close() })
	conf, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	conf.ConnConfig.RuntimeParams["search_path"] = schema
	conf.ConnConfig.RuntimeParams["application_name"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
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
		if _, err = pool.Exec(ctx, up); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(pool, profileSchool, "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Bootstrap(ctx, "owner@example.test", "Owner", profilePassword, "School"); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err = pool.QueryRow(ctx, "SELECT password_hash FROM users").Scan(&hash); err != nil {
		t.Fatal(err)
	}
	return profileFixture{s: s, h: httpserver.NewHandler(pool.Ping, s.origin, s.Register), pool: pool, schema: schema, hash: hash}
}

func (f profileFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f profileFixture) user(t *testing.T, email, role string, grants []string) User {
	t.Helper()
	var id string
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO users(school_id,email,full_name,password_hash,role) VALUES($1,$2,'User',$3,$4) RETURNING id::text`, profileSchool, email, f.hash, role).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, permission := range grants {
		f.exec(t, `INSERT INTO role_permissions(school_id,role_id,permission)
			SELECT school_id,id,$3 FROM roles WHERE school_id=$1 AND key=$2
			ON CONFLICT(school_id,role_id,permission) DO NOTHING`, profileSchool, role, permission)
	}
	return User{ID: id, SchoolID: profileSchool, Email: email, FullName: "User", Role: role, Active: true, ClassIDs: []string{}}
}

func (f profileFixture) cookie(t *testing.T, userID string) *http.Cookie {
	t.Helper()
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(buf)
	f.exec(t, `INSERT INTO user_sessions(token_hash,school_id,user_id,expires_at) VALUES($1,$2,$3,now()+interval '8 hours')`, tokenHash(token), profileSchool, userID)
	return &http.Cookie{Name: f.s.cookieName, Value: token}
}

func (f profileFixture) call(method, path string, body any, cookie *http.Cookie, csrf, origin string) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(payload))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", origin)
	r.Header.Set("X-CSRF-Token", csrf)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	return w
}

func profileResponse(t *testing.T, w *httptest.ResponseRecorder, status int, code string) (User, string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body.String())
	}
	var result struct {
		Data struct {
			User        User                                `json:"user"`
			CSRF        string                              `json:"csrf_token"`
			Permissions []string                            `json:"permissions"`
			School      struct{ ID, Name, Timezone string } `json:"school"`
		} `json:"data"`
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error.Code != code {
		t.Fatalf("code=%q want=%q", result.Error.Code, code)
	}
	if status == 200 && strings.Contains(w.Body.String(), `"csrf_token"`) {
		if result.Data.School.ID != profileSchool || result.Data.School.Timezone != "Asia/Jakarta" || result.Data.CSRF == "" || !reflect.DeepEqual(result.Data.Permissions, result.Data.User.Permissions()) {
			t.Fatal("auth/me session contract missing")
		}
	}
	for _, secret := range []string{"password_hash", "current_password", "new_password", profilePassword} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("private credentials returned")
		}
	}
	return result.Data.User, result.Data.CSRF
}

func TestProfilePreservesTeacherScopeAndOperatorAccess(t *testing.T) {
	f := newProfileFixture(t)
	teacher := f.user(t, "teacher@example.test", "teacher", []string{"reports.export"})
	var yearID, classID string
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx, `INSERT INTO academic_years(school_id,name,starts_on,ends_on,timezone,is_active) VALUES($1,'Current',(CURRENT_DATE-1),(CURRENT_DATE+1),'Asia/Jakarta',true) RETURNING id::text`, profileSchool).Scan(&yearID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO classes(school_id,academic_year_id,code,name) VALUES($1,$2,'A','Class A') RETURNING id::text`, profileSchool, yearID).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO user_class_assignments(school_id,user_id,class_id,academic_year_id,starts_at,ends_at) VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '1 hour')`, profileSchool, teacher.ID, classID, yearID)
	operator := f.user(t, "operator@example.test", "operator", []string{"attendance.manual"})
	for _, user := range []User{teacher, operator} {
		t.Run(user.Role, func(t *testing.T) {
			cookie := f.cookie(t, user.ID)
			other := f.cookie(t, user.ID)
			w := f.call("PATCH", "/auth/profile", map[string]string{"full_name": "  Updated User  ", "email": " " + strings.ToUpper(user.Email) + " "}, cookie, csrfToken(cookie.Value), f.s.origin)
			u, csrf := profileResponse(t, w, 200, "")
			if u.ID != user.ID || u.FullName != "Updated User" || u.Email != user.Email || u.Role != user.Role || !u.Active || csrf != csrfToken(cookie.Value) || len(w.Result().Cookies()) != 0 {
				t.Fatal("name-only edit changed credentials, access, or session")
			}
			if user.Role == "teacher" && !reflect.DeepEqual(u.ClassIDs, []string{classID}) {
				t.Fatal("teacher scope lost")
			}
			profileResponse(t, f.call("GET", "/auth/me", nil, other, "", ""), 200, "")
			var hash string
			if err := f.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, user.ID).Scan(&hash); err != nil || hash != f.hash {
				t.Fatal("name-only edit changed password")
			}
		})
	}
}

func TestProfileSensitiveChangesRotateSessionAndAuditWithoutSecrets(t *testing.T) {
	f := newProfileFixture(t)
	var ownerID string
	if err := f.pool.QueryRow(context.Background(), `SELECT id::text FROM users WHERE role='super_admin'`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	cookie, other := f.cookie(t, ownerID), f.cookie(t, ownerID)
	newPassword := "  Replacement password 1! " + strings.Repeat("a", 140) + "  "
	w := f.call("PATCH", "/auth/profile", map[string]string{"full_name": "Owner Updated", "email": " NEW@example.test ", "current_password": profilePassword, "new_password": newPassword}, cookie, csrfToken(cookie.Value), f.s.origin)
	u, csrf := profileResponse(t, w, 200, "")
	if u.ID != ownerID || u.Email != "new@example.test" || u.Role != "super_admin" || !u.Active || len(w.Result().Cookies()) != 1 {
		t.Fatal("self update lost owner access or session rotation")
	}
	rotated := w.Result().Cookies()[0]
	if rotated.Value == cookie.Value || csrf == csrfToken(cookie.Value) || csrf != csrfToken(rotated.Value) || !rotated.HttpOnly || strings.Contains(w.Body.String(), rotated.Value) {
		t.Fatal("session rotation or token secrecy missing")
	}
	profileResponse(t, f.call("GET", "/auth/me", nil, rotated, "", ""), 200, "")
	for _, old := range []*http.Cookie{cookie, other} {
		profileResponse(t, f.call("GET", "/auth/me", nil, old, "", ""), 401, "UNAUTHENTICATED")
	}
	var hash, metadata string
	var count int
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, ownerID).Scan(&hash); err != nil || !VerifyPassword(newPassword, hash) || VerifyPassword(strings.TrimSpace(newPassword), hash) || VerifyPassword(profilePassword, hash) {
		t.Fatal("new password not stored exactly")
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM user_sessions WHERE user_id=$1`, ownerID).Scan(&count); err != nil || count != 1 {
		t.Fatal("other sessions not revoked")
	}
	if err := f.pool.QueryRow(ctx, `SELECT metadata::text FROM audit_logs WHERE action='auth.profile_updated' AND actor_id=$1 AND target_id=$1 AND target_type='user'`, ownerID).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(metadata), &fields); err != nil || !reflect.DeepEqual(fields, map[string]any{"name_changed": true, "email_changed": true, "password_changed": true}) {
		t.Fatal("audit must contain only change booleans")
	}
	profileResponse(t, f.call("POST", "/auth/login", map[string]string{"email": "new@example.test", "password": newPassword}, nil, "", f.s.origin), 200, "")

	// Email-only changes reauthenticate and keep the password while rotating again.
	w = f.call("PATCH", "/auth/profile", map[string]string{"full_name": "Owner Updated", "email": "email-only@example.test", "current_password": newPassword}, rotated, csrf, f.s.origin)
	profileResponse(t, w, 200, "")
	var preserved string
	if err := f.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, ownerID).Scan(&preserved); err != nil || preserved != hash || w.Result().Cookies()[0].Value == rotated.Value {
		t.Fatal("email-only edit changed password or retained session")
	}
	emailCookie := w.Result().Cookies()[0]
	_, emailCSRF := profileResponse(t, w, 200, "")
	w = f.call("PATCH", "/auth/profile", map[string]string{"full_name": "Owner Updated", "email": "email-only@example.test", "current_password": newPassword, "new_password": "Abcd123!"}, emailCookie, emailCSRF, f.s.origin)
	profileResponse(t, w, 200, "")
	if err := f.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, ownerID).Scan(&hash); err != nil || !VerifyPassword("Abcd123!", hash) || w.Result().Cookies()[0].Value == emailCookie.Value {
		t.Fatal("password-only edit did not replace password and session")
	}
	profileResponse(t, f.call("GET", "/auth/me", nil, emailCookie, "", ""), 401, "UNAUTHENTICATED")
}

func TestProfileRejectsTamperingAndFailedReauthenticationWithoutMutation(t *testing.T) {
	f := newProfileFixture(t)
	u := f.user(t, "self@example.test", "operator", []string{"attendance.manual"})
	other := f.user(t, "other@example.test", "admin", []string{})
	cookie := f.cookie(t, u.ID)
	csrf := csrfToken(cookie.Value)
	base := map[string]string{"full_name": "Updated", "email": u.Email}
	for _, test := range []struct {
		name, field, value, code string
	}{
		{"id", "id", other.ID, "VALIDATION_ERROR"},
		{"school", "school_id", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "VALIDATION_ERROR"},
		{"role", "role", "super_admin", "VALIDATION_ERROR"},
		{"active", "is_active", "true", "VALIDATION_ERROR"},
		{"grants", "grants", "users.write", "VALIDATION_ERROR"},
		{"empty name", "full_name", "  ", "VALIDATION_ERROR"},
		{"long name", "full_name", strings.Repeat("名", 121), "VALIDATION_ERROR"},
		{"bad email", "email", "not-an-email", "VALIDATION_ERROR"},
		{"email reauth", "email", "changed@example.test", "CURRENT_PASSWORD_REQUIRED"},
		{"password reauth", "new_password", "Replacement password 1!", "CURRENT_PASSWORD_REQUIRED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := map[string]string{"full_name": base["full_name"], "email": base["email"], test.field: test.value}
			profileResponse(t, f.call("PATCH", "/auth/profile", body, cookie, csrf, f.s.origin), 400, test.code)
		})
	}
	for _, password := range []string{"Ab12!xy", "abcd123!", "ABCD123!", "Abcdefg!", "Abcd1234", "Abcd123 ", "Abcd123é"} {
		profileResponse(t, f.call("PATCH", "/auth/profile", map[string]string{"full_name": "Updated", "email": u.Email, "current_password": profilePassword, "new_password": password}, cookie, csrf, f.s.origin), 400, "VALIDATION_ERROR")
	}
	profileResponse(t, f.call("PATCH", "/auth/profile", map[string]string{"full_name": "Updated", "email": "changed@example.test", "current_password": "wrong password"}, cookie, csrf, f.s.origin), 400, "CURRENT_PASSWORD_INVALID")
	profileResponse(t, f.call("PATCH", "/auth/profile", map[string]string{"full_name": "Updated", "email": other.Email, "current_password": profilePassword}, cookie, csrf, f.s.origin), 409, "CONFLICT")
	profileResponse(t, f.call("PATCH", "/auth/profile", base, cookie, "wrong", f.s.origin), 403, "CSRF_INVALID")
	profileResponse(t, f.call("PATCH", "/auth/profile", base, cookie, csrf, ""), 403, "FORBIDDEN")
	profileResponse(t, f.call("PATCH", "/auth/profile", base, nil, csrf, f.s.origin), 401, "UNAUTHENTICATED")
	profileResponse(t, f.call("POST", "/auth/profile", base, cookie, csrf, f.s.origin), 405, "METHOD_NOT_ALLOWED")
	profileResponse(t, f.call("PATCH", "/auth/profile/"+other.ID, base, cookie, csrf, f.s.origin), 404, "NOT_FOUND")
	var email, name, hash, role string
	var active bool
	var audits int
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx, `SELECT email,full_name,password_hash,role,is_active FROM users WHERE id=$1`, u.ID).Scan(&email, &name, &hash, &role, &active); err != nil || email != u.Email || name != "User" || hash != f.hash || role != "operator" || !active {
		t.Fatal("rejected request changed account")
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='auth.profile_updated'`).Scan(&audits); err != nil || audits != 0 {
		t.Fatal("failed edit audit committed")
	}
	profileResponse(t, f.call("GET", "/auth/me", nil, cookie, "", ""), 200, "")
	// An ID query parameter cannot redirect the update to another account.
	updated, _ := profileResponse(t, f.call("PATCH", "/auth/profile?id="+other.ID, base, cookie, csrf, f.s.origin), 200, "")
	if updated.ID != u.ID {
		t.Fatal("profile query tampering selected another user")
	}
	if err := f.pool.QueryRow(ctx, `SELECT full_name FROM users WHERE id=$1`, other.ID).Scan(&name); err != nil || name != "User" {
		t.Fatal("other account changed")
	}
}

func TestProfileRateLimitsReauthenticationAndBoundsHashing(t *testing.T) {
	f := newProfileFixture(t)
	u := f.user(t, "limited@example.test", "operator", []string{})
	cookie := f.cookie(t, u.ID)
	body := map[string]string{"full_name": "User", "email": "changed@example.test", "current_password": "wrong password"}
	for range 10 {
		profileResponse(t, f.call("PATCH", "/auth/profile", body, cookie, csrfToken(cookie.Value), f.s.origin), 400, "CURRENT_PASSWORD_INVALID")
	}
	profileResponse(t, f.call("PATCH", "/auth/profile", body, cookie, csrfToken(cookie.Value), f.s.origin), 429, "RATE_LIMITED")
	profileResponse(t, f.call("PATCH", "/auth/profile", map[string]string{"full_name": "Allowed name edit", "email": u.Email}, cookie, csrfToken(cookie.Value), f.s.origin), 200, "")
	second := f.user(t, "busy@example.test", "teacher", []string{})
	secondCookie := f.cookie(t, second.ID)
	f.s.hashing <- struct{}{}
	f.s.hashing <- struct{}{}
	body["current_password"] = profilePassword
	profileResponse(t, f.call("PATCH", "/auth/profile", body, secondCookie, csrfToken(secondCookie.Value), f.s.origin), 429, "RATE_LIMITED")
	<-f.s.hashing
	<-f.s.hashing
}

func TestProfileAuditFailureRollsBackAccountAndSessions(t *testing.T) {
	f := newProfileFixture(t)
	u := f.user(t, "rollback@example.test", "operator", []string{})
	cookie, other := f.cookie(t, u.ID), f.cookie(t, u.ID)
	f.exec(t, `CREATE FUNCTION reject_profile_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='auth.profile_updated' THEN RAISE EXCEPTION 'test audit rejection'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_profile BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_profile_audit()`)
	w := f.call("PATCH", "/auth/profile", map[string]string{"full_name": "Rejected", "email": "rejected@example.test", "current_password": profilePassword, "new_password": "Rejected password 1!"}, cookie, csrfToken(cookie.Value), f.s.origin)
	profileResponse(t, w, 503, "DATABASE_UNAVAILABLE")
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("failed transaction issued cookie")
	}
	var email, name, hash string
	if err := f.pool.QueryRow(context.Background(), `SELECT email,full_name,password_hash FROM users WHERE id=$1`, u.ID).Scan(&email, &name, &hash); err != nil || email != u.Email || name != "User" || hash != f.hash {
		t.Fatal("failed audit left account changes")
	}
	for _, old := range []*http.Cookie{cookie, other} {
		profileResponse(t, f.call("GET", "/auth/me", nil, old, "", ""), 200, "")
	}
}

func (f profileFixture) waitForSchoolLock(t *testing.T, requests int) {
	t.Helper()
	f.waitForQueryLock(t, requests, "SELECT is_active FROM schools WHERE id=$1 FOR NO KEY UPDATE")
}

func (f profileFixture) waitForQueryLock(t *testing.T, requests int, query string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND query=$2`, f.schema, query).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count >= requests {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("request did not reach expected write lock")
}

func TestProfileRechecksLiveAuthenticationAndPasswordAfterHashing(t *testing.T) {
	for _, change := range []string{"school", "user", "session", "password"} {
		t.Run(change, func(t *testing.T) {
			f := newProfileFixture(t)
			u := f.user(t, "live@example.test", "operator", []string{})
			cookie := f.cookie(t, u.ID)
			ctx := context.Background()
			blocker, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(ctx)
			if _, err = blocker.Exec(ctx, `SELECT is_active FROM schools WHERE id=$1 FOR UPDATE`, profileSchool); err != nil {
				t.Fatal(err)
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				done <- f.call("PATCH", "/auth/profile", map[string]string{"full_name": "Rejected", "email": u.Email, "current_password": profilePassword, "new_password": "Replacement password 1!"}, cookie, csrfToken(cookie.Value), f.s.origin)
			}()
			f.waitForSchoolLock(t, 1)
			query, arg := "UPDATE schools SET is_active=false WHERE id=$1", profileSchool
			switch change {
			case "user":
				query, arg = "UPDATE users SET is_active=false WHERE id=$1", u.ID
			case "session":
				query, arg = "DELETE FROM user_sessions WHERE user_id=$1", u.ID
			case "password":
				query, arg = "UPDATE users SET password_hash='concurrent-password-hash' WHERE id=$1", u.ID
			}
			if _, err = blocker.Exec(ctx, query, arg); err != nil {
				t.Fatal(err)
			}
			if err = blocker.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			status, code := 401, "UNAUTHENTICATED"
			if change == "password" {
				status, code = 409, "ACCOUNT_CHANGED"
			}
			profileResponse(t, <-done, status, code)
			var name string
			if err = f.pool.QueryRow(ctx, `SELECT full_name FROM users WHERE id=$1`, u.ID).Scan(&name); err != nil || name != "User" {
				t.Fatal("stale request changed account")
			}
		})
	}
}

func TestProfileConcurrentPasswordChangesCannotOverwriteEachOther(t *testing.T) {
	f := newProfileFixture(t)
	u := f.user(t, "concurrent@example.test", "operator", []string{})
	cookies := []*http.Cookie{f.cookie(t, u.ID), f.cookie(t, u.ID)}
	ctx := context.Background()
	blocker, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	if _, err = blocker.Exec(ctx, `SELECT is_active FROM schools WHERE id=$1 FOR UPDATE`, profileSchool); err != nil {
		t.Fatal(err)
	}
	passwords := []string{"Concurrent replacement one 1!", "Concurrent replacement two 2!"}
	done := make(chan struct {
		index    int
		response *httptest.ResponseRecorder
	}, 2)
	for i, cookie := range cookies {
		go func() {
			done <- struct {
				index    int
				response *httptest.ResponseRecorder
			}{i, f.call("PATCH", "/auth/profile", map[string]string{"full_name": "User", "email": u.Email, "current_password": profilePassword, "new_password": passwords[i]}, cookie, csrfToken(cookie.Value), f.s.origin)}
		}()
	}
	f.waitForSchoolLock(t, 2)
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	winner := -1
	for range 2 {
		result := <-done
		if result.response.Code == 200 {
			if winner != -1 {
				t.Fatal("both stale password changes committed")
			}
			winner = result.index
			profileResponse(t, result.response, 200, "")
		} else {
			profileResponse(t, result.response, 401, "UNAUTHENTICATED")
		}
	}
	var hash string
	if err = f.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, u.ID).Scan(&hash); err != nil || winner == -1 || !VerifyPassword(passwords[winner], hash) {
		t.Fatal("winning password not preserved")
	}
}

func TestProfileConcurrentLogoutCompletesWithoutDeadlock(t *testing.T) {
	for _, sensitive := range []bool{false, true} {
		name := "current session"
		if sensitive {
			name = "other session during password change"
		}
		t.Run(name, func(t *testing.T) {
			f := newProfileFixture(t)
			u := f.user(t, "logout@example.test", "operator", []string{})
			cookie := f.cookie(t, u.ID)
			logoutCookie := cookie
			body := map[string]string{"full_name": "Updated", "email": u.Email}
			if sensitive {
				logoutCookie = f.cookie(t, u.ID)
				body["current_password"], body["new_password"] = profilePassword, "Replacement password 1!"
			}
			ctx := context.Background()
			blocker, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(ctx)
			if _, err = blocker.Exec(ctx, `SELECT is_active FROM schools WHERE id=$1 FOR UPDATE`, profileSchool); err != nil {
				t.Fatal(err)
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { done <- f.call("PATCH", "/auth/profile", body, cookie, csrfToken(cookie.Value), f.s.origin) }()
			f.waitForSchoolLock(t, 1)
			// Pause the existing logout transaction between its session deletion and audit.
			logout, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer logout.Rollback(ctx)
			if _, err = logout.Exec(ctx, `DELETE FROM user_sessions WHERE school_id=$1 AND token_hash=$2`, profileSchool, tokenHash(logoutCookie.Value)); err != nil {
				t.Fatal(err)
			}
			if _, err = logout.Exec(ctx, `SET LOCAL lock_timeout='2s'`); err != nil {
				t.Fatal(err)
			}
			if err = blocker.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			query := `SELECT expires_at>statement_timestamp() AND last_seen_at>statement_timestamp()-interval '30 minutes' FROM user_sessions WHERE school_id=$1 AND user_id=$2 AND token_hash=$3 FOR UPDATE`
			if sensitive {
				query = `SELECT 1 FROM user_sessions WHERE school_id=$1 AND user_id=$2 ORDER BY token_hash FOR UPDATE`
			}
			f.waitForQueryLock(t, 1, query)
			err = audit(ctx, logout, profileSchool, u.ID, "auth.logout", "user", u.ID, "test-logout")
			if err != nil {
				_ = logout.Rollback(ctx)
				t.Errorf("logout audit blocked by profile locks: %v", err)
			} else if err = logout.Commit(ctx); err != nil {
				t.Error(err)
			}
			status, code := 401, "UNAUTHENTICATED"
			if sensitive {
				status, code = 200, ""
			}
			profileResponse(t, <-done, status, code)
		})
	}
}
