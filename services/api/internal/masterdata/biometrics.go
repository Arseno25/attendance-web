package masterdata

import (
	"attendance.local/api/internal/auth"
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

func (s *Service) biometricRead(w http.ResponseWriter, r *http.Request) (*scope, bool) {
	tx, user, ok := s.auth.BeginAuthorized(w, r, "biometrics.enroll", false, "biometrics.revoke")
	if !ok {
		return nil, false
	}
	return &scope{tx: tx, user: user, classes: user.ClassIDs, classScoped: user.Scope == auth.ScopeAssignedClasses}, true
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
	s.list(w, r, sc, `SELECT st.id::text AS id,st.full_name,st.nis,st.status,current_class.class_name FROM students st LEFT JOIN LATERAL(SELECT c.name AS class_name FROM student_enrollments e JOIN classes c ON c.school_id=e.school_id AND c.id=e.class_id WHERE e.school_id=st.school_id AND e.student_id=st.id AND e.joined_at<=statement_timestamp() AND e.left_at>statement_timestamp() ORDER BY e.joined_at DESC LIMIT 1) current_class ON true WHERE st.school_id=$1 AND `+studentScope+` AND (st.nis ILIKE $4 OR st.full_name ILIKE $4)`, s.schoolID, sc.classScoped, sc.classes, "%"+search+"%")
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
	var active bool
	if err := sc.tx.QueryRow(r.Context(), `SELECT st.status='active' FROM students st WHERE st.school_id=$1 AND st.id=$4 AND `+studentScope, s.schoolID, sc.classScoped, sc.classes, id).Scan(&active); err != nil {
		dbError(w, err)
		return
	}
	metadata, err := one(r.Context(), sc.tx, `SELECT f.id::text AS id,f.created_at,f.model_sha256,f.frame_count,f.status FROM face_templates f JOIN students st ON st.school_id=f.school_id AND st.id=f.student_id WHERE f.school_id=$1 AND f.student_id=$2 AND st.status='active'`, s.schoolID, id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		dbError(w, err)
		return
	}
	state := s.ai.Status(r.Context())
	httpresponse.Data(w, 200, map[string]any{"enrolled": metadata != nil, "enrollment_allowed": active, "pipeline_ready": state.PipelineReady, "recognition_enabled": state.RecognitionEnabled, "block_reason": state.BlockReason, "template": metadata, "evaluation": state.Evaluation})
}

func (s *Service) enrollFace(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	sc, id, ok := s.biometricStudent(w, r, "biometrics.enroll", true)
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
	var in struct {
		Frames []string `json:"frames"`
	}
	if !httpresponse.DecodeLimit(w, r, &in, 2<<20) {
		return
	}
	if len(in.Frames) < 3 || len(in.Frames) > 5 {
		invalid(w, "Ambil 3–5 foto dari kamera.")
		return
	}
	for _, frame := range in.Frames {
		if !aiclient.ValidateImage(frame) {
			invalid(w, "Foto belum dapat diproses. Ambil ulang dari kamera.")
			return
		}
	}
	gallery, err := s.enrollmentGallery(r.Context(), sc.tx, id)
	if err != nil {
		httpresponse.Error(w, 503, "GALLERY_UNAVAILABLE", "Data wajah belum dapat diperiksa. Hubungi pengelola.")
		return
	}
	result, err := s.ai.Enroll(r.Context(), s.schoolID, id, "", in.Frames, gallery...)
	if err != nil {
		var validation *aiclient.ValidationError
		if errors.As(err, &validation) {
			httpresponse.Error(w, 422, validation.Code, validation.Message)
		} else {
			httpresponse.Error(w, 503, "AI_UNAVAILABLE", "Pendaftaran wajah belum tersedia. Coba lagi nanti.")
		}
		return
	}
	_, modelErr := hex.DecodeString(result.ModelSHA)
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(result.Ciphertext)
	nonce, nonceErr := base64.StdEncoding.Strict().DecodeString(result.Nonce)
	if err != nil || nonceErr != nil || len(ciphertext) != 528 || len(nonce) != 12 || len(result.ModelSHA) != 64 || modelErr != nil || result.FrameCount != len(in.Frames) || result.KeyID == "" || len(result.KeyID) > 80 || result.Status != "provisional" {
		httpresponse.Error(w, 503, "AI_INVALID_RESPONSE", "Pendaftaran wajah belum dapat disimpan. Coba lagi nanti.")
		return
	}
	_, err = sc.tx.Exec(r.Context(), `DELETE FROM face_templates WHERE school_id=$1 AND student_id=$2`, s.schoolID, id)
	var metadata map[string]any
	if err == nil {
		metadata, err = one(r.Context(), sc.tx, `INSERT INTO face_templates(school_id,student_id,consent_id,ciphertext,nonce,key_id,model_sha256,frame_count,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+templateMetadata, s.schoolID, id, nil, ciphertext, nonce, result.KeyID, result.ModelSHA, result.FrameCount, sc.user.ID)
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
	state := s.ai.Status(r.Context())
	httpresponse.Data(w, 201, map[string]any{"enrolled": true, "status": "provisional", "frame_count": result.FrameCount, "recognition_enabled": state.RecognitionEnabled, "block_reason": state.BlockReason, "template": metadata})
}

// Duplicate checks cover the school internally; another student's identity never leaves the API.
func (s *Service) enrollmentGallery(ctx context.Context, tx pgx.Tx, studentID string) ([]aiclient.Template, error) {
	r, err := tx.Query(ctx, `SELECT student_id::text,COALESCE(consent_id::text,''),ciphertext,nonce,key_id,model_sha256 FROM face_templates WHERE school_id=$1 AND student_id!=$2 ORDER BY student_id LIMIT 3001`, s.schoolID, studentID)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	gallery := []aiclient.Template{}
	for r.Next() {
		var item aiclient.Template
		var ciphertext, nonce []byte
		if err := r.Scan(&item.StudentID, &item.ConsentID, &ciphertext, &nonce, &item.KeyID, &item.ModelSHA); err != nil {
			return nil, err
		}
		item.Ciphertext, item.Nonce = base64.StdEncoding.EncodeToString(ciphertext), base64.StdEncoding.EncodeToString(nonce)
		gallery = append(gallery, item)
	}
	if len(gallery) > 3000 {
		return nil, errors.New("gallery capacity exceeded")
	}
	return gallery, r.Err()
}

func (s *Service) facePreview(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	// Preview needs the camera POST boundary while retaining shared read locks for concurrent previews.
	if _, ok := s.auth.Authenticate(w, r, true); !ok {
		return
	}
	sc, id, ok := s.biometricStudent(w, r, "biometrics.enroll", false)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	var active bool
	if err := sc.tx.QueryRow(r.Context(), `SELECT status='active' FROM students WHERE school_id=$1 AND id=$2`, s.schoolID, id).Scan(&active); err != nil {
		dbError(w, err)
		return
	}
	if !active {
		httpresponse.Error(w, 409, "STUDENT_NOT_ACTIVE", "Siswa harus aktif untuk menggunakan kamera wajah")
		return
	}
	var in struct {
		Image string `json:"image"`
	}
	if !httpresponse.DecodeLimit(w, r, &in, 512<<10) {
		return
	}
	if !aiclient.ValidateImage(in.Image) {
		httpresponse.Error(w, 422, "INVALID_IMAGE", "Foto belum dapat diproses. Ambil ulang dari kamera.")
		return
	}
	result, err := s.ai.FacePreview(r.Context(), in.Image)
	if err != nil {
		var validation *aiclient.ValidationError
		if errors.As(err, &validation) {
			httpresponse.Error(w, 422, "INVALID_IMAGE", "Gambar kamera tidak dapat diproses; ambil ulang")
		} else {
			httpresponse.Error(w, 503, "AI_UNAVAILABLE", "Pendaftaran wajah belum tersedia. Coba lagi nanti.")
		}
		return
	}
	httpresponse.Data(w, 200, result)
}

func (s *Service) revokeFace(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	sc, id, ok := s.biometricStudent(w, r, "biometrics.revoke", true)
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
		invalid(w, "Isi alasan penghapusan data wajah.")
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

func (s *Service) biometricStudent(w http.ResponseWriter, r *http.Request, perm string, write bool) (*scope, string, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return nil, "", false
	}
	sc, ok := s.begin(w, r, perm, write)
	if !ok {
		return nil, "", false
	}
	var found bool
	err := sc.tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM students st WHERE st.school_id=$1 AND st.id=$4 AND `+studentScope+`)`, s.schoolID, sc.classScoped, sc.classes, id).Scan(&found)
	if err != nil || !found {
		_ = sc.tx.Rollback(context.Background())
		if err != nil {
			dbError(w, err)
		} else {
			httpresponse.Error(w, 404, "NOT_FOUND", "Siswa tidak ditemukan")
		}
		return nil, "", false
	}
	return sc, id, true
}
