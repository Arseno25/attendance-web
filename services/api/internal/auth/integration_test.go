package auth_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	password := "ExamplePassword1!"
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
	expect(call("POST", "/users", map[string]any{"email": "forbidden-grant@example.test", "full_name": "Forbidden Grant", "role": "teacher", "password": password, "grants": []string{}}, cookie, csrf, true), 400)

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
	created := call("POST", "/users", map[string]any{"email": "teacher@example.test", "full_name": "Guru Uji", "role": "teacher", "password": password}, cookie, csrf, true)
	expect(created, 201)
	var user struct{ Data auth.User }
	if err = json.Unmarshal(created.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	expect(call("POST", "/users", map[string]any{"email": "teacher@example.test", "full_name": "Guru Uji", "role": "teacher", "password": password}, cookie, csrf, true), 409)
	teacher, teacherCSRF := login("teacher@example.test", nil)
	expect(call("GET", "/users", nil, teacher, "", true), 403)
	expect(call("GET", "/audit-logs", nil, teacher, "", true), 403)
	expect(call("GET", "/permissions", nil, teacher, "", true), 403)
	expect(call("GET", "/roles", nil, teacher, "", true), 403)
	var customRoleID string
	if err = pool.QueryRow(ctx, `INSERT INTO roles(school_id,key,name,description,scope,is_system) VALUES($1,'custom_roles_manager','Custom Roles Manager','', 'school',false) RETURNING id::text`, schoolID).Scan(&customRoleID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO role_permissions(school_id,role_id,permission) VALUES($1,$2,'roles.manage')`, schoolID, customRoleID); err != nil {
		t.Fatal(err)
	}
	var ownerHash string
	if err = pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE email='owner@example.test'`).Scan(&ownerHash); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO users(school_id,email,full_name,password_hash,role) VALUES($1,'custom-manager@example.test','Custom Manager',$2,'custom_roles_manager')`, schoolID, ownerHash); err != nil {
		t.Fatal(err)
	}
	customRoleCookie, customRoleCSRF := login("custom-manager@example.test", nil)
	expect(call("GET", "/permissions", nil, customRoleCookie, "", true), 403)
	expect(call("GET", "/roles", nil, customRoleCookie, "", true), 403)
	expect(call("PATCH", "/users/"+user.Data.ID, map[string]any{"role": "super_admin"}, customRoleCookie, customRoleCSRF, true), 403)

	catalog := call("GET", "/permissions", nil, cookie, "", true)
	expect(catalog, 200)
	rolesList := call("GET", "/roles", nil, cookie, "", true)
	expect(rolesList, 200)
	var catalogResult struct {
		Data []auth.Permission `json:"data"`
	}
	if err = json.Unmarshal(catalog.Body.Bytes(), &catalogResult); err != nil || len(catalogResult.Data) != 18 {
		t.Fatal("permission catalog missing or malformed")
	}
	for _, permission := range catalogResult.Data {
		if permission.Key == "consents.manage" {
			t.Fatal("retired consent permission exposed")
		}
	}
	var roleRows struct {
		Data []auth.Role `json:"data"`
	}
	if err = json.Unmarshal(rolesList.Body.Bytes(), &roleRows); err != nil || len(roleRows.Data) < 4 {
		t.Fatal("built-in roles missing or malformed")
	}
	var createdRole struct {
		Data auth.Role `json:"data"`
	}
	createdRoleResponse := call("POST", "/roles", map[string]any{"name": "  Petugas Kelas  ", "description": " Role khusus ", "scope": "assigned_classes", "permissions": []string{"students.read", "attendance.read"}}, cookie, csrf, true)
	expect(createdRoleResponse, 201)
	if err = json.Unmarshal(createdRoleResponse.Body.Bytes(), &createdRole); err != nil || createdRole.Data.Scope != auth.ScopeAssignedClasses || createdRole.Data.Name != "Petugas Kelas" || len(createdRole.Data.Permissions) != 2 || createdRole.Data.Key == "" {
		t.Fatal("created dynamic role response is invalid")
	}
	roleID := createdRole.Data.ID
	expect(call("PATCH", "/roles/"+roleID, map[string]any{"scope": "school", "permissions": []string{"students.read"}}, cookie, csrf, true), 200)
	var dynamicUser struct {
		Data auth.User `json:"data"`
	}
	createdDynamicUser := call("POST", "/users", map[string]any{"email": "dynamic@example.test", "full_name": "Pengguna Dinamis", "role": createdRole.Data.Key, "password": password}, cookie, csrf, true)
	expect(createdDynamicUser, 201)
	if strings.Contains(createdDynamicUser.Body.String(), "grants") || !strings.Contains(createdDynamicUser.Body.String(), `"role_name"`) {
		t.Fatal("user response should have dynamic role name and no grants")
	}
	if err = json.Unmarshal(createdDynamicUser.Body.Bytes(), &dynamicUser); err != nil || dynamicUser.Data.RoleName != "Petugas Kelas" {
		t.Fatal("dynamic user role name missing")
	}
	var dynamicUserSessions int
	for i := 1; i <= 2; i++ {
		hash := sha256.Sum256([]byte(fmt.Sprintf("dynamic-role-session-%d", i)))
		if _, err = pool.Exec(ctx, `INSERT INTO user_sessions(token_hash,school_id,user_id,expires_at) VALUES($1,$2,$3,now()+interval '8 hours') ON CONFLICT DO NOTHING`, hash[:], schoolID, dynamicUser.Data.ID); err != nil {
			t.Fatal(err)
		}
	}
	expect(call("PATCH", "/roles/"+roleID, map[string]any{"permissions": []string{"students.read", "students.write"}}, cookie, csrf, true), 200)
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM user_sessions WHERE school_id=$1 AND user_id=$2", schoolID, dynamicUser.Data.ID).Scan(&dynamicUserSessions); err != nil || dynamicUserSessions != 0 {
		t.Fatal("role update did not revoke all sessions for its users")
	}
	var roleAuditCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE school_id=$1 AND target_type='role' AND target_id=$2 AND action IN ('roles.created','roles.updated')`, schoolID, createdRole.Data.ID).Scan(&roleAuditCount); err != nil || roleAuditCount != 3 {
		t.Fatalf("role mutations were not audited transactionally: count=%d err=%v role=%s", roleAuditCount, err, createdRole.Data.ID)
	}
	expect(call("DELETE", "/roles/"+roleID, nil, cookie, csrf, true), 409)
	var superRoleID string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM roles WHERE school_id=$1 AND key='super_admin'`, schoolID).Scan(&superRoleID); err != nil {
		t.Fatal(err)
	}
	expect(call("PATCH", "/roles/"+superRoleID, map[string]any{"scope": "assigned_classes"}, cookie, csrf, true), 400)
	expect(call("PATCH", "/roles/"+superRoleID, map[string]any{"permissions": []string{"users.read", "roles.manage"}}, cookie, csrf, true), 400)
	expect(call("POST", "/roles", map[string]any{"name": "Invalid", "description": "", "scope": "school", "permissions": []string{"unknown.permission"}}, cookie, csrf, true), 400)
	expect(call("POST", "/roles", map[string]any{"name": "Invalid scope", "description": "", "scope": "district", "permissions": []string{}}, cookie, csrf, true), 400)
	expect(call("PATCH", "/roles/"+roleID, map[string]any{"permissions": []string{"students.read", "students.read"}}, cookie, csrf, true), 400)

	expect(call("PATCH", "/users/"+user.Data.ID, map[string]any{"is_active": false}, cookie, csrf, true), 200)
	expect(call("PATCH", "/users/"+user.Data.ID, map[string]any{"role": "super_admin"}, teacher, teacherCSRF, true), 401)
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
