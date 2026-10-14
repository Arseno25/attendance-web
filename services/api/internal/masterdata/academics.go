package masterdata

import (
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"net/http"
	"strings"
)

const yearColumns = `y.id::text AS id,y.school_id::text AS school_id,y.name,y.starts_on::text AS starts_on,y.ends_on::text AS ends_on,y.timezone,y.is_active,y.created_at,y.updated_at`
const classColumns = `c.id::text AS id,c.school_id::text AS school_id,c.academic_year_id::text AS academic_year_id,c.code,c.name,c.level,c.is_active,c.created_at,c.updated_at,y.name AS academic_year_name,y.is_active AS year_active`
const yearSelect = `SELECT ` + yearColumns + ` FROM academic_years y WHERE y.school_id=$1 AND (NOT $2::boolean OR EXISTS(SELECT 1 FROM classes c WHERE c.academic_year_id=y.id AND c.school_id=y.school_id AND c.id=ANY($3::uuid[])))`
const classSelect = `SELECT ` + classColumns + ` FROM classes c JOIN academic_years y ON y.id=c.academic_year_id AND y.school_id=c.school_id WHERE c.school_id=$1 AND (NOT $2::boolean OR c.id=ANY($3::uuid[]))`

func (s *Service) years(w http.ResponseWriter, r *http.Request) {
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
		s.list(w, r, sc, yearSelect, s.schoolID, sc.classScoped, sc.classes)
		return
	}
	var in struct {
		Name   string `json:"name"`
		Start  string `json:"starts_on"`
		End    string `json:"ends_on"`
		Active bool   `json:"is_active"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !text(in.Name, 120) || !date(in.Start) || !date(in.End) || in.Start > in.End {
		invalid(w, "Nama atau tanggal tahun ajaran tidak valid")
		return
	}
	previous, err := s.deactivateYears(r, sc, in.Active, "")
	if err != nil {
		dbError(w, err)
		return
	}
	var id string
	err = sc.tx.QueryRow(r.Context(), `INSERT INTO academic_years(school_id,name,starts_on,ends_on,timezone,is_active) SELECT id,$2,$3::date,$4::date,timezone,$5 FROM schools WHERE id=$1 RETURNING id::text`, s.schoolID, in.Name, in.Start, in.End, in.Active).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	entity, err := one(r.Context(), sc.tx, yearSelect+` AND y.id=$4`, s.schoolID, sc.classScoped, sc.classes, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 201, entity, "academics.years.created", "academic_year", map[string]any{"previous_active_year_ids": previous})
}
func (s *Service) deactivateYears(r *http.Request, sc *scope, activate bool, except string) ([]string, error) {
	ids := []string{}
	if !activate {
		return ids, nil
	}
	rs, err := sc.tx.Query(r.Context(), `UPDATE academic_years SET is_active=false,updated_at=now() WHERE school_id=$1 AND is_active AND ($2='' OR id!=NULLIF($2,'')::uuid) RETURNING id::text`, s.schoolID, except)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	for rs.Next() {
		var id string
		if err = rs.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rs.Err()
}
func (s *Service) year(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "PATCH") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	write := r.Method == "PATCH"
	perm := "academics.read"
	if write {
		perm = "academics.write"
	}
	sc, ok := s.begin(w, r, perm, write)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	entity, err := one(r.Context(), sc.tx, yearSelect+` AND y.id=$4`, s.schoolID, sc.classScoped, sc.classes, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if !write {
		httpresponse.Data(w, 200, entity)
		return
	}
	var in struct {
		Name   *string `json:"name"`
		Start  *string `json:"starts_on"`
		End    *string `json:"ends_on"`
		Active *bool   `json:"is_active"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	if in.Name == nil && in.Start == nil && in.End == nil && in.Active == nil {
		invalid(w, "Tidak ada perubahan")
		return
	}
	name := entity["name"].(string)
	start := entity["starts_on"].(string)
	end := entity["ends_on"].(string)
	active := entity["is_active"].(bool)
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.Start != nil {
		start = *in.Start
	}
	if in.End != nil {
		end = *in.End
	}
	if in.Active != nil {
		active = *in.Active
	}
	if !text(name, 120) || !date(start) || !date(end) || start > end {
		invalid(w, "Nama atau tanggal tahun ajaran tidak valid")
		return
	}
	if start != entity["starts_on"] || end != entity["ends_on"] {
		var count int
		err = sc.tx.QueryRow(r.Context(), `SELECT count(*) FROM classes WHERE school_id=$1 AND academic_year_id=$2`, s.schoolID, id).Scan(&count)
		if err != nil {
			dbError(w, err)
			return
		}
		if count > 0 {
			conflict(w, "Tanggal tahun ajaran yang sudah memiliki kelas tidak dapat diubah")
			return
		}
	}
	previous, err := s.deactivateYears(r, sc, active, id)
	if err != nil {
		dbError(w, err)
		return
	}
	_, err = sc.tx.Exec(r.Context(), `UPDATE academic_years SET name=$3,starts_on=$4::date,ends_on=$5::date,is_active=$6,updated_at=now() WHERE school_id=$1 AND id=$2`, s.schoolID, id, name, start, end, active)
	if err != nil {
		dbError(w, err)
		return
	}
	entity, err = one(r.Context(), sc.tx, yearSelect+` AND y.id=$4`, s.schoolID, sc.classScoped, sc.classes, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 200, entity, "academics.years.updated", "academic_year", map[string]any{"previous_active_year_ids": previous})
}
func (s *Service) classes(w http.ResponseWriter, r *http.Request) {
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
		year, ok := filterID(w, r, "academic_year_id")
		if !ok {
			return
		}
		active := r.URL.Query().Get("active")
		if active != "" && active != "true" && active != "false" {
			invalid(w, "Filter status tidak valid")
			return
		}
		search := strings.TrimSpace(r.URL.Query().Get("search"))
		if len(search) > 100 {
			invalid(w, "Pencarian terlalu panjang")
			return
		}
		s.list(w, r, sc, classSelect+` AND ($4='' OR c.academic_year_id=NULLIF($4,'')::uuid) AND ($5='' OR (c.is_active=NULLIF($5,'')::boolean AND ($5='false' OR y.is_active))) AND (c.code ILIKE $6 OR c.name ILIKE $6)`, s.schoolID, sc.classScoped, sc.classes, year, active, "%"+search+"%")
		return
	}
	var in struct {
		Year   string `json:"academic_year_id"`
		Code   string `json:"code"`
		Name   string `json:"name"`
		Level  string `json:"level"`
		Active *bool  `json:"is_active"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Level = strings.TrimSpace(in.Level)
	if !httpresponse.ValidID(in.Year) || !text(in.Code, 30) || !text(in.Name, 120) || len([]rune(in.Level)) > 30 {
		invalid(w, "Data kelas tidak valid")
		return
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	var id string
	err := sc.tx.QueryRow(r.Context(), `INSERT INTO classes(school_id,academic_year_id,code,name,level,is_active) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, s.schoolID, in.Year, in.Code, in.Name, in.Level, active).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	entity, err := one(r.Context(), sc.tx, classSelect+` AND c.id=$4`, s.schoolID, sc.classScoped, sc.classes, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 201, entity, "academics.classes.created", "class", nil)
}
func (s *Service) class(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "PATCH") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	write := r.Method == "PATCH"
	perm := "academics.read"
	if write {
		perm = "academics.write"
	}
	sc, ok := s.begin(w, r, perm, write)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	entity, err := one(r.Context(), sc.tx, classSelect+` AND c.id=$4`, s.schoolID, sc.classScoped, sc.classes, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if !write {
		httpresponse.Data(w, 200, entity)
		return
	}
	var in struct {
		Code   *string `json:"code"`
		Name   *string `json:"name"`
		Level  *string `json:"level"`
		Active *bool   `json:"is_active"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	if in.Code == nil && in.Name == nil && in.Level == nil && in.Active == nil {
		invalid(w, "Tidak ada perubahan")
		return
	}
	code := entity["code"].(string)
	name := entity["name"].(string)
	level := entity["level"].(string)
	active := entity["is_active"].(bool)
	if in.Code != nil {
		code = strings.TrimSpace(*in.Code)
	}
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.Level != nil {
		level = strings.TrimSpace(*in.Level)
	}
	if in.Active != nil {
		active = *in.Active
	}
	if !text(code, 30) || !text(name, 120) || len([]rune(level)) > 30 {
		invalid(w, "Data kelas tidak valid")
		return
	}
	_, err = sc.tx.Exec(r.Context(), `UPDATE classes SET code=$3,name=$4,level=$5,is_active=$6,updated_at=now() WHERE school_id=$1 AND id=$2`, s.schoolID, id, code, name, level, active)
	if err != nil {
		dbError(w, err)
		return
	}
	entity, err = one(r.Context(), sc.tx, classSelect+` AND c.id=$4`, s.schoolID, sc.classScoped, sc.classes, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 200, entity, "academics.classes.updated", "class", nil)
}
