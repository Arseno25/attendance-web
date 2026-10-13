package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"attendance.local/api/internal/platform/httpresponse"
	"github.com/jackc/pgx/v5"
)

const userColumns = `u.id::text,u.school_id::text,u.email,u.full_name,u.role,u.is_active,
 ARRAY(SELECT permission FROM permission_grants g WHERE g.school_id=u.school_id AND g.user_id=u.id ORDER BY permission)`

func scanUser(row pgx.Row) (User, error) {
	u := User{ClassIDs: []string{}}
	err := row.Scan(&u.ID, &u.SchoolID, &u.Email, &u.FullName, &u.Role, &u.Active, &u.Grants)
	return u, err
}

func (s *Service) users(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	permission := "users.read"
	if r.Method == "POST" {
		permission = "users.write"
	}
	actor, _, ok := s.current(w, r, r.Method == "POST", permission)
	if !ok {
		return
	}
	if r.Method == "POST" {
		s.createUser(w, r, actor)
		return
	}
	page, size, ok := httpresponse.Pagination(w, r)
	if !ok {
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > 100 {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Pencarian terlalu panjang")
		return
	}
	var total int
	err := s.pool.QueryRow(r.Context(), "SELECT count(*) FROM users WHERE school_id=$1 AND (email ILIKE $2 OR full_name ILIKE $2)", s.schoolID, "%"+search+"%").Scan(&total)
	if err != nil {
		databaseError(w, err)
		return
	}
	rows, err := s.pool.Query(r.Context(), "SELECT "+userColumns+" FROM users u WHERE u.school_id=$1 AND (u.email ILIKE $2 OR u.full_name ILIKE $2) ORDER BY u.created_at,u.id LIMIT $3 OFFSET $4", s.schoolID, "%"+search+"%", size, (page-1)*size)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			databaseError(w, err)
			return
		}
		users = append(users, u)
	}
	if err = rows.Err(); err != nil {
		databaseError(w, err)
		return
	}
	httpresponse.JSON(w, 200, map[string]any{"data": users, "pagination": map[string]int{"page": page, "page_size": size, "total": total}, "request_id": w.Header().Get("X-Request-ID")})
}

func (s *Service) adminTransaction(ctx context.Context, actor User) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var eligible bool
	err = tx.QueryRow(ctx, "SELECT is_active FROM schools WHERE id=$1 FOR UPDATE", s.schoolID).Scan(&eligible)
	if err == nil && eligible {
		err = tx.QueryRow(ctx, "SELECT is_active AND role='super_admin' FROM users WHERE school_id=$1 AND id=$2 FOR UPDATE", s.schoolID, actor.ID).Scan(&eligible)
	}
	if err != nil || !eligible {
		_ = tx.Rollback(context.Background())
		if err == nil {
			err = pgx.ErrNoRows
		}
		return nil, err
	}
	return tx, nil
}

func (s *Service) replaceGrants(ctx context.Context, tx pgx.Tx, userID, actorID string, grants []string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM permission_grants WHERE school_id=$1 AND user_id=$2", s.schoolID, userID); err != nil {
		return err
	}
	for _, grant := range grants {
		if _, err := tx.Exec(ctx, "INSERT INTO permission_grants(school_id,user_id,permission,granted_by) VALUES($1,$2,$3,$4)", s.schoolID, userID, grant, actorID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) createUser(w http.ResponseWriter, r *http.Request, actor User) {
	var input struct {
		Email    string   `json:"email"`
		FullName string   `json:"full_name"`
		Role     string   `json:"role"`
		Password string   `json:"password"`
		Grants   []string `json:"grants"`
	}
	if !httpresponse.Decode(w, r, &input) {
		return
	}
	email, err := normalizeEmail(input.Email)
	name := strings.TrimSpace(input.FullName)
	if err != nil || !validName(name) || !validRole(input.Role) || !validGrants(input.Grants) || !grantsForRole(input.Role, input.Grants) {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Email, nama, peran atau izin tidak valid")
		return
	}
	select {
	case s.hashing <- struct{}{}:
		defer func() { <-s.hashing }()
	default:
		httpresponse.Error(w, 429, "RATE_LIMITED", "Layanan sedang sibuk; coba kembali")
		return
	}
	hash, err := HashPassword(input.Password)
	input.Password = ""
	if err != nil {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Password harus 12–128 byte")
		return
	}
	tx, err := s.adminTransaction(r.Context(), actor)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	var id string
	err = tx.QueryRow(r.Context(), "INSERT INTO users(school_id,email,full_name,password_hash,role) VALUES($1,$2,$3,$4,$5) RETURNING id::text", s.schoolID, email, name, hash, input.Role).Scan(&id)
	if err == nil {
		err = s.replaceGrants(r.Context(), tx, id, actor.ID, input.Grants)
	}
	if err == nil {
		err = audit(r.Context(), tx, s.schoolID, actor.ID, "users.created", "user", id, w.Header().Get("X-Request-ID"))
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	u := User{ID: id, SchoolID: s.schoolID, Email: email, FullName: name, Role: input.Role, Active: true, Grants: input.Grants, ClassIDs: []string{}}
	if u.Grants == nil {
		u.Grants = []string{}
	}
	httpresponse.Data(w, 201, u)
}

func (s *Service) updateUser(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "PATCH") {
		return
	}
	actor, _, ok := s.current(w, r, true, "users.write")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !httpresponse.ValidID(id) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Pengguna tidak ditemukan")
		return
	}
	var input struct {
		Email    *string   `json:"email"`
		FullName *string   `json:"full_name"`
		Role     *string   `json:"role"`
		Active   *bool     `json:"is_active"`
		Grants   *[]string `json:"grants"`
	}
	if !httpresponse.Decode(w, r, &input) {
		return
	}
	if input.Email == nil && input.FullName == nil && input.Role == nil && input.Active == nil && input.Grants == nil {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Tidak ada perubahan")
		return
	}
	tx, err := s.adminTransaction(r.Context(), actor)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	u, err := scanUser(tx.QueryRow(r.Context(), "SELECT "+userColumns+" FROM users u WHERE u.school_id=$1 AND u.id=$2 FOR UPDATE", s.schoolID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Pengguna tidak ditemukan")
		return
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	previousRole, previousActive, previousEmail := u.Role, u.Active, u.Email
	if input.Email != nil {
		u.Email, err = normalizeEmail(*input.Email)
	}
	if input.FullName != nil {
		u.FullName = strings.TrimSpace(*input.FullName)
	}
	if input.Role != nil {
		u.Role = *input.Role
	}
	if input.Active != nil {
		u.Active = *input.Active
	}
	if input.Grants != nil {
		u.Grants = *input.Grants
	}
	if err != nil || !validName(u.FullName) || !validRole(u.Role) || !validGrants(u.Grants) || !grantsForRole(u.Role, u.Grants) {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Perubahan tidak valid")
		return
	}
	if u.ID == actor.ID && (!u.Active || u.Role != "super_admin" || u.Email != previousEmail) {
		httpresponse.Error(w, 409, "CONFLICT", "Anda tidak dapat mengurangi akses atau mengubah email akun sendiri")
		return
	}
	if previousRole == "super_admin" && previousActive && (!u.Active || u.Role != "super_admin") {
		var count int
		err = tx.QueryRow(r.Context(), "SELECT count(*) FROM users WHERE school_id=$1 AND role='super_admin' AND is_active", s.schoolID).Scan(&count)
		if err != nil {
			databaseError(w, err)
			return
		}
		if count <= 1 {
			httpresponse.Error(w, 409, "CONFLICT", "Super admin terakhir harus tetap aktif")
			return
		}
	}
	_, err = tx.Exec(r.Context(), "UPDATE users SET email=$3,full_name=$4,role=$5,is_active=$6,updated_at=now() WHERE school_id=$1 AND id=$2", s.schoolID, id, u.Email, u.FullName, u.Role, u.Active)
	if err == nil && input.Grants != nil {
		err = s.replaceGrants(r.Context(), tx, id, actor.ID, u.Grants)
	}
	if err == nil && (u.Role != previousRole || u.Active != previousActive || u.Email != previousEmail || input.Grants != nil) {
		_, err = tx.Exec(r.Context(), "DELETE FROM user_sessions WHERE school_id=$1 AND user_id=$2", s.schoolID, id)
	}
	if err == nil {
		err = audit(r.Context(), tx, s.schoolID, actor.ID, "users.updated", "user", id, w.Header().Get("X-Request-ID"))
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	httpresponse.Data(w, 200, u)
}

func (s *Service) auditLogs(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	if _, _, ok := s.current(w, r, false, "audit.read"); !ok {
		return
	}
	page, size, ok := httpresponse.Pagination(w, r)
	if !ok {
		return
	}
	type entry struct {
		ID         int64          `json:"id"`
		ActorID    *string        `json:"actor_id"`
		Action     string         `json:"action"`
		TargetType string         `json:"target_type"`
		TargetID   *string        `json:"target_id"`
		RequestID  string         `json:"request_id"`
		OccurredAt time.Time      `json:"occurred_at"`
		Metadata   map[string]any `json:"metadata"`
	}
	var total int
	if err := s.pool.QueryRow(r.Context(), "SELECT count(*) FROM audit_logs WHERE school_id=$1", s.schoolID).Scan(&total); err != nil {
		databaseError(w, err)
		return
	}
	rows, err := s.pool.Query(r.Context(), "SELECT id,actor_id::text,action,target_type,target_id::text,request_id,occurred_at,metadata FROM audit_logs WHERE school_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3", s.schoolID, size, (page-1)*size)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer rows.Close()
	entries := []entry{}
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.ID, &e.ActorID, &e.Action, &e.TargetType, &e.TargetID, &e.RequestID, &e.OccurredAt, &e.Metadata); err != nil {
			databaseError(w, err)
			return
		}
		entries = append(entries, e)
	}
	if err = rows.Err(); err != nil {
		databaseError(w, err)
		return
	}
	httpresponse.JSON(w, 200, map[string]any{"data": entries, "pagination": map[string]int{"page": page, "page_size": size, "total": total}, "request_id": w.Header().Get("X-Request-ID")})
}
