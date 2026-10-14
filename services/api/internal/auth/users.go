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

const userColumns = `u.id::text,u.school_id::text,u.email,u.full_name,u.role,u.is_active,r.name,r.scope,
 COALESCE(array(SELECT permission FROM role_permissions rp WHERE rp.school_id=r.school_id AND rp.role_id=r.id ORDER BY permission),ARRAY[]::text[])`

func scanUser(row pgx.Row) (User, error) {
	u := User{ClassIDs: []string{}, PermissionsList: []string{}}
	err := row.Scan(&u.ID, &u.SchoolID, &u.Email, &u.FullName, &u.Role, &u.Active, &u.RoleName, &u.Scope, &u.PermissionsList)
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
	rows, err := s.pool.Query(r.Context(), "SELECT "+userColumns+" FROM users u JOIN roles r ON r.school_id=u.school_id AND r.key=u.role WHERE u.school_id=$1 AND (u.email ILIKE $2 OR u.full_name ILIKE $2) ORDER BY u.created_at,u.id LIMIT $3 OFFSET $4", s.schoolID, "%"+search+"%", size, (page-1)*size)
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

func (s *Service) createUser(w http.ResponseWriter, r *http.Request, actor User) {
	var input struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if !httpresponse.Decode(w, r, &input) {
		return
	}
	email, err := normalizeEmail(input.Email)
	name := strings.TrimSpace(input.FullName)
	if err != nil || !validName(name) || strings.TrimSpace(input.Role) == "" {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Email, nama, atau peran tidak valid")
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
		httpresponse.Error(w, 400, "VALIDATION_ERROR", passwordRule)
		return
	}
	tx, err := s.adminTransaction(r.Context(), actor)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	valid, err := roleExists(r.Context(), tx, s.schoolID, input.Role)
	if err != nil {
		databaseError(w, err)
		return
	}
	if !valid {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Peran tidak ditemukan di sekolah ini")
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), "INSERT INTO users(school_id,email,full_name,password_hash,role) VALUES($1,$2,$3,$4,$5) RETURNING id::text", s.schoolID, email, name, hash, input.Role).Scan(&id)
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
	u := User{ID: id, SchoolID: s.schoolID, Email: email, FullName: name, Role: input.Role, Active: true, ClassIDs: []string{}}
	if err := s.loadSessionAccess(r.Context(), &u); err != nil {
		databaseError(w, err)
		return
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
		Email    *string `json:"email"`
		FullName *string `json:"full_name"`
		Role     *string `json:"role"`
		Active   *bool   `json:"is_active"`
	}
	if !httpresponse.Decode(w, r, &input) {
		return
	}
	if input.Email == nil && input.FullName == nil && input.Role == nil && input.Active == nil {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Tidak ada perubahan")
		return
	}
	tx, err := s.adminTransaction(r.Context(), actor)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	u, err := scanUser(tx.QueryRow(r.Context(), "SELECT "+userColumns+" FROM users u JOIN roles r ON r.school_id=u.school_id AND r.key=u.role WHERE u.school_id=$1 AND u.id=$2 FOR UPDATE OF u", s.schoolID, id))
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
	if err != nil || !validName(u.FullName) || strings.TrimSpace(u.Role) == "" {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Perubahan tidak valid")
		return
	}
	if u.ID == actor.ID && (!u.Active || u.Role != "super_admin" || u.Email != previousEmail) {
		httpresponse.Error(w, 409, "CONFLICT", "Anda tidak dapat mengurangi akses atau mengubah email akun sendiri")
		return
	}
	if u.Role != previousRole {
		valid, roleErr := roleExists(r.Context(), tx, s.schoolID, u.Role)
		if roleErr != nil {
			databaseError(w, roleErr)
			return
		}
		if !valid {
			httpresponse.Error(w, 400, "VALIDATION_ERROR", "Peran tidak ditemukan di sekolah ini")
			return
		}
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
	if err == nil && (u.Role != previousRole || u.Active != previousActive || u.Email != previousEmail) {
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
	if err = s.loadSessionAccess(r.Context(), &u); err != nil {
		databaseError(w, err)
		return
	}
	httpresponse.Data(w, 200, u)
}

func (s *Service) auditLogs(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	tx, _, ok := s.BeginAuthorized(w, r, "audit.read", false)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	page, size, ok := httpresponse.Pagination(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	actor, action, target := strings.ToLower(q.Get("actor_id")), strings.TrimSpace(q.Get("action")), strings.TrimSpace(q.Get("target_type"))
	from, to := q.Get("from"), q.Get("to")
	if (actor != "" && !httpresponse.ValidID(actor)) || len(action) > 100 || len(target) > 100 {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Filter audit tidak valid")
		return
	}
	for _, day := range []string{from, to} {
		if day != "" {
			date, err := time.Parse("2006-01-02", day)
			if err != nil || date.Year() < 1 || date.Year() >= 9999 {
				httpresponse.Error(w, 400, "VALIDATION_ERROR", "Tanggal audit tidak valid")
				return
			}
		}
	}
	if from != "" && to != "" && from > to {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Rentang audit tidak valid")
		return
	}
	where := ` FROM audit_logs a JOIN schools sc ON sc.id=a.school_id LEFT JOIN users u ON u.school_id=a.school_id AND u.id=a.actor_id WHERE a.school_id=$1 AND ($2='' OR a.actor_id=NULLIF($2,'')::uuid) AND ($3='' OR a.action=$3) AND ($4='' OR a.occurred_at>=NULLIF($4,'')::date::timestamp AT TIME ZONE sc.timezone) AND ($5='' OR a.occurred_at<(NULLIF($5,'')::date+1)::timestamp AT TIME ZONE sc.timezone) AND ($6='' OR a.target_type=$6)`
	args := []any{s.schoolID, actor, action, from, to, target}
	type entry struct {
		ID         int64          `json:"id"`
		ActorID    *string        `json:"actor_id"`
		ActorName  *string        `json:"actor_name"`
		Action     string         `json:"action"`
		TargetType string         `json:"target_type"`
		TargetID   *string        `json:"target_id"`
		RequestID  string         `json:"request_id"`
		OccurredAt time.Time      `json:"occurred_at"`
		Metadata   map[string]any `json:"metadata"`
	}
	var total int
	if err := tx.QueryRow(r.Context(), "SELECT count(*)"+where, args...).Scan(&total); err != nil {
		databaseError(w, err)
		return
	}
	args = append(args, size, (page-1)*size)
	rows, err := tx.Query(r.Context(), `SELECT a.id,a.actor_id::text,u.full_name,a.action,a.target_type,a.target_id::text,a.request_id,a.occurred_at,a.metadata`+where+` ORDER BY a.id DESC LIMIT $7 OFFSET $8`, args...)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer rows.Close()
	entries := []entry{}
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.ID, &e.ActorID, &e.ActorName, &e.Action, &e.TargetType, &e.TargetID, &e.RequestID, &e.OccurredAt, &e.Metadata); err != nil {
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
