package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"attendance.local/api/internal/platform/httpresponse"
	"github.com/jackc/pgx/v5"
)

func (s *Service) loadAccess(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, u *User) error {
	u.PermissionsList = []string{}
	u.PermissionKeys = []string{}
	u.ClassIDs = []string{}
	err := q.QueryRow(ctx, `SELECT r.name,r.scope,
		COALESCE(array(SELECT permission FROM role_permissions rp WHERE rp.school_id=r.school_id AND rp.role_id=r.id ORDER BY permission),ARRAY[]::text[])
		FROM roles r WHERE r.school_id=$1 AND r.key=$2`, s.schoolID, u.Role).
		Scan(&u.RoleName, &u.Scope, &u.PermissionsList)
	if err != nil {
		return err
	}
	u.PermissionKeys = append([]string{}, u.PermissionsList...)
	if u.Scope == ScopeAssignedClasses {
		rows, err := q.Query(ctx, `SELECT DISTINCT a.class_id::text
			FROM user_class_assignments a
			JOIN classes c ON c.school_id=a.school_id AND c.id=a.class_id
			JOIN academic_years y ON y.school_id=c.school_id AND y.id=c.academic_year_id
		WHERE a.school_id=$1 AND a.user_id=$2 AND a.is_active AND c.is_active AND y.is_active
			AND a.starts_at<=statement_timestamp() AND a.ends_at>statement_timestamp()
		ORDER BY a.class_id::text`, s.schoolID, u.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var classID string
			if err = rows.Scan(&classID); err != nil {
				return err
			}
			u.ClassIDs = append(u.ClassIDs, classID)
		}
		return rows.Err()
	}
	return nil
}

func (s *Service) loadSessionAccess(ctx context.Context, u *User) error {
	return s.loadAccess(ctx, s.pool, u)
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
		err = s.loadSessionAccess(ctx, &u)
	}
	return u, err
}

func (s *Service) sessionData(w http.ResponseWriter, r *http.Request, u User, token string) {
	var school struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Timezone string `json:"timezone"`
		AppName  string `json:"app_name"`
		LogoURL  string `json:"logo_url"`
	}
	var logoRevision, photoRevision string
	if err := s.pool.QueryRow(r.Context(), "SELECT id::text,name,timezone,app_name,COALESCE(md5(logo),''),COALESCE((SELECT md5(avatar) FROM users WHERE school_id=$1 AND id=$2),'') FROM schools WHERE id=$1", s.schoolID, u.ID).Scan(&school.ID, &school.Name, &school.Timezone, &school.AppName, &logoRevision, &photoRevision); err != nil {
		databaseError(w, err)
		return
	}
	school.LogoURL = imageURL("/school/logo", logoRevision)
	u.AvatarURL = imageURL("/auth/profile/photo", photoRevision)
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
	ip := s.clientIP(r)
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
	if err != nil || len(input.Password) == 0 {
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
	if err = s.loadSessionAccess(r.Context(), &u); err != nil {
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
	if err != nil || !validName(name) || !validName(schoolName) || (s.schoolID != "" && !httpresponse.ValidID(s.schoolID)) {
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
	schoolID := s.schoolID
	if schoolID == "" {
		// Serialize first-time setup so concurrent bootstraps cannot create separate schools.
		if _, err = tx.Exec(ctx, "LOCK TABLE schools IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return err
		}
		schoolID, err = ResolveSchoolID(ctx, tx)
		if errors.Is(err, ErrSchoolNotConfigured) {
			err = tx.QueryRow(ctx, "INSERT INTO schools(name) VALUES($1) RETURNING id::text", strings.TrimSpace(schoolName)).Scan(&schoolID)
		}
	} else {
		_, err = tx.Exec(ctx, "INSERT INTO schools(id,name) VALUES($1,$2) ON CONFLICT(id) DO NOTHING", schoolID, strings.TrimSpace(schoolName))
	}
	if err != nil {
		return err
	}
	if err = seedBuiltinRoles(ctx, tx, schoolID); err != nil {
		return err
	}
	var active bool
	err = tx.QueryRow(ctx, "SELECT is_active FROM schools WHERE id=$1 FOR UPDATE", schoolID).Scan(&active)
	if err != nil {
		return err
	}
	if !active {
		return errors.New("school inactive")
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE school_id=$1", schoolID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("bootstrap already completed")
	}
	var id string
	err = tx.QueryRow(ctx, "INSERT INTO users(school_id,email,full_name,password_hash,role) VALUES($1,$2,$3,$4,'super_admin') RETURNING id::text", schoolID, email, strings.TrimSpace(name), hash).Scan(&id)
	if err == nil {
		err = audit(ctx, tx, schoolID, id, "auth.bootstrap", "user", id, "bootstrap-cli")
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EndCurrentSession participates in a handoff transaction; clear the cookie only after commit.
func (s *Service) EndCurrentSession(ctx context.Context, tx pgx.Tx, r *http.Request) error {
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "DELETE FROM user_sessions WHERE school_id=$1 AND token_hash=$2", s.schoolID, tokenHash(cookie.Value))
	return err
}
func (s *Service) ClearSessionCookie(w http.ResponseWriter) { s.setCookie(w, "", true) }
