package masterdata_test

import (
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/masterdata"
	"attendance.local/api/internal/platform/httpserver"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMasterDatabaseScopesAndHistory(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run real database integration")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "master_test_" + strings.ToLower(rand.Text()[:12])
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
	a, err := auth.New(pool, schoolID, "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	password := strings.Repeat("a", 16)
	if err = a.Bootstrap(ctx, "owner@example.test", "Owner", password, "School"); err != nil {
		t.Fatal(err)
	}
	master := masterdata.New(pool, schoolID, a)
	h := httpserver.NewHandler(pool.Ping, "http://localhost:3000", func(m *http.ServeMux) { a.Register(m); master.Register(m) })
	cookie := (*http.Cookie)(nil)
	csrf := ""
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(payload))
		r.Header.Set("Origin", "http://localhost:3000")
		r.Header.Set("Content-Type", "application/json")
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
	expect := func(w *httptest.ResponseRecorder, status int) map[string]any {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	login := func(email string) {
		w := call("POST", "/auth/login", map[string]string{"email": email, "password": password})
		data := expect(w, 200)["data"].(map[string]any)
		cookie = w.Result().Cookies()[0]
		csrf = data["csrf_token"].(string)
	}
	login("owner@example.test")
	expect(call("GET", "/academic-years", nil), 200)
	now := time.Now().UTC()
	start := now.AddDate(0, -6, 0).Format("2006-01-02")
	end := now.AddDate(0, 6, 0).Format("2006-01-02")
	year := expect(call("POST", "/academic-years", map[string]any{"name": "Tahun Uji", "starts_on": start, "ends_on": end, "is_active": true}), 201)["data"].(map[string]any)
	yearID := year["id"].(string)
	expect(call("POST", "/academic-years", map[string]any{"name": "Overlap", "starts_on": start, "ends_on": end}), 409)
	class := expect(call("POST", "/classes", map[string]any{"academic_year_id": yearID, "code": "7A", "name": "Kelas 7A", "level": "7"}), 201)["data"].(map[string]any)
	classID := class["id"].(string)
	class2 := expect(call("POST", "/classes", map[string]any{"academic_year_id": yearID, "code": "7B", "name": "Kelas 7B", "level": "7"}), 201)["data"].(map[string]any)
	class2ID := class2["id"].(string)
	expect(call("POST", "/classes", map[string]any{"academic_year_id": yearID, "code": "7a", "name": "Duplicate", "level": "7"}), 409)
	expect(call("PATCH", "/academic-years/"+yearID, map[string]any{"starts_on": now.AddDate(0, -7, 0).Format("2006-01-02")}), 409)
	student := expect(call("POST", "/students", map[string]any{"nis": "001", "full_name": "Siswa Uji", "class_id": classID}), 201)["data"].(map[string]any)
	studentID := student["id"].(string)
	if student["class_id"] != classID {
		t.Fatal("create response omitted current class", student)
	}
	expect(call("POST", "/students", map[string]any{"nis": "001", "full_name": "Duplicate", "class_id": classID}), 409)
	teacher := expect(call("POST", "/users", map[string]any{"email": "teacher@example.test", "full_name": "Guru", "role": "teacher", "password": password}), 201)["data"].(map[string]any)
	teacherID := teacher["id"].(string)
	assignment := expect(call("POST", "/teacher-assignments", map[string]any{"teacher_id": teacherID, "class_id": classID}), 201)["data"].(map[string]any)
	if assignment["is_current"] != true {
		t.Fatal("new assignment response not current", assignment)
	}
	expect(call("POST", "/teacher-assignments", map[string]any{"teacher_id": teacherID, "class_id": classID}), 409)
	login("teacher@example.test")
	expect(call("POST", "/students", map[string]any{"nis": "bad", "full_name": "Denied", "class_id": classID}), 403)
	teacherList := expect(call("GET", "/classes", nil), 200)["data"].([]any)
	if len(teacherList) != 1 {
		t.Fatal("teacher saw unassigned classes")
	}
	studentList := expect(call("GET", "/students", nil), 200)["data"].([]any)
	if len(studentList) != 1 {
		t.Fatal("teacher lost assigned student")
	}
	expect(call("GET", "/classes/"+class2ID, nil), 404)
	expect(call("GET", "/students/"+studentID, nil), 200)
	login("owner@example.test")
	expect(call("POST", "/students/"+studentID+"/placements", map[string]any{"class_id": class2ID, "reason": "Pindah kelas"}), 201)
	detail := expect(call("GET", "/students/"+studentID, nil), 200)["data"].(map[string]any)
	if len(detail["placements"].([]any)) != 2 {
		t.Fatal("transfer lost history")
	}
	expect(call("POST", "/students/"+studentID+"/placements", map[string]any{"class_id": classID, "reason": "Backdate", "effective_at": now.AddDate(0, 0, -1).Format(time.RFC3339)}), 409)
	var overlapID string
	err = pool.QueryRow(ctx, `INSERT INTO student_enrollments(school_id,student_id,class_id,academic_year_id,joined_at,left_at,reason) SELECT school_id,student_id,class_id,academic_year_id,joined_at,left_at,'overlap' FROM student_enrollments WHERE student_id=$1 LIMIT 1 RETURNING id::text`, studentID).Scan(&overlapID)
	if err == nil {
		t.Fatal("DB allowed overlapping placements")
	}
	login("teacher@example.test")
	expect(call("GET", "/students/"+studentID, nil), 404)
	list := expect(call("GET", "/students", nil), 200)["data"].([]any)
	if len(list) != 0 {
		t.Fatal("teacher saw transferred student")
	}
	login("owner@example.test")
	expect(call("PATCH", "/teacher-assignments/"+assignment["id"].(string), map[string]any{"is_active": false}), 200)
	login("teacher@example.test")
	classes := expect(call("GET", "/classes", nil), 200)["data"].([]any)
	if len(classes) != 0 {
		t.Fatal("revoked assignment remained readable")
	}
	login("owner@example.test")
	savedCSRF := csrf
	csrf = "wrong"
	expect(call("POST", "/classes", map[string]any{}), 403)
	csrf = savedCSRF
	expect(call("GET", "/students?page_size=101", nil), 400)
	expect(call("PATCH", "/students/"+studentID, map[string]any{"status": "graduated"}), 200)
	// Reactivation starts a new interval, including when returning to the same class.
	expect(call("PATCH", "/students/"+studentID, map[string]any{"status": "active"}), 200)
	expect(call("POST", "/students/"+studentID+"/placements", map[string]any{"class_id": class2ID, "reason": "Aktif kembali"}), 201)
	expect(call("POST", "/students/"+studentID+"/placements", map[string]any{"class_id": classID, "reason": "Future", "effective_at": time.Now().Add(time.Hour).Format(time.RFC3339)}), 400)
	expect(call("GET", "/master-data/summary", nil), 200)
	expect(call("GET", "/audit-logs", nil), 200)
	otherSchool := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	otherYear := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	otherClass := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	if _, err := pool.Exec(ctx, `INSERT INTO schools(id,name) VALUES($1,'Other')`, otherSchool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO academic_years(id,school_id,name,starts_on,ends_on,timezone,is_active) VALUES($1,$2,'Other',$3,$4,'Asia/Jakarta',true)`, otherYear, otherSchool, start, end); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO classes(id,school_id,academic_year_id,code,name) VALUES($1,$2,$3,'OTHER','Other class')`, otherClass, otherSchool, otherYear); err != nil {
		t.Fatal(err)
	}
	expect(call("GET", "/classes/"+otherClass, nil), 404)
	expect(call("PATCH", "/academic-years/"+otherYear, map[string]any{"name": "Denied"}), 404)
	expect(call("POST", "/students", map[string]any{"nis": "cross", "full_name": "Denied", "class_id": otherClass}), 404)
	expect(call("POST", "/classes", map[string]any{"code": "cross", "name": "Denied", "academic_year_id": otherYear}), 404)
	expect(call("PATCH", "/teacher-assignments/"+otherClass, map[string]any{"is_active": false}), 404)
	if _, err := pool.Exec(ctx, `INSERT INTO student_enrollments(school_id,student_id,class_id,academic_year_id,joined_at,left_at,reason) VALUES($1,$2,$3,$4,now(),now()+interval '1 day','cross')`, schoolID, studentID, otherClass, otherYear); err == nil {
		t.Fatal("cross-school FK permitted placement")
	}
	var deniedCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM students WHERE nis='cross'`).Scan(&deniedCount); err != nil || deniedCount != 0 {
		t.Fatal("failed creation leaked partial student", err)
	}
	// A failed activation must roll back deactivation of the existing active year.
	expect(call("POST", "/academic-years", map[string]any{"name": "Activation conflict", "starts_on": start, "ends_on": end, "is_active": true}), 409)
	activeYear := expect(call("GET", "/academic-years/"+yearID, nil), 200)["data"].(map[string]any)
	if activeYear["is_active"] != true {
		t.Fatal("failed activation changed active year")
	}
	var count int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE action LIKE 'students.%' OR action LIKE 'academics.%'").Scan(&count)
	if count < 7 {
		t.Fatal("master audit missing")
	}
}
