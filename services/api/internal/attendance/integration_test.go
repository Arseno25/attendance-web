package attendance_test

import (
	"attendance.local/api/internal/attendance"
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/masterdata"
	"attendance.local/api/internal/platform/holidays"
	"attendance.local/api/internal/platform/httpserver"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
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
	password := "ExamplePassword1!"
	if err = a.Bootstrap(ctx, "owner@example.test", "Owner", password, "School"); err != nil {
		t.Fatal(err)
	}

	holidayTime := time.Now().In(time.FixedZone("Jakarta", 7*3600)).AddDate(0, 0, 2)
	holidayDate, jointDate := holidayTime.Format("2006-01-02"), holidayTime.AddDate(0, 0, 1).Format("2006-01-02")
	providerFailure, providerEmpty := false, false
	var providerStarted, providerRelease chan struct{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if providerStarted != nil {
			close(providerStarted)
			<-providerRelease
		}
		if providerFailure {
			w.WriteHeader(503)
			return
		}
		var year int
		fmt.Sscanf(r.URL.Path, "/holidays/%d", &year)
		data := []holidays.Day{}
		for _, d := range []holidays.Day{{Date: holidayDate, Name: "Synthetic national holiday", Type: "national_holiday"}, {Date: jointDate, Name: "Synthetic joint leave", Type: "joint_leave"}} {
			if strings.HasPrefix(d.Date, fmt.Sprint(year)) {
				data = append(data, d)
			}
		}
		if len(data) == 0 {
			data = append(data, holidays.Day{Date: fmt.Sprintf("%d-01-01", year), Name: "Synthetic New Year", Type: "national_holiday"})
		}
		if providerEmpty {
			data = []holidays.Day{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]int{"year": year, "count": len(data)}, "data": data})
	}))
	defer provider.Close()
	attendanceService := attendance.New(pool, schoolID, a)
	attendanceService.SetHolidays(holidays.New(provider.URL))
	master := masterdata.New(pool, schoolID, a)
	h := httpserver.NewHandler(pool.Ping, "http://localhost:3000", func(m *http.ServeMux) {
		a.Register(m)
		master.Register(m)
		attendanceService.Register(m)
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
	cachedSessions := map[string]struct {
		cookie *http.Cookie
		csrf   string
	}{}
	login := func(email string) {
		if cached, exists := cachedSessions[email]; exists {
			cookie = cached.cookie
			csrf = cached.csrf
			if call("GET", "/auth/me", nil).Code == 200 {
				return
			}
		}
		w := call("POST", "/auth/login", map[string]string{"email": email, "password": password})
		data := expect(w, 200)["data"].(map[string]any)
		cookie = w.Result().Cookies()[0]
		csrf = data["csrf_token"].(string)
		cachedSessions[email] = struct {
			cookie *http.Cookie
			csrf   string
		}{cookie, csrf}
	}
	login("owner@example.test")
	expect(call("GET", "/reports/attendance", nil), 200)
	expect(call("GET", "/attendance-schedules", nil), 200)
	now := time.Now()
	start := now.AddDate(0, -6, 0).Format("2006-01-02")
	end := now.AddDate(0, 6, 0).Format("2006-01-02")
	year := expect(call("POST", "/academic-years", map[string]any{"name": "Attendance year", "starts_on": start, "ends_on": end, "is_active": true}), 201)["data"].(map[string]any)
	class := expect(call("POST", "/classes", map[string]any{"academic_year_id": year["id"], "code": "A", "name": "A"}), 201)["data"].(map[string]any)
	class2 := expect(call("POST", "/classes", map[string]any{"academic_year_id": year["id"], "code": "B", "name": "B"}), 201)["data"].(map[string]any)
	student := expect(call("POST", "/students", map[string]any{"nis": "001", "full_name": "=SUM(1,2)", "class_id": class["id"]}), 201)["data"].(map[string]any)
	student2 := expect(call("POST", "/students", map[string]any{"nis": "002", "full_name": "Student;=1+2", "class_id": class["id"]}), 201)["data"].(map[string]any)
	student3 := expect(call("POST", "/students", map[string]any{"nis": "003", "full_name": "Student three", "class_id": class["id"]}), 201)["data"].(map[string]any)
	dry := expect(call("POST", "/students/imports/dry-run", map[string]any{"class_id": class2["id"], "csv": "nis,nisn,full_name\nIMPORT-1,,Imported Student\nIMPORT-2,,Another Student\n"}), 201)["data"].(map[string]any)
	batchID := dry["id"].(string)
	expect(call("POST", "/students/imports/"+batchID+"/commit", map[string]any{"hash": "wrong"}), 409)
	committed := expect(call("POST", "/students/imports/"+batchID+"/commit", map[string]any{"hash": dry["hash"]}), 200)["data"].(map[string]any)
	if committed["imported"] != float64(2) {
		t.Fatal(committed)
	}
	expect(call("POST", "/students/imports/"+batchID+"/commit", map[string]any{"hash": dry["hash"]}), 200)
	invalidBatch := expect(call("POST", "/students/imports/dry-run", map[string]any{"class_id": class2["id"], "csv": "nis,nisn,full_name\n001,,Existing Student\nNEW-FAILED,,Rollback Student\n"}), 201)["data"].(map[string]any)
	if invalidBatch["valid"] != false {
		t.Fatal("duplicate accepted", invalidBatch)
	}
	expect(call("POST", "/students/imports/"+invalidBatch["id"].(string)+"/commit", map[string]any{"hash": invalidBatch["hash"]}), 409)
	var rolledBack int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM students WHERE nis='NEW-FAILED'`).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatal("invalid import wrote data", rolledBack, err)
	}
	expect(call("POST", "/users", map[string]any{"email": "deviceonly@example.test", "full_name": "Device only", "role": "operator", "password": password}), 201)
	login("deviceonly@example.test")
	healthOptions := expect(call("GET", "/kiosk/session-options", nil), 200)["data"].([]any)
	if len(healthOptions) != 0 {
		t.Fatal("scheduled/closed sessions exposed", healthOptions)
	}
	login("owner@example.test")
	// Enrollment remains available before automatic recognition is evaluated.
	expect(call("POST", "/students/"+student["id"].(string)+"/guardians", nil), 404)
	expect(call("POST", "/students/"+student["id"].(string)+"/consents", nil), 404)
	expect(call("POST", "/students/"+student["id"].(string)+"/enrollments", nil), 400)
	face := expect(call("GET", "/students/"+student["id"].(string)+"/face-status", nil), 200)["data"].(map[string]any)
	if face["enrollment_allowed"] != true || face["recognition_enabled"] != false {
		t.Fatal(face)
	}
	device := expect(call("POST", "/devices", map[string]string{"name": "Gate", "location": "Entrance"}), 201)["data"].(map[string]any)
	expect(call("GET", "/devices", nil), 200)
	expect(call("POST", "/devices/"+device["id"].(string)+"/revoke", map[string]string{"reason": "Retired"}), 200)
	expect(call("POST", "/devices/"+device["id"].(string)+"/pairing-token", nil), 409)
	teacher := expect(call("POST", "/users", map[string]any{"email": "teacher@example.test", "full_name": "Teacher", "role": "teacher", "password": password}), 201)["data"].(map[string]any)
	assignment := expect(call("POST", "/class-assignments", map[string]any{"user_id": teacher["id"], "class_id": class["id"]}), 201)["data"].(map[string]any)
	operatorRole := expect(call("POST", "/roles", map[string]any{"name": "Attendance operator", "scope": "school", "permissions": []string{"attendance.manual"}}), 201)["data"].(map[string]any)
	operator := expect(call("POST", "/users", map[string]any{"email": "operator@example.test", "full_name": "Operator", "role": operatorRole["key"], "password": password}), 201)["data"].(map[string]any)
	scheduleBody := map[string]any{"class_id": class["id"], "name": "Daily", "checkout_start": "23:30", "weekdays": []int{1, 2, 3, 4, 5, 6, 7}, "checkin_start": "00:00", "checkin_cutoff": "00:01", "grace_minutes": 0, "close_time": "23:59"}
	schedule := expect(call("POST", "/attendance-schedules", scheduleBody), 201)["data"].(map[string]any)
	scheduleID := schedule["id"].(string)
	expect(call("POST", "/attendance-schedules", scheduleBody), 409)
	expect(call("PATCH", "/attendance-schedules/"+scheduleID, map[string]any{"grace_minutes": 1}), 200)
	expect(call("PATCH", "/attendance-schedules/"+scheduleID, map[string]any{"close_time": "00:00"}), 400)
	var day string
	if err := pool.QueryRow(ctx, `SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date::text`).Scan(&day); err != nil {
		t.Fatal(err)
	}
	expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": scheduleID, "date": day}), 409)
	if err := attendanceService.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if existing := expect(call("GET", "/attendance-sessions?date="+day, nil), 200)["data"].([]any); len(existing) != 0 {
		t.Fatal("unknown calendar generated attendance session", existing)
	}
	expect(call("POST", "/school/calendar/sync", map[string]any{"year": now.Year()}), 200)

	if holidayTime.Year() != now.Year() {
		expect(call("POST", "/school/calendar/sync", map[string]any{"year": holidayTime.Year()}), 200)
	}
	if holidayTime.AddDate(0, 0, 1).Year() != holidayTime.Year() {
		expect(call("POST", "/school/calendar/sync", map[string]any{"year": holidayTime.AddDate(0, 0, 1).Year()}), 200)
	}
	national := expect(call("GET", "/school/calendar/"+holidayDate, nil), 200)["data"].(map[string]any)
	joint := expect(call("GET", "/school/calendar/"+jointDate, nil), 200)["data"].(map[string]any)
	if national["source"] != "national_holiday" || national["is_school_day"] != false || joint["source"] != "joint_leave" {
		t.Fatal("holidays not applied", national, joint)
	}
	expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": scheduleID, "date": holidayDate}), 409)
	expect(call("PUT", "/school/calendar/"+holidayDate, map[string]any{"is_school_day": true, "reason": "Replacement school day"}), 200)
	expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": scheduleID, "date": holidayDate}), 201)
	expect(call("DELETE", "/school/calendar/"+holidayDate, nil), 200)
	if reset := expect(call("GET", "/school/calendar/"+holidayDate, nil), 200)["data"].(map[string]any); reset["is_override"] != false || reset["is_school_day"] != false {
		t.Fatal("reset lost automatic holiday", reset)
	}
	expect(call("PUT", "/school/calendar", map[string]any{"from": holidayDate, "to": jointDate, "is_school_day": false, "reason": "Libur semester"}), 200)
	providerFailure = true
	expect(call("POST", "/school/calendar/sync", map[string]any{"year": holidayTime.Year()}), 503)
	providerFailure = false
	providerEmpty = true
	expect(call("POST", "/school/calendar/sync", map[string]any{"year": holidayTime.Year()}), 503)
	providerEmpty = false
	after := expect(call("GET", "/school/calendar/"+holidayDate, nil), 200)["data"].(map[string]any)
	if after["source"] != "school" || after["reason"] != "Libur semester" || after["holiday_name"] != "Synthetic national holiday" {
		t.Fatal("sync destroyed local/cached calendar", after)
	}
	monthData := expect(call("GET", "/school/calendar/month?month="+holidayDate[:7], nil), 200)["data"].(map[string]any)
	if len(monthData["days"].([]any)) < 28 || monthData["sync"].(map[string]any)["synced_at"] == nil {
		t.Fatal("month/cache unavailable", monthData)
	}
	expect(call("PUT", "/school/calendar", map[string]any{"from": jointDate, "to": holidayDate, "is_school_day": false, "reason": "Reversed"}), 400)
	expect(call("PUT", "/school/calendar", map[string]any{"from": "2026-01-01", "to": "2027-01-02", "is_school_day": false, "reason": "Too long"}), 400)
	expect(call("POST", "/school/calendar/sync", map[string]any{"year": 99999}), 400)
	expect(call("GET", "/school/calendar/month?month=invalid", nil), 400)
	expect(call("DELETE", "/school/calendar/"+holidayDate, nil), 200)
	expect(call("DELETE", "/school/calendar/"+jointDate, nil), 200)
	expect(call("POST", "/school/calendar/sync", map[string]any{"year": holidayTime.Year()}), 200)

	syncRole := expect(call("POST", "/roles", map[string]any{"name": "School calendar writer", "scope": "school", "permissions": []string{"academics.write", "attendance.read"}}), 201)["data"].(map[string]any)
	syncUser := expect(call("POST", "/users", map[string]any{"email": "calendar-sync@example.test", "full_name": "Sync writer", "role": syncRole["key"], "password": password}), 201)["data"].(map[string]any)
	var beforeSync, afterSync time.Time
	if err := pool.QueryRow(ctx, `SELECT synced_at FROM school_holiday_sync WHERE school_id=$1 AND year=$2`, schoolID, holidayTime.Year()).Scan(&beforeSync); err != nil {
		t.Fatal(err)
	}
	login("calendar-sync@example.test")
	req := httptest.NewRequest("POST", "/api/v1/school/calendar/sync", strings.NewReader(fmt.Sprintf(`{"year":%d}`, holidayTime.Year())))
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(cookie)
	providerStarted, providerRelease = make(chan struct{}), make(chan struct{})
	syncResult := make(chan *httptest.ResponseRecorder, 1)
	go func() { response := httptest.NewRecorder(); h.ServeHTTP(response, req); syncResult <- response }()
	select {
	case <-providerStarted:
	case <-time.After(3 * time.Second):
		close(providerRelease)
		t.Fatal("holiday provider did not start")
	}
	login("owner@example.test")
	expect(call("PATCH", "/users/"+syncUser["id"].(string), map[string]any{"is_active": false}), 200)
	close(providerRelease)
	expect(<-syncResult, 401)
	providerStarted, providerRelease = nil, nil
	if err := pool.QueryRow(ctx, `SELECT synced_at FROM school_holiday_sync WHERE school_id=$1 AND year=$2`, schoolID, holidayTime.Year()).Scan(&afterSync); err != nil || !beforeSync.Equal(afterSync) {
		t.Fatal("revoked sync request mutated holiday cache", beforeSync, afterSync, err)
	}
	limitedRole := expect(call("POST", "/roles", map[string]any{"name": "Class calendar writer", "scope": "assigned_classes", "permissions": []string{"academics.write"}}), 201)["data"].(map[string]any)
	limitedUser := expect(call("POST", "/users", map[string]any{"email": "calendar-class@example.test", "full_name": "Calendar scoped", "role": limitedRole["key"], "password": password}), 201)["data"].(map[string]any)
	expect(call("POST", "/class-assignments", map[string]any{"user_id": limitedUser["id"], "class_id": class["id"]}), 201)
	login("calendar-class@example.test")
	expect(call("GET", "/school/calendar/month?month="+day[:7], nil), 200)
	expect(call("PUT", "/school/calendar/"+day, map[string]any{"is_school_day": false, "reason": "Outside authority"}), 403)
	expect(call("DELETE", "/school/calendar/"+day, nil), 403)
	expect(call("PUT", "/school/calendar", map[string]any{"from": day, "to": day, "is_school_day": false, "reason": "Outside authority"}), 403)
	expect(call("POST", "/school/calendar/sync", map[string]any{"year": now.Year()}), 403)
	login("owner@example.test")
	expect(call("GET", "/school/calendar/"+day, nil), 200)
	expect(call("PUT", "/school/calendar/"+day, map[string]any{"is_school_day": false, "reason": "Holiday"}), 200)
	expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": scheduleID, "date": day}), 409)
	expect(call("PUT", "/school/calendar/"+day, map[string]any{"is_school_day": true, "reason": "School day"}), 200)
	se := expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": scheduleID, "date": day}), 201)["data"].(map[string]any)
	sessionID := se["id"].(string)
	if se["checkout_at"] == nil {
		t.Fatal("session missing departure snapshot", se)
	}
	expect(call("PATCH", "/attendance-schedules/"+scheduleID, map[string]any{"checkout_start": "23:00"}), 200)
	frozenCheckout := expect(call("GET", "/attendance-sessions/"+sessionID, nil), 200)["data"].(map[string]any)
	if frozenCheckout["checkout_at"] != se["checkout_at"] {
		t.Fatal("schedule edit changed existing session", frozenCheckout)
	}
	// The remaining lifecycle fixtures move the session window independently.
	if _, err := pool.Exec(ctx, `UPDATE attendance_sessions SET checkout_at=NULL WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}

	expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": scheduleID, "date": day}), 409)
	expect(call("PUT", "/school/calendar/"+day, map[string]any{"is_school_day": false, "reason": "Holiday after scheduling"}), 200)
	expect(call("POST", "/attendance-sessions/"+sessionID+"/open", map[string]any{"reason": "Must not open on holiday"}), 409)
	if err := attendance.New(pool, schoolID, a).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if blocked := expect(call("GET", "/attendance-sessions/"+sessionID, nil), 200)["data"].(map[string]any); blocked["status"] != "scheduled" {
		t.Fatal("worker opened a scheduled session on a holiday", blocked)
	}
	expect(call("PUT", "/school/calendar/"+day, map[string]any{"is_school_day": true, "reason": "School day restored"}), 200)
	expect(call("POST", "/attendance-sessions/"+sessionID+"/open", map[string]any{"reason": "Start"}), 200)
	expect(call("PUT", "/school/calendar/"+day, map[string]any{"is_school_day": false, "reason": "Do not rewrite an open session"}), 200)
	if err := attendance.New(pool, schoolID, a).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if opened := expect(call("GET", "/attendance-sessions/"+sessionID, nil), 200)["data"].(map[string]any); opened["status"] != "open" {
		t.Fatal("calendar edit changed an already open session", opened)
	}
	expect(call("PUT", "/school/calendar/"+day, map[string]any{"is_school_day": true, "reason": "Continue school day"}), 200)
	activeDevice := expect(call("POST", "/devices", map[string]string{"name": "Gate 2", "location": "Side"}), 201)["data"].(map[string]any)
	pairing := expect(call("POST", "/devices/"+activeDevice["id"].(string)+"/pairing-token", nil), 201)["data"].(map[string]any)
	pairBody := map[string]any{"pairing_token": pairing["pairing_token"], "session_id": sessionID}

	restrictedRole := expect(call("POST", "/roles", map[string]any{"name": "Scoped device operator", "scope": "assigned_classes", "permissions": []string{"devices.health"}}), 201)["data"].(map[string]any)
	restrictedUser := expect(call("POST", "/users", map[string]any{"email": "restricted-device@example.test", "full_name": "Restricted device", "role": restrictedRole["key"], "password": password}), 201)["data"].(map[string]any)
	login("restricted-device@example.test")
	if data := expect(call("GET", "/kiosk/session-options", nil), 200)["data"].([]any); len(data) != 0 {
		t.Errorf("unassigned operator saw %d sessions", len(data))
	}
	expect(call("POST", "/kiosk/session", pairBody), 404)
	login("deviceonly@example.test")
	pairResponse := call("POST", "/kiosk/session", pairBody)
	paired := expect(pairResponse, 201)["data"].(map[string]any)
	expect(call("GET", "/auth/me", nil), 401)
	login("deviceonly@example.test")
	if paired["device_name"] != "Gate 2" || paired["session_name"] == nil {
		t.Fatal("pairing labels missing", paired)
	}
	login("deviceonly@example.test")
	options := expect(call("GET", "/kiosk/session-options", nil), 200)["data"].([]any)
	if len(options) != 1 || options[0].(map[string]any)["id"] != sessionID {
		t.Fatal("open options wrong", options)
	}
	login("owner@example.test")
	deviceCookie := pairResponse.Result().Cookies()[0]
	deviceCall := func(path string, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1"+path, nil)
		r.Header.Set("Origin", "http://localhost:3000")
		r.Header.Set("X-CSRF-Token", token)
		r.AddCookie(deviceCookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	expect(call("POST", "/kiosk/session", pairBody), 401)
	expires, err := time.Parse(time.RFC3339Nano, paired["expires_at"].(string))
	if err != nil || time.Until(expires) < 29*24*time.Hour || deviceCookie.MaxAge < 29*24*3600 {
		t.Fatal("device must remain paired between school days", expires, deviceCookie.MaxAge)
	}
	if _, err := pool.Exec(ctx, `UPDATE attendance_sessions SET status='closed',closed_at=clock_timestamp() WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	var otherSession, otherSchedule string
	if err := pool.QueryRow(ctx, `INSERT INTO attendance_schedules(school_id,class_id,name,timezone,weekdays,checkin_start,checkin_cutoff,close_time,is_active) SELECT school_id,$2,'Other class',timezone,weekdays,checkin_start,checkin_cutoff,close_time,false FROM attendance_schedules WHERE id=(SELECT schedule_id FROM attendance_sessions WHERE id=$1) RETURNING id::text`, sessionID, class2["id"]).Scan(&otherSchedule); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO attendance_sessions(school_id,schedule_id,class_id,date,name,timezone,starts_at,late_after,closes_at,status,opened_at) SELECT school_id,$2,$3,date,'Other class session',timezone,starts_at,late_after,closes_at,'open',clock_timestamp() FROM attendance_sessions WHERE id=$1 RETURNING id::text`, sessionID, otherSchedule, class2["id"]).Scan(&otherSession); err != nil {
		t.Fatal(err)
	}
	waiting := expect(deviceCall("/kiosk/heartbeat", paired["csrf_token"].(string)), 200)["data"].(map[string]any)
	if waiting["session_ready"] != false || waiting["device_name"] != "Gate 2" {
		t.Fatal("closed session must preserve device connection", waiting)
	}
	expect(deviceCall("/kiosk/capture-challenge", paired["csrf_token"].(string)), 409)
	if _, err := pool.Exec(ctx, `DELETE FROM attendance_sessions WHERE id=$1`, otherSession); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM attendance_schedules WHERE id=$1`, otherSchedule); err != nil {
		t.Fatal(err)
	}
	// Heartbeat refreshes inactivity expiry without needing the operator login again.
	if _, err := pool.Exec(ctx, `UPDATE device_sessions SET expires_at=clock_timestamp()+interval '10 minutes' WHERE device_id=$1`, activeDevice["id"]); err != nil {
		t.Fatal(err)
	}
	renewal := deviceCall("/kiosk/heartbeat", paired["csrf_token"].(string))
	expect(renewal, 200)
	if renewal.Result().Cookies()[0].MaxAge < 29*24*3600 {
		t.Fatal("device cookie was not renewed")
	}
	var nextSession string
	if err := pool.QueryRow(ctx, `INSERT INTO attendance_sessions(school_id,schedule_id,class_id,date,name,timezone,starts_at,late_after,closes_at,status,opened_at) SELECT school_id,schedule_id,class_id,date+1,'Next school day',timezone,starts_at,late_after,closes_at,'open',clock_timestamp() FROM attendance_sessions WHERE id=$1 RETURNING id::text`, sessionID).Scan(&nextSession); err != nil {
		t.Fatal(err)
	}
	next := expect(deviceCall("/kiosk/heartbeat", paired["csrf_token"].(string)), 200)["data"].(map[string]any)
	if next["session_id"] != nextSession || next["session_ready"] != true {
		t.Fatal("paired device did not follow next class session", next)
	}
	// Two open sessions must pause attendance, never choose an arbitrary roster.
	if _, err := pool.Exec(ctx, `UPDATE attendance_sessions SET status='open',closed_at=NULL WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	ambiguous := expect(deviceCall("/kiosk/heartbeat", paired["csrf_token"].(string)), 200)["data"].(map[string]any)
	if ambiguous["connection_status"] != "multiple_sessions" || ambiguous["session_ready"] != false {
		t.Fatal("ambiguous sessions must wait", ambiguous)
	}
	expect(deviceCall("/kiosk/capture-challenge", paired["csrf_token"].(string)), 409)
	if _, err := pool.Exec(ctx, `UPDATE device_sessions SET session_id=$1 WHERE session_id=$2`, sessionID, nextSession); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM attendance_sessions WHERE id=$1`, nextSession); err != nil {
		t.Fatal(err)
	}
	expect(deviceCall("/kiosk/heartbeat", "wrong"), 403)
	expect(deviceCall("/kiosk/heartbeat", paired["csrf_token"].(string)), 200)
	expect(deviceCall("/kiosk/recognitions", paired["csrf_token"].(string)), 400)
	login("owner@example.test")
	roleList := expect(call("GET", "/roles", nil), 200)["data"].([]any)
	var deviceRoleID string
	for _, item := range roleList {
		row := item.(map[string]any)
		if row["key"] == "operator" {
			deviceRoleID = row["id"].(string)
		}
	}
	expect(call("PATCH", "/roles/"+deviceRoleID, map[string]any{"permissions": []string{}}), 200)
	expect(deviceCall("/kiosk/heartbeat", paired["csrf_token"].(string)), 401)
	expect(call("PATCH", "/roles/"+deviceRoleID, map[string]any{"permissions": []string{"devices.health"}}), 200)
	expect(deviceCall("/kiosk/heartbeat", paired["csrf_token"].(string)), 401)

	var leaked int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE metadata::text LIKE '%PRIVATE-%' OR metadata::text LIKE '%'||$1||'%'`, pairing["pairing_token"]).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("sensitive audit leak %d %v", leaked, err)
	}

	restrictedAssignment := expect(call("POST", "/class-assignments", map[string]any{"user_id": restrictedUser["id"], "class_id": class["id"]}), 201)["data"].(map[string]any)
	for _, scenario := range []string{"assignment_gap", "assignment_expiry", "assignment_revoke", "role_change", "account_disabled"} {
		var grantEnds time.Time
		if scenario == "assignment_gap" {
			if err := pool.QueryRow(ctx, `UPDATE user_class_assignments SET ends_at=clock_timestamp()+interval '1 second' WHERE id=$1 RETURNING ends_at`, restrictedAssignment["id"]).Scan(&grantEnds); err != nil {
				t.Fatal(err)
			}
		}
		pairing := expect(call("POST", "/devices/"+activeDevice["id"].(string)+"/pairing-token", nil), 201)["data"].(map[string]any)
		login("restricted-device@example.test")
		response := call("POST", "/kiosk/session", map[string]any{"pairing_token": pairing["pairing_token"], "session_id": sessionID})
		data := expect(response, 201)["data"].(map[string]any)
		scopedCookie := response.Result().Cookies()[0]
		heartbeatScoped := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest("POST", "/api/v1/kiosk/heartbeat", nil)
			req.Header.Set("Origin", "http://localhost:3000")
			req.Header.Set("X-CSRF-Token", data["csrf_token"].(string))
			req.AddCookie(scopedCookie)
			result := httptest.NewRecorder()
			h.ServeHTTP(result, req)
			return result
		}
		expect(heartbeatScoped(), 200)
		if scenario == "assignment_gap" {
			expiry, err := time.Parse(time.RFC3339Nano, data["expires_at"].(string))
			if err != nil || expiry.After(grantEnds) {
				t.Fatalf("device credential outlives its class grant: %v %v", expiry, err)
			}
			time.Sleep(time.Until(grantEnds.Add(10 * time.Millisecond)))
		}
		login("owner@example.test")
		switch scenario {
		case "assignment_expiry":
			if _, err := pool.Exec(ctx, `UPDATE user_class_assignments SET starts_at=clock_timestamp()-interval '2 hours',ends_at=clock_timestamp()-interval '1 hour' WHERE id=$1`, restrictedAssignment["id"]); err != nil {
				t.Fatal(err)
			}
		case "assignment_revoke":
			expect(call("PATCH", "/class-assignments/"+restrictedAssignment["id"].(string), map[string]any{"is_active": false}), 200)
		case "role_change":
			expect(call("PATCH", "/users/"+restrictedUser["id"].(string), map[string]any{"role": "teacher"}), 200)
		case "account_disabled":
			expect(call("PATCH", "/users/"+restrictedUser["id"].(string), map[string]any{"is_active": false}), 200)
		}
		if scenario != "assignment_gap" {
			expect(heartbeatScoped(), 401)
		}
		// Restore only fixture authority; an invalidated device credential must stay invalid.
		expect(call("PATCH", "/users/"+restrictedUser["id"].(string), map[string]any{"role": restrictedRole["key"], "is_active": true}), 200)
		if _, err := pool.Exec(ctx, `UPDATE user_class_assignments SET starts_at=clock_timestamp()-interval '1 hour',ends_at=clock_timestamp()+interval '1 hour',is_active=true,revoked_at=NULL WHERE id=$1`, restrictedAssignment["id"]); err != nil {
			t.Fatal(err)
		}
		expect(heartbeatScoped(), 401)
	}
	expect(call("POST", "/devices/"+activeDevice["id"].(string)+"/revoke", map[string]string{"reason": "Compromised"}), 200)
	expect(deviceCall("/kiosk/heartbeat", paired["csrf_token"].(string)), 401)
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
	beforeReport := expect(call("GET", "/reports/attendance?date="+day, nil), 200)
	if beforeReport["totals"].(map[string]any)["not_recorded"] != float64(3) {
		t.Fatal("report made early absence", beforeReport)
	}
	beforeDashboard := expect(call("GET", "/dashboard/summary?date="+day, nil), 200)["data"].(map[string]any)
	if len(beforeDashboard["trend"].([]any)) != 7 || beforeDashboard["totals"].(map[string]any)["attended"] != float64(0) {
		t.Fatal("initial dashboard invalid", beforeDashboard)
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

	lateSummary := expect(call("GET", "/dashboard/summary?date="+day, nil), 200)["data"].(map[string]any)["totals"].(map[string]any)
	if lateSummary["late"] != float64(1) || lateSummary["attended"] != float64(1) || lateSummary["present"] != float64(0) || lateSummary["roster_total"] != float64(3) || lateSummary["attendance_rate"] != 33.3 {
		t.Fatal("late double-counted", lateSummary)
	}
	manual2 := map[string]any{"event_id": "44444444-4444-4444-8444-444444444444", "session_id": sessionID, "student_id": student2["id"], "status": "sick", "reason": "Verified sickness"}
	expect(call("POST", "/attendances/manual", manual2), 201)
	login("teacher@example.test")
	expect(call("GET", "/students/"+student["id"].(string)+"/guardians", nil), 404)
	expect(call("GET", "/students/"+student["id"].(string)+"/consents", nil), 404)
	expect(call("POST", "/devices", map[string]string{"name": "Denied", "location": "Denied"}), 403)
	expect(call("POST", "/kiosk/session", pairBody), 403)

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
	approverRole := expect(call("POST", "/roles", map[string]any{"name": "Attendance approver", "scope": "assigned_classes", "permissions": []string{"attendance.approve", "attendance.read"}}), 201)["data"].(map[string]any)
	approver := expect(call("POST", "/users", map[string]any{"email": "approver@example.test", "full_name": "Approver", "role": approverRole["key"], "password": password}), 201)["data"].(map[string]any)
	expect(call("POST", "/class-assignments", map[string]any{"user_id": approver["id"], "class_id": class["id"]}), 201)
	login("approver@example.test")
	expect(call("GET", "/classes", nil), 200)
	expect(call("GET", "/academic-years", nil), 200)
	expect(call("POST", "/classes", map[string]any{"name": "Not allowed"}), 403)
	expect(call("GET", "/attendance-corrections/"+own["id"].(string), nil), 200)
	if pending := expect(call("GET", "/attendance-corrections?status=pending", nil), 200)["data"].([]any); len(pending) == 0 {
		t.Fatal("class approver cannot see another request")
	}
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
	login("owner@example.test")
	expect(call("POST", "/attendances/manual", map[string]any{"event_id": "55555555-5555-4555-8555-555555555555", "session_id": sessionID, "student_id": student3["id"], "status": "present", "reason": "Closed"}), 409)
	login("owner@example.test")
	expect(call("PATCH", "/class-assignments/"+assignment["id"].(string), map[string]any{"is_active": false}), 200)
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
	fullReport := expect(call("GET", "/reports/attendance?date="+day+"&page_size=1", nil), 200)
	if len(fullReport["data"].([]any)) != 1 || fullReport["pagination"].(map[string]any)["total"] != float64(3) || fullReport["totals"].(map[string]any)["roster_total"] != float64(3) {
		t.Fatal("report totals depended on page", fullReport)
	}
	daily, ok := fullReport["daily"].([]any)
	if !ok || len(daily) != 1 || daily[0].(map[string]any)["roster_total"] != float64(3) || daily[0].(map[string]any)["date"] != day {
		t.Fatal("daily report must aggregate all matching rows independent of pagination", fullReport)
	}
	reportEnd, _ := time.Parse("2006-01-02", day)
	week := expect(call("GET", "/reports/attendance?from="+reportEnd.AddDate(0, 0, -6).Format("2006-01-02")+"&to="+day, nil), 200)["daily"].([]any)
	if len(week) != 7 || week[0].(map[string]any)["roster_total"] != float64(0) || week[0].(map[string]any)["absent"] != float64(0) {
		t.Fatal("daily report must include empty days without inventing absence", week)
	}
	perStudent := expect(call("GET", "/reports/attendance?date="+day+"&student_id="+student["id"].(string), nil), 200)
	if perStudent["totals"].(map[string]any)["roster_total"] != float64(1) || perStudent["daily"].([]any)[0].(map[string]any)["roster_total"] != float64(1) {
		t.Fatal("student filter drift", perStudent)
	}
	filtered := expect(call("GET", "/reports/attendance?date="+day+"&status=sick", nil), 200)
	if filtered["totals"].(map[string]any)["roster_total"] != float64(1) || filtered["data"].([]any)[0].(map[string]any)["student_id"] != student2["id"] {
		t.Fatal("status filter drift", filtered)
	}
	expect(call("GET", "/reports/attendance?from=2026-01-01&to=2027-02-01", nil), 400)
	expect(call("GET", "/reports/attendance?from=2026-02-02&to=2026-02-01", nil), 400)
	expect(call("GET", "/reports/attendance?class_id=invalid", nil), 400)
	expect(call("GET", "/reports/attendance?date="+day+"&from="+day+"&to="+day, nil), 400)
	if _, err := pool.Exec(ctx, `INSERT INTO user_class_assignments(school_id,user_id,class_id,academic_year_id,starts_at,ends_at) SELECT $1,$2,$3,id,clock_timestamp(),(ends_on+1)::timestamp AT TIME ZONE timezone FROM academic_years WHERE school_id=$1 AND id=$4`, schoolID, teacher["id"], class["id"], year["id"]); err != nil {
		t.Fatal(err)
	}
	login("teacher@example.test")
	scoped := expect(call("GET", "/reports/attendance?date="+day+"&class_id="+class["id"].(string), nil), 200)
	if len(scoped["data"].([]any)) != 3 {
		t.Fatal("historical scope lost after transfer", scoped)
	}
	exported := call("GET", "/reports/attendance?date="+day+"&format=csv&page_size=1", nil)
	if exported.Code != 200 || !strings.HasPrefix(exported.Header().Get("Content-Type"), "text/csv") || !strings.Contains(exported.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("CSV failed", exported.Code, exported.Body.String())
	}
	csvRows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(exported.Body.String(), "\ufeff"))).ReadAll()
	if err != nil || len(csvRows) != 4 {
		t.Fatal("CSV was paginated", csvRows, err)
	}
	formulaEscaped := false
	for _, row := range csvRows[1:] {
		if row[5] == "'=SUM(1,2)" {
			formulaEscaped = true
		}
	}
	if !strings.Contains(exported.Body.String(), `"Student;=1+2"`) {
		t.Fatal("semicolon field not quoted for spreadsheet locales")
	}
	if !formulaEscaped {
		t.Fatal("formula-like CSV value not escaped", csvRows)
	}
	var exportedRows int
	if err := pool.QueryRow(ctx, `SELECT (metadata->>'row_count')::int FROM audit_logs WHERE action='reports.attendance.exported' ORDER BY id DESC LIMIT 1`).Scan(&exportedRows); err != nil || exportedRows != 3 {
		t.Fatal("CSV audit not committed", exportedRows, err)
	}
	for _, path := range []string{"/reports/attendance?class_id=" + class2["id"].(string), "/dashboard/summary?class_id=" + class2["id"].(string)} {
		result := expect(call("GET", path, nil), 200)
		if path[1] == 'r' {
			if len(result["data"].([]any)) != 0 {
				t.Fatal("teacher saw other class")
			}
		} else if result["data"].(map[string]any)["totals"].(map[string]any)["roster_total"] != float64(0) {
			t.Fatal("teacher dashboard leak")
		}
	}
	login("operator@example.test")
	expect(call("GET", "/reports/attendance", nil), 403)
	expect(call("GET", "/reports/attendance?format=csv", nil), 403)
	login("owner@example.test")
	auditExport := expect(call("GET", "/audit-logs?action=reports.attendance.exported&actor_id="+teacher["id"].(string)+"&from="+day+"&to="+day, nil), 200)["data"].([]any)
	if len(auditExport) != 1 || auditExport[0].(map[string]any)["actor_name"] != "Teacher" {
		t.Fatal("audit filters drift", auditExport)
	}
	expect(call("GET", "/audit-logs?actor_id=bad", nil), 400)
	var reassignment string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM user_class_assignments WHERE school_id=$1 AND user_id=$2 AND is_active`, schoolID, teacher["id"]).Scan(&reassignment); err != nil {
		t.Fatal(err)
	}
	expect(call("PATCH", "/class-assignments/"+reassignment, map[string]any{"is_active": false}), 200)
	login("teacher@example.test")
	revokedReport := expect(call("GET", "/reports/attendance?date="+day, nil), 200)
	if len(revokedReport["data"].([]any)) != 0 || revokedReport["daily"].([]any)[0].(map[string]any)["roster_total"] != float64(0) {
		t.Fatal("report survived assignment revocation")
	}
	revokedCSV := call("GET", "/reports/attendance?format=csv&date="+day, nil)
	revokedRows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(revokedCSV.Body.String(), "\ufeff"))).ReadAll()
	if revokedCSV.Code != 200 || err != nil || len(revokedRows) != 1 {
		t.Fatal("CSV leaked after revocation", revokedCSV.Code, err)
	}
	login("owner@example.test")
	// Existing session eligibility is frozen; editing the calendar must not silently cancel it.
	schoolDay, _ := time.Parse("2006-01-02", day)
	tomorrow := schoolDay.AddDate(0, 0, 1).Format("2006-01-02")
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
	delete(scheduleBody, "checkout_start")
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
	expect(call("PATCH", "/roles/"+operatorRole["id"].(string), map[string]any{"permissions": []string{}}), 200)
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

	expect(call("GET", "/reports/attendance?class_id=bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", nil), 200)
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
	// 10k cap applies before CSV headers/audit, without truncation or a partial file.
	_, err = pool.Exec(ctx, `WITH st AS (
 INSERT INTO students(school_id,nis,full_name) SELECT $1,'CAP-'||g,'Cap fixture '||g FROM generate_series(1,335) g RETURNING school_id,id,nis,full_name
 ), e AS (
 INSERT INTO student_enrollments(school_id,student_id,class_id,academic_year_id,joined_at,left_at,reason) SELECT st.school_id,st.id,$2,y.id,y.starts_on::timestamp AT TIME ZONE y.timezone,(y.ends_on+1)::timestamp AT TIME ZONE y.timezone,'Cap fixture' FROM st JOIN academic_years y ON y.school_id=st.school_id AND y.id=$3 RETURNING school_id,id,student_id,class_id
 ), sched AS (
 INSERT INTO attendance_schedules(school_id,class_id,name,timezone,weekdays,checkin_start,checkin_cutoff,close_time,is_active) SELECT $1,$2,'Cap '||g,'Asia/Jakarta',ARRAY[1,2,3,4,5,6,7],'00:00','00:01','00:03',false FROM generate_series(1,31) g RETURNING school_id,id,class_id
 ), se AS (
 INSERT INTO attendance_sessions(school_id,schedule_id,class_id,date,name,timezone,starts_at,late_after,closes_at,status,opened_at,closed_at) SELECT school_id,id,class_id,$4::date,'Cap','Asia/Jakarta',$4::date::timestamp AT TIME ZONE 'Asia/Jakarta',($4::date+time '00:01') AT TIME ZONE 'Asia/Jakarta',($4::date+time '00:03') AT TIME ZONE 'Asia/Jakarta','closed',clock_timestamp(),clock_timestamp() FROM sched RETURNING school_id,id,class_id
 ) INSERT INTO attendance_session_participants(school_id,session_id,student_id,class_id,enrollment_id,nis,full_name) SELECT se.school_id,se.id,e.student_id,se.class_id,e.id,st.nis,st.full_name FROM se CROSS JOIN e JOIN st ON st.id=e.student_id`, schoolID, class2["id"], year["id"], day)
	if err != nil {
		t.Fatal(err)
	}
	var auditBefore, auditAfter int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='reports.attendance.exported'`).Scan(&auditBefore); err != nil {
		t.Fatal(err)
	}
	capped := call("GET", "/reports/attendance?format=csv&date="+day+"&class_id="+class2["id"].(string), nil)
	capError := expect(capped, 400)
	if capError["error"].(map[string]any)["code"] != "EXPORT_LIMIT_EXCEEDED" || capped.Header().Get("Content-Disposition") != "" {
		t.Fatal("partial CSV on cap", capError)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='reports.attendance.exported'`).Scan(&auditAfter); err != nil || auditBefore != auditAfter {
		t.Fatal("failed CSV audited as successful", auditBefore, auditAfter, err)
	}

	login("owner@example.test")
	var outSession string
	err = pool.QueryRow(ctx, `INSERT INTO attendance_sessions(school_id,schedule_id,class_id,date,name,timezone,starts_at,late_after,closes_at,status,opened_at) SELECT school_id,schedule_id,class_id,date+200,'Checkout fixture',timezone,clock_timestamp()-interval '1 hour',clock_timestamp()+interval '1 minute',clock_timestamp()+interval '1 hour','open',clock_timestamp() FROM attendance_sessions WHERE id=$1 RETURNING id::text`, sessionID).Scan(&outSession)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO attendance_session_participants(school_id,session_id,student_id,class_id,enrollment_id,nis,full_name) SELECT school_id,$2,student_id,class_id,enrollment_id,nis,full_name FROM attendance_session_participants WHERE session_id=$1 AND student_id=$3`, sessionID, outSession, student2["id"])
	if err != nil {
		t.Fatal(err)
	}
	outBody := map[string]any{"event_id": "99999999-9999-4999-8999-999999999999", "session_id": outSession, "student_id": student2["id"], "reason": "Verified departure"}
	expect(call("POST", "/attendances/check-out", outBody), 409)
	expect(call("POST", "/attendances/manual", map[string]any{"event_id": "88888888-8888-4888-8888-888888888888", "session_id": outSession, "student_id": student2["id"], "status": "present", "reason": "Checkin"}), 201)
	out := expect(call("POST", "/attendances/check-out", outBody), 201)["data"].(map[string]any)
	if out["record"].(map[string]any)["check_out_at"] == nil {
		t.Fatal(out)
	}
	expect(call("POST", "/attendances/check-out", outBody), 201)
	outBody["reason"] = "Different reason"
	expect(call("POST", "/attendances/check-out", outBody), 409)
	var outs int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM attendance_events e JOIN attendance_records ar ON ar.id=e.record_id WHERE ar.session_id=$1 AND e.event_type='check_out'`, outSession).Scan(&outs); err != nil || outs != 1 {
		t.Fatal("duplicate checkout", outs, err)
	}

	exportRole := expect(call("POST", "/roles", map[string]any{"name": "Reports only", "scope": "assigned_classes", "permissions": []string{"reports.export"}}), 201)["data"].(map[string]any)
	exporter := expect(call("POST", "/users", map[string]any{"email": "reports@example.test", "full_name": "Reports only", "role": exportRole["key"], "password": password}), 201)["data"].(map[string]any)
	expect(call("POST", "/class-assignments", map[string]any{"user_id": exporter["id"], "class_id": class["id"]}), 201)
	login("reports@example.test")
	expect(call("GET", "/reports/attendance?date="+day, nil), 200)
	if csv := call("GET", "/reports/attendance?format=csv&date="+day, nil); csv.Code != 200 {
		t.Fatal("report-only role cannot export", csv.Code, csv.Body.String())
	}
	expect(call("GET", "/classes", nil), 200)
	expect(call("GET", "/academic-years", nil), 200)
	expect(call("GET", "/attendance-sessions", nil), 403)
	login("owner@example.test")
	wsServer := httptest.NewServer(h)
	defer wsServer.Close()
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http") + "/api/v1/ws/attendance?session_id=" + outSession
	conn, res, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Origin": {"http://localhost:3000"}, "Cookie": {cookie.String()}})
	if err != nil {
		t.Fatalf("websocket: %v status=%v", err, res)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var notification map[string]any
	if err = conn.ReadJSON(&notification); err != nil || notification["event"] != "attendance.changed" {
		t.Fatalf("initial refetch: %v %v", notification, err)
	}
	if strings.Contains(fmt.Sprint(notification), "Imported Student") {
		t.Fatal("PII in notification")
	}
	_, res, err = websocket.DefaultDialer.Dial(wsURL, http.Header{"Origin": {"https://wrong.example"}, "Cookie": {cookie.String()}})
	if err == nil || res == nil || res.StatusCode != 403 {
		t.Fatalf("bad origin websocket: %v %v", err, res)
	}
	_, err = pool.Exec(ctx, `UPDATE attendance_sessions SET starts_at=clock_timestamp()-interval '1 hour',late_after=clock_timestamp()-interval '30 minutes',closes_at=clock_timestamp()-interval '1 minute',status='closed',closed_at=clock_timestamp() WHERE id=$1`, outSession)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err = conn.ReadJSON(&notification); err != nil {
		t.Fatal("closed notification missing", err)
	}
	expect(call("POST", "/attendance-sessions/"+outSession+"/reconcile", map[string]string{"reason": "All present verified"}), 200)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err = conn.ReadJSON(&notification); err != nil {
		t.Fatal("reconciliation notification missing", err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM user_sessions WHERE school_id=$1`, schoolID); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err = conn.ReadJSON(&notification); err == nil {
		t.Fatal("websocket survived session revocation")
	}

}
