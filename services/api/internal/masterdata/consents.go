package masterdata

import (
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"net/http"
	"strings"
	"time"
)

func (s *Service) consentStudent(w http.ResponseWriter, r *http.Request, perm string, write bool) (*scope, string, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return nil, "", false
	}
	sc, ok := s.begin(w, r, perm, write)
	if !ok {
		return nil, "", false
	}
	var found bool
	err := sc.tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM students WHERE school_id=$1 AND id=$2)`, s.schoolID, id).Scan(&found)
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
func (s *Service) guardians(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	sc, id, ok := s.consentStudent(w, r, "consents.manage", r.Method == "POST")
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	if r.Method == "GET" {
		s.list(w, r, sc, `SELECT id::text AS id,full_name,relationship,verification_ref,created_at FROM guardians WHERE school_id=$1 AND student_id=$2`, s.schoolID, id)
		return
	}
	var in struct {
		Name         string `json:"full_name"`
		Relationship string `json:"relationship"`
		Ref          string `json:"verification_ref"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Ref = strings.TrimSpace(in.Ref)
	if !text(in.Name, 120) || !text(in.Ref, 120) || (in.Relationship != "parent" && in.Relationship != "guardian") {
		invalid(w, "Nama, hubungan dan referensi verifikasi wali wajib valid")
		return
	}
	entity, err := one(r.Context(), sc.tx, `INSERT INTO guardians(school_id,student_id,full_name,relationship,verification_ref,verified_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text AS id,full_name,relationship,created_at`, s.schoolID, id, in.Name, in.Relationship, in.Ref, sc.user.ID)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 201, entity, "guardians.verified", "guardian", nil)
}
func (s *Service) consents(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	sc, id, ok := s.consentStudent(w, r, "consents.manage", r.Method == "POST")
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	if r.Method == "GET" {
		s.list(w, r, sc, `SELECT co.id::text AS id,co.guardian_id::text, g.full_name AS guardian_name,co.policy_version,co.evidence_ref,co.valid_until,co.withdrawn_at,co.created_at FROM biometric_consents co JOIN guardians g ON g.school_id=co.school_id AND g.id=co.guardian_id WHERE co.school_id=$1 AND co.student_id=$2`, s.schoolID, id)
		return
	}
	var in struct {
		Guardian string `json:"guardian_id"`
		Policy   string `json:"policy_version"`
		Evidence string `json:"evidence_ref"`
		Until    string `json:"valid_until"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Policy = strings.TrimSpace(in.Policy)
	in.Evidence = strings.TrimSpace(in.Evidence)
	until, err := time.Parse(time.RFC3339, in.Until)
	if err != nil || until.Year() > 9998 || !until.After(time.Now()) || !httpresponse.ValidID(in.Guardian) || !text(in.Policy, 80) || !text(in.Evidence, 120) {
		invalid(w, "Wali, versi kebijakan, referensi bukti dan masa berlaku wajib valid")
		return
	}
	var eligible bool
	err = sc.tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM biometric_consents WHERE school_id=$1 AND student_id=$2 AND withdrawn_at IS NULL AND valid_until>clock_timestamp())`, s.schoolID, id).Scan(&eligible)
	if err != nil {
		dbError(w, err)
		return
	}
	if eligible {
		conflict(w, "Consent masih berlaku; cabut sebelum mencatat consent baru")
		return
	}
	entity, err := one(r.Context(), sc.tx, `INSERT INTO biometric_consents(school_id,student_id,guardian_id,policy_version,evidence_ref,valid_until,recorded_by) SELECT $1,$2,$3,$4,$5,$6,$7 FROM students WHERE school_id=$1 AND id=$2 AND status='active' RETURNING id::text AS id,valid_until,created_at`, s.schoolID, id, in.Guardian, in.Policy, in.Evidence, until, sc.user.ID)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 201, entity, "consents.recorded", "consent", nil)
}
func (s *Service) withdraw(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sc, ok := s.begin(w, r, "consents.manage", true)
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
		invalid(w, "Alasan pencabutan wajib diisi")
		return
	}
	entity, err := one(r.Context(), sc.tx, `SELECT id::text AS id,student_id::text AS student_id,withdrawn_at FROM biometric_consents WHERE school_id=$1 AND id=$2 FOR UPDATE`, s.schoolID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if entity["withdrawn_at"] != nil {
		httpresponse.Data(w, 200, entity)
		return
	}
	entity, err = one(r.Context(), sc.tx, `UPDATE biometric_consents SET withdrawn_at=clock_timestamp(),withdrawn_by=$3,withdrawal_reason=$4 WHERE school_id=$1 AND id=$2 RETURNING id::text AS id,student_id::text,withdrawn_at`, s.schoolID, id, sc.user.ID, in.Reason)
	if err == nil {
		_, err = sc.tx.Exec(r.Context(), `INSERT INTO biometric_tombstones(school_id,consent_id,student_id) VALUES($1,$2,$3)`, s.schoolID, id, entity["student_id"])
	}
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 200, entity, "consents.withdrawn", "consent", nil)
}
