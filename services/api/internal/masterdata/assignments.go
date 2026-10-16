package masterdata

import (
	"context"
	"net/http"
	"strings"
	"time"

	"attendance.local/api/internal/platform/httpresponse"
)

const assignmentSelect = `SELECT a.id::text AS id,a.user_id::text AS user_id,u.full_name AS user_name,a.class_id::text AS class_id,c.code AS class_code,c.name AS class_name,a.academic_year_id::text AS academic_year_id,y.name AS academic_year_name,a.starts_at,a.ends_at,a.is_active,a.revoked_at,a.created_at,(a.is_active AND u.is_active AND r.scope='assigned_classes' AND c.is_active AND y.is_active AND a.starts_at<=statement_timestamp() AND a.ends_at>statement_timestamp()) AS is_current FROM user_class_assignments a JOIN users u ON u.id=a.user_id AND u.school_id=a.school_id JOIN roles r ON r.school_id=u.school_id AND r.key=u.role JOIN classes c ON c.id=a.class_id AND c.school_id=a.school_id JOIN academic_years y ON y.id=a.academic_year_id AND y.school_id=a.school_id WHERE a.school_id=$1 AND (NOT $2::boolean OR a.user_id=$3::uuid)`

func (s *Service) assignments(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	write := r.Method == "POST"
	permission := "academics.read"
	if write {
		permission = "academics.write"
	}
	sc, ok := s.begin(w, r, permission, write)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	if !write {
		classID, valid := filterID(w, r, "class_id")
		if !valid {
			return
		}
		s.list(w, r, sc, assignmentSelect+` AND ($4='' OR a.class_id=NULLIF($4,'')::uuid) AND (NOT $5::boolean OR a.class_id=ANY($6::uuid[]))`, s.schoolID, sc.classScoped, sc.user.ID, classID, sc.classScoped, sc.classes)
		return
	}
	if sc.classScoped {
		httpresponse.Error(w, 403, "FORBIDDEN", "Penugasan kelas hanya dapat dikelola oleh pengelola sekolah")
		return
	}
	var input struct {
		User  string `json:"user_id"`
		Class string `json:"class_id"`
		Start string `json:"starts_at"`
		End   string `json:"ends_at"`
	}
	if !httpresponse.Decode(w, r, &input) {
		return
	}
	input.User = strings.ToLower(input.User)
	input.Class = strings.ToLower(input.Class)
	if !httpresponse.ValidID(input.User) || !httpresponse.ValidID(input.Class) {
		invalid(w, "Pengguna dan kelas wajib dipilih")
		return
	}
	bounds, err := s.classBounds(r, sc, input.Class)
	if err != nil {
		dbError(w, err)
		return
	}
	var eligible bool
	err = sc.tx.QueryRow(r.Context(), `SELECT u.is_active AND r.scope='assigned_classes'
		FROM users u JOIN roles r ON r.school_id=u.school_id AND r.key=u.role
		WHERE u.school_id=$1 AND u.id=$2`, s.schoolID, input.User).Scan(&eligible)
	if err != nil {
		dbError(w, err)
		return
	}
	if !eligible {
		invalid(w, "Penugasan memerlukan pengguna aktif dengan peran berscope kelas")
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
	if input.Start != "" {
		start, err = time.Parse(time.RFC3339Nano, input.Start)
	}
	if err != nil {
		invalid(w, "Waktu mulai tidak valid")
		return
	}
	if input.End != "" {
		end, err = time.Parse(time.RFC3339Nano, input.End)
	}
	if err != nil || !start.Before(end) || start.Before(lower) || end.After(bounds["ends_at"].(time.Time)) {
		invalid(w, "Periode penugasan harus di dalam tahun ajaran")
		return
	}
	var id string
	err = sc.tx.QueryRow(r.Context(), `INSERT INTO user_class_assignments(school_id,user_id,class_id,academic_year_id,starts_at,ends_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, s.schoolID, input.User, input.Class, bounds["academic_year_id"], start, end).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	entity, err := one(r.Context(), sc.tx, assignmentSelect+` AND a.id=$4`, s.schoolID, false, sc.user.ID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 201, entity, "academics.assignments.created", "user_class_assignment", nil)
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
	if sc.classScoped {
		httpresponse.Error(w, 403, "FORBIDDEN", "Penugasan kelas hanya dapat dikelola oleh pengelola sekolah")
		return
	}
	var input struct {
		Active *bool `json:"is_active"`
	}
	if !httpresponse.Decode(w, r, &input) {
		return
	}
	if input.Active == nil || *input.Active {
		invalid(w, "Penugasan hanya dapat dicabut; buat penugasan baru untuk mengganti periode")
		return
	}
	var active bool
	var userID string
	if err := sc.tx.QueryRow(r.Context(), `SELECT is_active,user_id::text FROM user_class_assignments WHERE school_id=$1 AND id=$2`, s.schoolID, id).Scan(&active, &userID); err != nil {
		dbError(w, err)
		return
	}
	if !active {
		conflict(w, "Penugasan sudah dicabut")
		return
	}
	result, err := sc.tx.Exec(r.Context(), `UPDATE user_class_assignments SET is_active=false,revoked_at=clock_timestamp() WHERE school_id=$1 AND id=$2 AND is_active`, s.schoolID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		conflict(w, "Penugasan tidak tersedia atau sudah dicabut")
		return
	}
	if _, err = sc.tx.Exec(r.Context(), `DELETE FROM device_sessions WHERE school_id=$1 AND paired_by=$2`, s.schoolID, userID); err != nil {
		dbError(w, err)
		return
	}
	entity, err := one(r.Context(), sc.tx, assignmentSelect+` AND a.id=$4`, s.schoolID, false, sc.user.ID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 200, entity, "academics.assignments.revoked", "user_class_assignment", nil)
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
	query := `SELECT u.id::text AS id,u.full_name AS name,r.key AS role,u.created_at
		FROM users u JOIN roles r ON r.school_id=u.school_id AND r.key=u.role
		WHERE u.school_id=$1 AND u.is_active AND r.scope='assigned_classes'`
	if sc.classScoped {
		query += ` AND u.id=$2`
	}
	query += ` ORDER BY u.full_name COLLATE "C",u.id`
	args := []any{s.schoolID}
	if sc.classScoped {
		args = append(args, sc.user.ID)
	}
	s.list(w, r, sc, query, args...)
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
	var students, classes, users int
	var year *string
	err := sc.tx.QueryRow(r.Context(), `SELECT count(*) FROM (`+studentSelect+` AND st.status='active') visible`, s.schoolID, sc.classScoped, sc.classes, "").Scan(&students)
	if err == nil {
		err = sc.tx.QueryRow(r.Context(), `SELECT count(*) FROM (`+classSelect+` AND c.is_active AND y.is_active) visible`, s.schoolID, sc.classScoped, sc.classes).Scan(&classes)
	}
	if err == nil {
		err = sc.tx.QueryRow(r.Context(), `SELECT count(DISTINCT a.user_id) FROM (`+assignmentSelect+`) a WHERE a.is_current AND (NOT $2::boolean OR a.user_id::uuid=$3::uuid) AND (NOT $4::boolean OR a.class_id::uuid=ANY($5::uuid[]))`, s.schoolID, false, sc.user.ID, sc.classScoped, sc.classes).Scan(&users)
	}
	if err == nil {
		err = sc.tx.QueryRow(r.Context(), `SELECT max(name) FROM (`+yearSelect+` AND y.is_active) visible`, s.schoolID, sc.classScoped, sc.classes).Scan(&year)
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 200, map[string]any{"students": students, "classes": classes, "assigned_users": users, "active_year": year})
}
