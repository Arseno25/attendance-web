package auth

import (
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"github.com/jackc/pgx/v5"
	"net/http"
	"slices"
)

// BeginAuthorized serializes school writes and checks live permissions after acquiring the lock. Alternatives allow any explicitly named permission.
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
	actor.Grants = []string{}
	actor.ClassIDs = []string{}
	rows, err := tx.Query(r.Context(), `SELECT permission FROM permission_grants WHERE school_id=$1 AND user_id=$2`, s.schoolID, actor.ID)
	if err == nil {
		actor.Grants, err = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	if err != nil {
		fail()
		databaseError(w, err)
		return nil, User{}, false
	}
	allowed := slices.Contains(actor.Permissions(), permission)
	for _, alternative := range alternatives {
		allowed = allowed || slices.Contains(actor.Permissions(), alternative)
	}
	// Manual operators need the scoped roster and may request corrections of their entries.
	if (permission == "attendance.read" || permission == "corrections.request") && slices.Contains(actor.Permissions(), "attendance.manual") {
		allowed = true
	}
	if !allowed {
		fail()
		httpresponse.Error(w, 403, "FORBIDDEN", "Anda tidak memiliki izin untuk data ini")
		return nil, User{}, false
	}
	if actor.Role == "teacher" {
		rows, err = tx.Query(r.Context(), `SELECT DISTINCT a.class_id::text FROM teacher_assignments a JOIN classes c ON c.school_id=a.school_id AND c.id=a.class_id JOIN academic_years y ON y.school_id=c.school_id AND y.id=c.academic_year_id WHERE a.school_id=$1 AND a.teacher_id=$2 AND a.is_active AND c.is_active AND y.is_active AND a.starts_at<=statement_timestamp() AND a.ends_at>statement_timestamp()`, s.schoolID, actor.ID)
		if err == nil {
			actor.ClassIDs, err = pgx.CollectRows(rows, pgx.RowTo[string])
		}
		if err != nil {
			fail()
			databaseError(w, err)
			return nil, User{}, false
		}
	}
	return tx, actor, true
}
