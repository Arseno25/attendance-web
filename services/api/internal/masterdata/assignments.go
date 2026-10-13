package masterdata

import (
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"net/http"
	"time"
)

const assignmentSelect = `SELECT a.id::text AS id,a.teacher_id::text AS teacher_id,u.full_name AS teacher_name,a.class_id::text AS class_id,c.code AS class_code,c.name AS class_name,a.academic_year_id::text AS academic_year_id,y.name AS academic_year_name,a.starts_at,a.ends_at,a.is_active,a.revoked_at,a.created_at,(a.is_active AND u.is_active AND u.role='teacher' AND c.is_active AND y.is_active AND a.starts_at<=statement_timestamp() AND a.ends_at>statement_timestamp()) AS is_current FROM teacher_assignments a JOIN users u ON u.id=a.teacher_id AND u.school_id=a.school_id JOIN classes c ON c.id=a.class_id AND c.school_id=a.school_id JOIN academic_years y ON y.id=a.academic_year_id AND y.school_id=a.school_id WHERE a.school_id=$1 AND ($2!='teacher' OR a.teacher_id=$3::uuid)`

func (s *Service) assignments(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	write := r.Method == "POST"
	perm := "academics.read"
	if write {
		perm = "academics.write"
	}
	sc, ok := s.begin(w, r, perm, write)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	if !write {
		class, ok := filterID(w, r, "class_id")
		if !ok {
			return
		}
		s.list(w, r, sc, assignmentSelect+` AND ($4='' OR a.class_id=NULLIF($4,'')::uuid)`, s.schoolID, sc.user.Role, sc.user.ID, class)
		return
	}
	var in struct {
		Teacher string `json:"teacher_id"`
		Class   string `json:"class_id"`
		Start   string `json:"starts_at"`
		End     string `json:"ends_at"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	if !httpresponse.ValidID(in.Teacher) || !httpresponse.ValidID(in.Class) {
		invalid(w, "Guru dan kelas wajib dipilih")
		return
	}
	bounds, err := s.classBounds(r, sc, in.Class)
	if err != nil {
		dbError(w, err)
		return
	}
	var eligible bool
	err = sc.tx.QueryRow(r.Context(), `SELECT role='teacher' AND is_active FROM users WHERE school_id=$1 AND id=$2`, s.schoolID, in.Teacher).Scan(&eligible)
	if err != nil {
		dbError(w, err)
		return
	}
	if !eligible {
		invalid(w, "Penugasan memerlukan akun guru aktif")
		return
	}
	var start time.Time
	if err = sc.tx.QueryRow(r.Context(), `SELECT clock_timestamp()`).Scan(&start); err != nil {
		dbError(w, err)
		return
	}
	end := bounds["ends_at"].(time.Time)
	lower := bounds["starts_at"].(time.Time)
	if start.Before(lower) {
		start = lower
	}
	if in.Start != "" {
		start, err = time.Parse(time.RFC3339Nano, in.Start)
	}
	if err != nil {
		invalid(w, "Waktu mulai tidak valid")
		return
	}
	if in.End != "" {
		end, err = time.Parse(time.RFC3339Nano, in.End)
	}
	if err != nil || !start.Before(end) || start.Before(lower) || end.After(bounds["ends_at"].(time.Time)) {
		invalid(w, "Periode penugasan harus di dalam tahun ajaran")
		return
	}
	var id string
	err = sc.tx.QueryRow(r.Context(), `INSERT INTO teacher_assignments(school_id,teacher_id,class_id,academic_year_id,starts_at,ends_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, s.schoolID, in.Teacher, in.Class, bounds["academic_year_id"], start, end).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	entity, err := one(r.Context(), sc.tx, assignmentSelect+` AND a.id=$4`, s.schoolID, sc.user.Role, sc.user.ID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 201, entity, "academics.assignments.created", "teacher_assignment", nil)
}
func (s *Service) assignment(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "PATCH") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sc, ok := s.begin(w, r, "academics.write", true)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	var in struct {
		Active *bool `json:"is_active"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	if in.Active == nil || *in.Active {
		invalid(w, "Penugasan hanya dapat dicabut; buat penugasan baru untuk mengganti periode")
		return
	}
	var active bool
	if err := sc.tx.QueryRow(r.Context(), `SELECT is_active FROM teacher_assignments WHERE school_id=$1 AND id=$2`, s.schoolID, id).Scan(&active); err != nil {
		dbError(w, err)
		return
	}
	if !active {
		conflict(w, "Penugasan sudah dicabut")
		return
	}
	result, err := sc.tx.Exec(r.Context(), `UPDATE teacher_assignments SET is_active=false,revoked_at=clock_timestamp() WHERE school_id=$1 AND id=$2 AND is_active`, s.schoolID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		conflict(w, "Penugasan tidak tersedia atau sudah dicabut")
		return
	}
	entity, err := one(r.Context(), sc.tx, assignmentSelect+` AND a.id=$4`, s.schoolID, sc.user.Role, sc.user.ID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 200, entity, "academics.assignments.revoked", "teacher_assignment", nil)
}
func (s *Service) teachers(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	sc, ok := s.begin(w, r, "academics.write", false)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	if sc.user.Role != "admin" && sc.user.Role != "super_admin" {
		httpresponse.Error(w, 403, "FORBIDDEN", "Daftar guru hanya untuk pengelola akademik")
		return
	}
	s.list(w, r, sc, `SELECT id::text AS id,full_name AS name,created_at FROM users WHERE school_id=$1 AND role='teacher' AND is_active`, s.schoolID)
}
func (s *Service) summary(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	sc, ok := s.begin(w, r, "academics.read", false)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	var students, classes, teachers int
	var year *string
	err := sc.tx.QueryRow(r.Context(), `SELECT count(*) FROM (`+studentSelect+` AND st.status='active') visible`, s.schoolID, sc.user.Role, sc.classes, "").Scan(&students)
	if err == nil {
		err = sc.tx.QueryRow(r.Context(), `SELECT count(*) FROM (`+classSelect+` AND c.is_active AND y.is_active) visible`, s.schoolID, sc.user.Role, sc.classes).Scan(&classes)
	}
	if err == nil {
		err = sc.tx.QueryRow(r.Context(), `SELECT count(DISTINCT a.teacher_id) FROM (`+assignmentSelect+`) a WHERE a.is_current`, s.schoolID, sc.user.Role, sc.user.ID).Scan(&teachers)
	}
	if err == nil {
		err = sc.tx.QueryRow(r.Context(), `SELECT max(name) FROM (`+yearSelect+` AND y.is_active) visible`, s.schoolID, sc.user.Role, sc.classes).Scan(&year)
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 200, map[string]any{"students": students, "classes": classes, "teachers": teachers, "active_year": year})
}
