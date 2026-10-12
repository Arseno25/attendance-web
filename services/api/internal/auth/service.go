package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"time"

	"attendance.local/api/internal/platform/httpresponse"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool                                    *pgxpool.Pool
	schoolID, origin, cookieName, dummyHash string
	secure                                  bool
	limits                                  limiter
	hashing                                 chan struct{}
}

func New(pool *pgxpool.Pool, schoolID, origin string) (*Service, error) {
	dummy, err := HashPassword("comparison-only-unknown-account")
	if err != nil {
		return nil, err
	}
	s := &Service{pool: pool, schoolID: strings.ToLower(schoolID), origin: origin, cookieName: "attendance_session", secure: strings.HasPrefix(origin, "https://"), dummyHash: dummy, hashing: make(chan struct{}, 2)}
	if s.secure {
		s.cookieName = "__Host-attendance_session"
	}
	return s, nil
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/auth/login", s.login)
	mux.HandleFunc("/api/v1/auth/logout", s.logout)
	mux.HandleFunc("/api/v1/auth/me", s.me)
	mux.HandleFunc("/api/v1/users", s.users)
	mux.HandleFunc("/api/v1/users/{id}", s.updateUser)
	mux.HandleFunc("/api/v1/audit-logs", s.auditLogs)
}

func method(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	if slices.Contains(allowed, r.Method) {
		return true
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	httpresponse.Error(w, 405, "METHOD_NOT_ALLOWED", "Method tidak diizinkan")
	return false
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Gunakan JSON yang valid")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(target); err == nil {
		err = d.Decode(new(any))
		if err == io.EOF {
			return true
		}
	}
	httpresponse.Error(w, 400, "VALIDATION_ERROR", "Request tidak valid atau terlalu besar")
	return false
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || len(email) > 254 {
		return "", errors.New("invalid email")
	}
	return email, nil
}

func validName(name string) bool { return len(strings.TrimSpace(name)) > 0 && len([]rune(name)) <= 120 }
func validID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	b, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil && len(b) == 16
}
func validRole(role string) bool { _, ok := rolePermissions[role]; return ok }
func validGrants(grants []string) bool {
	for i, grant := range grants {
		if !slices.Contains(Grantable, grant) || slices.Contains(grants[:i], grant) {
			return false
		}
	}
	return true
}

func grantsForRole(role string, grants []string) bool {
	for _, grant := range grants {
		if !GrantAllowed(role, grant) {
			return false
		}
	}
	return true
}
func tokenHash(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }
func csrfToken(token string) string {
	sum := sha256.Sum256([]byte("attendance-csrf-v1:" + token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) setCookie(w http.ResponseWriter, token string, remove bool) {
	c := &http.Cookie{Name: s.cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: 8 * 60 * 60, Expires: time.Now().Add(8 * time.Hour)}
	if remove {
		c.MaxAge = -1
		c.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, c)
}

func (s *Service) writeOrigin(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Origin") != s.origin {
		httpresponse.Error(w, 403, "FORBIDDEN", "Origin tidak diizinkan")
		return false
	}
	return true
}

func (s *Service) current(w http.ResponseWriter, r *http.Request, write bool, permission string) (User, string, bool) {
	if write && !s.writeOrigin(w, r) {
		return User{}, "", false
	}
	cookie, err := r.Cookie(s.cookieName)
	var u User
	if err == nil && len(cookie.Value) == 64 {
		if _, decodeErr := hex.DecodeString(cookie.Value); decodeErr != nil {
			err = pgx.ErrNoRows
		} else {
			u, err = s.session(r.Context(), cookie.Value)
		}
	} else {
		err = pgx.ErrNoRows
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.setCookie(w, "", true)
			httpresponse.Error(w, 401, "UNAUTHENTICATED", "Silakan masuk kembali")
		} else {
			databaseError(w, err)
		}
		return User{}, "", false
	}
	if write && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrfToken(cookie.Value))) != 1 {
		httpresponse.Error(w, 403, "CSRF_INVALID", "Sesi verifikasi tidak valid; muat ulang halaman")
		return User{}, "", false
	}
	if permission != "" && !u.Can(permission, s.schoolID, "") {
		httpresponse.Error(w, 403, "FORBIDDEN", "Anda tidak memiliki izin untuk tindakan ini")
		return User{}, "", false
	}
	return u, cookie.Value, true
}

func databaseError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		httpresponse.Error(w, 409, "CONFLICT", "Data sudah digunakan")
		return
	}
	httpresponse.Error(w, 503, "DATABASE_UNAVAILABLE", "Layanan belum tersedia; coba kembali")
}

func pagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	page, size := 1, 20
	var err error
	if q := r.URL.Query().Get("page"); q != "" {
		page, err = strconv.Atoi(q)
		if err != nil {
			page = 0
		}
	}
	if q := r.URL.Query().Get("page_size"); q != "" {
		size, err = strconv.Atoi(q)
		if err != nil {
			size = 0
		}
	}
	if page < 1 || page > 100000 || size < 1 || size > 100 {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Pagination tidak valid")
		return 0, 0, false
	}
	return page, size, true
}

func audit(ctx context.Context, tx pgx.Tx, school, actor, action, target, id, requestID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs(school_id,actor_id,action,target_type,target_id,request_id) VALUES($1,NULLIF($2,'')::uuid,$3,$4,NULLIF($5,'')::uuid,$6)`, school, actor, action, target, id, requestID)
	return err
}
