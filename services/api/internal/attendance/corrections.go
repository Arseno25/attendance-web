package attendance

import (
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

const correctionSelect = `SELECT ac.id::text AS id,ac.session_id::text AS session_id,ac.student_id::text AS student_id,ac.requested_by::text AS requested_by,requester.full_name AS requester_name,p.nis,p.full_name,c.name AS class_name,se.date::text AS session_date,ac.previous_version,ac.previous_status,ac.new_status,ac.check_in_at,ac.reason,ac.status,ac.decided_by::text AS decided_by,ac.decision_reason,ac.decided_at,ac.created_at FROM attendance_corrections ac JOIN attendance_session_participants p ON p.school_id=ac.school_id AND p.session_id=ac.session_id AND p.student_id=ac.student_id JOIN attendance_sessions se ON se.school_id=ac.school_id AND se.id=ac.session_id JOIN classes c ON c.school_id=se.school_id AND c.id=se.class_id JOIN users requester ON requester.school_id=ac.school_id AND requester.id=ac.requested_by WHERE ac.school_id=$1 AND (NOT $2::boolean OR se.class_id=ANY($3::uuid[])) AND (NOT $4::boolean OR ac.requested_by=$5::uuid)`

func (s *Service) corrections(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	write := r.Method == "POST"
	tx, u, ok := s.begin(w, r, "corrections.request", write)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if !write {
		status := r.URL.Query().Get("status")
		if status != "" && status != "pending" && status != "approved" && status != "rejected" {
			invalid(w, "Status koreksi tidak valid")
			return
		}
		s.list(w, r, tx, correctionSelect+` AND ($6='' OR ac.status=$6)`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, u.Scope == auth.ScopeAssignedClasses, u.ID, status)
		return
	}
	var in struct {
		Session string `json:"session_id"`
		Student string `json:"student_id"`
		Status  string `json:"status"`
		Checkin string `json:"check_in_at"`
		Local   string `json:"check_in_local"`
		Reason  string `json:"reason"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if !httpresponse.ValidID(in.Session) || !httpresponse.ValidID(in.Student) || !validStatus(in.Status) || !text(in.Reason, 500) {
		invalid(w, "Data koreksi tidak valid")
		return
	}
	se, err := s.getSession(r, tx, u, in.Session, true)
	if err != nil {
		dbError(w, err)
		return
	}
	if se["status"] != "open" && se["status"] != "closed" {
		conflict(w, "Koreksi hanya untuk sesi yang telah dibuka")
		return
	}
	var checkin any
	if in.Status == "present" || in.Status == "late" {
		at, e := time.Parse(time.RFC3339Nano, in.Checkin)
		if in.Local != "" {
			if in.Checkin != "" {
				invalid(w, "Gunakan satu waktu hadir")
				return
			}
			layout := "2006-01-02T15:04"
			if len(in.Local) == 19 {
				layout = "2006-01-02T15:04:05"
			}
			_, e = time.Parse(layout, in.Local)
			if e == nil {
				e = tx.QueryRow(r.Context(), `SELECT $1::timestamp AT TIME ZONE $2`, in.Local, se["timezone"]).Scan(&at)
			}
		}
		var now time.Time
		if err = tx.QueryRow(r.Context(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
			dbError(w, err)
			return
		}
		if e != nil || at.Before(se["starts_at"].(time.Time)) || !at.Before(se["closes_at"].(time.Time)) || at.After(now) || (at.After(se["late_after"].(time.Time)) != (in.Status == "late")) {
			invalid(w, "Waktu hadir harus berada di periode sesi, tidak di masa depan, dan sesuai status terlambat")
			return
		}
		checkin = at
	} else if in.Checkin != "" || in.Local != "" {
		invalid(w, "Status tanpa kehadiran tidak memakai waktu check-in")
		return
	}
	var exists bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM attendance_session_participants WHERE school_id=$1 AND session_id=$2 AND student_id=$3)`, s.schoolID, in.Session, in.Student).Scan(&exists)
	if err != nil {
		dbError(w, err)
		return
	}
	if !exists {
		httpresponse.Error(w, 404, "NOT_FOUND", "Siswa tidak ada dalam daftar absensi ini.")
		return
	}
	version := 0
	previous := "not_recorded"
	err = tx.QueryRow(r.Context(), `SELECT version,status FROM attendance_records WHERE school_id=$1 AND session_id=$2 AND student_id=$3 FOR UPDATE`, s.schoolID, in.Session, in.Student).Scan(&version, &previous)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		dbError(w, err)
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO attendance_corrections(school_id,session_id,student_id,requested_by,previous_version,previous_status,new_status,check_in_at,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`, s.schoolID, in.Session, in.Student, u.ID, version, previous, in.Status, checkin, in.Reason).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	data, err := one(r.Context(), tx, correctionSelect+` AND ac.id=$6`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, u.Scope == auth.ScopeAssignedClasses, u.ID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, tx, u, 201, data, "attendance.corrections.requested", "attendance_correction", nil)
}
func (s *Service) correction(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tx, u, ok := s.begin(w, r, "corrections.request", false)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	data, err := one(r.Context(), tx, correctionSelect+` AND ac.id=$6`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, u.Scope == auth.ScopeAssignedClasses, u.ID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 200, data)
}
func (s *Service) decision(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tx, u, ok := s.begin(w, r, "attendance.approve", true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	var in struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if (in.Decision != "approved" && in.Decision != "rejected") || !text(in.Reason, 500) {
		invalid(w, "Keputusan dan alasan wajib valid")
		return
	}
	data, err := one(r.Context(), tx, correctionSelect+` AND ac.id=$6 FOR UPDATE OF ac`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, u.Scope == auth.ScopeAssignedClasses, u.ID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if data["requested_by"] == u.ID {
		httpresponse.Error(w, 403, "SELF_APPROVAL_DENIED", "Koreksi memerlukan petugas yang berbeda")
		return
	}
	if data["status"] != "pending" {
		conflict(w, "Koreksi sudah diputuskan")
		return
	}
	se, err := s.getSession(r, tx, u, data["session_id"].(string), true)
	if err != nil {
		dbError(w, err)
		return
	}
	if se["status"] != "open" && se["status"] != "closed" {
		conflict(w, "Sesi tidak dapat dikoreksi")
		return
	}
	if in.Decision == "approved" {
		current := 0
		err = tx.QueryRow(r.Context(), `SELECT version FROM attendance_records WHERE school_id=$1 AND session_id=$2 AND student_id=$3 FOR UPDATE`, s.schoolID, data["session_id"], data["student_id"]).Scan(&current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			dbError(w, err)
			return
		}
		if current != int(data["previous_version"].(int32)) {
			conflict(w, "Catatan berubah sejak permintaan; tolak lalu ajukan ulang")
			return
		}
		var now time.Time
		err = tx.QueryRow(r.Context(), `SELECT clock_timestamp()`).Scan(&now)
		if err != nil {
			dbError(w, err)
			return
		}
		if data["new_status"] == "absent" && (se["status"] != "closed" || now.Before(se["closes_at"].(time.Time))) {
			conflict(w, "Alfa hanya disahkan setelah sesi dan waktu jadwal berakhir")
			return
		}
		var recordID string
		err = tx.QueryRow(r.Context(), `INSERT INTO attendance_records(school_id,session_id,student_id,status,check_in_at,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(session_id,student_id) DO UPDATE SET status=EXCLUDED.status,check_in_at=EXCLUDED.check_in_at,check_out_at=NULL,reason=EXCLUDED.reason,actor_id=EXCLUDED.actor_id,version=attendance_records.version+1,updated_at=now() RETURNING id::text`, s.schoolID, data["session_id"], data["student_id"], data["new_status"], data["check_in_at"], data["reason"], u.ID).Scan(&recordID)
		if err != nil {
			dbError(w, err)
			return
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO attendance_events(school_id,record_id,actor_id,event_type,previous_status,new_status,reason) VALUES($1,$2,$3,'correction',$4,$5,$6)`, s.schoolID, recordID, u.ID, data["previous_status"], data["new_status"], data["reason"])
		if err != nil {
			dbError(w, err)
			return
		}
	}
	_, err = tx.Exec(r.Context(), `UPDATE attendance_corrections SET status=$3,decided_by=$4,decision_reason=$5,decided_at=clock_timestamp() WHERE school_id=$1 AND id=$2`, s.schoolID, id, in.Decision, u.ID, in.Reason)
	if err != nil {
		dbError(w, err)
		return
	}
	data, err = one(r.Context(), tx, correctionSelect+` AND ac.id=$6`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, u.Scope == auth.ScopeAssignedClasses, u.ID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, tx, u, 200, data, "attendance.corrections."+in.Decision, "attendance_correction", map[string]any{"session_id": data["session_id"], "student_id": data["student_id"]})
}
