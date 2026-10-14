package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"net/mail"
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
	dummy, err := HashPassword("Comparison-only-unknown-account-1!")
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
	mux.HandleFunc("/api/v1/auth/profile", s.profile)
	mux.HandleFunc("/api/v1/auth/profile/photo", s.photo)
	mux.HandleFunc("/api/v1/school/settings", s.schoolSettings)
	mux.HandleFunc("/api/v1/school/branding", s.branding)
	mux.HandleFunc("/api/v1/school/logo", s.logo)
	mux.HandleFunc("/api/v1/users", s.users)
	mux.HandleFunc("/api/v1/users/{id}", s.updateUser)
	mux.HandleFunc("/api/v1/permissions", s.permissionCatalog)
	mux.HandleFunc("/api/v1/roles", s.roles)
	mux.HandleFunc("/api/v1/roles/{id}", s.roleByID)
	mux.HandleFunc("/api/v1/audit-logs", s.auditLogs)
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

func roleExists(ctx context.Context, tx pgx.Tx, schoolID, key string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM roles WHERE school_id=$1 AND key=$2)", schoolID, key).Scan(&exists)
	return exists, err
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

func audit(ctx context.Context, tx pgx.Tx, school, actor, action, target, id, requestID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs(school_id,actor_id,action,target_type,target_id,request_id) VALUES($1,NULLIF($2,'')::uuid,$3,$4,NULLIF($5,'')::uuid,$6)`, school, actor, action, target, id, requestID)
	return err
}

// Authenticate shares the session, Origin and CSRF boundary with domain handlers.
func (s *Service) Authenticate(w http.ResponseWriter, r *http.Request, write bool) (User, bool) {
	u, _, ok := s.current(w, r, write, "")
	return u, ok
}

func (s *Service) SecureCookies() bool { return s.secure }
func (s *Service) CheckOrigin(w http.ResponseWriter, r *http.Request) bool {
	return s.writeOrigin(w, r)
}
