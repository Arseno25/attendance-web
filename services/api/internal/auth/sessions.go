package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"attendance.local/api/internal/platform/httpresponse"
	"github.com/jackc/pgx/v5"
)

func (s *Service) grants(ctx context.Context, u *User) error {
	u.Grants = []string{}
	u.ClassIDs = []string{}
	rows, err := s.pool.Query(ctx, "SELECT permission FROM permission_grants WHERE school_id=$1 AND user_id=$2 ORDER BY permission", s.schoolID, u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var grant string
		if err = rows.Scan(&grant); err != nil {
			return err
		}
		u.Grants = append(u.Grants, grant)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if u.Role == "teacher" {
		err = s.pool.QueryRow(ctx, `SELECT ARRAY(SELECT DISTINCT a.class_id::text FROM teacher_assignments a JOIN classes c ON c.school_id=a.school_id AND c.id=a.class_id JOIN academic_years y ON y.school_id=c.school_id AND y.id=c.academic_year_id JOIN users t ON t.id=a.teacher_id AND t.school_id=a.school_id WHERE a.school_id=$1 AND a.teacher_id=$2 AND t.is_active AND t.role='teacher' AND a.is_active AND c.is_active AND y.is_active AND a.starts_at<=statement_timestamp() AND a.ends_at>statement_timestamp())`, s.schoolID, u.ID).Scan(&u.ClassIDs)
	}
	return err
}

func (s *Service) session(ctx context.Context, token string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `UPDATE user_sessions AS sess SET last_seen_at=now()
 FROM users u, schools sc WHERE sess.token_hash=$1 AND sess.school_id=$2
 AND u.id=sess.user_id AND u.school_id=sess.school_id AND sc.id=u.school_id
 AND u.is_active AND sc.is_active AND sess.expires_at>statement_timestamp()
 AND sess.last_seen_at>statement_timestamp()-interval '30 minutes'
 RETURNING u.id::text,u.school_id::text,u.email,u.full_name,u.role,u.is_active`, tokenHash(token), s.schoolID).
		Scan(&u.ID, &u.SchoolID, &u.Email, &u.FullName, &u.Role, &u.Active)
	if err == nil {
		err = s.grants(ctx, &u)
	}
	return u, err
}

func (s *Service) sessionData(w http.ResponseWriter, r *http.Request, u User, token string) {
	var school struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Timezone string `json:"timezone"`
	}
	if err := s.pool.QueryRow(r.Context(), "SELECT id::text,name,timezone FROM schools WHERE id=$1", s.schoolID).Scan(&school.ID, &school.Name, &school.Timezone); err != nil {
		databaseError(w, err)
		return
	}
	httpresponse.Data(w, 200, map[string]any{"user": u, "permissions": u.Permissions(), "school": school, "csrf_token": csrfToken(token)})
}

func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	u, token, ok := s.current(w, r, false, "")
	if !ok {
		return
	}
	s.sessionData(w, r, u, token)
}

func (s *Service) login(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") || !s.writeOrigin(w, r) {
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !s.limits.allow("ip:"+hex.EncodeToString(tokenHash(ip)), 30, time.Now()) {
		httpresponse.Error(w, 429, "RATE_LIMITED", "Terlalu banyak percobaan; coba lagi nanti")
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !httpresponse.Decode(w, r, &input) {
		return
	}
	email, err := normalizeEmail(input.Email)
	if err != nil || len(input.Password) == 0 || len(input.Password) > 128 {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Email atau password tidak valid")
		return
	}
	if !s.limits.allow("account:"+hex.EncodeToString(tokenHash(email)), 10, time.Now()) {
		httpresponse.Error(w, 429, "RATE_LIMITED", "Terlalu banyak percobaan; coba lagi nanti")
		return
	}
	select {
	case s.hashing <- struct{}{}:
		defer func() { <-s.hashing }()
	default:
		httpresponse.Error(w, 429, "RATE_LIMITED", "Layanan sedang sibuk; coba kembali")
		return
	}
	var u User
	var hash string
	err = s.pool.QueryRow(r.Context(), `SELECT u.id::text,u.school_id::text,u.email,u.full_name,u.role,u.is_active,u.password_hash
 FROM users u JOIN schools sc ON sc.id=u.school_id WHERE u.school_id=$1 AND u.email=$2 AND sc.is_active`, s.schoolID, email).
		Scan(&u.ID, &u.SchoolID, &u.Email, &u.FullName, &u.Role, &u.Active, &hash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		databaseError(w, err)
		return
	}
	found := err == nil
	if !found {
		hash = s.dummyHash
	}
	if !VerifyPassword(input.Password, hash) || !found || !u.Active {
		_, auditErr := s.pool.Exec(r.Context(), `INSERT INTO audit_logs(school_id,action,target_type,request_id) SELECT id,'auth.login_failed','session',$2 FROM schools WHERE id=$1`, s.schoolID, w.Header().Get("X-Request-ID"))
		if auditErr != nil {
			databaseError(w, auditErr)
			return
		}
		httpresponse.Error(w, 401, "INVALID_CREDENTIALS", "Email atau password tidak sesuai")
		return
	}
	input.Password = ""
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		databaseError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	var eligible bool
	err = tx.QueryRow(r.Context(), "SELECT is_active FROM schools WHERE id=$1 FOR SHARE", s.schoolID).Scan(&eligible)
	if err == nil && eligible {
		err = tx.QueryRow(r.Context(), "SELECT is_active AND password_hash=$3 FROM users WHERE school_id=$1 AND id=$2 FOR SHARE", s.schoolID, u.ID, hash).Scan(&eligible)
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	if !eligible {
		httpresponse.Error(w, 401, "INVALID_CREDENTIALS", "Email atau password tidak sesuai")
		return
	}
	bytes := make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		databaseError(w, err)
		return
	}
	token := hex.EncodeToString(bytes)
	if old, err := r.Cookie(s.cookieName); err == nil {
		_, err = tx.Exec(r.Context(), "DELETE FROM user_sessions WHERE school_id=$1 AND token_hash=$2", s.schoolID, tokenHash(old.Value))
		if err != nil {
			databaseError(w, err)
			return
		}
	}
	_, err = tx.Exec(r.Context(), "DELETE FROM user_sessions WHERE school_id=$1 AND (expires_at<=statement_timestamp() OR last_seen_at<=statement_timestamp()-interval '30 minutes')", s.schoolID)
	if err == nil {
		_, err = tx.Exec(r.Context(), "INSERT INTO user_sessions(token_hash,school_id,user_id,expires_at) VALUES($1,$2,$3,now()+interval '8 hours')", tokenHash(token), s.schoolID, u.ID)
	}
	if err == nil {
		err = audit(r.Context(), tx, s.schoolID, u.ID, "auth.login", "user", u.ID, w.Header().Get("X-Request-ID"))
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	if err = s.grants(r.Context(), &u); err != nil {
		databaseError(w, err)
		return
	}
	s.setCookie(w, token, false)
	s.sessionData(w, r, u, token)
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	u, token, ok := s.current(w, r, true, "")
	if !ok {
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		databaseError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(r.Context(), "DELETE FROM user_sessions WHERE school_id=$1 AND token_hash=$2", s.schoolID, tokenHash(token))
	if err == nil {
		err = audit(r.Context(), tx, s.schoolID, u.ID, "auth.logout", "user", u.ID, w.Header().Get("X-Request-ID"))
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	s.setCookie(w, "", true)
	httpresponse.Data(w, 200, map[string]bool{"logged_out": true})
}

func (s *Service) Bootstrap(ctx context.Context, email, name, password, schoolName string) error {
	email, err := normalizeEmail(email)
	if err != nil || !validName(name) || !validName(schoolName) || !httpresponse.ValidID(s.schoolID) {
		return errors.New("invalid bootstrap input")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, "INSERT INTO schools(id,name) VALUES($1,$2) ON CONFLICT(id) DO NOTHING", s.schoolID, strings.TrimSpace(schoolName))
	if err != nil {
		return err
	}
	var active bool
	err = tx.QueryRow(ctx, "SELECT is_active FROM schools WHERE id=$1 FOR UPDATE", s.schoolID).Scan(&active)
	if err != nil {
		return err
	}
	if !active {
		return errors.New("school inactive")
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE school_id=$1", s.schoolID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("bootstrap already completed")
	}
	var id string
	err = tx.QueryRow(ctx, "INSERT INTO users(school_id,email,full_name,password_hash,role) VALUES($1,$2,$3,$4,'super_admin') RETURNING id::text", s.schoolID, email, strings.TrimSpace(name), hash).Scan(&id)
	if err == nil {
		err = audit(ctx, tx, s.schoolID, id, "auth.bootstrap", "user", id, "bootstrap-cli")
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
