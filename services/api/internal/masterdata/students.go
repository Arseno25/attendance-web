package masterdata

import (
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

const studentScope = `(NOT $2::boolean OR (st.status='active' AND EXISTS(SELECT 1 FROM student_enrollments e WHERE e.school_id=st.school_id AND e.student_id=st.id AND e.class_id=ANY($3::uuid[]) AND e.joined_at<=statement_timestamp() AND e.left_at>statement_timestamp())))`

const studentSelect = `SELECT st.id::text AS id,st.school_id::text AS school_id,st.nis,st.nisn,st.full_name,st.status,st.created_at,st.updated_at,
 latest.class_id::text AS class_id,latest.class_name,latest.academic_year_id::text AS academic_year_id,latest.year_name AS academic_year_name
 FROM students st LEFT JOIN LATERAL(SELECT e.class_id,c.name AS class_name,e.academic_year_id,y.name AS year_name FROM student_enrollments e JOIN classes c ON c.id=e.class_id AND c.school_id=e.school_id JOIN academic_years y ON y.id=e.academic_year_id AND y.school_id=e.school_id WHERE e.student_id=st.id AND e.school_id=st.school_id AND ($4='' OR e.academic_year_id=NULLIF($4,'')::uuid) AND (NOT $2::boolean OR e.class_id=ANY($3::uuid[])) AND e.joined_at<=statement_timestamp() ORDER BY e.joined_at DESC LIMIT 1) latest ON true
 WHERE st.school_id=$1 AND ` + studentScope
const studentColumns = `id::text AS id,school_id::text AS school_id,nis,nisn,full_name,status,created_at,updated_at`

func studentStatus(status string) bool {
	return status == "active" || status == "inactive" || status == "graduated" || status == "transferred"
}
func nullable(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}
func (s *Service) students(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	write := r.Method == "POST"
	perm := "students.read"
	if write {
		perm = "students.write"
	}
	sc, ok := s.begin(w, r, perm, write)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	if !write {
		year, ok := filterID(w, r, "academic_year_id")
		if !ok {
			return
		}
		class, ok := filterID(w, r, "class_id")
		if !ok {
			return
		}
		search := strings.TrimSpace(r.URL.Query().Get("search"))
		status := r.URL.Query().Get("status")
		if len(search) > 100 || (status != "" && !studentStatus(status)) {
			invalid(w, "Filter siswa tidak valid")
			return
		}
		s.list(w, r, sc, studentSelect+` AND ($4='' OR latest.academic_year_id IS NOT NULL) AND ($5='' OR latest.class_id=NULLIF($5,'')::uuid) AND (st.nis ILIKE $6 OR st.full_name ILIKE $6) AND ($7='' OR st.status=$7)`, s.schoolID, sc.classScoped, sc.classes, year, class, "%"+search+"%", status)
		return
	}
	var in struct {
		NIS    string `json:"nis"`
		NISN   string `json:"nisn"`
		Name   string `json:"full_name"`
		Status string `json:"status"`
		Class  string `json:"class_id"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.NIS = strings.TrimSpace(in.NIS)
	in.NISN = strings.TrimSpace(in.NISN)
	in.Name = strings.TrimSpace(in.Name)
	if in.Status == "" {
		in.Status = "active"
	}
	if !text(in.NIS, 30) || len([]rune(in.NISN)) > 32 || !text(in.Name, 120) || !studentStatus(in.Status) || !httpresponse.ValidID(in.Class) {
		invalid(w, "Data siswa tidak valid")
		return
	}
	var id string
	err := sc.tx.QueryRow(r.Context(), `INSERT INTO students(school_id,nis,nisn,full_name,status) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, s.schoolID, in.NIS, nullable(in.NISN), in.Name, in.Status).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	if _, err = s.insertPlacement(r, sc, id, in.Class, "", "Penempatan awal"); err != nil {
		dbError(w, err)
		return
	}
	entity, err := one(r.Context(), sc.tx, studentSelect+` AND st.id=$5`, s.schoolID, sc.classScoped, sc.classes, "", id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 201, entity, "students.created", "student", nil)
}
func (s *Service) student(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "PATCH") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	write := r.Method == "PATCH"
	perm := "students.read"
	if write {
		perm = "students.write"
	}
	sc, ok := s.begin(w, r, perm, write)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	entity, err := one(r.Context(), sc.tx, studentSelect+` AND st.id=$5`, s.schoolID, sc.classScoped, sc.classes, "", id)
	if err != nil {
		dbError(w, err)
		return
	}
	if !write {
		history, err := rows(r.Context(), sc.tx, `SELECT e.id::text AS id,e.class_id::text AS class_id,c.name AS class_name,c.code AS class_code,e.academic_year_id::text AS academic_year_id,y.name AS academic_year_name,e.joined_at,e.left_at,e.reason FROM student_enrollments e JOIN classes c ON c.id=e.class_id AND c.school_id=e.school_id JOIN academic_years y ON y.id=e.academic_year_id AND y.school_id=e.school_id WHERE e.school_id=$1 AND e.student_id=$2 AND (NOT $3::boolean OR e.class_id=ANY($4::uuid[])) ORDER BY e.joined_at DESC`, s.schoolID, id, sc.classScoped, sc.classes)
		if err != nil {
			dbError(w, err)
			return
		}
		if history == nil {
			history = []map[string]any{}
		}
		entity["placements"] = history
		httpresponse.Data(w, 200, entity)
		return
	}
	var in struct {
		NIS    *string `json:"nis"`
		NISN   *string `json:"nisn"`
		Name   *string `json:"full_name"`
		Status *string `json:"status"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	if in.NIS == nil && in.NISN == nil && in.Name == nil && in.Status == nil {
		invalid(w, "Tidak ada perubahan")
		return
	}
	nis := entity["nis"].(string)
	nisn, _ := entity["nisn"].(string)
	name := entity["full_name"].(string)
	status := entity["status"].(string)
	oldStatus := status
	if in.NIS != nil {
		nis = strings.TrimSpace(*in.NIS)
	}
	if in.NISN != nil {
		nisn = strings.TrimSpace(*in.NISN)
	}
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.Status != nil {
		status = *in.Status
	}
	if !text(nis, 30) || len([]rune(nisn)) > 32 || !text(name, 120) || !studentStatus(status) {
		invalid(w, "Data siswa tidak valid")
		return
	}
	entity, err = one(r.Context(), sc.tx, `UPDATE students SET nis=$3,nisn=$4,full_name=$5,status=$6,updated_at=now() WHERE school_id=$1 AND id=$2 RETURNING `+studentColumns, s.schoolID, id, nis, nullable(nisn), name, status)
	if err != nil {
		dbError(w, err)
		return
	}
	if oldStatus == "active" && status != "active" {
		_, err = sc.tx.Exec(r.Context(), `UPDATE student_enrollments SET left_at=clock_timestamp() WHERE school_id=$1 AND student_id=$2 AND joined_at<clock_timestamp() AND left_at>clock_timestamp()`, s.schoolID, id)
		if err != nil {
			dbError(w, err)
			return
		}
	}
	s.finish(w, r, sc, 200, entity, "students.updated", "student", map[string]any{"previous_status": oldStatus, "new_status": status})
}
func (s *Service) classBounds(r *http.Request, sc *scope, classID string) (map[string]any, error) {
	return one(r.Context(), sc.tx, `SELECT c.id::text AS id,c.academic_year_id::text AS academic_year_id,y.starts_on::timestamp AT TIME ZONE y.timezone AS starts_at,(y.ends_on+1)::timestamp AT TIME ZONE y.timezone AS ends_at FROM classes c JOIN academic_years y ON y.id=c.academic_year_id AND y.school_id=c.school_id WHERE c.school_id=$1 AND c.id=$2 AND c.is_active AND y.is_active`, s.schoolID, classID)
}
func (s *Service) insertPlacement(r *http.Request, sc *scope, studentID, classID, effective, reason string) (map[string]any, error) {
	bounds, err := s.classBounds(r, sc, classID)
	if err != nil {
		return nil, err
	}
	var clock time.Time
	if err = sc.tx.QueryRow(r.Context(), `SELECT clock_timestamp()`).Scan(&clock); err != nil {
		return nil, err
	}
	at := clock
	if effective != "" {
		at, err = time.Parse(time.RFC3339Nano, effective)
		if err != nil {
			return nil, err
		}
	}
	start := bounds["starts_at"].(time.Time)
	end := bounds["ends_at"].(time.Time)
	if at.Before(start) || !at.Before(end) || at.After(clock) {
		return nil, errInvalidTime
	}
	return one(r.Context(), sc.tx, `INSERT INTO student_enrollments(school_id,student_id,class_id,academic_year_id,joined_at,left_at,reason) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text AS id,class_id::text AS class_id,academic_year_id::text AS academic_year_id,joined_at,left_at,reason`, s.schoolID, studentID, classID, bounds["academic_year_id"], at, end, reason)
}
func (s *Service) placement(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sc, ok := s.begin(w, r, "students.write", true)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	var in struct {
		Class     string `json:"class_id"`
		Effective string `json:"effective_at"`
		Reason    string `json:"reason"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if !httpresponse.ValidID(in.Class) || !text(in.Reason, 500) {
		invalid(w, "Kelas dan alasan wajib diisi")
		return
	}
	var active bool
	err := sc.tx.QueryRow(r.Context(), `SELECT status='active' FROM students WHERE school_id=$1 AND id=$2 FOR UPDATE`, s.schoolID, id).Scan(&active)
	if err != nil {
		dbError(w, err)
		return
	}
	if !active {
		conflict(w, "Penempatan hanya untuk siswa aktif")
		return
	}
	var at time.Time
	if err = sc.tx.QueryRow(r.Context(), `SELECT clock_timestamp()`).Scan(&at); err != nil {
		dbError(w, err)
		return
	}
	if in.Effective != "" {
		at, err = time.Parse(time.RFC3339Nano, in.Effective)
		if err != nil {
			invalid(w, "Waktu penempatan tidak valid")
			return
		}
	}
	var latest, previousEnd time.Time
	var previousClass string
	err = sc.tx.QueryRow(r.Context(), `SELECT joined_at,left_at,class_id::text FROM student_enrollments WHERE school_id=$1 AND student_id=$2 ORDER BY joined_at DESC LIMIT 1 FOR UPDATE`, s.schoolID, id).Scan(&latest, &previousEnd, &previousClass)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		dbError(w, err)
		return
	}
	if err == nil && (!at.After(latest) || (strings.EqualFold(previousClass, in.Class) && at.Before(previousEnd))) {
		conflict(w, "Penempatan tidak dapat menimpa histori atau memakai kelas yang sama")
		return
	}
	if _, err = sc.tx.Exec(r.Context(), `UPDATE student_enrollments SET left_at=$3 WHERE school_id=$1 AND student_id=$2 AND joined_at<$3 AND left_at>$3`, s.schoolID, id, at); err != nil {
		dbError(w, err)
		return
	}
	entity, err := s.insertPlacement(r, sc, id, in.Class, at.Format(time.RFC3339Nano), in.Reason)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 201, entity, "students.placed", "student_enrollment", map[string]any{"student_id": id, "previous_class_id": previousClass, "new_class_id": entity["class_id"], "effective_at": at})
}
