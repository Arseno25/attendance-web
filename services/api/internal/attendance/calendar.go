package attendance

import (
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"net/http"
	"strings"
	"time"
)

func validDate(v string) bool {
	t, e := time.Parse("2006-01-02", v)
	return e == nil && t.Year() >= 1 && t.Year() < 9999
}

// School overrides take priority; an unknown holiday year cannot create automatic absences.
func calendarEligibility(day, requireData, weekday string) string {
	return `COALESCE(cd.is_school_day, (` + weekday + `) AND NOT EXISTS(SELECT 1 FROM school_holidays h WHERE h.school_id=c.school_id AND h.date=` + day + `) AND (NOT ` + requireData + `::boolean OR EXISTS(SELECT 1 FROM school_holiday_sync hs WHERE hs.school_id=c.school_id AND hs.year=extract(year FROM ` + day + `) AND hs.synced_at IS NOT NULL)))`
}

const calendarDays = `SELECT d::date::text AS date,COALESCE(cd.is_school_day,CASE WHEN h.name IS NOT NULL THEN false END) AS is_school_day,
 COALESCE(cd.reason,h.name,'Mengikuti hari pada jadwal kelas') AS reason,(cd.date IS NOT NULL) AS is_override,
 CASE WHEN cd.date IS NOT NULL THEN 'school' WHEN h.national THEN 'national_holiday' WHEN h.name IS NOT NULL THEN 'joint_leave' ELSE 'schedule' END AS source,COALESCE(h.name,'') AS holiday_name
 FROM generate_series($2::date,$3::date,interval '1 day') d
 LEFT JOIN school_calendar_days cd ON cd.school_id=$1 AND cd.date=d::date
 LEFT JOIN LATERAL(SELECT string_agg(name,', ' ORDER BY type,name) AS name,bool_or(type='national_holiday') AS national FROM school_holidays WHERE school_id=$1 AND date=d::date) h ON true`

func schoolCalendarWrite(w http.ResponseWriter, u auth.User) bool {
	if u.Scope != auth.ScopeSchool {
		httpresponse.Error(w, 403, "FORBIDDEN", "Pengaturan kalender sekolah hanya dapat diubah pengelola sekolah.")
		return false
	}
	return true
}
func (s *Service) calendar(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "PUT", "DELETE") {
		return
	}
	day := r.PathValue("date")
	if !validDate(day) {
		invalid(w, "Tanggal tidak valid")
		return
	}
	write := r.Method != "GET"
	perm := "attendance.read"
	if write {
		perm = "academics.write"
	}
	tx, u, ok := s.begin(w, r, perm, write, "academics.write")
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if !write {
		data, err := one(r.Context(), tx, calendarDays, s.schoolID, day, day)
		if err != nil {
			dbError(w, err)
			return
		}
		httpresponse.Data(w, 200, data)
		return
	}
	if !schoolCalendarWrite(w, u) {
		return
	}
	if r.Method == "DELETE" {
		if _, err := tx.Exec(r.Context(), `DELETE FROM school_calendar_days WHERE school_id=$1 AND date=$2::date`, s.schoolID, day); err != nil {
			dbError(w, err)
			return
		}
		s.finish(w, r, tx, u, 200, map[string]any{"date": day}, "attendance.calendar.reset", "school_calendar_day", map[string]any{"date": day})
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
		invalid(w, "Hari sekolah/libur dan alasan wajib diisi")
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
	if !httpresponse.Method(w, r, "GET", "PUT") {
		return
	}
	write := r.Method == "PUT"
	perm := "attendance.read"
	if write {
		perm = "academics.write"
	}
	tx, u, ok := s.begin(w, r, perm, write, "academics.write")
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if write {
		if !schoolCalendarWrite(w, u) {
			return
		}
		var in struct {
			From      string `json:"from"`
			To        string `json:"to"`
			SchoolDay *bool  `json:"is_school_day"`
			Reason    string `json:"reason"`
		}
		if !httpresponse.Decode(w, r, &in) {
			return
		}
		in.Reason = strings.TrimSpace(in.Reason)
		start, a := time.Parse("2006-01-02", in.From)
		end, b := time.Parse("2006-01-02", in.To)
		if a != nil || b != nil || !validDate(in.From) || !validDate(in.To) || in.To < in.From || end.Sub(start) > 365*24*time.Hour || in.SchoolDay == nil || !text(in.Reason, 500) {
			invalid(w, "Isi rentang tanggal maksimal 366 hari, jenis hari, dan alasan yang valid.")
			return
		}
		result, err := tx.Exec(r.Context(), `INSERT INTO school_calendar_days(school_id,date,is_school_day,reason) SELECT $1,d::date,$4,$5 FROM generate_series($2::date,$3::date,interval '1 day') d ON CONFLICT(school_id,date) DO UPDATE SET is_school_day=EXCLUDED.is_school_day,reason=EXCLUDED.reason,updated_at=now()`, s.schoolID, in.From, in.To, *in.SchoolDay, in.Reason)
		if err != nil {
			dbError(w, err)
			return
		}
		s.finish(w, r, tx, u, 200, map[string]any{"from": in.From, "to": in.To, "days": result.RowsAffected()}, "attendance.calendar.range_updated", "school_calendar_day", map[string]any{"from": in.From, "to": in.To, "is_school_day": *in.SchoolDay})
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if (from != "" && !validDate(from)) || (to != "" && !validDate(to)) || (from != "" && to != "" && from > to) {
		invalid(w, "Rentang kalender tidak valid")
		return
	}
	s.list(w, r, tx, `SELECT date::text AS id,date::text,is_school_day,reason,updated_at AS created_at FROM school_calendar_days WHERE school_id=$1 AND ($2='' OR date>=NULLIF($2,'')::date) AND ($3='' OR date<=NULLIF($3,'')::date)`, s.schoolID, from, to)
}
func (s *Service) calendarMonth(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	month, err := time.Parse("2006-01", r.URL.Query().Get("month"))
	if err != nil || month.Year() < 1900 || month.Year() > 2200 {
		invalid(w, "Bulan kalender tidak valid")
		return
	}
	tx, _, ok := s.begin(w, r, "attendance.read", false, "academics.write")
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	days, err := rows(r.Context(), tx, calendarDays, s.schoolID, month.Format("2006-01-02"), month.AddDate(0, 1, -1).Format("2006-01-02"))
	if err != nil {
		dbError(w, err)
		return
	}
	sync, err := one(r.Context(), tx, `SELECT $2::integer AS year,hs.synced_at,hs.attempted_at,COALESCE(hs.status,'not_synced') AS status FROM schools sc LEFT JOIN school_holiday_sync hs ON hs.school_id=sc.id AND hs.year=$2 WHERE sc.id=$1`, s.schoolID, month.Year())
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 200, map[string]any{"days": days, "sync": sync})
}
