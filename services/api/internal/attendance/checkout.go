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

func (s *Service) checkout(w http.ResponseWriter, r *http.Request) {
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
		Reason  string `json:"reason"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Event = strings.ToLower(in.Event)
	in.Session = strings.ToLower(in.Session)
	in.Student = strings.ToLower(in.Student)
	in.Reason = strings.TrimSpace(in.Reason)
	if !httpresponse.ValidID(in.Event) || !httpresponse.ValidID(in.Session) || !httpresponse.ValidID(in.Student) || !text(in.Reason, 500) {
		invalid(w, "Event, sesi, siswa dan alasan wajib valid")
		return
	}
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(b)
	fingerprint := hex.EncodeToString(sum[:])
	var previous string
	var response map[string]any
	err := tx.QueryRow(r.Context(), `SELECT fingerprint,response FROM request_ledger WHERE school_id=$1 AND actor_id=$2 AND event_id=$3 AND operation='attendance.check_out'`, s.schoolID, u.ID, in.Event).Scan(&previous, &response)
	if err == nil {
		if previous != fingerprint {
			conflict(w, "Event ID digunakan dengan payload berbeda")
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
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM request_ledger WHERE school_id=$1 AND actor_id=$2 AND event_id=$3)`, s.schoolID, u.ID, in.Event).Scan(&reused)
	if err != nil {
		dbError(w, err)
		return
	}
	if reused {
		conflict(w, "Event ID digunakan untuk operasi lain")
		return
	}
	se, err := s.getSession(r, tx, u, in.Session, true)
	if err != nil {
		dbError(w, err)
		return
	}
	var now time.Time
	err = tx.QueryRow(r.Context(), `SELECT clock_timestamp()`).Scan(&now)
	if err != nil {
		dbError(w, err)
		return
	}
	if se["status"] != "open" || now.Before(se["starts_at"].(time.Time)) || !now.Before(se["closes_at"].(time.Time)) {
		conflict(w, "Pencatatan pulang hanya untuk sesi terbuka")
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
	if errors.Is(err, pgx.ErrNoRows) {
		conflict(w, "Siswa belum check-in")
		return
	}
	if err != nil {
		dbError(w, err)
		return
	}
	if record["status"] != "present" && record["status"] != "late" {
		conflict(w, "Pulang hanya untuk siswa hadir atau terlambat")
		return
	}
	outcome := "already_recorded"
	if record["check_out_at"] == nil {
		outcome = "recorded"
		record, err = one(r.Context(), tx, `UPDATE attendance_records SET check_out_at=$3,version=version+1,updated_at=clock_timestamp() WHERE school_id=$1 AND id=$2 RETURNING `+recordColumns, s.schoolID, record["id"], now)
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO attendance_events(school_id,record_id,actor_id,event_type,new_status,reason) VALUES($1,$2,$3,'check_out',$4,$5)`, s.schoolID, record["id"], u.ID, record["status"], in.Reason)
		}
		if err == nil {
			err = s.audit(r, tx, u.ID, "attendance.check_out.recorded", "attendance_record", record["id"].(string), nil)
		}
		if err != nil {
			dbError(w, err)
			return
		}
	}
	response = map[string]any{"outcome": outcome, "record": record}
	payload, _ := json.Marshal(response)
	_, err = tx.Exec(r.Context(), `INSERT INTO request_ledger(school_id,actor_id,event_id,operation,fingerprint,response) VALUES($1,$2,$3,'attendance.check_out',$4,$5::jsonb)`, s.schoolID, u.ID, in.Event, fingerprint, string(payload))
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 201, response)
}
