package masterdata_test

import (
	"attendance.local/api/internal/attendance"
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/masterdata"
	"attendance.local/api/internal/platform/aiclient"
	"attendance.local/api/internal/platform/httpserver"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBiometricEnrollmentConsentRevocationAndDevicePrivacy(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run real database integration")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "biometric_test_" + strings.ToLower(rand.Text()[:12])
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
	aiCalls := 0
	previewCalls := 0
	previewStatus := 200
	previewResponse := `{"faces":[{"box":{"x":0.1,"y":0.2,"width":0.3,"height":0.4},"student_id":"PRIVATE-ID","label":"PRIVATE-CANDIDATE","embedding":[1]}],"face_count":1,"quality":"ready","message":"PRIVATE-CANDIDATE","similarity":0.99}`
	var previewEntered, previewRelease chan struct{}
	galleryCount := -1
	decision := "unknown"
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 48) {
			t.Error("missing service authorization")
		}
		if r.URL.Path == "/internal/v1/status" {
			_ = json.NewEncoder(w).Encode(map[string]any{"pipeline_ready": true, "recognition_enabled": false, "block_reason": "MODEL_LIVENESS_NOT_EVALUATED", "evaluation": map[string]any{"status": "not_evaluated"}})
			return
		}
		if r.URL.Path == "/internal/v1/enroll" {
			aiCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"ciphertext": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, 528)), "nonce": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{43}, 12)), "key_id": "synthetic-test", "model_sha256": strings.Repeat("a", 64), "frame_count": 3, "status": "provisional"})
			return
		}
		if r.URL.Path == "/internal/v1/face-preview" {
			previewCalls++
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input) != 1 || input["image"] == "" {
				t.Error("preview sent unexpected fields", input, err)
			}
			if previewEntered != nil {
				close(previewEntered)
				<-previewRelease
			}
			w.WriteHeader(previewStatus)
			_, _ = w.Write([]byte(previewResponse))
			return
		}
		if r.URL.Path == "/internal/v1/recognize" {
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			galleryCount = len(request["gallery"].([]any))
			_ = json.NewEncoder(w).Encode(map[string]any{"decision": decision, "recognition_enabled": false, "event_id": request["event_id"], "gallery_revision": request["gallery_revision"], "model_sha256": strings.Repeat("a", 64), "faces": []any{map[string]any{"box": map[string]float64{"x": 0.1, "y": 0.1, "width": 0.5, "height": 0.5}, "student_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "label": "PRIVATE-CANDIDATE"}}})
			return
		}
		http.Error(w, "not found", 404)
	}))
	defer aiServer.Close()
	ai, err := aiclient.New(aiServer.URL, strings.Repeat("s", 48))
	if err != nil {
		t.Fatal(err)
	}
	master := masterdata.New(pool, schoolID, a)
	master.SetAI(ai)
	attendanceService := attendance.New(pool, schoolID, a)
	attendanceService.SetAI(ai)
	h := httpserver.NewHandler(pool.Ping, "http://localhost:3000", func(m *http.ServeMux) { a.Register(m); master.Register(m); attendanceService.Register(m) })
	cookie := (*http.Cookie)(nil)
	csrf := ""
	origin := "http://localhost:3000"
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(payload))
		r.Header.Set("Origin", origin)
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
	year := expect(call("POST", "/academic-years", map[string]any{"name": "Biometric test", "starts_on": now.AddDate(0, -1, 0).Format("2006-01-02"), "ends_on": now.AddDate(0, 1, 0).Format("2006-01-02"), "is_active": true}), 201)["data"].(map[string]any)
	class := expect(call("POST", "/classes", map[string]any{"academic_year_id": year["id"], "code": "BIO", "name": "BIO"}), 201)["data"].(map[string]any)
	student := expect(call("POST", "/students", map[string]any{"nis": "BIO-001", "full_name": "Synthetic metadata student", "class_id": class["id"]}), 201)["data"].(map[string]any)
	id := student["id"].(string)
	frames := []string{}
	for _, shade := range []uint8{32, 64, 96} {
		pixels := image.NewRGBA(image.Rect(0, 0, 240, 240))
		for y := 0; y < 240; y++ {
			for x := 0; x < 240; x++ {
				pixels.Set(x, y, color.RGBA{shade, shade, shade, 255})
			}
		}
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, pixels, nil); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, base64.StdEncoding.EncodeToString(encoded.Bytes()))
	}
	enroll := map[string]any{"frames": frames}
	preview := map[string]string{"image": frames[0]}
	previewPath := "/students/" + id + "/face-preview"
	expect(call("POST", previewPath, preview), 410)
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 410)
	if aiCalls != 0 || previewCalls != 0 {
		t.Fatal("AI called without consent")
	}
	guardian := expect(call("POST", "/students/"+id+"/guardians", map[string]any{"full_name": "Synthetic guardian", "relationship": "parent", "verification_ref": "SYNTHETIC-TEST"}), 201)["data"].(map[string]any)
	newConsent := func() string {
		return expect(call("POST", "/students/"+id+"/consents", map[string]any{"guardian_id": guardian["id"], "policy_version": "synthetic-test", "evidence_ref": "SYNTHETIC-TEST", "valid_until": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}), 201)["data"].(map[string]any)["id"].(string)
	}
	consent := newConsent()
	enrolledOfficer := expect(call("POST", "/users", map[string]any{"email": "enroller@example.test", "full_name": "Synthetic enroller", "role": "operator", "password": password, "grants": []string{"biometrics.enroll"}}), 201)["data"].(map[string]any)
	expect(call("POST", "/users", map[string]any{"email": "revoker@example.test", "full_name": "Synthetic revoker", "role": "admin", "password": password, "grants": []string{"biometrics.revoke"}}), 201)
	expect(call("POST", "/users", map[string]any{"email": "unauthorized@example.test", "full_name": "Synthetic unauthorized", "role": "operator", "password": password}), 201)
	login("enroller@example.test")
	previewData := expect(call("POST", previewPath, preview), 200)["data"].(map[string]any)
	if previewData["quality"] != "ready" || previewData["face_count"] != float64(1) || strings.Contains(stringMustJSON(previewData), "PRIVATE") || strings.Contains(stringMustJSON(previewData), "embedding") || strings.Contains(stringMustJSON(previewData), "similarity") {
		t.Fatal("preview leaked private metadata or wrong quality", previewData)
	}
	selectRows := expect(call("GET", "/biometrics/students?search=bio-001", nil), 200)
	selected := selectRows["data"].([]any)
	if len(selected) != 1 {
		t.Fatal("enroller cannot select current school student", selected)
	}
	minimal := selected[0].(map[string]any)
	if len(minimal) != 5 || minimal["id"] != id || minimal["full_name"] != student["full_name"] || minimal["nis"] != "BIO-001" || minimal["status"] != "active" || minimal["class_name"] != "BIO" {
		t.Fatal("wrong biometric selector fields", minimal)
	}
	pagination := selectRows["pagination"].(map[string]any)
	if pagination["page_size"] != float64(20) || pagination["total"] != float64(1) {
		t.Fatal("wrong default pagination", pagination)
	}
	expect(call("GET", "/biometrics/students?search="+strings.Repeat("x", 101), nil), 400)
	expect(call("GET", "/biometrics/students?page_size=101", nil), 400)
	if list := expect(call("GET", "/biometrics/students?search=nonexistent", nil), 200)["data"].([]any); len(list) != 0 {
		t.Fatal("search ignored")
	}
	expect(call("GET", "/students/"+id+"/face-status", nil), 200)
	expect(call("GET", "/students", nil), 403)
	expect(call("GET", "/classes", nil), 403)
	expect(call("POST", "/students/"+id+"/face-revoke", map[string]string{"reason": "must deny"}), 403)
	login("unauthorized@example.test")
	expect(call("POST", previewPath, preview), 403)
	expect(call("GET", "/biometrics/students", nil), 403)
	expect(call("GET", "/students/"+id+"/face-status", nil), 403)
	login("revoker@example.test")
	expect(call("POST", previewPath, preview), 403)
	expect(call("GET", "/biometrics/students", nil), 200)
	expect(call("GET", "/students/"+id+"/face-status", nil), 200)
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 403)
	login("owner@example.test")
	expect(call("POST", "/users", map[string]any{"email": "teacher@example.test", "full_name": "Synthetic teacher", "role": "teacher", "password": password}), 201)
	login("teacher@example.test")
	expect(call("POST", previewPath, preview), 403)
	expect(call("GET", "/biometrics/students", nil), 403)
	expect(call("GET", "/students/"+id+"/face-status", nil), 403)
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 403)
	expect(call("POST", "/students/"+id+"/face-revoke", map[string]string{"reason": "test"}), 403)
	login("owner@example.test")
	saved := csrf
	csrf = "wrong"
	expect(call("POST", previewPath, preview), 403)
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 403)
	csrf = saved
	for _, value := range []string{"", "http://untrusted.test", "http://localhost:3000/"} {
		origin = value
		expect(call("POST", previewPath, preview), 403)
	}
	origin = "http://localhost:3000"
	expect(call("GET", previewPath, nil), 405)
	expect(call("POST", previewPath, map[string]any{"image": frames[0], "student_id": id}), 400)
	expect(call("POST", previewPath, map[string]any{"image": strings.Repeat("a", 512<<10)}), 400)
	for _, value := range []string{"", "not-jpeg", "data:image/jpeg;base64," + frames[0]} {
		expect(call("POST", previewPath, map[string]string{"image": value}), 422)
	}
	if previewCalls != 1 {
		t.Fatal("rejected preview reached AI", previewCalls)
	}
	validPreviewResponse := previewResponse
	for _, value := range []string{
		`{"faces":[],"face_count":1,"quality":"ready"}`,
		`{"faces":[{"box":{"x":0.9,"y":0,"width":0.5,"height":0.5}}],"face_count":1,"quality":"ready"}`,
		`{"faces":[],"face_count":0,"quality":"unrecognized"}`,
	} {
		previewResponse = value
		expect(call("POST", previewPath, preview), 503)
	}
	previewResponse = validPreviewResponse
	previewStatus = 422
	previewResponse = `{"error":{"code":"PRIVATE-ID","message":"PRIVATE-CANDIDATE"}}`
	if result := expect(call("POST", previewPath, preview), 422); strings.Contains(stringMustJSON(result), "PRIVATE") {
		t.Fatal("preview validation error leaked AI metadata", result)
	}
	previewStatus, previewResponse = 200, validPreviewResponse
	master.SetAI(nil)
	expect(call("POST", previewPath, preview), 503)
	master.SetAI(ai)
	// Revocation waits behind the authorized read snapshot, then the next preview is refused.
	previewEntered, previewRelease = make(chan struct{}), make(chan struct{})
	previewDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { previewDone <- call("POST", previewPath, preview) }()
	select {
	case <-previewEntered:
	case <-time.After(3 * time.Second):
		close(previewRelease)
		t.Fatal("preview did not reach AI")
	}
	withdrawDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		withdrawDone <- call("POST", "/consents/"+consent+"/withdraw", map[string]string{"reason": "Synthetic preview revocation"})
	}()
	select {
	case <-withdrawDone:
		close(previewRelease)
		t.Fatal("revocation bypassed authorized read lock")
	case <-time.After(50 * time.Millisecond):
	}
	close(previewRelease)
	select {
	case w := <-previewDone:
		expect(w, 200)
	case <-time.After(3 * time.Second):
		t.Fatal("preview blocked")
	}
	select {
	case w := <-withdrawDone:
		expect(w, 200)
	case <-time.After(3 * time.Second):
		t.Fatal("revocation blocked")
	}
	previewEntered, previewRelease = nil, nil
	expect(call("POST", previewPath, preview), 410)
	consent = newConsent()
	// A clock-based expiry during inference must also suppress the preview result.
	if _, err := pool.Exec(ctx, `UPDATE biometric_consents SET created_at=clock_timestamp()-interval '1 hour',valid_until=clock_timestamp()+interval '200 milliseconds' WHERE id=$1`, consent); err != nil {
		t.Fatal(err)
	}
	previewEntered, previewRelease = make(chan struct{}), make(chan struct{})
	go func() { previewDone <- call("POST", previewPath, preview) }()
	select {
	case <-previewEntered:
	case <-time.After(3 * time.Second):
		close(previewRelease)
		t.Fatal("expiring preview did not reach AI")
	}
	time.Sleep(250 * time.Millisecond)
	close(previewRelease)
	select {
	case w := <-previewDone:
		expect(w, 410)
	case <-time.After(3 * time.Second):
		t.Fatal("expiring preview blocked")
	}
	previewEntered, previewRelease = nil, nil
	consent = newConsent()
	expect(call("POST", "/students/"+id+"/enrollments", map[string]any{"frames": []string{frames[0]}}), 400)
	expect(call("POST", "/students/"+id+"/enrollments", map[string]any{"frames": []string{"data:image/jpeg;base64," + frames[0], frames[1], frames[2]}}), 400)
	expect(call("POST", "/students/"+id+"/enrollments", map[string]any{"frames": frames, "student_id": id}), 400)
	enrolled := expect(call("POST", "/students/"+id+"/enrollments", enroll), 201)["data"].(map[string]any)
	if enrolled["status"] != "provisional" || enrolled["recognition_enabled"] != false || strings.Contains(stringMustJSON(enrolled), "ciphertext") {
		t.Fatal("public template metadata leaked", enrolled)
	}
	status := expect(call("GET", "/students/"+id+"/face-status", nil), 200)["data"].(map[string]any)
	if status["enrolled"] != true || status["pipeline_ready"] != true || status["recognition_enabled"] != false {
		t.Fatal(status)
	}
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT revision FROM gallery_state WHERE school_id=$1`, schoolID).Scan(&revision); err != nil || revision != 1 {
		t.Fatal("missing revision", revision, err)
	}
	// Imported/legacy data may contain more than one live consent; templates keep their own consent binding.
	var newerConsent string
	if err := pool.QueryRow(ctx, `INSERT INTO biometric_consents(school_id,student_id,guardian_id,policy_version,evidence_ref,recorded_by,valid_until,created_at) SELECT school_id,student_id,guardian_id,policy_version,'SYNTHETIC-NEWER-CONSENT',recorded_by,valid_until,clock_timestamp() FROM biometric_consents WHERE id=$1 RETURNING id::text`, consent).Scan(&newerConsent); err != nil {
		t.Fatal(err)
	}
	login("revoker@example.test")
	newerStatus := expect(call("GET", "/students/"+id+"/face-status", nil), 200)["data"].(map[string]any)
	if newerStatus["enrolled"] != true || newerStatus["consent_eligible"] != true || newerStatus["template"] == nil || newerStatus["template"].(map[string]any)["id"] != enrolled["template"].(map[string]any)["id"] {
		t.Fatal("newer valid consent hid existing eligible template", newerStatus)
	}
	revoked := expect(call("POST", "/students/"+id+"/face-revoke", map[string]string{"reason": "Synthetic revocation"}), 200)["data"].(map[string]any)
	if revoked["deleted_templates"] != float64(1) {
		t.Fatal(revoked)
	}
	login("owner@example.test")
	expect(call("POST", "/consents/"+newerConsent+"/withdraw", map[string]string{"reason": "Synthetic overlapping-consent fixture cleanup"}), 200)
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 201)
	expect(call("POST", "/consents/"+consent+"/withdraw", map[string]string{"reason": "Synthetic withdrawal"}), 200)
	expect(call("POST", previewPath, preview), 410)
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM face_templates`).Scan(&count); err != nil || count != 0 {
		t.Fatal("withdrawal retained ciphertext", count, err)
	}
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 410)
	consent = newConsent()
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 201)
	expect(call("PATCH", "/students/"+id, map[string]string{"status": "inactive"}), 200)
	expect(call("POST", previewPath, preview), 409)
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM face_templates`).Scan(&count); err != nil || count != 0 {
		t.Fatal("inactive retained ciphertext", count, err)
	}
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 409)
	login("revoker@example.test")
	inactiveRows := expect(call("GET", "/biometrics/students?search=BIO-001", nil), 200)["data"].([]any)
	if len(inactiveRows) != 1 || inactiveRows[0].(map[string]any)["status"] != "inactive" {
		t.Fatal("inactive student unavailable for revocation", inactiveRows)
	}
	expect(call("GET", "/students/"+id+"/face-status", nil), 200)
	login("owner@example.test")
	expect(call("PATCH", "/students/"+id, map[string]string{"status": "active"}), 200)
	expect(call("POST", "/students/"+id+"/placements", map[string]any{"class_id": class["id"], "reason": "Synthetic reactivation"}), 201)
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 201)
	if _, err := pool.Exec(ctx, `UPDATE biometric_consents SET created_at=clock_timestamp()-interval '2 hours',valid_until=clock_timestamp()-interval '1 hour' WHERE id=$1`, consent); err != nil {
		t.Fatal(err)
	}
	expect(call("POST", previewPath, preview), 410)
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 410)
	consent = newConsent()
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 201)
	// Another school's student must not be reachable through the known UUID.
	other := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if _, err := pool.Exec(ctx, `INSERT INTO schools(id,name) VALUES($1,'Synthetic other school');`, other); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO students(id,school_id,nis,full_name) VALUES($1,$1,'OTHER','Synthetic other student')`, other); err != nil {
		t.Fatal(err)
	}
	expect(call("GET", "/students/"+other+"/face-status", nil), 404)
	expect(call("POST", "/students/"+other+"/face-preview", preview), 404)
	login("enroller@example.test")
	expect(call("GET", "/students/"+other+"/face-status", nil), 404)
	otherRows := expect(call("GET", "/biometrics/students?search=OTHER", nil), 200)["data"].([]any)
	if len(otherRows) != 0 {
		t.Fatal("cross-school selector leak", otherRows)
	}
	expect(call("GET", "/biometrics/students?page=2&page_size=1", nil), 200)
	oldCookie, oldCSRF := cookie, csrf
	login("owner@example.test")
	expect(call("PATCH", "/users/"+enrolledOfficer["id"].(string), map[string]any{"grants": []string{}}), 200)
	cookie, csrf = oldCookie, oldCSRF
	expect(call("POST", previewPath, preview), 401)
	expect(call("GET", "/biometrics/students", nil), 401)
	login("enroller@example.test")
	expect(call("POST", previewPath, preview), 403)
	expect(call("GET", "/biometrics/students", nil), 403)
	expect(call("GET", "/students/"+id+"/face-status", nil), 403)
	login("owner@example.test")
	// Device recognition sees only current consent and the session roster; unknown candidates never leak.
	schedule := expect(call("POST", "/attendance-schedules", map[string]any{"class_id": class["id"], "name": "Synthetic schedule", "weekdays": []int{1, 2, 3, 4, 5, 6, 7}, "checkin_start": "00:00", "checkin_cutoff": "00:01", "close_time": "23:59"}), 201)["data"].(map[string]any)
	session := expect(call("POST", "/attendance-sessions", map[string]any{"schedule_id": schedule["id"], "date": now.In(time.FixedZone("Jakarta", 7*3600)).Format("2006-01-02")}), 201)["data"].(map[string]any)
	sessionID := session["id"].(string)
	if _, err := pool.Exec(ctx, `UPDATE attendance_sessions SET starts_at=clock_timestamp()-interval '10 minutes',late_after=clock_timestamp()+interval '10 minutes',closes_at=clock_timestamp()+interval '1 hour' WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	expect(call("POST", "/attendance-sessions/"+sessionID+"/open", map[string]string{"reason": "Synthetic test"}), 200)
	expect(call("POST", "/attendance-sessions/"+sessionID+"/participants", map[string]any{"student_id": id, "reason": "Synthetic metadata roster"}), 201)
	device := expect(call("POST", "/devices", map[string]string{"name": "Synthetic device", "location": "Synthetic test"}), 201)["data"].(map[string]any)
	pairing := expect(call("POST", "/devices/"+device["id"].(string)+"/pairing-token", nil), 201)["data"].(map[string]any)
	pairedResponse := call("POST", "/kiosk/session", map[string]any{"pairing_token": pairing["pairing_token"], "session_id": sessionID})
	paired := expect(pairedResponse, 201)["data"].(map[string]any)
	deviceCookie := pairedResponse.Result().Cookies()[0]
	deviceCall := func(body any, token string) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/api/v1/kiosk/recognize", bytes.NewReader(payload))
		r.Header.Set("Origin", "http://localhost:3000")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", token)
		r.AddCookie(deviceCookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	capture := map[string]any{"image": frames[0], "event_id": "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "track_id": "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "captured_at": time.Now().UTC().Format(time.RFC3339Nano)}
	expect(deviceCall(capture, "wrong"), 403)
	for _, value := range []string{"unknown", "accepted"} {
		decision = value
		result := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any)
		if result["decision"] != "unknown" || result["recognition_enabled"] != false || strings.Contains(stringMustJSON(result), "PRIVATE-CANDIDATE") || strings.Contains(stringMustJSON(result), "student_id") {
			t.Fatal("unknown identity leaked", result)
		}
	}
	if galleryCount != 1 {
		t.Fatal("eligible template missing", galleryCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM attendance_records`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unknown created attendance", count, err)
	}
	login("owner@example.test")
	expect(call("POST", "/consents/"+consent+"/withdraw", map[string]string{"reason": "Synthetic withdrawal during kiosk"}), 200)
	expect(deviceCall(capture, paired["csrf_token"].(string)), 200)
	if galleryCount != 0 {
		t.Fatal("withdrawn template stale in gallery", galleryCount)
	}
	capture["captured_at"] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	expect(deviceCall(capture, paired["csrf_token"].(string)), 400)
	var sensitiveAudit int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE metadata::text LIKE '%ciphertext%' OR metadata::text LIKE '%PRIVATE-CANDIDATE%' OR metadata::text LIKE '%frames%'`).Scan(&sensitiveAudit); err != nil || sensitiveAudit != 0 {
		t.Fatal("sensitive audit", sensitiveAudit, err)
	}
}
func stringMustJSON(value any) string { encoded, _ := json.Marshal(value); return string(encoded) }
