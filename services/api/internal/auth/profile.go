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

func (s *Service) profile(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "PATCH") {
		return
	}
	actor, token, ok := s.current(w, r, true, "")
	if !ok {
		return
	}
	var input struct {
		FullName        string `json:"full_name"`
		Email           string `json:"email"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !httpresponse.Decode(w, r, &input) {
		return
	}
	email, err := normalizeEmail(input.Email)
	name := strings.TrimSpace(input.FullName)
	passwordChanged := input.NewPassword != ""
	if err != nil || !validName(name) {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Nama atau email tidak valid")
		return
	}
	if passwordChanged && !validNewPassword(input.NewPassword) {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", passwordRule)
		return
	}
	sensitive := email != actor.Email || passwordChanged
	var expectedHash, replacementHash string
	if sensitive {
		if input.CurrentPassword == "" {
			httpresponse.Error(w, 400, "CURRENT_PASSWORD_REQUIRED", "Password saat ini diperlukan untuk mengubah email atau password")
			return
		}
		if !s.limits.allow("profile:"+actor.ID, 10, time.Now()) {
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
		err = s.pool.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE school_id=$1 AND id=$2 AND is_active`, s.schoolID, actor.ID).Scan(&expectedHash)
		if errors.Is(err, pgx.ErrNoRows) {
			httpresponse.Error(w, 401, "UNAUTHENTICATED", "Silakan masuk kembali")
			return
		}
		if err != nil {
			databaseError(w, err)
			return
		}
		if !VerifyPassword(input.CurrentPassword, expectedHash) {
			httpresponse.Error(w, 400, "CURRENT_PASSWORD_INVALID", "Password saat ini tidak sesuai")
			return
		}
		if passwordChanged {
			replacementHash, err = HashPassword(input.NewPassword)
			if err != nil {
				databaseError(w, err)
				return
			}
		}
	}
	input.CurrentPassword, input.NewPassword = "", ""
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		databaseError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	var active bool
	// Serialize school writes while allowing logout's audit foreign-key check.
	err = tx.QueryRow(r.Context(), "SELECT is_active FROM schools WHERE id=$1 FOR NO KEY UPDATE", s.schoolID).Scan(&active)
	if err != nil {
		databaseError(w, err)
		return
	}
	if !active {
		httpresponse.Error(w, 401, "UNAUTHENTICATED", "Silakan masuk kembali")
		return
	}
	// Logout locks the session before its user-linked audit; keep that lock order.
	var live bool
	err = tx.QueryRow(r.Context(), `SELECT expires_at>statement_timestamp() AND last_seen_at>statement_timestamp()-interval '30 minutes' FROM user_sessions WHERE school_id=$1 AND user_id=$2 AND token_hash=$3 FOR UPDATE`, s.schoolID, actor.ID, tokenHash(token)).Scan(&live)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		databaseError(w, err)
		return
	}
	if !live {
		httpresponse.Error(w, 401, "UNAUTHENTICATED", "Silakan masuk kembali")
		return
	}
	if sensitive {
		_, err = tx.Exec(r.Context(), `SELECT 1 FROM user_sessions WHERE school_id=$1 AND user_id=$2 ORDER BY token_hash FOR UPDATE`, s.schoolID, actor.ID)
		if err != nil {
			databaseError(w, err)
			return
		}
	}
	var u User
	var liveHash string
	err = tx.QueryRow(r.Context(), "SELECT "+userColumns+",u.password_hash FROM users u JOIN roles r ON r.school_id=u.school_id AND r.key=u.role WHERE u.school_id=$1 AND u.id=$2 FOR UPDATE OF u", s.schoolID, actor.ID).
		Scan(&u.ID, &u.SchoolID, &u.Email, &u.FullName, &u.Role, &u.Active, &u.RoleName, &u.Scope, &u.PermissionsList, &liveHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		databaseError(w, err)
		return
	}
	if !u.Active {
		httpresponse.Error(w, 401, "UNAUTHENTICATED", "Silakan masuk kembali")
		return
	}
	if u.Email != actor.Email || (sensitive && liveHash != expectedHash) {
		httpresponse.Error(w, 409, "ACCOUNT_CHANGED", "Akun telah berubah; muat ulang sebelum mencoba kembali")
		return
	}
	nameChanged := u.FullName != name
	emailChanged := u.Email != email
	if !passwordChanged {
		replacementHash = liveHash
	}
	_, err = tx.Exec(r.Context(), `UPDATE users SET full_name=$3,email=$4,password_hash=$5,updated_at=now() WHERE school_id=$1 AND id=$2`, s.schoolID, u.ID, name, email, replacementHash)
	if err == nil && sensitive {
		buf := make([]byte, 32)
		if _, err = rand.Read(buf); err == nil {
			token = hex.EncodeToString(buf)
			_, err = tx.Exec(r.Context(), `DELETE FROM user_sessions WHERE school_id=$1 AND user_id=$2`, s.schoolID, u.ID)
		}
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO user_sessions(token_hash,school_id,user_id,expires_at) VALUES($1,$2,$3,now()+interval '8 hours')`, tokenHash(token), s.schoolID, u.ID)
		}
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(school_id,actor_id,action,target_type,target_id,request_id,metadata) VALUES($1,$2,'auth.profile_updated','user',$2,$3,jsonb_build_object('name_changed',$4::boolean,'email_changed',$5::boolean,'password_changed',$6::boolean))`, s.schoolID, u.ID, w.Header().Get("X-Request-ID"), nameChanged, emailChanged, passwordChanged)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	u.FullName, u.Email = name, email
	if sensitive {
		s.setCookie(w, token, false)
	}
	if err = s.loadSessionAccess(r.Context(), &u); err != nil {
		databaseError(w, err)
		return
	}
	s.sessionData(w, r, u, token)
}
