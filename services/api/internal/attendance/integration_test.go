package attendance_test

import (
	"attendance.local/api/internal/attendance"
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/masterdata"
	"attendance.local/api/internal/platform/httpserver"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAttendanceDatabaseWorkflow(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run real database integration")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "attendance_test_" + strings.ToLower(rand.Text()[:12])
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
	h := httpserver.NewHandler(pool.Ping, "http://localhost:3000", func(m *http.ServeMux) {
		a.Register(m)
		master.Register(m)
		attendance.New(pool, schoolID, a).Register(m)
	})
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
	expect(call("GET", "/attendance-schedules", nil), 200)
	now := time.Now()
	start := now.AddDate(0, -6, 0).Format("2006-01-02")
	end := now.AddDate(0, 6, 0).Format("2006-01-02")
	year := expect(call("POST", "/academic-years", map[string]any{"name": "Attendance year", "starts_on": start, "ends_on": end, "is_active": true}), 201)["data"].(map[string]any)
	class := expect(call("POST", "/classes", map[string]any{"academic_year_id": year["id"], "code": "A", "name": "A"}), 201)["data"].(map[string]any)
	class2 := expect(call("POST", "/classes", map[string]any{"academic_year_id": year["id"], "code": "B", "name": "B"}), 201)["data"].(map[string]any)
	student := expect(call("POST", "/students", map[string]any{"nis": "001", "full_name": "Student one", "class_id": class["id"]}), 201)["data"].(map[string]any)
	student2 := expect(call("POST", "/students", map[string]any{"nis": "002", "full_name": "Student two", "class_id": class["id"]}), 201)["data"].(map[string]any)
	student3 := expect(call("POST", "/students", map[string]any{"nis": "003", "full_name": "Student three", "class_id": class["id"]}), 201)["data"].(map[string]any)
	teacher := expect(call("POST", "/users", map[string]any{"email": "teacher@example.test", "full_name": "Teacher", "role": "teacher", "password": password}), 201)["data"].(map[string]any)
	assignment := expect(call("POST", "/teacher-assignments", map[string]any{"teacher_id": teacher["id"], "class_id": class["id"]}), 201)["data"].(map[string]any)
	operator := expect(call("POST", "/users", map[string]any{"email": "operator@example.test", "full_name": "Operator", "role": "operator", "password": password, "grants": []string{"attendance.manual"}}), 201)["data"].(map[string]any)
	scheduleBody := map[string]any{"class_id": class["id"], "name": "Daily", "weekdays": []int{1, 2, 3, 4, 5, 6, 7}, "checkin_start": "00:00", "checkin_cutoff": "00:01", "grace_minutes": 0, "close_time": "23:59"}
	schedule := expect(call("POST", "/attendance-schedules", scheduleBody), 201)["data"].(map[string]any)
	scheduleID := schedule["id"].(string)
	expect(call("POST", "/attendance-schedules", scheduleBody), 409)
	expect(call("PATCH", "/attendance-schedules/"+scheduleID, map[string]any{"grace_minutes": 1}), 200)
	expect(call("PATCH", "/attendance-schedules/"+scheduleID, map[string]any{"close_time": "00:00"}), 400)
	var day string
	if err := pool.QueryRow(ctx, `SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date::text`).Scan(&day); err != nil {
		t.Fatal(err)
	}
	expect(call("GET", "/school/calendar/"+day, nil), 200)
	expect(call("PUT", "/school/calendar/"+day, map[string]any{"is_school_day": false, "reason": "Holiday"}), 200)
	expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": scheduleID, "date": day}), 409)
	expect(call("PUT", "/school/calendar/"+day, map[string]any{"is_school_day": true, "reason": "School day"}), 200)
	se := expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": scheduleID, "date": day}), 201)["data"].(map[string]any)
	sessionID := se["id"].(string)
	expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": scheduleID, "date": day}), 409)
	expect(call("POST", "/attendance-sessions/"+sessionID+"/open", map[string]any{"reason": "Start"}), 200)
	for _, st := range []map[string]any{student, student2, student3} {
		expect(call("POST", "/attendance-sessions/"+sessionID+"/participants", map[string]any{"student_id": st["id"], "reason": "Administrative roster addition"}), 201)
	}
	expect(call("POST", "/attendance-sessions/"+sessionID+"/participants", map[string]any{"student_id": student["id"], "reason": "Duplicate"}), 409)
	expect(call("GET", "/attendance-sessions/"+sessionID+"/participants?status=pending_review", nil), 200)
	roster := expect(call("GET", "/attendance-sessions/"+sessionID+"/participants", nil), 200)["data"].([]any)
	if len(roster) != 3 {
		t.Fatal("roster missing")
	}
	for _, v := range roster {
		if v.(map[string]any)["status"] != "not_recorded" {
			t.Fatal("premature absence")
		}
	}
	manual := map[string]any{"event_id": "11111111-1111-4111-8111-111111111111", "session_id": sessionID, "student_id": student["id"], "status": "present", "reason": "Verified by staff"}
	first := expect(call("POST", "/attendances/manual", manual), 201)["data"].(map[string]any)
	record := first["record"].(map[string]any)
	if record["status"] != "late" || first["outcome"] != "recorded" {
		t.Fatal("server late classification failed", first)
	}
	retry := expect(call("POST", "/attendances/manual", manual), 201)["data"].(map[string]any)
	if retry["record"].(map[string]any)["check_in_at"] != record["check_in_at"] {
		t.Fatal("retry changed first time")
	}
	manual["reason"] = "Different payload"
	expect(call("POST", "/attendances/manual", manual), 409)
	manual["reason"] = "Verified by staff"
	var group sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 2)
	for _, event := range []string{"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"} {
		group.Add(1)
		go func(event string) {
			defer group.Done()
			results <- call("POST", "/attendances/manual", map[string]any{"event_id": event, "session_id": sessionID, "student_id": student["id"], "status": "present", "reason": "Retry"})
		}(event)
	}
	group.Wait()
	close(results)
	for w := range results {
		data := expect(w, 201)["data"].(map[string]any)
		if data["outcome"] != "already_recorded" {
			t.Fatal("concurrent check-in duplicated")
		}
	}
	var events int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM attendance_events WHERE record_id=$1`, record["id"]).Scan(&events)
	if events != 1 {
		t.Fatal("duplicate events", events)
	}
	for _, mutation := range []string{
		`UPDATE attendance_events SET reason='tampered'`,
		`DELETE FROM attendance_session_participants`,
		`UPDATE request_ledger SET fingerprint='tampered'`,
	} {
		_, err := pool.Exec(ctx, mutation)
		var constraint *pgconn.PgError
		if !errors.As(err, &constraint) || constraint.Code != "23514" {
			t.Fatal("history mutation not blocked by immutable trigger", mutation, err)
		}
	}

	manual2 := map[string]any{"event_id": "44444444-4444-4444-8444-444444444444", "session_id": sessionID, "student_id": student2["id"], "status": "sick", "reason": "Verified sickness"}
	expect(call("POST", "/attendances/manual", manual2), 201)
	login("teacher@example.test")
	expect(call("GET", "/attendance-sessions/"+sessionID, nil), 200)
	expect(call("POST", "/attendances/manual", manual), 403)
	expect(call("POST", "/students/"+student["id"].(string)+"/placements", map[string]any{"class_id": class2["id"], "reason": "Denied"}), 403)
	correction := expect(call("POST", "/attendance-corrections", map[string]any{"session_id": sessionID, "student_id": student["id"], "status": "excused", "reason": "Verified excuse"}), 201)["data"].(map[string]any)
	correctionID := correction["id"].(string)
	login("owner@example.test")
	decisions := make(chan int, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			decisions <- call("POST", "/attendance-corrections/"+correctionID+"/decision", map[string]any{"decision": "approved", "reason": "Checked concurrently"}).Code
		}()
	}
	group.Wait()
	close(decisions)
	decisionCodes := map[int]int{}
	for code := range decisions {
		decisionCodes[code]++
	}
	if decisionCodes[200] != 1 || decisionCodes[409] != 1 {
		t.Fatal("double decision committed", decisionCodes)
	}
	own := expect(call("POST", "/attendance-corrections", map[string]any{"session_id": sessionID, "student_id": student2["id"], "status": "excused", "reason": "Own request"}), 201)["data"].(map[string]any)
	expect(call("POST", "/attendance-corrections/"+own["id"].(string)+"/decision", map[string]any{"decision": "approved", "reason": "Self"}), 403)
	login("operator@example.test")
	expect(call("GET", "/attendance-sessions/"+sessionID+"/participants", nil), 200)
	expect(call("POST", "/attendance-sessions/"+sessionID+"/reconcile", map[string]any{"reason": "Denied"}), 403)
	expect(call("POST", "/attendances/manual", manual2), 409)
	login("owner@example.test")
	expect(call("POST", "/students/"+student["id"].(string)+"/placements", map[string]any{"class_id": class2["id"], "reason": "Move after snapshot"}), 201)
	login("teacher@example.test")
	roster = expect(call("GET", "/attendance-sessions/"+sessionID+"/participants", nil), 200)["data"].([]any)
	if len(roster) != 3 {
		t.Fatal("historical roster changed after transfer")
	}
	expect(call("POST", "/attendance-corrections/"+own["id"].(string)+"/decision", map[string]any{"decision": "rejected", "reason": "Teacher cannot approve"}), 403)
	login("owner@example.test")
	expect(call("POST", "/attendance-sessions/"+sessionID+"/close", map[string]any{"reason": "End"}), 200)
	expect(call("POST", "/attendance-sessions/"+sessionID+"/reconcile", map[string]any{"reason": "Too early"}), 409)
	// Move the fixture's planned cutoff into the past to exercise finalization without sleeping.
	if _, err := pool.Exec(ctx, `UPDATE attendance_sessions SET closes_at=clock_timestamp()-interval '1 second' WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	expect(call("POST", "/attendance-sessions/"+sessionID+"/reconcile", map[string]any{"reason": "Pending request"}), 409)
	// Approver is distinct from requester; teacher never receives approval grant.
	expect(call("POST", "/users", map[string]any{"email": "approver@example.test", "full_name": "Approver", "role": "admin", "password": password, "grants": []string{"attendance.approve"}}), 201)
	login("approver@example.test")
	expect(call("POST", "/attendance-corrections/"+own["id"].(string)+"/decision", map[string]any{"decision": "rejected", "reason": "Sick record correct"}), 200)
	expect(call("POST", "/attendance-sessions/"+sessionID+"/reconcile", map[string]any{"reason": "Administrative review complete"}), 200)
	expect(call("POST", "/attendance-sessions/"+sessionID+"/reconcile", map[string]any{"reason": "Retry"}), 200)
	roster = expect(call("GET", "/attendance-sessions/"+sessionID+"/participants", nil), 200)["data"].([]any)
	statuses := map[string]string{}
	for _, v := range roster {
		m := v.(map[string]any)
		statuses[m["student_id"].(string)] = m["status"].(string)
	}
	if statuses[student["id"].(string)] != "excused" || statuses[student2["id"].(string)] != "sick" || statuses[student3["id"].(string)] != "absent" {
		t.Fatal("reconcile overwrote statuses", statuses)
	}
	expect(call("POST", "/attendances/manual", map[string]any{"event_id": "55555555-5555-4555-8555-555555555555", "session_id": sessionID, "student_id": student3["id"], "status": "present", "reason": "Closed"}), 409)
	login("owner@example.test")
	expect(call("PATCH", "/teacher-assignments/"+assignment["id"].(string), map[string]any{"is_active": false}), 200)
	login("teacher@example.test")
	expect(call("GET", "/attendance-sessions/"+sessionID+"/participants", nil), 404)
	login("owner@example.test")
	savedCSRF := csrf
	csrf = "wrong"
	expect(call("POST", "/attendances/manual", manual), 403)
	csrf = savedCSRF
	expect(call("GET", "/attendance-sessions?page_size=101", nil), 400)
	var count int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action LIKE 'attendance.%'`).Scan(&count)
	if count < 10 {
		t.Fatal("attendance audit missing", count)
	}
	// Existing session eligibility is frozen; editing the calendar must not silently cancel it.
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	future := expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": scheduleID, "date": tomorrow}), 201)["data"].(map[string]any)
	expect(call("PUT", "/school/calendar/"+tomorrow, map[string]any{"is_school_day": false, "reason": "Changed after creation"}), 200)
	if err := attendance.New(pool, schoolID, a).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	frozen := expect(call("GET", "/attendance-sessions/"+future["id"].(string), nil), 200)["data"].(map[string]any)
	if frozen["status"] != "scheduled" {
		t.Fatal("calendar edit silently cancelled existing session", frozen)
	}
	// First-insert contention uses a new participant, not an existing record.
	student4 := expect(call("POST", "/students", map[string]any{"nis": "004", "full_name": "Student four", "class_id": class["id"]}), 201)["data"].(map[string]any)
	// A separate fixture session is open for this concurrency check.
	scheduleBody["class_id"] = class2["id"]
	scheduleBody["name"] = "Worker class B"
	schedB := expect(call("POST", "/attendance-schedules", scheduleBody), 201)["data"].(map[string]any)
	var historicStudent string
	if err := pool.QueryRow(ctx, `INSERT INTO students(school_id,nis,full_name) VALUES($1,'005','Historical roster fixture') RETURNING id::text`, schoolID).Scan(&historicStudent); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO student_enrollments(school_id,student_id,class_id,academic_year_id,joined_at,left_at,reason) SELECT $1,$2,$3,id,$5::date::timestamp AT TIME ZONE timezone,(ends_on+1)::timestamp AT TIME ZONE timezone,'Historical fixture' FROM academic_years WHERE school_id=$1 AND id=$4`, schoolID, historicStudent, class2["id"], year["id"], day); err != nil {
		t.Fatal(err)
	}
	engineB := attendance.New(pool, schoolID, a)
	if err := engineB.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	var workerCreations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='attendance.sessions.created' AND metadata->>'source'='lifecycle_worker'`).Scan(&workerCreations); err != nil || workerCreations != 1 {
		t.Fatal("worker creation audit missing", workerCreations, err)
	}
	var sessionB string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM attendance_sessions WHERE schedule_id=$1 AND date=$2::date`, schedB["id"], day).Scan(&sessionB); err != nil {
		t.Fatal(err)
	}
	workerRoster := expect(call("GET", "/attendance-sessions/"+sessionB+"/participants", nil), 200)["data"].([]any)
	if len(workerRoster) != 1 || workerRoster[0].(map[string]any)["student_id"] != historicStudent || workerRoster[0].(map[string]any)["status"] != "not_recorded" {
		t.Fatal("worker opening did not snapshot initial placement", workerRoster)
	}
	// The newly enrolled student moves into B, then is explicitly added with an audit reason.
	expect(call("POST", "/students/"+student4["id"].(string)+"/placements", map[string]any{"class_id": class2["id"], "reason": "Test first insert"}), 201)
	expect(call("POST", "/attendance-sessions/"+sessionB+"/participants", map[string]any{"student_id": student4["id"], "reason": "Administrative roster addition"}), 201)
	firstResults := make(chan *httptest.ResponseRecorder, 2)
	for _, event := range []string{"66666666-6666-4666-8666-666666666666", "77777777-7777-4777-8777-777777777777"} {
		group.Add(1)
		go func(event string) {
			defer group.Done()
			firstResults <- call("POST", "/attendances/manual", map[string]any{"event_id": event, "session_id": sessionB, "student_id": student4["id"], "status": "present", "reason": "Concurrent first insert"})
		}(event)
	}
	group.Wait()
	close(firstResults)
	outcomes := map[string]int{}
	for w := range firstResults {
		m := expect(w, 201)["data"].(map[string]any)
		outcomes[m["outcome"].(string)]++
	}
	if outcomes["recorded"] != 1 || outcomes["already_recorded"] != 1 {
		t.Fatal("first insert concurrency duplicated", outcomes)
	}
	var firstEvents int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM attendance_events e JOIN attendance_records ar ON ar.id=e.record_id AND ar.school_id=e.school_id WHERE ar.session_id=$1 AND ar.student_id=$2`, sessionB, student4["id"]).Scan(&firstEvents)
	if firstEvents != 1 {
		t.Fatal("first insert event duplicated")
	}
	// Teacher unassigned B sees neither its session nor its candidates; worker close never adds absence.
	login("teacher@example.test")
	expect(call("GET", "/attendance-sessions/"+sessionB, nil), 404)
	expect(call("GET", "/attendance-sessions/"+sessionB+"/participants", nil), 404)
	login("owner@example.test")
	if _, err := pool.Exec(ctx, `UPDATE attendance_sessions SET closes_at=clock_timestamp()-interval '1 second' WHERE id=$1`, sessionB); err != nil {
		t.Fatal(err)
	}
	if err := engineB.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	closed := expect(call("GET", "/attendance-sessions/"+sessionB, nil), 200)["data"].(map[string]any)
	if closed["status"] != "closed" || closed["reconciled_at"] != nil {
		t.Fatal("worker incorrectly reconciled session")
	}
	workerRoster = expect(call("GET", "/attendance-sessions/"+sessionB+"/participants", nil), 200)["data"].([]any)
	for _, v := range workerRoster {
		m := v.(map[string]any)
		if m["student_id"] == historicStudent && m["status"] != "not_recorded" {
			t.Fatal("worker assigned negative attendance")
		}
	}
	stale := expect(call("POST", "/attendance-corrections", map[string]any{"session_id": sessionID, "student_id": student2["id"], "status": "excused", "reason": "Stale fixture request"}), 201)["data"].(map[string]any)
	if _, err := pool.Exec(ctx, `UPDATE attendance_records SET version=version+1 WHERE session_id=$1 AND student_id=$2`, sessionID, student2["id"]); err != nil {
		t.Fatal(err)
	}
	login("approver@example.test")
	expect(call("POST", "/attendance-corrections/"+stale["id"].(string)+"/decision", map[string]any{"decision": "approved", "reason": "Must conflict"}), 409)
	expect(call("POST", "/attendance-corrections/"+stale["id"].(string)+"/decision", map[string]any{"decision": "rejected", "reason": "Record changed"}), 200)
	// Permissions and sessions are rechecked; stale operator session cannot keep its revoked grant.
	login("operator@example.test")
	oldCookie, oldCSRF := cookie, csrf
	login("owner@example.test")
	expect(call("PATCH", "/users/"+operator["id"].(string), map[string]any{"grants": []string{}}), 200)
	cookie, csrf = oldCookie, oldCSRF
	expect(call("GET", "/attendance-sessions/"+sessionID+"/participants", nil), 401)
	login("operator@example.test")
	expect(call("GET", "/attendance-sessions", nil), 403)
	login("owner@example.test")
	if _, err := pool.Exec(ctx, `INSERT INTO schools(id,name) VALUES('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb','Other school')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO attendance_records(school_id,session_id,student_id,status,reason,actor_id) VALUES('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',$1,$2,'sick','cross school',$3)`, sessionB, historicStudent, operator["id"]); err == nil {
		t.Fatal("cross-school attendance FK accepted")
	}
	expect(call("GET", "/attendance-sessions/bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", nil), 404)

	// Worker retry does not generate duplicates or finalize absence.
	engine := attendance.New(pool, schoolID, a)
	if err := engine.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err := engine.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	var sessions int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM attendance_sessions WHERE schedule_id=$1 AND date=$2::date`, scheduleID, day).Scan(&sessions)
	if sessions != 1 {
		t.Fatal("worker duplicated session")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='attendance.sessions.created' AND metadata->>'source'='lifecycle_worker'`).Scan(&workerCreations); err != nil || workerCreations != 1 {
		t.Fatal("worker retry duplicated creation audit", workerCreations, err)
	}
}
