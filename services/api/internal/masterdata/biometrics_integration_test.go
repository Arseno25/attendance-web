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

func TestBiometricEnrollmentWithoutConsentAndDevicePrivacy(t *testing.T) {
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
	password := "ExamplePassword1!"
	if err = a.Bootstrap(ctx, "owner@example.test", "Owner", password, "School"); err != nil {
		t.Fatal(err)
	}
	var portrait bytes.Buffer
	if err := jpeg.Encode(&portrait, image.NewRGBA(image.Rect(0, 0, 192, 192)), nil); err != nil {
		t.Fatal(err)
	}
	photoImage := base64.StdEncoding.EncodeToString(portrait.Bytes())
	photoCalls := 0
	invalidPhoto := false
	invalidEnrollmentPhoto := false
	var photoMutation func()
	aiCalls := 0
	rejectDuplicate := false
	var enrollmentGallery []any
	previewCalls := 0
	previewStatus := 200
	previewResponse := `{"faces":[{"box":{"x":0.1,"y":0.2,"width":0.3,"height":0.4},"student_id":"PRIVATE-ID","label":"PRIVATE-CANDIDATE","embedding":[1]}],"face_count":1,"quality":"ready","message":"PRIVATE-CANDIDATE","similarity":0.99}`
	galleryCount := -1
	galleryConsent := ""
	decision := "unknown"
	runtimeEvidence := false
	similarity := 0.89
	rechecking := false
	engineStatus := ""
	var recognitionStarted, recognitionRelease chan struct{}
	mutateGallery := false
	recognizedID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
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
			var input map[string]any
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			enrollmentGallery, _ = input["gallery"].([]any)
			if rejectDuplicate && len(enrollmentGallery) > 0 {
				w.WriteHeader(422)
				_, _ = w.Write([]byte(`{"error":{"code":"DUPLICATE_IDENTITY","message":"Wajah ini sudah terdaftar. Periksa data siswa."}}`))
				return
			}
			photoKey := "synthetic-test"
			if invalidEnrollmentPhoto {
				photoKey = "mismatched-key"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ciphertext": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, 528)), "nonce": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{43}, 12)), "key_id": "synthetic-test", "model_sha256": strings.Repeat("a", 64), "frame_count": 3, "status": "provisional", "reference_photo": map[string]string{"ciphertext": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{44}, 128)), "nonce": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{45}, 12)), "key_id": photoKey}})
			return
		}
		if r.URL.Path == "/internal/v1/reference-photo" {
			photoCalls++
			if photoMutation != nil {
				photoMutation()
			}
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input["school_id"] != schoolID || input["student_id"] == "" {
				t.Error("unbound photo decryption")
			}
			if invalidPhoto {
				http.Error(w, "unavailable", 503)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"image": photoImage})
			return
		}
		if r.URL.Path == "/internal/v1/face-preview" {
			previewCalls++
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input) != 1 || input["image"] == "" {
				t.Error("preview sent unexpected fields", input, err)
			}
			w.WriteHeader(previewStatus)
			_, _ = w.Write([]byte(previewResponse))
			return
		}
		if r.URL.Path == "/internal/v1/recognize" {
			if recognitionStarted != nil {
				close(recognitionStarted)
				<-recognitionRelease
			}
			if mutateGallery {
				if _, err := pool.Exec(ctx, `UPDATE gallery_state SET revision=revision+1 WHERE school_id=$1`, schoolID); err != nil {
					t.Error(err)
				}
			}
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			gallery := request["gallery"].([]any)
			galleryCount = len(gallery)
			galleryConsent = ""
			if galleryCount == 1 {
				galleryConsent, _ = gallery[0].(map[string]any)["consent_id"].(string)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"decision": decision, "recognition_enabled": runtimeEvidence && !rechecking, "liveness_passed": runtimeEvidence && !rechecking, "engine_status": engineStatus, "is_confirmed": runtimeEvidence, "event_id": request["event_id"], "gallery_revision": request["gallery_revision"], "model_sha256": strings.Repeat("a", 64), "faces": []any{map[string]any{"box": map[string]float64{"x": 0.1, "y": 0.1, "width": 0.5, "height": 0.5}, "student_id": recognizedID, "label": "PRIVATE-CANDIDATE", "similarity": similarity}}})
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
	expect(call("POST", previewPath, preview), 200)
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 201)
	expect(call("POST", "/students/"+id+"/guardians", nil), 404)
	expect(call("GET", "/students/"+id+"/consents", nil), 404)
	expect(call("POST", "/consents/"+id+"/withdraw", nil), 404)
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM biometric_consents`).Scan(&count); err != nil || count != 0 {
		t.Fatal("enrollment fabricated consent", err)
	}

	enrollerRole := expect(call("POST", "/roles", map[string]any{"name": "Synthetic enroller", "scope": "school", "permissions": []string{"biometrics.enroll"}}), 201)["data"].(map[string]any)
	revokerRole := expect(call("POST", "/roles", map[string]any{"name": "Synthetic revoker", "scope": "school", "permissions": []string{"biometrics.revoke"}}), 201)["data"].(map[string]any)
	expect(call("POST", "/users", map[string]any{"email": "enroller@example.test", "full_name": "Synthetic enroller", "role": enrollerRole["key"], "password": password}), 201)
	expect(call("POST", "/users", map[string]any{"email": "revoker@example.test", "full_name": "Synthetic revoker", "role": revokerRole["key"], "password": password}), 201)
	expect(call("POST", "/users", map[string]any{"email": "unauthorized@example.test", "full_name": "Synthetic unauthorized", "role": "operator", "password": password}), 201)
	{ // Biometric access must follow assigned classes.
		role := expect(call("POST", "/roles", map[string]any{"name": "Class face operator", "scope": "assigned_classes", "permissions": []string{"biometrics.enroll", "biometrics.revoke"}}), 201)["data"].(map[string]any)
		user := expect(call("POST", "/users", map[string]any{"email": "classface@example.test", "full_name": "Class operator", "role": role["key"], "password": password}), 201)["data"].(map[string]any)
		foreignClass := expect(call("POST", "/classes", map[string]any{"academic_year_id": year["id"], "code": "OTHER", "name": "Other"}), 201)["data"].(map[string]any)
		foreign := expect(call("POST", "/students", map[string]any{"nis": "OTHER-001", "full_name": "Other class student", "class_id": foreignClass["id"]}), 201)["data"].(map[string]any)
		login("classface@example.test")
		if data := expect(call("GET", "/biometrics/students", nil), 200)["data"].([]any); len(data) != 0 {
			t.Fatal("unassigned face operator saw students", data)
		}
		for _, studentID := range []string{id, foreign["id"].(string)} {
			expect(call("GET", "/students/"+studentID+"/face-status", nil), 404)
			expect(call("GET", "/students/"+studentID+"/face-photo", nil), 404)
			for _, action := range []string{"face-preview", "enrollments", "face-revoke", "biometrics/revoke"} {
				expect(call("POST", "/students/"+studentID+"/"+action, nil), 404)
			}
		}
		login("owner@example.test")
		expect(call("POST", "/class-assignments", map[string]any{"user_id": user["id"], "class_id": class["id"]}), 201)
		login("classface@example.test")
		visible := expect(call("GET", "/biometrics/students", nil), 200)["data"].([]any)
		if len(visible) != 1 || visible[0].(map[string]any)["id"] != id {
			t.Fatal("face selector did not follow class assignment", visible)
		}
		expect(call("GET", "/students/"+id+"/face-status", nil), 200)
		for _, action := range []string{"face-status", "face-photo", "face-preview", "enrollments", "face-revoke"} {
			method := "POST"
			if action == "face-status" || action == "face-photo" {
				method = "GET"
			}
			expect(call(method, "/students/"+foreign["id"].(string)+"/"+action, nil), 404)
		}
		login("owner@example.test")
	}
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
	if len(minimal) != 6 || minimal["id"] != id || minimal["full_name"] != student["full_name"] || minimal["nis"] != "BIO-001" || minimal["status"] != "active" || minimal["class_name"] != "BIO" {
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
	expect(call("GET", "/students/"+id+"/face-photo", nil), 403)
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
	expect(call("GET", "/students/"+id+"/face-photo", nil), 403)
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
	if previewCalls != 2 {
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
	photoResponse := call("GET", "/students/"+id+"/face-photo", nil)
	if data := expect(photoResponse, 200)["data"].(map[string]any); data["image"] != photoImage || !strings.Contains(photoResponse.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("private portrait unavailable or cacheable")
	}
	if status["template"].(map[string]any)["reference_photo_version"] == nil {
		t.Fatal("missing portrait metadata")
	}
	invalidPhoto = true
	expect(call("GET", "/students/"+id+"/face-photo", nil), 503)
	invalidPhoto = false
	oldVersion := status["template"].(map[string]any)["id"]
	invalidEnrollmentPhoto = true
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 503)
	invalidEnrollmentPhoto = false
	if result := expect(call("GET", "/students/"+id+"/face-status", nil), 200)["data"].(map[string]any); result["template"].(map[string]any)["id"] != oldVersion {
		t.Fatal("invalid photo replaced enrollment")
	}
	login("classface@example.test")
	photoMutation = func() {
		deadline, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		tx, err := pool.Begin(deadline)
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(deadline, `SELECT id FROM schools WHERE id=$1 FOR UPDATE`, schoolID); err != nil {
			t.Error("photo decryption held school lock", err)
			return
		}
		if _, err = tx.Exec(deadline, `DELETE FROM user_class_assignments WHERE class_id=$1`, class["id"]); err != nil {
			t.Error(err)
			return
		}
		if err = tx.Commit(deadline); err != nil {
			t.Error(err)
		}
	}
	expect(call("GET", "/students/"+id+"/face-photo", nil), 404)
	photoMutation = nil
	login("owner@example.test")
	var classUserID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users WHERE email='classface@example.test'`).Scan(&classUserID); err != nil {
		t.Fatal(err)
	}
	expect(call("POST", "/class-assignments", map[string]any{"user_id": classUserID, "class_id": class["id"]}), 201)
	photoMutation = func() {
		if _, err := pool.Exec(ctx, `DELETE FROM face_templates WHERE student_id=$1`, id); err != nil {
			t.Error(err)
		}
	}
	expect(call("GET", "/students/"+id+"/face-photo", nil), 404)
	photoMutation = nil
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 201)
	var foreignID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM students WHERE nis='OTHER-001'`).Scan(&foreignID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO face_templates(school_id,student_id,ciphertext,nonce,key_id,model_sha256,frame_count,created_by) SELECT school_id,$2,ciphertext,nonce,key_id,model_sha256,frame_count,created_by FROM face_templates WHERE student_id=$1`, id, foreignID); err != nil {
		t.Fatal(err)
	}
	expect(call("GET", "/students/"+foreignID+"/face-photo", nil), 404)
	rejectDuplicate = true
	login("classface@example.test")
	duplicate := expect(call("POST", "/students/"+id+"/enrollments", enroll), 422)
	if len(enrollmentGallery) != 1 || enrollmentGallery[0].(map[string]any)["student_id"] != foreignID || strings.Contains(stringMustJSON(duplicate), foreignID) {
		t.Fatal("duplicate check lost other-class gallery or exposed its identity")
	}
	if templates := expect(call("GET", "/students/"+id+"/face-status", nil), 200)["data"].(map[string]any); templates["enrolled"] != true {
		t.Fatal("duplicate rejection removed existing enrollment")
	}
	login("owner@example.test")
	rejectDuplicate = false
	if _, err := pool.Exec(ctx, `DELETE FROM face_templates WHERE student_id=$1`, foreignID); err != nil {
		t.Fatal(err)
	}

	var consentMissing bool
	if err := pool.QueryRow(ctx, `SELECT consent_id IS NULL FROM face_templates WHERE student_id=$1`, id).Scan(&consentMissing); err != nil || !consentMissing {
		t.Fatal("new enrollment bound to consent", err)
	}
	if status["enrollment_allowed"] != true {
		t.Fatal("active student cannot enroll", status)
	}
	if _, present := status["consent_required"]; present {
		t.Fatal("retired consent policy exposed")
	}
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

	deviceRequest := func(path string, body any, token string) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/api/v1/kiosk"+path, bytes.NewReader(payload))
		r.Header.Set("Origin", "http://localhost:3000")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", token)
		r.AddCookie(deviceCookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	frameNumber := 0
	var lastCapture map[string]any
	deviceCall := func(body any, token string) *httptest.ResponseRecorder {
		permit := deviceRequest("/capture-challenge", nil, token)
		if permit.Code != 201 {
			return permit
		}
		data := expect(permit, 201)["data"].(map[string]any)
		input := map[string]any{}
		for k, v := range body.(map[string]any) {
			input[k] = v
		}
		input["event_id"] = data["event_id"]
		frameNumber++
		pixels := image.NewRGBA(image.Rect(0, 0, 240, 240))
		for y := 0; y < 240; y++ {
			for x := 0; x < 240; x++ {
				pixels.Set(x, y, color.RGBA{uint8(frameNumber * 5), uint8(x), uint8(y), 255})
			}
		}
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, pixels, nil); err != nil {
			t.Fatal(err)
		}
		input["image"] = base64.StdEncoding.EncodeToString(encoded.Bytes())
		input["captured_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		lastCapture = input
		return deviceRequest("/recognize", input, token)
	}
	capture := map[string]any{"image": frames[0], "event_id": "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "track_id": "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "captured_at": time.Now().UTC().Format(time.RFC3339Nano)}
	expect(deviceCall(capture, "wrong"), 403)
	for _, value := range []string{"unknown", "accepted"} {
		decision = value
		result := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any)
		if result["decision"] != "unknown" || result["recognition_enabled"] != false || strings.Contains(stringMustJSON(result), "PRIVATE-CANDIDATE") || strings.Contains(stringMustJSON(result), "student_id") || result["reference_photo"] != nil {
			t.Fatal("unknown identity leaked", result)
		}
	}

	// A capture is fresh only with an unused, unexpired server permit and image.
	expect(deviceRequest("/recognize", lastCapture, paired["csrf_token"].(string)), 409)
	replay := map[string]any{}
	for k, v := range lastCapture {
		replay[k] = v
	}
	permit := expect(deviceRequest("/capture-challenge", nil, paired["csrf_token"].(string)), 201)["data"].(map[string]any)
	replay["event_id"] = permit["event_id"]
	expect(deviceRequest("/recognize", replay, paired["csrf_token"].(string)), 409)
	if _, err := pool.Exec(ctx, `UPDATE kiosk_capture_challenges SET expires_at=clock_timestamp()-interval '1 second' WHERE event_id=$1`, permit["event_id"]); err != nil {
		t.Fatal(err)
	}
	replay["image"] = frames[2]
	expect(deviceRequest("/recognize", replay, paired["csrf_token"].(string)), 409)
	permit = expect(deviceRequest("/capture-challenge", nil, paired["csrf_token"].(string)), 201)["data"].(map[string]any)
	if _, err := pool.Exec(ctx, `UPDATE kiosk_capture_challenges SET session_token_hash=$2 WHERE event_id=$1`, permit["event_id"], bytes.Repeat([]byte{99}, 32)); err != nil {
		t.Fatal(err)
	}
	replay["event_id"] = permit["event_id"]
	expect(deviceRequest("/recognize", replay, paired["csrf_token"].(string)), 409)
	if galleryCount != 1 {
		t.Fatal("eligible template missing", galleryCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM attendance_records`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unknown created attendance", count, err)
	}

	login("owner@example.test")
	recognitionStarted, recognitionRelease = make(chan struct{}), make(chan struct{})
	scanFinished := make(chan *httptest.ResponseRecorder, 1)
	go func() { scanFinished <- deviceCall(capture, paired["csrf_token"].(string)) }()
	select {
	case <-recognitionStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("inference did not start")
	}
	readFinished := make(chan *httptest.ResponseRecorder, 1)
	go func() { readFinished <- call("GET", "/students", nil) }()
	blocked := false
	select {
	case w := <-readFinished:
		expect(w, 200)
	case <-time.After(2 * time.Second):
		blocked = true
	}
	close(recognitionRelease)
	expect(<-scanFinished, 200)
	if blocked {
		<-readFinished
		t.Error("slow inference blocked authorized school reads")
	}
	recognitionStarted, recognitionRelease = nil, nil
	mutateGallery = true
	expect(deviceCall(capture, paired["csrf_token"].(string)), 409)
	mutateGallery = false

	// Live stable recognition writes once and labels from the roster, without an offline report.
	runtimeEvidence, recognizedID = true, id
	if _, err := pool.Exec(ctx, `UPDATE attendance_sessions SET late_after=starts_at+interval '1 minute',checkout_at=clock_timestamp()-interval '1 second' WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	missingArrival := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any)["attendance"].(map[string]any)
	if missingArrival["outcome"] != "check_in_required" || missingArrival["record"] != nil {
		t.Fatal("departure fabricated arrival", missingArrival)
	}
	if _, err := pool.Exec(ctx, `UPDATE attendance_sessions SET checkout_at=NULL,late_after=clock_timestamp()+interval '10 minutes' WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	decision, rechecking, engineStatus = "verification_required", true, "VERIFYING"
	tracking := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any)
	if tracking["identity_continuous"] != true || tracking["recognition_enabled"] != false || tracking["attendance"] != nil || tracking["reference_photo"] != nil || strings.Contains(stringMustJSON(tracking), "PRIVATE-CANDIDATE") {
		t.Fatal("tracking recheck recorded attendance or leaked candidate", tracking)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM attendance_records`).Scan(&count); err != nil || count != 0 {
		t.Fatal("PAD recheck wrote attendance", count, err)
	}
	decision, rechecking, engineStatus = "accepted", false, "CONFIRMED"
	similarity = 1.01
	expect(deviceCall(capture, paired["csrf_token"].(string)), 503)
	similarity = 0.89
	accepted := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any)
	if accepted["reference_photo"] != photoImage || accepted["decision"] != "accepted" || accepted["attendance"].(map[string]any)["outcome"] != "recorded" {
		t.Fatal("live match did not record attendance", accepted)
	}
	if strings.Contains(stringMustJSON(accepted), "PRIVATE-CANDIDATE") {
		t.Fatal("upstream label used instead of roster")
	}
	expect(deviceCall(capture, paired["csrf_token"].(string)), 200)
	capture["event_id"] = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	repeated := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any)
	if repeated["reference_photo"] != photoImage || repeated["attendance"].(map[string]any)["outcome"] != "already_recorded" {
		t.Fatal("second camera event duplicated attendance", repeated)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM attendance_records WHERE student_id=$1`, id).Scan(&count); err != nil || count != 1 {
		t.Fatal("recognition did not persist exactly once", count, err)
	}
	if photoCalls < 3 {
		t.Fatal("reference photo path was not exercised")
	}
	var photoInLedger bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM request_ledger WHERE response::text LIKE '%reference_photo%' OR response::text LIKE '%image%')`).Scan(&photoInLedger); err != nil || photoInLedger {
		t.Fatal("portrait persisted in ledger", err)
	}
	invalidPhoto = true
	if result := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any); result["attendance"].(map[string]any)["outcome"] != "already_recorded" || result["reference_photo"] != nil {
		t.Fatal("optional photo failure broke attendance")
	}
	invalidPhoto = false
	photoMutation = func() {
		deadline, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		tx, err := pool.Begin(deadline)
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(deadline, `SELECT id FROM schools WHERE id=$1 FOR UPDATE`, schoolID); err != nil {
			t.Error("kiosk photo decryption held school lock", err)
			return
		}
		if _, err = tx.Exec(deadline, `UPDATE face_templates SET id=gen_random_uuid() WHERE student_id=$1`, id); err != nil {
			t.Error(err)
			return
		}
		if err = tx.Commit(deadline); err != nil {
			t.Error(err)
		}
	}
	stalePortrait := expect(deviceCall(capture, paired["csrf_token"].(string)), 409)
	if strings.Contains(stringMustJSON(stalePortrait), photoImage) || strings.Contains(stringMustJSON(stalePortrait), "Synthetic metadata student") {
		t.Fatal("replaced reference disclosed to kiosk")
	}
	photoMutation = func() {
		if _, err := pool.Exec(ctx, `UPDATE camera_devices SET revoked_at=clock_timestamp() WHERE id=$1`, device["id"]); err != nil {
			t.Error(err)
		}
	}
	expect(deviceCall(capture, paired["csrf_token"].(string)), 401)
	photoMutation = nil
	if _, err := pool.Exec(ctx, `UPDATE camera_devices SET revoked_at=NULL WHERE id=$1`, device["id"]); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM attendance_records WHERE student_id=$1`, id).Scan(&count); err != nil || count != 1 {
		t.Fatal("portrait revocation changed committed attendance", err)
	}
	firstRecord := accepted["attendance"].(map[string]any)["record"].(map[string]any)
	if repeated["attendance"].(map[string]any)["record"].(map[string]any)["check_in_at"] != firstRecord["check_in_at"] {
		t.Fatal("repeat changed arrival time")
	}
	if _, err := pool.Exec(ctx, `UPDATE attendance_records SET status='sick',check_in_at=NULL WHERE student_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	capture["event_id"] = "aeaeaeae-aeae-4eae-8eae-aeaeaeaeaeae"
	locked := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any)["attendance"].(map[string]any)
	if locked["outcome"] != "status_locked" || locked["record"].(map[string]any)["status"] != "sick" {
		t.Fatal("manual status overwritten", locked)
	}
	if _, err := pool.Exec(ctx, `UPDATE attendance_records SET status='present',check_in_at=$2 WHERE student_id=$1`, id, firstRecord["check_in_at"]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO attendance_corrections(school_id,session_id,student_id,requested_by,previous_version,previous_status,new_status,reason) SELECT school_id,session_id,student_id,actor_id,version,status,'sick','Synthetic correction' FROM attendance_records WHERE student_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	capture["event_id"] = "afafafaf-afaf-4faf-8faf-afafafafafaf"
	pending := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any)["attendance"].(map[string]any)
	if pending["outcome"] != "correction_pending" {
		t.Fatal("pending correction ignored", pending)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM attendance_corrections WHERE student_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	// Departure follows the snapshotted session time, not browser input or another scan.
	if _, err := pool.Exec(ctx, `UPDATE attendance_sessions SET late_after=starts_at+interval '1 minute',checkout_at=clock_timestamp()-interval '1 second' WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	capture["event_id"] = "abababab-abab-4bab-8bab-abababababab"
	departure := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any)["attendance"].(map[string]any)
	if departure["outcome"] != "checked_out" {
		t.Fatal("scheduled departure missing", departure)
	}
	departedRecord := departure["record"].(map[string]any)
	if departedRecord["check_out_at"] == nil || departedRecord["check_in_at"] != firstRecord["check_in_at"] {
		t.Fatal("departure damaged timestamps", departure)
	}
	capture["event_id"] = "acacacac-acac-4cac-8cac-acacacacacac"
	duplicateDeparture := expect(deviceCall(capture, paired["csrf_token"].(string)), 200)["data"].(map[string]any)["attendance"].(map[string]any)
	if duplicateDeparture["outcome"] != "already_checked_out" || duplicateDeparture["record"].(map[string]any)["check_out_at"] != departedRecord["check_out_at"] {
		t.Fatal("departure repeat changed timestamp", duplicateDeparture)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM request_ledger WHERE operation='attendance.recognition'`).Scan(&count); err != nil || count != 2 {
		t.Fatal("repeated frames generated ledger writes", count, err)
	}
	capture["event_id"] = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	capture["image"] = frames[1]
	expect(deviceRequest("/recognize", capture, paired["csrf_token"].(string)), 409)
	capture["image"] = frames[0]
	runtimeEvidence = false

	// Legacy IDs remain encryption context, never an enrollment or gallery prerequisite.
	legacyID := "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	if _, err := pool.Exec(ctx, `INSERT INTO guardians(id,school_id,student_id,full_name,relationship,verification_ref,verified_by) SELECT $1,$2,$3,'Synthetic legacy guardian','parent','synthetic',id FROM users WHERE email='owner@example.test'`, legacyID, schoolID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO biometric_consents(id,school_id,student_id,guardian_id,policy_version,evidence_ref,recorded_by,created_at,valid_until) SELECT $1,$2,$3,$1,'legacy','synthetic',id,clock_timestamp()-interval '2 days',clock_timestamp()-interval '1 day' FROM users WHERE email='owner@example.test'`, legacyID, schoolID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE face_templates SET consent_id=$1 WHERE student_id=$2`, legacyID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE biometric_consents SET withdrawn_at=clock_timestamp(),withdrawn_by=recorded_by,withdrawal_reason='Synthetic legacy record' WHERE id=$1`, legacyID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO biometric_tombstones(school_id,consent_id,student_id) VALUES($1,$2,$3)`, schoolID, legacyID, id); err != nil {
		t.Fatal(err)
	}
	if err := attendanceService.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	expect(deviceCall(capture, paired["csrf_token"].(string)), 200)
	if galleryCount != 1 || galleryConsent != legacyID {
		t.Fatal("legacy encryption context lost", galleryCount, galleryConsent)
	}
	login("owner@example.test")
	legacyStatus := expect(call("GET", "/students/"+id+"/face-status", nil), 200)["data"].(map[string]any)
	if legacyStatus["enrolled"] != true || legacyStatus["enrollment_allowed"] != true {
		t.Fatal("legacy eligibility still depends on consent", legacyStatus)
	}
	var retiredPermissions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM role_permissions WHERE permission='consents.manage'`).Scan(&retiredPermissions); err != nil || retiredPermissions != 0 {
		t.Fatal("retired permission remains", retiredPermissions, err)
	}

	login("owner@example.test")
	expect(call("POST", "/students/"+id+"/face-revoke", map[string]string{"reason": "Synthetic removal"}), 200)
	expect(call("GET", "/students/"+id+"/face-photo", nil), 404)
	expect(deviceCall(capture, paired["csrf_token"].(string)), 200)
	if galleryCount != 0 {
		t.Fatal("removed template in gallery")
	}
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 201)
	if _, err := pool.Exec(ctx, `UPDATE students SET status='inactive' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	expect(call("POST", previewPath, preview), 409)
	expect(call("POST", "/students/"+id+"/enrollments", enroll), 409)
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM face_templates WHERE student_id=$1`, id).Scan(&count); err != nil || count != 0 {
		t.Fatal("inactive student retained template", err)
	}
	expect(call("GET", "/students/"+id+"/face-photo", nil), 404)
	var sensitiveAudit int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE metadata::text LIKE '%ciphertext%' OR metadata::text LIKE '%PRIVATE-CANDIDATE%' OR metadata::text LIKE '%frames%'`).Scan(&sensitiveAudit); err != nil || sensitiveAudit != 0 {
		t.Fatal("sensitive audit", err)
	}
}
func stringMustJSON(value any) string { encoded, _ := json.Marshal(value); return string(encoded) }
