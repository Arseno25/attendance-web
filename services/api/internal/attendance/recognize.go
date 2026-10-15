package attendance

import (
	"attendance.local/api/internal/platform/aiclient"
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type capture struct {
	Image      string `json:"image"`
	EventID    string `json:"event_id"`
	TrackID    string `json:"track_id"`
	CapturedAt string `json:"captured_at"`
}

func stableRecognition(result aiclient.Recognition) bool {
	if !result.Confirmed || len(result.Faces) != 1 || len(result.ModelSHA) != 64 {
		return false
	}
	_, err := hex.DecodeString(result.ModelSHA)
	return err == nil
}
func recognitionApproved(result aiclient.Recognition) bool {
	return result.RecognitionEnabled && result.Decision == "accepted" && result.LivenessPassed && stableRecognition(result)
}

func validBox(box aiclient.Box) bool {
	for _, v := range []float64{box.X, box.Y, box.Width, box.Height} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return false
		}
	}
	return box.Width > 0 && box.Height > 0 && box.X+box.Width <= 1.000001 && box.Y+box.Height <= 1.000001
}
func (s *Service) recognize(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	tx, device, ok := s.kioskAuth(w, r, true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	var in capture
	if !httpresponse.DecodeLimit(w, r, &in, 512<<10) {
		return
	}
	in.EventID = strings.ToLower(in.EventID)
	in.TrackID = strings.ToLower(in.TrackID)
	taken, err := time.Parse(time.RFC3339Nano, in.CapturedAt)
	if !httpresponse.ValidID(in.EventID) || !httpresponse.ValidID(in.TrackID) || err != nil || math.Abs(time.Since(taken).Seconds()) > 30 || !aiclient.ValidateImage(in.Image) {
		invalid(w, "Foto kamera belum dapat diproses. Ambil ulang dan coba lagi.")
		return
	}
	gallery := []aiclient.Template{}
	records, err := tx.Query(r.Context(), `SELECT f.student_id::text,COALESCE(f.consent_id::text,''),f.ciphertext,f.nonce,f.key_id,f.model_sha256 FROM face_templates f JOIN students st ON st.school_id=f.school_id AND st.id=f.student_id JOIN attendance_session_participants p ON p.school_id=f.school_id AND p.student_id=f.student_id AND p.session_id=$2 WHERE f.school_id=$1 AND st.status='active' ORDER BY f.student_id LIMIT 3001`, s.schoolID, device["session_id"])
	if err != nil {
		dbError(w, err)
		return
	}
	for records.Next() {
		var entry aiclient.Template
		var cipher, nonce []byte
		err = records.Scan(&entry.StudentID, &entry.ConsentID, &cipher, &nonce, &entry.KeyID, &entry.ModelSHA)
		if err != nil {
			break
		}
		entry.Ciphertext = base64.StdEncoding.EncodeToString(cipher)
		entry.Nonce = base64.StdEncoding.EncodeToString(nonce)
		gallery = append(gallery, entry)
	}
	records.Close()
	if err == nil {
		err = records.Err()
	}
	if err != nil {
		dbError(w, err)
		return
	}
	if len(gallery) > 3000 {
		httpresponse.Error(w, 503, "GALLERY_LIMIT", "Galeri melebihi kapasitas pilot; gunakan absensi manual")
		return
	}
	var revision int64
	err = tx.QueryRow(r.Context(), `SELECT COALESCE((SELECT revision FROM gallery_state WHERE school_id=$1),0)`, s.schoolID).Scan(&revision)
	if err != nil {
		dbError(w, err)
		return
	}
	result, err := s.ai.Recognize(r.Context(), map[string]any{"school_id": s.schoolID, "session_id": device["session_id"], "device_id": device["device_id"], "event_id": in.EventID, "track_id": in.TrackID, "image": in.Image, "gallery_revision": revision, "gallery": gallery})
	if err != nil {
		var validation *aiclient.ValidationError
		if errors.As(err, &validation) {
			httpresponse.Error(w, 422, validation.Code, validation.Message)
		} else {
			httpresponse.Error(w, 503, "AI_UNAVAILABLE", "Layanan wajah belum tersedia; gunakan absensi manual")
		}
		return
	}
	if result.EventID != in.EventID || result.GalleryRevision != revision || len(result.Faces) > 100 {
		httpresponse.Error(w, 503, "AI_INVALID_RESPONSE", "Pendaftaran wajah belum dapat disimpan. Coba lagi nanti.")
		return
	}
	for _, face := range result.Faces {
		if !validBox(face.Box) || (face.Similarity != nil && (math.IsNaN(*face.Similarity) || math.IsInf(*face.Similarity, 0) || *face.Similarity < -1 || *face.Similarity > 1)) {
			httpresponse.Error(w, 503, "AI_INVALID_RESPONSE", "Kotak wajah tidak valid")
			return
		}
	}
	approved := recognitionApproved(result)
	var attendance map[string]any
	continuing := result.Decision == "verification_required" && result.EngineStatus == "VERIFYING" && !result.LivenessPassed && stableRecognition(result)
	if approved || continuing {
		attendance, err = s.acceptRecognition(r, tx, device, in, result, approved)
		if err != nil {
			if errors.Is(err, errState) {
				httpresponse.Error(w, 409, "RECOGNITION_REJECTED", "Pengenalan tidak dapat diterima; gunakan absensi manual")
			} else {
				dbError(w, err)
			}
			return
		}
	} else {
		result.RecognitionEnabled = false
		result.BlockReason = ""
		if result.Decision == "accepted" {
			result.Decision = "unknown"
		}
		if result.Decision != "unknown" && result.Decision != "no_face" && result.Decision != "inconclusive" {
			result.Decision = "inconclusive"
		}
		for i := range result.Faces {
			result.Faces[i].StudentID = ""
			result.Faces[i].Label = ""
		}
	}
	if result.Faces == nil {
		result.Faces = []aiclient.Face{}
	}
	response := map[string]any{"decision": result.Decision, "faces": result.Faces, "recognition_enabled": approved, "identity_continuous": continuing, "block_reason": result.BlockReason, "engine_status": result.EngineStatus, "reason": result.Reason, "server_time": device["server_time"]}
	if attendance != nil {
		response["attendance"] = attendance
	}
	httpresponse.Data(w, 200, response)
}

func (s *Service) acceptRecognition(r *http.Request, tx pgx.Tx, device map[string]any, in capture, result aiclient.Recognition, recordAttendance bool) (map[string]any, error) {
	student := result.Faces[0].StudentID
	if !httpresponse.ValidID(student) {
		return nil, errState
	}
	var name, actor string
	var now, late time.Time
	var checkout *time.Time
	cookie, _ := r.Cookie(s.deviceCookieName())
	if cookie == nil {
		return nil, errState
	}
	err := tx.QueryRow(r.Context(), `SELECT p.full_name,ds.paired_by::text,clock_timestamp(),a.late_after,a.checkout_at FROM attendance_session_participants p JOIN attendance_sessions a ON a.school_id=p.school_id AND a.id=p.session_id JOIN students st ON st.school_id=p.school_id AND st.id=p.student_id JOIN face_templates f ON f.school_id=st.school_id AND f.student_id=st.id JOIN device_sessions ds ON ds.school_id=p.school_id AND ds.session_id=p.session_id JOIN users u ON u.school_id=ds.school_id AND u.id=ds.paired_by WHERE p.school_id=$1 AND p.session_id=$2 AND p.student_id=$3 AND ds.token_hash=$4 AND ds.expires_at>clock_timestamp() AND u.is_active AND st.status='active' AND a.status='open' AND a.starts_at<=clock_timestamp() AND a.closes_at>clock_timestamp() AND f.model_sha256=$5`, s.schoolID, device["session_id"], student, deviceHash(cookie.Value), result.ModelSHA).Scan(&name, &actor, &now, &late, &checkout)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errState
	}
	if err != nil {
		return nil, err
	}
	result.Faces[0].Label = name
	// A fresh match may retain an already verified name while PAD rechecks; it cannot write attendance.
	if !recordAttendance {
		return nil, nil
	}
	fingerprintRaw, _ := json.Marshal([]any{in, device["session_id"], device["device_id"], student})
	hash := sha256.Sum256(fingerprintRaw)
	fingerprint := hex.EncodeToString(hash[:])
	var previous, operation string
	var response map[string]any
	err = tx.QueryRow(r.Context(), `SELECT fingerprint,operation,response FROM request_ledger WHERE school_id=$1 AND actor_id=$2 AND event_id=$3`, s.schoolID, actor, in.EventID).Scan(&previous, &operation, &response)
	if err == nil {
		if previous != fingerprint || operation != "attendance.recognition" {
			return nil, errState
		}
		return response, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	mode := "check_in"
	if checkout != nil && !now.Before(*checkout) {
		mode = "check_out"
	}
	record, err := one(r.Context(), tx, `SELECT `+recordColumns+` FROM attendance_records WHERE school_id=$1 AND session_id=$2 AND student_id=$3 FOR UPDATE`, s.schoolID, device["session_id"], student)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	missing := errors.Is(err, pgx.ErrNoRows)
	response = map[string]any{"mode": mode, "record": record}
	var pending bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM attendance_corrections WHERE school_id=$1 AND session_id=$2 AND student_id=$3 AND status='pending')`, s.schoolID, device["session_id"], student).Scan(&pending)
	if err != nil {
		return nil, err
	}
	switch {
	case pending:
		response["outcome"] = "correction_pending"
		return response, nil
	case !missing && record["status"] != "present" && record["status"] != "late":
		response["outcome"] = "status_locked"
		return response, nil
	case mode == "check_out" && (missing || record["check_in_at"] == nil):
		response["outcome"] = "check_in_required"
		response["record"] = nil
		return response, nil
	case !missing && record["check_out_at"] != nil:
		response["outcome"] = "already_checked_out"
		return response, nil
	case mode == "check_in" && !missing:
		response["outcome"] = "already_recorded"
		return response, nil
	}
	outcome, event := "recorded", "check_in"
	if mode == "check_out" {
		outcome, event = "checked_out", "check_out"
		record, err = one(r.Context(), tx, `UPDATE attendance_records SET check_out_at=$3,version=version+1,updated_at=clock_timestamp() WHERE school_id=$1 AND id=$2 RETURNING `+recordColumns, s.schoolID, record["id"], now)
	} else {
		status := "present"
		if now.After(late) {
			status = "late"
		}
		record, err = one(r.Context(), tx, `INSERT INTO attendance_records(school_id,session_id,student_id,status,check_in_at,reason,actor_id) VALUES($1,$2,$3,$4,$5,'Absensi melalui kamera',$6) RETURNING `+recordColumns, s.schoolID, device["session_id"], student, status, now, actor)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO attendance_events(school_id,record_id,actor_id,event_type,new_status,reason) VALUES($1,$2,$3,$4,$5,'Absensi melalui kamera')`, s.schoolID, record["id"], actor, event, record["status"])
	}
	if err == nil {
		err = s.audit(r, tx, actor, "attendance.recognition."+outcome, "attendance_record", record["id"].(string), map[string]any{"accepted_count": 1, "mode": mode, "model_sha256": result.ModelSHA})
	}
	if err != nil {
		return nil, err
	}
	response["outcome"], response["record"] = outcome, record
	payload, _ := json.Marshal(response)
	_, err = tx.Exec(r.Context(), `INSERT INTO request_ledger(school_id,actor_id,event_id,operation,fingerprint,response) VALUES($1,$2,$3,'attendance.recognition',$4,$5::jsonb)`, s.schoolID, actor, in.EventID, fingerprint, string(payload))
	if err == nil {
		err = tx.Commit(r.Context())
	}
	return response, err
}
