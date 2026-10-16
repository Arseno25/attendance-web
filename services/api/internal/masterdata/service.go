package masterdata

import (
	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/platform/aiclient"
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
	"time"
	"unicode/utf8"
)

var errInvalidTime = errors.New("invalid academic interval")

type Service struct {
	pool     *pgxpool.Pool
	schoolID string
	auth     *auth.Service
	ai       *aiclient.Client
}

func New(pool *pgxpool.Pool, schoolID string, a *auth.Service) *Service {
	return &Service{pool: pool, schoolID: strings.ToLower(schoolID), auth: a}
}
func (s *Service) SetAI(client *aiclient.Client) { s.ai = client }
func (s *Service) Register(m *http.ServeMux) {
	m.HandleFunc("/api/v1/students/{id}/face-status", s.faceStatus)
	m.HandleFunc("/api/v1/students/{id}/face-photo", s.facePhoto)
	m.HandleFunc("/api/v1/students/{id}/face-preview", s.facePreview)
	m.HandleFunc("/api/v1/biometrics/students", s.biometricStudents)
	m.HandleFunc("/api/v1/students/{id}/enrollments", s.enrollFace)
	m.HandleFunc("/api/v1/students/{id}/face-revoke", s.revokeFace)
	m.HandleFunc("/api/v1/students/{id}/biometrics/revoke", s.revokeFace)
	m.HandleFunc("/api/v1/academic-years", s.years)
	m.HandleFunc("/api/v1/academic-years/{id}", s.year)
	m.HandleFunc("/api/v1/classes", s.classes)
	m.HandleFunc("/api/v1/classes/{id}", s.class)
	m.HandleFunc("/api/v1/students", s.students)
	m.HandleFunc("/api/v1/students/imports/dry-run", s.importDry)
	m.HandleFunc("/api/v1/students/imports/{id}/commit", s.importCommit)
	m.HandleFunc("/api/v1/students/{id}", s.student)
	m.HandleFunc("/api/v1/students/{id}/placements", s.placement)
	m.HandleFunc("/api/v1/class-assignments", s.assignments)
	m.HandleFunc("/api/v1/class-assignments/{id}", s.assignment)
	m.HandleFunc("/api/v1/class-assignment-users", s.teachers)
	m.HandleFunc("/api/v1/master-data/summary", s.summary)
}

type scope struct {
	tx          pgx.Tx
	user        auth.User
	classes     []string
	classScoped bool
}

func (s *Service) begin(w http.ResponseWriter, r *http.Request, permission string, write bool, alternatives ...string) (*scope, bool) {
	tx, user, ok := s.auth.BeginAuthorized(w, r, permission, write, alternatives...)
	if !ok {
		return nil, false
	}
	return &scope{tx: tx, user: user, classes: user.ClassIDs, classScoped: user.Scope == auth.ScopeAssignedClasses}, true
}
func rows(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]map[string]any, error) {
	r, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(r, pgx.RowToMap)
}
func one(ctx context.Context, tx pgx.Tx, query string, args ...any) (map[string]any, error) {
	r, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectOneRow(r, pgx.RowToMap)
}
func dbError(w http.ResponseWriter, err error) {
	if errors.Is(err, errInvalidTime) {
		invalid(w, "Waktu harus berada dalam tahun ajaran aktif dan tidak di masa depan")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Data tidak ditemukan")
		return
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505", "23P01":
			httpresponse.Error(w, 409, "CONFLICT", "Data sudah digunakan atau periode bertumpang tindih")
			return
		case "23503":
			httpresponse.Error(w, 404, "NOT_FOUND", "Referensi tidak tersedia di sekolah ini")
			return
		case "23514", "22P02", "22007", "22008":
			invalid(w, "Data atau rentang waktu tidak valid")
			return
		}
	}
	httpresponse.Error(w, 503, "DATABASE_UNAVAILABLE", "Layanan belum tersedia; coba kembali")
}
func invalid(w http.ResponseWriter, message string) {
	httpresponse.Error(w, 400, "VALIDATION_ERROR", message)
}
func conflict(w http.ResponseWriter, message string) { httpresponse.Error(w, 409, "CONFLICT", message) }
func text(v string, max int) bool {
	return strings.TrimSpace(v) != "" && utf8.RuneCountInString(v) <= max
}
func date(v string) bool { _, err := time.Parse("2006-01-02", v); return err == nil }
func (s *Service) audit(ctx context.Context, sc *scope, action, target, id, requestID string, meta map[string]any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = sc.tx.Exec(ctx, `INSERT INTO audit_logs(school_id,actor_id,action,target_type,target_id,request_id,metadata) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)`, s.schoolID, sc.user.ID, action, target, id, requestID, string(b))
	return err
}
func (s *Service) finish(w http.ResponseWriter, r *http.Request, sc *scope, status int, entity map[string]any, action, target string, meta map[string]any) {
	id, _ := entity["id"].(string)
	err := s.audit(r.Context(), sc, action, target, id, w.Header().Get("X-Request-ID"), meta)
	if err == nil {
		err = sc.tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, status, entity)
}
func (s *Service) list(w http.ResponseWriter, r *http.Request, sc *scope, query string, args ...any) {
	page, size, ok := httpresponse.Pagination(w, r)
	if !ok {
		return
	}
	var total int
	err := sc.tx.QueryRow(r.Context(), "SELECT count(*) FROM ("+query+") AS scoped", args...).Scan(&total)
	if err != nil {
		dbError(w, err)
		return
	}
	// LIMIT values come from validated integers; query fragments are fixed in handlers.
	query += " ORDER BY created_at DESC,id LIMIT " + integer(size) + " OFFSET " + integer((page-1)*size)
	data, err := rows(r.Context(), sc.tx, query, args...)
	if err != nil {
		dbError(w, err)
		return
	}
	if data == nil {
		data = []map[string]any{}
	}
	httpresponse.JSON(w, 200, map[string]any{"data": data, "pagination": map[string]int{"page": page, "page_size": size, "total": total}, "request_id": w.Header().Get("X-Request-ID")})
}
func integer(n int) string { return strconv.Itoa(n) }
func filterID(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	v := r.URL.Query().Get(key)
	if v != "" && !httpresponse.ValidID(v) {
		invalid(w, "Filter tidak valid")
		return "", false
	}
	return strings.ToLower(v), true
}
func pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	v := r.PathValue("id")
	if !httpresponse.ValidID(v) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Data tidak ditemukan")
		return "", false
	}
	return strings.ToLower(v), true
}
