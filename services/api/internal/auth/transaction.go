package auth

import (
	"context"
	"errors"
	"net/http"

	"attendance.local/api/internal/platform/httpresponse"
	"github.com/jackc/pgx/v5"
)

// UserCan checks the current authority behind a delegated device credential.
func (s *Service) UserCan(ctx context.Context, tx pgx.Tx, userID, permission, classID string) (bool, error) {
	u := User{ID: userID, SchoolID: s.schoolID}
	err := tx.QueryRow(ctx, `SELECT role,is_active FROM users WHERE school_id=$1 AND id=$2`, s.schoolID, userID).Scan(&u.Role, &u.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err == nil {
		err = s.loadAccess(ctx, tx, &u)
	}
	return err == nil && u.Can(permission, s.schoolID, classID), err
}

// BeginAuthorized serializes school writes and checks live permissions after acquiring the lock.
func (s *Service) BeginAuthorized(w http.ResponseWriter, r *http.Request, permission string, write bool, alternatives ...string) (pgx.Tx, User, bool) {
	actor, ok := s.Authenticate(w, r, write)
	if !ok {
		return nil, User{}, false
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		databaseError(w, err)
		return nil, User{}, false
	}
	fail := func() { _ = tx.Rollback(context.Background()) }
	lock := "FOR SHARE"
	if write {
		lock = "FOR UPDATE"
	}
	var active bool
	err = tx.QueryRow(r.Context(), "SELECT is_active FROM schools WHERE id=$1 "+lock, s.schoolID).Scan(&active)
	if err == nil && active {
		err = tx.QueryRow(r.Context(), "SELECT role,is_active FROM users WHERE school_id=$1 AND id=$2 FOR SHARE", s.schoolID, actor.ID).Scan(&actor.Role, &actor.Active)
	}
	if err != nil {
		fail()
		databaseError(w, err)
		return nil, User{}, false
	}
	cookie, _ := r.Cookie(s.cookieName)
	var live bool
	if cookie != nil {
		err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_sessions WHERE school_id=$1 AND user_id=$2 AND token_hash=$3 AND expires_at>statement_timestamp() AND last_seen_at>statement_timestamp()-interval '30 minutes')`, s.schoolID, actor.ID, tokenHash(cookie.Value)).Scan(&live)
	}
	if err != nil {
		fail()
		databaseError(w, err)
		return nil, User{}, false
	}
	if !active || !actor.Active || !live {
		fail()
		httpresponse.Error(w, 401, "UNAUTHENTICATED", "Silakan masuk kembali")
		return nil, User{}, false
	}
	if err = s.loadAccess(r.Context(), tx, &actor); err != nil {
		fail()
		databaseError(w, err)
		return nil, User{}, false
	}
	allowed := permission == "" || actor.Can(permission, s.schoolID, "")
	for _, alternative := range alternatives {
		allowed = allowed || actor.Can(alternative, s.schoolID, "")
	}
	// Manual roster operators also access roster and correction endpoints.
	if (permission == "attendance.read" || permission == "corrections.request") && actor.Can("attendance.manual", s.schoolID, "") {
		allowed = true
	}
	if !allowed {
		fail()
		httpresponse.Error(w, 403, "FORBIDDEN", "Anda tidak memiliki izin untuk data ini")
		return nil, User{}, false
	}
	return tx, actor, true
}
