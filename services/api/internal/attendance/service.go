package attendance

import (
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/platform/aiclient"
	"attendance.local/api/internal/platform/holidays"
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type Service struct {
	pool      *pgxpool.Pool
	schoolID  string
	auth      *auth.Service
	liveSlots chan struct{}
	ai        *aiclient.Client
	holidays  *holidays.Client
	holidayMu sync.Mutex
}

func New(p *pgxpool.Pool, school string, a *auth.Service) *Service {
	return &Service{pool: p, schoolID: strings.ToLower(school), auth: a, liveSlots: make(chan struct{}, 32)}
}
func (s *Service) SetHolidays(client *holidays.Client) { s.holidays = client }
func (s *Service) SetAI(client *aiclient.Client)       { s.ai = client }
func (s *Service) Register(m *http.ServeMux) {
	m.HandleFunc("/api/v1/devices", s.devices)
	m.HandleFunc("/api/v1/devices/{id}/revoke", s.deviceRevoke)
	m.HandleFunc("/api/v1/devices/{id}/pairing-token", s.pairingToken)
	m.HandleFunc("/api/v1/kiosk/session", s.kioskSession)
	m.HandleFunc("/api/v1/kiosk/session-options", s.kioskOptions)
	m.HandleFunc("/api/v1/kiosk/me", s.kioskMe)
	m.HandleFunc("/api/v1/kiosk/capture-challenge", s.captureChallenge)
	m.HandleFunc("/api/v1/kiosk/heartbeat", s.heartbeat)
	m.HandleFunc("/api/v1/kiosk/recognitions", s.recognize)
	m.HandleFunc("/api/v1/kiosk/recognize", s.recognize)
	m.HandleFunc("/api/v1/ws/attendance", s.live)
	m.HandleFunc("/api/v1/reports/attendance", s.report)
	m.HandleFunc("/api/v1/dashboard/summary", s.dashboard)
	m.HandleFunc("/api/v1/attendance-schedules", s.schedules)
	m.HandleFunc("/api/v1/attendance-schedules/{id}", s.schedule)
	m.HandleFunc("/api/v1/school/calendar", s.calendarList)
	m.HandleFunc("/api/v1/school/calendar/month", s.calendarMonth)
	m.HandleFunc("/api/v1/school/calendar/sync", s.calendarSync)
	m.HandleFunc("/api/v1/school/calendar/{date}", s.calendar)
	m.HandleFunc("/api/v1/attendance-sessions", s.sessions)
	m.HandleFunc("/api/v1/attendance-sessions/{id}", s.session)
	m.HandleFunc("/api/v1/attendance-sessions/{id}/participants", s.participants)
	m.HandleFunc("/api/v1/attendance-sessions/{id}/eligible-students", s.eligibleStudents)
	for _, action := range []string{"open", "close", "reconcile", "cancel"} {
		m.HandleFunc("/api/v1/attendance-sessions/{id}/"+action, func(w http.ResponseWriter, r *http.Request) { s.transition(w, r, action) })
	}
	m.HandleFunc("/api/v1/attendances", s.records)
	m.HandleFunc("/api/v1/attendances/manual", s.manual)
	m.HandleFunc("/api/v1/attendances/check-out", s.checkout)
	m.HandleFunc("/api/v1/attendance-corrections", s.corrections)
	m.HandleFunc("/api/v1/attendance-corrections/{id}", s.correction)
	m.HandleFunc("/api/v1/attendance-corrections/{id}/decision", s.decision)
}
func rows(ctx context.Context, tx pgx.Tx, q string, args ...any) ([]map[string]any, error) {
	r, e := tx.Query(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	return pgx.CollectRows(r, pgx.RowToMap)
}
func one(ctx context.Context, tx pgx.Tx, q string, args ...any) (map[string]any, error) {
	r, e := tx.Query(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	return pgx.CollectOneRow(r, pgx.RowToMap)
}
func invalid(w http.ResponseWriter, msg string)  { httpresponse.Error(w, 400, "VALIDATION_ERROR", msg) }
func conflict(w http.ResponseWriter, msg string) { httpresponse.Error(w, 409, "CONFLICT", msg) }
func dbError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Data tidak ditemukan")
		return
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505", "23P01":
			conflict(w, "Data duplikat atau koreksi sedang diproses")
			return
		case "23503":
			httpresponse.Error(w, 404, "NOT_FOUND", "Referensi tidak tersedia di sekolah ini")
			return
		case "23514", "22P02", "22007", "22008":
			invalid(w, "Data atau periode tidak valid")
			return
		}
	}
	httpresponse.Error(w, 503, "DATABASE_UNAVAILABLE", "Layanan belum tersedia; coba kembali")
}
func text(v string, max int) bool {
	return strings.TrimSpace(v) != "" && utf8.RuneCountInString(v) <= max
}
func pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	v := strings.ToLower(r.PathValue("id"))
	if !httpresponse.ValidID(v) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Data tidak ditemukan")
		return "", false
	}
	return v, true
}
func (s *Service) audit(r *http.Request, tx pgx.Tx, actor, action, target, id string, meta map[string]any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(school_id,actor_id,action,target_type,target_id,request_id,metadata) VALUES($1,NULLIF($2,'')::uuid,$3,$4,NULLIF($5,'')::uuid,$6,$7::jsonb)`, s.schoolID, actor, action, target, id, wRequestID(r), string(b))
	return err
}

// Worker and handlers supply request IDs without exposing record bodies in logs.
func wRequestID(r *http.Request) string { return r.Header.Get("X-Internal-Request-ID") }
func (s *Service) begin(w http.ResponseWriter, r *http.Request, perm string, write bool, alternatives ...string) (pgx.Tx, auth.User, bool) {
	r.Header.Set("X-Internal-Request-ID", w.Header().Get("X-Request-ID"))
	return s.auth.BeginAuthorized(w, r, perm, write, alternatives...)
}
func (s *Service) finish(w http.ResponseWriter, r *http.Request, tx pgx.Tx, user auth.User, status int, data map[string]any, action, target string, meta map[string]any) {
	id, _ := data["id"].(string)
	err := s.audit(r, tx, user.ID, action, target, id, meta)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, status, data)
}
func (s *Service) list(w http.ResponseWriter, r *http.Request, tx pgx.Tx, q string, args ...any) {
	page, size, ok := httpresponse.Pagination(w, r)
	if !ok {
		return
	}
	var total int
	err := tx.QueryRow(r.Context(), "SELECT count(*) FROM ("+q+") scoped", args...).Scan(&total)
	if err != nil {
		dbError(w, err)
		return
	}
	data, err := rows(r.Context(), tx, q+" ORDER BY created_at DESC,id LIMIT "+strconv.Itoa(size)+" OFFSET "+strconv.Itoa((page-1)*size), args...)
	if err != nil {
		dbError(w, err)
		return
	}
	if data == nil {
		data = []map[string]any{}
	}
	httpresponse.JSON(w, 200, map[string]any{"data": data, "pagination": map[string]int{"page": page, "page_size": size, "total": total}, "request_id": w.Header().Get("X-Request-ID")})
}
