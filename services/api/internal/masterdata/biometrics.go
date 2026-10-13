package masterdata

import (
	"attendance.local/api/internal/platform/aiclient"
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

const templateMetadata = `id::text AS id,created_at,model_sha256,frame_count,status`
const consentEligibility = `SELECT c.id::text FROM biometric_consents c JOIN students st ON st.school_id=c.school_id AND st.id=c.student_id WHERE c.school_id=$1 AND c.student_id=$2 AND st.status='active' AND c.withdrawn_at IS NULL AND c.valid_until>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM biometric_tombstones t WHERE t.school_id=c.school_id AND t.consent_id=c.id) ORDER BY c.created_at DESC LIMIT 1`

func (s *Service) biometricRead(w http.ResponseWriter, r *http.Request) (*scope, bool) {
	tx, user, ok := s.auth.BeginAuthorized(w, r, "biometrics.enroll", false, "biometrics.revoke")
	if !ok {
		return nil, false
	}
	return &scope{tx: tx, user: user, classes: user.ClassIDs}, true
}

func (s *Service) biometricStudents(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	sc, ok := s.biometricRead(w, r)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > 100 {
		invalid(w, "Filter siswa tidak valid")
		return
	}
	s.list(w, r, sc, `SELECT st.id::text AS id,st.full_name,st.nis,st.status,current_class.class_name FROM students st LEFT JOIN LATERAL(SELECT c.name AS class_name FROM student_enrollments e JOIN classes c ON c.school_id=e.school_id AND c.id=e.class_id WHERE e.school_id=st.school_id AND e.student_id=st.id AND e.joined_at<=statement_timestamp() AND e.left_at>statement_timestamp() ORDER BY e.joined_at DESC LIMIT 1) current_class ON true WHERE st.school_id=$1 AND (st.nis ILIKE $2 OR st.full_name ILIKE $2)`, s.schoolID, "%"+search+"%")
}

func (s *Service) faceStatus(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sc, ok := s.biometricRead(w, r)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	var found bool
	if err := sc.tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM students WHERE school_id=$1 AND id=$2)`, s.schoolID, id).Scan(&found); err != nil {
		dbError(w, err)
		return
	}
	if !found {
		httpresponse.Error(w, 404, "NOT_FOUND", "Siswa tidak ditemukan")
		return
	}
	var consent string
	err := sc.tx.QueryRow(r.Context(), consentEligibility, s.schoolID, id).Scan(&consent)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		dbError(w, err)
		return
	}
	eligible := err == nil
	var metadata map[string]any
	if eligible {
		metadata, err = one(r.Context(), sc.tx, `SELECT f.id::text AS id,f.created_at,f.model_sha256,f.frame_count,f.status FROM face_templates f JOIN students st ON st.school_id=f.school_id AND st.id=f.student_id JOIN biometric_consents c ON c.school_id=f.school_id AND c.id=f.consent_id AND c.student_id=f.student_id WHERE f.school_id=$1 AND f.student_id=$2 AND st.status='active' AND c.withdrawn_at IS NULL AND c.valid_until>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM biometric_tombstones t WHERE t.school_id=f.school_id AND t.consent_id=f.consent_id)`, s.schoolID, id)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			dbError(w, err)
			return
		}
	}
	state := s.ai.Status(r.Context())
	httpresponse.Data(w, 200, map[string]any{"enrolled": metadata != nil, "consent_eligible": eligible, "pipeline_ready": state.PipelineReady, "recognition_enabled": state.RecognitionEnabled, "block_reason": state.BlockReason, "template": metadata, "evaluation": state.Evaluation})
}

func (s *Service) enrollFace(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	sc, id, ok := s.consentStudent(w, r, "biometrics.enroll", true)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	var active bool
	err := sc.tx.QueryRow(r.Context(), `SELECT status='active' FROM students WHERE school_id=$1 AND id=$2`, s.schoolID, id).Scan(&active)
	if err != nil {
		dbError(w, err)
		return
	}
	if !active {
		httpresponse.Error(w, 409, "STUDENT_NOT_ACTIVE", "Siswa harus aktif untuk mendaftarkan wajah")
		return
	}
	var consent string
	err = sc.tx.QueryRow(r.Context(), consentEligibility, s.schoolID, id).Scan(&consent)
	if errors.Is(err, pgx.ErrNoRows) {
		httpresponse.Error(w, 410, "CONSENT_REQUIRED", "Consent wali belum tersedia, kedaluwarsa atau sudah dicabut")
		return
	}
	if err != nil {
		dbError(w, err)
		return
	}
	var in struct {
		Frames []string `json:"frames"`
	}
	if !httpresponse.DecodeLimit(w, r, &in, 2<<20) {
		return
	}
	if len(in.Frames) < 3 || len(in.Frames) > 5 {
		invalid(w, "Ambil 3–5 frame JPEG kamera")
		return
	}
	for _, frame := range in.Frames {
		if !aiclient.ValidateImage(frame) {
			invalid(w, "Gunakan base64 JPEG 160–1280 piksel, maksimal 384 KiB per frame")
			return
		}
	}
	result, err := s.ai.Enroll(r.Context(), s.schoolID, id, consent, in.Frames)
	if err != nil {
		var validation *aiclient.ValidationError
		if errors.As(err, &validation) {
			httpresponse.Error(w, 422, validation.Code, validation.Message)
		} else {
			httpresponse.Error(w, 503, "AI_UNAVAILABLE", "Layanan wajah belum tersedia; coba kembali")
		}
		return
	}
	_, modelErr := hex.DecodeString(result.ModelSHA)
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(result.Ciphertext)
	nonce, nonceErr := base64.StdEncoding.Strict().DecodeString(result.Nonce)
	if err != nil || nonceErr != nil || len(ciphertext) != 528 || len(nonce) != 12 || len(result.ModelSHA) != 64 || modelErr != nil || result.FrameCount != len(in.Frames) || result.KeyID == "" || len(result.KeyID) > 80 || result.Status != "provisional" {
		httpresponse.Error(w, 503, "AI_INVALID_RESPONSE", "Respons layanan wajah tidak valid")
		return
	}
	_, err = sc.tx.Exec(r.Context(), `DELETE FROM face_templates WHERE school_id=$1 AND student_id=$2`, s.schoolID, id)
	var metadata map[string]any
	if err == nil {
		metadata, err = one(r.Context(), sc.tx, `INSERT INTO face_templates(school_id,student_id,consent_id,ciphertext,nonce,key_id,model_sha256,frame_count,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+templateMetadata, s.schoolID, id, consent, ciphertext, nonce, result.KeyID, result.ModelSHA, result.FrameCount, sc.user.ID)
	}
	if err != nil {
		dbError(w, err)
		return
	}
	err = s.audit(r.Context(), sc, "biometrics.enrolled", "face_template", metadata["id"].(string), w.Header().Get("X-Request-ID"), map[string]any{"frame_count": result.FrameCount, "status": "provisional", "model_sha256": result.ModelSHA})
	if err == nil {
		err = sc.tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 201, map[string]any{"enrolled": true, "status": "provisional", "frame_count": result.FrameCount, "recognition_enabled": false, "block_reason": aiclient.BlockReason, "template": metadata})
}

func (s *Service) revokeFace(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	sc, id, ok := s.consentStudent(w, r, "biometrics.revoke", true)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	var in struct {
		Reason string `json:"reason"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if !text(in.Reason, 500) {
		invalid(w, "Alasan pencabutan wajah wajib diisi")
		return
	}
	command, err := sc.tx.Exec(r.Context(), `DELETE FROM face_templates WHERE school_id=$1 AND student_id=$2`, s.schoolID, id)
	if err == nil {
		err = s.audit(r.Context(), sc, "biometrics.revoked", "student", id, w.Header().Get("X-Request-ID"), map[string]any{"deleted_templates": command.RowsAffected(), "reason": in.Reason})
	}
	if err == nil {
		err = sc.tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 200, map[string]any{"enrolled": false, "deleted_templates": command.RowsAffected(), "recognition_enabled": false})
}
