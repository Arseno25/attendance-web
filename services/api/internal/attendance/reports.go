package attendance

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"

	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/platform/httpresponse"
	"github.com/jackc/pgx/v5"
)

type reportFilter struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Class   string `json:"class_id"`
	Year    string `json:"academic_year_id"`
	Student string `json:"student_id"`
	Session string `json:"session_id"`
	Status  string `json:"status"`
	Search  string `json:"search"`
}

func (s *Service) reportFilters(w http.ResponseWriter, r *http.Request, tx pgx.Tx, dashboard bool) (reportFilter, bool) {
	q := r.URL.Query()
	f := reportFilter{From: q.Get("from"), To: q.Get("to"), Class: strings.ToLower(q.Get("class_id")), Year: strings.ToLower(q.Get("academic_year_id")), Student: strings.ToLower(q.Get("student_id")), Session: strings.ToLower(q.Get("session_id")), Status: q.Get("status"), Search: strings.TrimSpace(q.Get("search"))}
	day := q.Get("date")
	if (day != "" || dashboard) && (f.From != "" || f.To != "") {
		invalid(w, "Gunakan tanggal atau rentang laporan, bukan keduanya")
		return f, false
	}
	if day != "" {
		f.From = day
		f.To = day
	}
	if f.From == "" && f.To == "" {
		var today string
		if err := tx.QueryRow(r.Context(), `SELECT (statement_timestamp() AT TIME ZONE timezone)::date::text FROM schools WHERE id=$1`, s.schoolID).Scan(&today); err != nil {
			dbError(w, err)
			return f, false
		}
		f.From = today
		f.To = today
	}
	from, e := time.Parse("2006-01-02", f.From)
	to, e2 := time.Parse("2006-01-02", f.To)
	if e != nil || e2 != nil || !validDate(f.From) || !validDate(f.To) || to.Before(from) || to.Sub(from) > 365*24*time.Hour {
		invalid(w, "Rentang tanggal wajib valid dan maksimal 366 hari")
		return f, false
	}
	for _, id := range []string{f.Class, f.Year, f.Student, f.Session} {
		if id != "" && !httpresponse.ValidID(id) {
			invalid(w, "Filter identitas tidak valid")
			return f, false
		}
	}
	if len([]rune(f.Search)) > 100 || (f.Status != "" && f.Status != "not_recorded" && f.Status != "pending_review" && !validStatus(f.Status)) {
		invalid(w, "Filter status atau pencarian tidak valid")
		return f, false
	}
	return f, true
}
func (f reportFilter) args(s *Service, u auth.User) []any {
	return []any{s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs, f.From, f.To, f.Class, f.Year, f.Student, f.Session, f.Status, "%" + f.Search + "%"}
}

const reportSelect = `SELECT se.id::text AS session_id,p.student_id::text AS student_id,p.class_id::text AS class_id,c.name AS class_name,c.academic_year_id::text AS academic_year_id,y.name AS academic_year_name,se.name AS session_name,se.date::text AS date,se.timezone,se.status AS session_status,p.nis,p.full_name,coalesce(ar.status,'not_recorded') AS status,ar.check_in_at,ar.check_out_at,ar.reason,coalesce(ar.updated_at,p.created_at) AS updated_at
 FROM attendance_session_participants p JOIN attendance_sessions se ON se.school_id=p.school_id AND se.id=p.session_id JOIN classes c ON c.school_id=p.school_id AND c.id=p.class_id JOIN academic_years y ON y.school_id=c.school_id AND y.id=c.academic_year_id LEFT JOIN attendance_records ar ON ar.school_id=p.school_id AND ar.session_id=p.session_id AND ar.student_id=p.student_id
 WHERE p.school_id=$1 AND (NOT $2::boolean OR p.class_id=ANY($3::uuid[])) AND se.status IN('open','closed') AND se.date BETWEEN $4::date AND $5::date AND ($6='' OR p.class_id=NULLIF($6,'')::uuid) AND ($7='' OR c.academic_year_id=NULLIF($7,'')::uuid) AND ($8='' OR p.student_id=NULLIF($8,'')::uuid) AND ($9='' OR se.id=NULLIF($9,'')::uuid) AND ($10='' OR coalesce(ar.status,'not_recorded')=$10) AND (p.nis ILIKE $11 OR p.full_name ILIKE $11)`
const reportTotals = `count(*) AS roster_total,count(DISTINCT student_id) AS students_total,count(DISTINCT session_id) AS sessions_total,count(*) FILTER(WHERE status='present') AS present,count(*) FILTER(WHERE status='late') AS late,count(*) FILTER(WHERE status IN('present','late')) AS attended,count(*) FILTER(WHERE status='excused') AS excused,count(*) FILTER(WHERE status='sick') AS sick,count(*) FILTER(WHERE status='absent') AS absent,count(*) FILTER(WHERE status='not_recorded') AS not_recorded,count(*) FILTER(WHERE status='pending_review') AS pending_review,count(*) FILTER(WHERE status!='not_recorded') AS recorded,coalesce(round(100.0*count(*) FILTER(WHERE status IN('present','late'))/nullif(count(*),0),1),0)::float8 AS attendance_rate`

func (s *Service) totals(r *http.Request, tx pgx.Tx, u auth.User, f reportFilter) (map[string]any, error) {
	return one(r.Context(), tx, `SELECT `+reportTotals+` FROM (`+reportSelect+`) filtered`, f.args(s, u)...)
}

func (s *Service) dailyTotals(r *http.Request, tx pgx.Tx, u auth.User, f reportFilter) ([]map[string]any, error) {
	return rows(r.Context(), tx, `WITH filtered AS (`+reportSelect+`), daily AS(SELECT date,`+reportTotals+` FROM filtered GROUP BY date) SELECT d.day::date::text AS date,coalesce(a.roster_total,0) AS roster_total,coalesce(a.students_total,0) AS students_total,coalesce(a.sessions_total,0) AS sessions_total,coalesce(a.present,0) AS present,coalesce(a.late,0) AS late,coalesce(a.attended,0) AS attended,coalesce(a.excused,0) AS excused,coalesce(a.sick,0) AS sick,coalesce(a.absent,0) AS absent,coalesce(a.not_recorded,0) AS not_recorded,coalesce(a.pending_review,0) AS pending_review,coalesce(a.recorded,0) AS recorded,coalesce(a.attendance_rate,0)::float8 AS attendance_rate FROM generate_series($4::date,$5::date,interval '1 day') d(day) LEFT JOIN daily a ON a.date=d.day::date::text ORDER BY d.day`, f.args(s, u)...)
}

const reportOrder = ` ORDER BY date DESC,class_name,full_name,session_id,student_id`

func (s *Service) report(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	format := r.URL.Query().Get("format")
	if format != "" && format != "json" && format != "csv" {
		invalid(w, "Format laporan tidak valid")
		return
	}
	perm := "attendance.read"
	if format == "csv" {
		perm = "reports.export"
	}
	tx, u, ok := s.begin(w, r, perm, false, "reports.export")
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if format != "csv" && !u.Can("attendance.read", s.schoolID, "") && !u.Can("reports.export", s.schoolID, "") {
		httpresponse.Error(w, 403, "FORBIDDEN", "Anda tidak memiliki izin untuk data ini")
		return
	}
	f, ok := s.reportFilters(w, r, tx, false)
	if !ok {
		return
	}
	if format == "csv" {
		s.exportReport(w, r, tx, u, f)
		return
	}
	page, size, ok := httpresponse.Pagination(w, r)
	if !ok {
		return
	}
	totals, err := s.totals(r, tx, u, f)
	if err != nil {
		dbError(w, err)
		return
	}
	data, err := rows(r.Context(), tx, reportSelect+reportOrder+" LIMIT "+strconv.Itoa(size)+" OFFSET "+strconv.Itoa((page-1)*size), f.args(s, u)...)
	if err != nil {
		dbError(w, err)
		return
	}
	if data == nil {
		data = []map[string]any{}
	}
	daily, err := s.dailyTotals(r, tx, u, f)
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.JSON(w, 200, map[string]any{"data": data, "totals": totals, "daily": daily, "filters": f, "pagination": map[string]any{"page": page, "page_size": size, "total": totals["roster_total"]}, "request_id": w.Header().Get("X-Request-ID")})
}
func (s *Service) dashboard(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	tx, u, ok := s.begin(w, r, "attendance.read", false)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if !u.Can("attendance.read", s.schoolID, "") {
		httpresponse.Error(w, 403, "FORBIDDEN", "Anda tidak memiliki izin untuk data ini")
		return
	}
	f, ok := s.reportFilters(w, r, tx, true)
	if !ok {
		return
	}
	totals, err := s.totals(r, tx, u, f)
	if err != nil {
		dbError(w, err)
		return
	}
	trendFilter := f
	last, _ := time.Parse("2006-01-02", f.To)
	trendFilter.From = last.AddDate(0, 0, -6).Format("2006-01-02")
	trend, err := s.dailyTotals(r, tx, u, trendFilter)
	if err != nil {
		dbError(w, err)
		return
	}
	recent, err := rows(r.Context(), tx, `SELECT * FROM (`+reportSelect+`) filtered WHERE status!='not_recorded' ORDER BY updated_at DESC,session_id,student_id LIMIT 10`, f.args(s, u)...)
	if err != nil {
		dbError(w, err)
		return
	}
	if recent == nil {
		recent = []map[string]any{}
	}
	ops, err := one(r.Context(), tx, `SELECT
 (SELECT count(*) FROM attendance_sessions se WHERE se.school_id=$1 AND (NOT $2::boolean OR se.class_id=ANY($3::uuid[])) AND se.status='open' AND se.starts_at<=clock_timestamp() AND se.closes_at>clock_timestamp()) AS open_sessions,
 (SELECT count(*) FROM camera_devices d WHERE d.school_id=$1 AND d.revoked_at IS NULL) AS total_devices,
 (SELECT count(*) FROM camera_devices d WHERE d.school_id=$1 AND d.revoked_at IS NULL AND d.last_seen_at>=clock_timestamp()-interval '5 minutes') AS online_devices,
 (SELECT count(*) FROM camera_devices d WHERE d.school_id=$1 AND d.revoked_at IS NOT NULL) AS revoked_devices`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs)
	if err != nil {
		dbError(w, err)
		return
	}
	aiState := s.ai.Status(r.Context())
	ops["pipeline_ready"] = aiState.PipelineReady
	ops["recognition_enabled"] = aiState.RecognitionEnabled
	ops["operation_mode"] = aiState.OperationMode
	httpresponse.Data(w, 200, map[string]any{"totals": totals, "trend": trend, "recent": recent, "operations": ops, "filters": f})
}

// CSV escaping prevents new cells; the apostrophe prevents formula-like values on initial spreadsheet import.
func csvText(v string) string {
	trim := strings.TrimLeftFunc(v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
	if strings.HasPrefix(v, "\t") || strings.HasPrefix(v, "\r") || strings.HasPrefix(v, "\n") || strings.ContainsAny(firstRune(trim), "=+-@＝＋－＠") {
		return "'" + v
	}
	return v
}
func firstRune(v string) string {
	for _, r := range v {
		return string(r)
	}
	return ""
}
func (s *Service) exportReport(w http.ResponseWriter, r *http.Request, tx pgx.Tx, u auth.User, f reportFilter) {
	data, err := rows(r.Context(), tx, reportSelect+reportOrder+` LIMIT 10001`, f.args(s, u)...)
	if err != nil {
		dbError(w, err)
		return
	}
	if len(data) > 10000 {
		httpresponse.Error(w, 400, "EXPORT_LIMIT_EXCEEDED", "Ekspor maksimal 10.000 baris; persempit rentang atau filter")
		return
	}
	var buffer bytes.Buffer
	buffer.WriteString("\xef\xbb\xbf")
	// Quote every cell so embedded semicolons cannot split cells in spreadsheet locales.
	writeRow := func(line []string) {
		for i, v := range line {
			line[i] = `"` + strings.ReplaceAll(csvText(v), `"`, `""`) + `"`
		}
		buffer.WriteString(strings.Join(line, ",") + "\r\n")
	}
	writeRow([]string{"Tanggal", "Tahun ajaran", "Kelas", "Sesi", "NIS", "Nama siswa", "Status", "Waktu hadir lokal", "Zona waktu", "Alasan", "Status sesi", "Waktu pulang lokal"})
	labels := map[string]string{"present": "Hadir", "late": "Terlambat", "excused": "Izin", "sick": "Sakit", "absent": "Alfa", "not_recorded": "Belum tercatat", "pending_review": "Perlu ditinjau"}
	zones := map[string]*time.Location{}
	for _, item := range data {
		checkin := ""
		if at, ok := item["check_in_at"].(time.Time); ok {
			name := item["timezone"].(string)
			zone := zones[name]
			if zone == nil {
				var e error
				zone, e = time.LoadLocation(name)
				if e != nil {
					dbError(w, e)
					return
				}
				zones[name] = zone
			}
			checkin = at.In(zone).Format("2006-01-02 15:04:05")
		}
		checkout := ""
		if at, ok := item["check_out_at"].(time.Time); ok {
			zone := zones[item["timezone"].(string)]
			if zone == nil {
				dbError(w, fmt.Errorf("missing checkin zone"))
				return
			}
			checkout = at.In(zone).Format("2006-01-02 15:04:05")
		}
		reason, _ := item["reason"].(string)
		line := []string{item["date"].(string), item["academic_year_name"].(string), item["class_name"].(string), item["session_name"].(string), item["nis"].(string), item["full_name"].(string), labels[item["status"].(string)], checkin, item["timezone"].(string), reason, item["session_status"].(string), checkout}
		writeRow(line)
	}
	auditFilter := f
	auditFilter.Search = ""
	err = s.audit(r, tx, u.ID, "reports.attendance.exported", "attendance_report", "", map[string]any{"filters": auditFilter, "search_filtered": f.Search != "", "row_count": len(data)})
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="attendance_%s_%s.csv"`, f.From, f.To))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(200)
	_, _ = w.Write(buffer.Bytes())
}
