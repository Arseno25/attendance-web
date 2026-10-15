package attendance

import (
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"net/http"
	"strings"
	"time"
)

const scheduleSelect = `SELECT a.id::text AS id,a.class_id::text AS class_id,c.name AS class_name,y.name AS academic_year_name,a.name,a.timezone,a.weekdays,a.checkin_start::text,a.checkin_cutoff::text,a.grace_minutes,a.close_time::text,a.checkout_start::text,a.is_active,a.created_at,a.updated_at FROM attendance_schedules a JOIN classes c ON c.id=a.class_id AND c.school_id=a.school_id JOIN academic_years y ON y.id=c.academic_year_id AND y.school_id=c.school_id WHERE a.school_id=$1 AND (NOT $2::boolean OR a.class_id=ANY($3::uuid[]))`

type scheduleInput struct {
	Class    string `json:"class_id"`
	Name     string `json:"name"`
	Days     []int  `json:"weekdays"`
	Start    string `json:"checkin_start"`
	Cutoff   string `json:"checkin_cutoff"`
	Close    string `json:"close_time"`
	Checkout string `json:"checkout_start"`
	Grace    int    `json:"grace_minutes"`
	Active   *bool  `json:"is_active"`
}

func minute(v string) (int, bool) {
	t, e := time.Parse("15:04", v)
	return t.Hour()*60 + t.Minute(), e == nil
}
func validSchedule(in scheduleInput) bool {
	start, a := minute(in.Start)
	cut, b := minute(in.Cutoff)
	end, c := minute(in.Close)
	if in.Checkout != "" {
		checkout, ok := minute(in.Checkout)
		if !ok || checkout <= cut+in.Grace || checkout >= end {
			return false
		}
	}
	seen := map[int]bool{}
	for _, d := range in.Days {
		if d < 1 || d > 7 || seen[d] {
			return false
		}
		seen[d] = true
	}
	return text(in.Name, 120) && len(in.Days) > 0 && a && b && c && in.Grace >= 0 && in.Grace <= 120 && start < cut && cut+in.Grace < end
}
func (s *Service) schedules(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	write := r.Method == "POST"
	perm := "attendance.read"
	if write {
		perm = "academics.write"
	}
	tx, u, ok := s.begin(w, r, perm, write)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if !write {
		s.list(w, r, tx, scheduleSelect, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs)
		return
	}
	var in scheduleInput
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !httpresponse.ValidID(in.Class) || !validSchedule(in) {
		invalid(w, "Nama, kelas, hari dan urutan jam jadwal wajib valid")
		return
	}
	var timezone string
	err := tx.QueryRow(r.Context(), `SELECT y.timezone FROM classes c JOIN academic_years y ON y.school_id=c.school_id AND y.id=c.academic_year_id WHERE c.school_id=$1 AND c.id=$2 AND c.is_active AND y.is_active`, s.schoolID, in.Class).Scan(&timezone)
	if err != nil {
		dbError(w, err)
		return
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO attendance_schedules(school_id,class_id,name,timezone,weekdays,checkin_start,checkin_cutoff,grace_minutes,close_time,is_active,checkout_start) VALUES($1,$2,$3,$4,$5,$6::time,$7::time,$8,$9::time,$10,NULLIF($11,'')::time) RETURNING id::text`, s.schoolID, in.Class, in.Name, timezone, in.Days, in.Start, in.Cutoff, in.Grace, in.Close, active, in.Checkout).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	data, err := one(r.Context(), tx, scheduleSelect+` AND a.id=$4`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, tx, u, 201, data, "attendance.schedules.created", "attendance_schedule", nil)
}
func (s *Service) schedule(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "PATCH") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tx, u, ok := s.begin(w, r, "academics.write", true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	data, err := one(r.Context(), tx, scheduleSelect+` AND a.id=$4`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, id)
	if err != nil {
		dbError(w, err)
		return
	}
	var patch struct {
		Name     *string `json:"name"`
		Days     *[]int  `json:"weekdays"`
		Start    *string `json:"checkin_start"`
		Cutoff   *string `json:"checkin_cutoff"`
		Close    *string `json:"close_time"`
		Checkout *string `json:"checkout_start"`
		Grace    *int    `json:"grace_minutes"`
		Active   *bool   `json:"is_active"`
	}
	if !httpresponse.Decode(w, r, &patch) {
		return
	}
	if patch.Name == nil && patch.Days == nil && patch.Start == nil && patch.Cutoff == nil && patch.Close == nil && patch.Checkout == nil && patch.Grace == nil && patch.Active == nil {
		invalid(w, "Tidak ada perubahan")
		return
	}
	in := scheduleInput{Name: data["name"].(string), Start: data["checkin_start"].(string)[:5], Cutoff: data["checkin_cutoff"].(string)[:5], Close: data["close_time"].(string)[:5], Grace: int(data["grace_minutes"].(int32))}
	if value, ok := data["checkout_start"].(string); ok {
		in.Checkout = value[:5]
	}
	for _, d := range data["weekdays"].([]any) {
		in.Days = append(in.Days, int(d.(int32)))
	}
	if patch.Name != nil {
		in.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Days != nil {
		in.Days = *patch.Days
	}
	if patch.Start != nil {
		in.Start = *patch.Start
	}
	if patch.Cutoff != nil {
		in.Cutoff = *patch.Cutoff
	}
	if patch.Close != nil {
		in.Close = *patch.Close
	}
	if patch.Checkout != nil {
		in.Checkout = *patch.Checkout
	}
	if patch.Grace != nil {
		in.Grace = *patch.Grace
	}
	active := data["is_active"].(bool)
	if patch.Active != nil {
		active = *patch.Active
	}
	if !validSchedule(in) {
		invalid(w, "Hari dan urutan jam jadwal tidak valid")
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE attendance_schedules SET name=$3,weekdays=$4,checkin_start=$5::time,checkin_cutoff=$6::time,grace_minutes=$7,close_time=$8::time,is_active=$9,checkout_start=NULLIF($10,'')::time,updated_at=now() WHERE school_id=$1 AND id=$2`, s.schoolID, id, in.Name, in.Days, in.Start, in.Cutoff, in.Grace, in.Close, active, in.Checkout)
	if err != nil {
		dbError(w, err)
		return
	}
	data, err = one(r.Context(), tx, scheduleSelect+` AND a.id=$4`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, tx, u, 200, data, "attendance.schedules.updated", "attendance_schedule", nil)
}
func validDate(v string) bool {
	t, e := time.Parse("2006-01-02", v)
	return e == nil && t.Year() >= 1 && t.Year() < 9999
}
func (s *Service) calendar(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "PUT") {
		return
	}
	day := r.PathValue("date")
	if !validDate(day) {
		invalid(w, "Tanggal tidak valid")
		return
	}
	write := r.Method == "PUT"
	perm := "attendance.read"
	if write {
		perm = "academics.write"
	}
	tx, u, ok := s.begin(w, r, perm, write)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if !write {
		data, err := one(r.Context(), tx, `SELECT $2::text AS date,c.is_school_day,coalesce(c.reason,'Mengikuti hari jadwal') AS reason,(c.date IS NOT NULL) AS is_override FROM schools sc LEFT JOIN school_calendar_days c ON c.school_id=sc.id AND c.date=$2::date WHERE sc.id=$1`, s.schoolID, day)
		if err != nil {
			dbError(w, err)
			return
		}
		httpresponse.Data(w, 200, data)
		return
	}
	var in struct {
		SchoolDay *bool  `json:"is_school_day"`
		Reason    string `json:"reason"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.SchoolDay == nil || !text(in.Reason, 500) {
		invalid(w, "Hari efektif/libur dan alasan wajib diisi")
		return
	}
	data, err := one(r.Context(), tx, `INSERT INTO school_calendar_days(school_id,date,is_school_day,reason) VALUES($1,$2::date,$3,$4) ON CONFLICT(school_id,date) DO UPDATE SET is_school_day=EXCLUDED.is_school_day,reason=EXCLUDED.reason,updated_at=now() RETURNING date::text,is_school_day,reason,updated_at`, s.schoolID, day, *in.SchoolDay, in.Reason)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, tx, u, 200, data, "attendance.calendar.updated", "school_calendar_day", map[string]any{"date": day, "is_school_day": *in.SchoolDay})
}
func (s *Service) calendarList(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	tx, _, ok := s.begin(w, r, "attendance.read", false)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if (from != "" && !validDate(from)) || (to != "" && !validDate(to)) || (from != "" && to != "" && from > to) {
		invalid(w, "Rentang kalender tidak valid")
		return
	}
	s.list(w, r, tx, `SELECT date::text AS id,date::text,is_school_day,reason,updated_at AS created_at FROM school_calendar_days WHERE school_id=$1 AND ($2='' OR date>=NULLIF($2,'')::date) AND ($3='' OR date<=NULLIF($3,'')::date)`, s.schoolID, from, to)
}
