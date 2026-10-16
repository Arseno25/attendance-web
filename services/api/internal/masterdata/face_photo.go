package masterdata

import (
	"attendance.local/api/internal/platform/aiclient"
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"encoding/base64"
	"net/http"
)

func (s *Service) facePhoto(w http.ResponseWriter, r *http.Request) {
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
	var version string
	var ciphertext, nonce []byte
	var photo aiclient.ReferencePhoto
	err := sc.tx.QueryRow(r.Context(), `SELECT f.id::text,f.reference_ciphertext,f.reference_nonce,f.key_id FROM face_templates f JOIN students st ON st.school_id=f.school_id AND st.id=f.student_id WHERE st.school_id=$1 AND st.id=$4 AND st.status='active' AND `+studentScope+` AND f.reference_ciphertext IS NOT NULL`, s.schoolID, sc.classScoped, sc.classes, id).Scan(&version, &ciphertext, &nonce, &photo.KeyID)
	if err != nil {
		dbError(w, err)
		return
	}
	if err = sc.tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	photo.Ciphertext = base64.StdEncoding.EncodeToString(ciphertext)
	photo.Nonce = base64.StdEncoding.EncodeToString(nonce)
	image, err := s.ai.ReferencePhoto(r.Context(), s.schoolID, id, photo)
	if err != nil {
		httpresponse.Error(w, 503, "PHOTO_UNAVAILABLE", "Foto pendaftaran belum dapat ditampilkan. Coba lagi nanti.")
		return
	}
	// Recheck both permissions and template identity after decrypting outside the transaction.
	sc, ok = s.biometricRead(w, r)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	var current string
	err = sc.tx.QueryRow(r.Context(), `SELECT f.id::text FROM face_templates f JOIN students st ON st.school_id=f.school_id AND st.id=f.student_id WHERE st.school_id=$1 AND st.id=$4 AND st.status='active' AND `+studentScope+` AND f.id=$5`, s.schoolID, sc.classScoped, sc.classes, id, version).Scan(&current)
	if err != nil {
		dbError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	httpresponse.Data(w, 200, map[string]string{"image": image})
}
