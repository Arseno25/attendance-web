package attendance

import (
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

var errState = errors.New("session state conflict")

// Calendar changes apply before opening; open sessions retain their roster/history.
const sessionEligibility = `c.is_active AND y.is_active AND COALESCE(cd.is_school_day, true)`

const sessionSelect = `SELECT se.id::text AS id,se.schedule_id::text AS schedule_id,se.class_id::text AS class_id,c.name AS class_name,se.name,se.date::text,se.timezone,se.starts_at,se.late_after,se.checkout_at,se.closes_at,se.status,se.opened_at,se.closed_at,se.reconciled_at,se.created_at,(SELECT count(*) FROM attendance_session_participants p WHERE p.school_id=se.school_id AND p.session_id=se.id) AS roster_count,(SELECT count(*) FROM attendance_records ar WHERE ar.school_id=se.school_id AND ar.session_id=se.id) AS recorded_count FROM attendance_sessions se JOIN classes c ON c.school_id=se.school_id AND c.id=se.class_id WHERE se.school_id=$1 AND (NOT $2::boolean OR se.class_id=ANY($3::uuid[]))`

func (s *Service) getSession(r *http.Request, tx pgx.Tx, u auth.User, id string, lock bool) (map[string]any, error) {
	q := sessionSelect + ` AND se.id=$4`
	if lock {
		q += ` FOR UPDATE OF se`
	}
	return one(r.Context(), tx, q, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, id)
}
func (s *Service) createSession(ctx context.Context, tx pgx.Tx, scheduleID, day string) (string, error) {
	var eligible bool
	err := tx.QueryRow(ctx, `SELECT coalesce(cd.is_school_day,extract(isodow FROM $3::date)::integer=ANY(a.weekdays)) FROM attendance_schedules a JOIN classes c ON c.school_id=a.school_id AND c.id=a.class_id JOIN academic_years y ON y.school_id=c.school_id AND y.id=c.academic_year_id LEFT JOIN school_calendar_days cd ON cd.school_id=a.school_id AND cd.date=$3::date WHERE a.school_id=$1 AND a.id=$2 AND a.is_active AND c.is_active AND y.is_active AND $3::date BETWEEN y.starts_on AND y.ends_on`, s.schoolID, scheduleID, day).Scan(&eligible)
	if err != nil {
		return "", err
	}
	if !eligible {
		return "", errState
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO attendance_sessions(school_id,schedule_id,class_id,date,name,timezone,starts_at,late_after,closes_at,checkout_at) SELECT school_id,id,class_id,$3::date,name,timezone,($3::date+checkin_start) AT TIME ZONE timezone,($3::date+checkin_cutoff+grace_minutes*interval '1 minute') AT TIME ZONE timezone,($3::date+close_time) AT TIME ZONE timezone,($3::date+checkout_start) AT TIME ZONE timezone FROM attendance_schedules WHERE school_id=$1 AND id=$2 ON CONFLICT(schedule_id,date) DO NOTHING RETURNING id::text`, s.schoolID, scheduleID, day).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = errState
	}
	return id, err
}
func (s *Service) sessions(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	write := r.Method == "POST"
	perm := "attendance.read"
	if write {
		perm = "attendance.manual"
	}
	tx, u, ok := s.begin(w, r, perm, write)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if !write {
		day, class := r.URL.Query().Get("date"), r.URL.Query().Get("class_id")
		if (day != "" && !validDate(day)) || (class != "" && !httpresponse.ValidID(class)) {
			invalid(w, "Filter tanggal atau kelas tidak valid")
			return
		}
		s.list(w, r, tx, sessionSelect+` AND ($4='' OR se.date=NULLIF($4,'')::date) AND ($5='' OR se.class_id=NULLIF($5,'')::uuid)`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, day, class)
		return
	}
	var in struct {
		Schedule string `json:"schedule_id"`
		Date     string `json:"date"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	if !httpresponse.ValidID(in.Schedule) || !validDate(in.Date) {
		invalid(w, "Jadwal dan tanggal wajib valid")
		return
	}
	id, err := s.createSession(r.Context(), tx, in.Schedule, in.Date)
	if errors.Is(err, errState) {
		conflict(w, "Hari tidak efektif atau sesi tanggal ini sudah tersedia")
		return
	}
	if err != nil {
		dbError(w, err)
		return
	}
	data, err := s.getSession(r, tx, u, id, false)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, tx, u, 201, data, "attendance.sessions.created", "attendance_session", nil)
}
func (s *Service) session(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tx, u, ok := s.begin(w, r, "attendance.read", false)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	data, err := s.getSession(r, tx, u, id, false)
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 200, data)
}
func (s *Service) openSession(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `INSERT INTO attendance_session_participants(school_id,session_id,student_id,class_id,enrollment_id,nis,full_name) SELECT se.school_id,se.id,st.id,se.class_id,e.id,st.nis,st.full_name FROM attendance_sessions se JOIN student_enrollments e ON e.school_id=se.school_id AND e.class_id=se.class_id AND e.joined_at<=se.starts_at AND e.left_at>se.starts_at JOIN students st ON st.school_id=e.school_id AND st.id=e.student_id WHERE se.school_id=$1 AND se.id=$2 AND st.status='active'`, s.schoolID, id)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE attendance_sessions SET status='open',opened_at=clock_timestamp() WHERE school_id=$1 AND id=$2`, s.schoolID, id)
	}
	return err
}
func (s *Service) transition(w http.ResponseWriter, r *http.Request, action string) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	perm := "attendance.manual"
	if action == "reconcile" {
		perm = "attendance.approve"
	}
	tx, u, ok := s.begin(w, r, perm, true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	var in struct {
		Reason string `json:"reason"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	if !text(in.Reason, 500) {
		invalid(w, "Alasan tindakan wajib diisi")
		return
	}
	data, err := s.getSession(r, tx, u, id, true)
	if err != nil {
		dbError(w, err)
		return
	}
	state := data["status"].(string)
	var now time.Time
	if err = tx.QueryRow(r.Context(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		dbError(w, err)
		return
	}
	switch action {
	case "open":
		if state == "open" {
			httpresponse.Data(w, 200, data)
			return
		}
		if state != "scheduled" || now.Before(data["starts_at"].(time.Time)) || !now.Before(data["closes_at"].(time.Time)) {
			conflict(w, "Sesi hanya dapat dibuka pada window jadwal")
			return
		}
		var eligible bool
		err = tx.QueryRow(r.Context(), `SELECT `+sessionEligibility+` FROM attendance_sessions se JOIN attendance_schedules a ON a.school_id=se.school_id AND a.id=se.schedule_id JOIN classes c ON c.school_id=se.school_id AND c.id=se.class_id JOIN academic_years y ON y.school_id=c.school_id AND y.id=c.academic_year_id LEFT JOIN school_calendar_days cd ON cd.school_id=se.school_id AND cd.date=se.date WHERE se.school_id=$1 AND se.id=$2`, s.schoolID, id).Scan(&eligible)
		if err != nil {
			dbError(w, err)
			return
		}
		if !eligible {
			conflict(w, "Sesi tidak dapat dibuka pada hari libur atau kelas/tahun ajaran nonaktif")
			return
		}
		err = s.openSession(r.Context(), tx, id)
	case "close":
		if state == "closed" {
			httpresponse.Data(w, 200, data)
			return
		}
		if state != "open" {
			conflict(w, "Hanya sesi terbuka yang dapat ditutup")
			return
		}
		_, err = tx.Exec(r.Context(), `UPDATE attendance_sessions SET status='closed',closed_at=clock_timestamp() WHERE school_id=$1 AND id=$2`, s.schoolID, id)
	case "cancel":
		if state == "cancelled" {
			httpresponse.Data(w, 200, data)
			return
		}
		if state != "scheduled" {
			conflict(w, "Pembatalan hanya untuk sesi yang belum dibuka")
			return
		}
		_, err = tx.Exec(r.Context(), `UPDATE attendance_sessions SET status='cancelled',closed_at=clock_timestamp() WHERE school_id=$1 AND id=$2`, s.schoolID, id)
	case "reconcile":
		if data["reconciled_at"] != nil {
			httpresponse.Data(w, 200, data)
			return
		}
		if state != "closed" || now.Before(data["closes_at"].(time.Time)) {
			conflict(w, "Rekonsiliasi menunggu sesi ditutup dan waktu akhir jadwal")
			return
		}
		var pending int
		err = tx.QueryRow(r.Context(), `SELECT count(*) FROM attendance_corrections WHERE school_id=$1 AND session_id=$2 AND status='pending'`, s.schoolID, id).Scan(&pending)
		if err != nil {
			dbError(w, err)
			return
		}
		if pending > 0 {
			conflict(w, "Selesaikan koreksi pending sebelum rekonsiliasi")
			return
		}
		_, err = tx.Exec(r.Context(), `WITH inserted AS(INSERT INTO attendance_records(school_id,session_id,student_id,status,reason,actor_id) SELECT p.school_id,p.session_id,p.student_id,'absent',$3,$4 FROM attendance_session_participants p WHERE p.school_id=$1 AND p.session_id=$2 ON CONFLICT(session_id,student_id) DO NOTHING RETURNING id,school_id) INSERT INTO attendance_events(school_id,record_id,actor_id,event_type,new_status,reason) SELECT school_id,id,$4,'reconciliation','absent',$3 FROM inserted`, s.schoolID, id, in.Reason, u.ID)
		if err == nil {
			_, err = tx.Exec(r.Context(), `UPDATE attendance_sessions SET reconciled_at=clock_timestamp() WHERE school_id=$1 AND id=$2`, s.schoolID, id)
		}
	}
	if err != nil {
		dbError(w, err)
		return
	}
	data, err = s.getSession(r, tx, u, id, false)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, tx, u, 200, data, "attendance.sessions."+action, "attendance_session", map[string]any{"reason": in.Reason})
}

const participantSelect = `SELECT p.student_id::text AS id,p.student_id::text,p.session_id::text,p.class_id::text,p.nis,p.full_name,p.created_at,ar.id::text AS record_id,coalesce(ar.status,'not_recorded') AS status,ar.check_in_at,ar.check_out_at,ar.reason,coalesce(ar.version,0) AS version,EXISTS(SELECT 1 FROM attendance_corrections ac WHERE ac.school_id=p.school_id AND ac.session_id=p.session_id AND ac.student_id=p.student_id AND ac.status='pending') AS correction_pending FROM attendance_session_participants p LEFT JOIN attendance_records ar ON ar.school_id=p.school_id AND ar.session_id=p.session_id AND ar.student_id=p.student_id WHERE p.school_id=$1 AND p.session_id=$2`

func (s *Service) participants(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	write := r.Method == "POST"
	perm := "attendance.read"
	if write {
		perm = "attendance.manual"
	}
	tx, u, ok := s.begin(w, r, perm, write)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	se, err := s.getSession(r, tx, u, id, write)
	if err != nil {
		dbError(w, err)
		return
	}
	if !write {
		search, status := r.URL.Query().Get("search"), r.URL.Query().Get("status")
		if len(search) > 100 || (status != "" && status != "not_recorded" && status != "pending_review" && !validStatus(status)) {
			invalid(w, "Periksa pilihan pencarian siswa.")
			return
		}
		s.list(w, r, tx, participantSelect+` AND (p.full_name ILIKE $3 OR p.nis ILIKE $3) AND ($4='' OR coalesce(ar.status,'not_recorded')=$4)`, s.schoolID, id, "%"+search+"%", status)
		return
	}
	var in struct {
		Student string `json:"student_id"`
		Reason  string `json:"reason"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	if !httpresponse.ValidID(in.Student) || !text(in.Reason, 500) {
		invalid(w, "Pilih siswa dan isi alasan penambahan.")
		return
	}
	var now time.Time
	if err = tx.QueryRow(r.Context(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		dbError(w, err)
		return
	}
	if se["status"] != "open" || !now.Before(se["closes_at"].(time.Time)) {
		conflict(w, "Roster tambahan hanya pada sesi terbuka")
		return
	}
	var studentID string
	err = tx.QueryRow(r.Context(), `INSERT INTO attendance_session_participants(school_id,session_id,student_id,class_id,enrollment_id,nis,full_name,roster_reason) SELECT st.school_id,$2,st.id,e.class_id,e.id,st.nis,st.full_name,$4 FROM students st JOIN student_enrollments e ON e.school_id=st.school_id AND e.student_id=st.id WHERE st.school_id=$1 AND st.id=$3 AND st.status='active' AND e.class_id=$5 AND e.joined_at<=statement_timestamp() AND e.left_at>statement_timestamp() RETURNING student_id::text`, s.schoolID, id, in.Student, in.Reason, se["class_id"]).Scan(&studentID)
	if err != nil {
		dbError(w, err)
		return
	}
	data, err := one(r.Context(), tx, participantSelect+` AND p.student_id=$3`, s.schoolID, id, studentID)
	if err != nil {
		dbError(w, err)
		return
	}
	err = s.audit(r, tx, u.ID, "attendance.roster.added", "attendance_session", id, map[string]any{"student_id": studentID, "reason": in.Reason})
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 201, data)
}

func (s *Service) eligibleStudents(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tx, u, ok := s.begin(w, r, "attendance.manual", false)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	se, err := s.getSession(r, tx, u, id, false)
	if err != nil {
		dbError(w, err)
		return
	}
	s.list(w, r, tx, `SELECT st.id::text AS id,st.full_name,st.nis,st.created_at FROM students st JOIN student_enrollments e ON e.school_id=st.school_id AND e.student_id=st.id WHERE st.school_id=$1 AND st.status='active' AND e.class_id=$2 AND e.joined_at<=statement_timestamp() AND e.left_at>statement_timestamp() AND NOT EXISTS(SELECT 1 FROM attendance_session_participants p WHERE p.school_id=st.school_id AND p.session_id=$3 AND p.student_id=st.id)`, s.schoolID, se["class_id"], id)
}
