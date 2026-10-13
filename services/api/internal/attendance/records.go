package attendance

import (
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

func validStatus(v string) bool {
	return v == "present" || v == "late" || v == "excused" || v == "sick" || v == "absent"
}

const recordColumns = `id::text AS id,session_id::text AS session_id,student_id::text AS student_id,status,check_in_at,check_out_at,reason,actor_id::text AS actor_id,version,created_at,updated_at`

func (s *Service) records(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	tx, u, ok := s.begin(w, r, "attendance.read", false)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	id := r.URL.Query().Get("session_id")
	if id != "" && !httpresponse.ValidID(id) {
		invalid(w, "Sesi tidak valid")
		return
	}
	s.list(w, r, tx, `SELECT ar.id::text AS id,ar.session_id::text,ar.student_id::text,p.nis,p.full_name,ar.status,ar.check_in_at,ar.check_out_at,ar.reason,ar.version,ar.created_at FROM attendance_records ar JOIN attendance_session_participants p ON p.school_id=ar.school_id AND p.session_id=ar.session_id AND p.student_id=ar.student_id WHERE ar.school_id=$1 AND ($2::text!='teacher' OR p.class_id=ANY($3::uuid[])) AND ($4='' OR ar.session_id=NULLIF($4,'')::uuid)`, s.schoolID, u.Role, u.ClassIDs, id)
}
func (s *Service) manual(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	tx, u, ok := s.begin(w, r, "attendance.manual", true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	var in struct {
		Event   string `json:"event_id"`
		Session string `json:"session_id"`
		Student string `json:"student_id"`
		Status  string `json:"status"`
		Reason  string `json:"reason"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	in.Event = strings.ToLower(in.Event)
	in.Session = strings.ToLower(in.Session)
	in.Student = strings.ToLower(in.Student)
	if !httpresponse.ValidID(in.Event) || !httpresponse.ValidID(in.Session) || !httpresponse.ValidID(in.Student) || !text(in.Reason, 500) || (in.Status != "present" && in.Status != "excused" && in.Status != "sick") {
		invalid(w, "Sesi, siswa, status, alasan dan event ID wajib valid")
		return
	}
	b, _ := json.Marshal(in)
	hash := sha256.Sum256(b)
	fingerprint := hex.EncodeToString(hash[:])
	var previous string
	var response map[string]any
	err := tx.QueryRow(r.Context(), `SELECT fingerprint,response FROM request_ledger WHERE school_id=$1 AND actor_id=$2 AND event_id=$3 AND operation='attendance.manual'`, s.schoolID, u.ID, in.Event).Scan(&previous, &response)
	if err == nil {
		if previous != fingerprint {
			conflict(w, "Event ID sudah digunakan dengan payload berbeda")
			return
		}
		httpresponse.Data(w, 201, response)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		dbError(w, err)
		return
	}
	var reused bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM request_ledger WHERE school_id=$1 AND actor_id=$2 AND event_id=$3)`, s.schoolID, u.ID, in.Event).Scan(&reused); err != nil {
		dbError(w, err)
		return
	}
	if reused {
		conflict(w, "Event ID sudah digunakan untuk operasi berbeda")
		return
	}
	se, err := s.getSession(r, tx, u, in.Session, true)
	if err != nil {
		dbError(w, err)
		return
	}
	var now time.Time
	if err = tx.QueryRow(r.Context(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		dbError(w, err)
		return
	}
	if se["status"] != "open" || now.Before(se["starts_at"].(time.Time)) || !now.Before(se["closes_at"].(time.Time)) {
		httpresponse.Error(w, 409, "SESSION_NOT_OPEN", "Sesi tidak terbuka; gunakan permintaan koreksi untuk histori")
		return
	}
	var eligible bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM attendance_session_participants WHERE school_id=$1 AND session_id=$2 AND student_id=$3)`, s.schoolID, in.Session, in.Student).Scan(&eligible)
	if err != nil {
		dbError(w, err)
		return
	}
	if !eligible {
		httpresponse.Error(w, 404, "NOT_FOUND", "Siswa tidak termasuk roster sesi")
		return
	}
	var pending bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM attendance_corrections WHERE school_id=$1 AND session_id=$2 AND student_id=$3 AND status='pending')`, s.schoolID, in.Session, in.Student).Scan(&pending)
	if err != nil {
		dbError(w, err)
		return
	}
	if pending {
		conflict(w, "Selesaikan koreksi pending terlebih dahulu")
		return
	}
	record, err := one(r.Context(), tx, `SELECT `+recordColumns+` FROM attendance_records WHERE school_id=$1 AND session_id=$2 AND student_id=$3 FOR UPDATE`, s.schoolID, in.Session, in.Student)
	outcome := "recorded"
	if err == nil {
		status := record["status"].(string)
		if !((in.Status == "present" && (status == "present" || status == "late")) || in.Status == status) {
			conflict(w, "Status sudah tercatat; perubahan memerlukan koreksi")
			return
		}
		outcome = "already_recorded"
	} else if !errors.Is(err, pgx.ErrNoRows) {
		dbError(w, err)
		return
	} else {
		status := in.Status
		var checkin any
		if status == "present" {
			checkin = now
			if now.After(se["late_after"].(time.Time)) {
				status = "late"
			}
		}
		record, err = one(r.Context(), tx, `INSERT INTO attendance_records(school_id,session_id,student_id,status,check_in_at,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+recordColumns, s.schoolID, in.Session, in.Student, status, checkin, in.Reason, u.ID)
		if err != nil {
			dbError(w, err)
			return
		}
		kind := "manual_adjustment"
		if checkin != nil {
			kind = "check_in"
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO attendance_events(school_id,record_id,actor_id,event_type,new_status,reason) VALUES($1,$2,$3,$4,$5,$6)`, s.schoolID, record["id"], u.ID, kind, status, in.Reason)
		if err == nil {
			err = s.audit(r, tx, u.ID, "attendance.manual.recorded", "attendance_record", record["id"].(string), map[string]any{"session_id": in.Session, "student_id": in.Student, "status": status})
		}
		if err != nil {
			dbError(w, err)
			return
		}
	}
	response = map[string]any{"outcome": outcome, "record": record}
	payload, _ := json.Marshal(response)
	_, err = tx.Exec(r.Context(), `INSERT INTO request_ledger(school_id,actor_id,event_id,operation,fingerprint,response) VALUES($1,$2,$3,'attendance.manual',$4,$5::jsonb)`, s.schoolID, u.ID, in.Event, fingerprint, string(payload))
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 201, response)
}
