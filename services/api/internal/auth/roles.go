package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"

	"attendance.local/api/internal/platform/httpresponse"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var roleKeyPart = regexp.MustCompile(`[^a-z0-9]+`)

var builtinRoles = []struct {
	key, name, description string
	scope                  RoleScope
	permissions            []string
}{
	{"super_admin", "Superadmin", "Pengelola penuh sekolah", ScopeSchool, []string{
		"users.read", "users.write", "roles.manage", "audit.read", "school.write",
		"students.read", "students.write", "academics.read", "academics.write",
		"attendance.read", "attendance.manual", "attendance.approve", "corrections.request",
		"reports.export", "devices.manage", "devices.health", "biometrics.enroll",
		"biometrics.revoke",
	}},
	{"admin", "Admin", "Administrator sekolah", ScopeSchool, []string{
		"students.read", "students.write", "academics.read", "academics.write", "attendance.read",
		"attendance.manual", "corrections.request", "reports.export", "devices.manage", "devices.health",
	}},
	{"teacher", "Guru", "Pengguna dengan penugasan kelas", ScopeAssignedClasses, []string{
		"students.read", "academics.read", "attendance.read", "corrections.request", "reports.export",
	}},
	{"operator", "Operator", "Operator layanan sekolah", ScopeSchool, []string{"devices.health"}},
}

// seedBuiltinRoles creates missing built-ins without restoring edits on existing roles.
func seedBuiltinRoles(ctx context.Context, tx pgx.Tx, schoolID string) error {
	for _, builtin := range builtinRoles {
		var roleID string
		err := tx.QueryRow(ctx, `INSERT INTO roles(school_id,key,name,description,scope,is_system)
			VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(school_id,key) DO NOTHING RETURNING id::text`,
			schoolID, builtin.key, builtin.name, builtin.description, builtin.scope, builtin.key == "super_admin").Scan(&roleID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		for _, permission := range builtin.permissions {
			if _, err = tx.Exec(ctx, `INSERT INTO role_permissions(school_id,role_id,permission) VALUES($1,$2,$3)`, schoolID, roleID, permission); err != nil {
				return err
			}
		}
	}
	return nil
}

func rolePermissionsValid(permissions []string) bool {
	seen := make(map[string]bool, len(permissions))
	for _, permission := range permissions {
		if !knownPermission(permission) || seen[permission] {
			return false
		}
		seen[permission] = true
	}
	return true
}

func normalizeRolePermissions(permissions []string) []string {
	result := append([]string(nil), permissions...)
	sort.Strings(result)
	if result == nil {
		return []string{}
	}
	return result
}

func roleKey(name string) (string, error) {
	part := strings.Trim(roleKeyPart.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if part == "" {
		part = "role"
	}
	if len(part) > 32 {
		part = strings.Trim(part[:32], "_")
	}
	suffix := make([]byte, 16)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	return "custom_" + part + "_" + hex.EncodeToString(suffix), nil
}

func (s *Service) roleActor(w http.ResponseWriter, r *http.Request, write bool) (User, bool) {
	actor, _, ok := s.current(w, r, write, "roles.manage")
	if !ok {
		return User{}, false
	}
	if !actor.IsSuperAdmin() {
		httpresponse.Error(w, 403, "FORBIDDEN", "Hanya Superadmin yang dapat mengelola peran")
		return User{}, false
	}
	return actor, true
}

func (s *Service) permissionCatalog(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	if _, ok := s.roleActor(w, r, false); !ok {
		return
	}
	httpresponse.Data(w, 200, PermissionCatalog())
}

func (s *Service) roles(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	actor, ok := s.roleActor(w, r, r.Method == "POST")
	if !ok {
		return
	}
	if r.Method == "POST" {
		s.createRole(w, r, actor)
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT r.id::text,r.key,r.name,r.description,r.scope,r.is_system,
		COALESCE(array_agg(rp.permission ORDER BY rp.permission) FILTER (WHERE rp.permission IS NOT NULL),ARRAY[]::text[]),
		(SELECT count(*)::int FROM users u WHERE u.school_id=r.school_id AND u.role=r.key)
		FROM roles r LEFT JOIN role_permissions rp ON rp.school_id=r.school_id AND rp.role_id=r.id
		WHERE r.school_id=$1 GROUP BY r.id ORDER BY r.is_system DESC,r.name COLLATE "C",r.key COLLATE "C"`, s.schoolID)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer rows.Close()
	result := []Role{}
	for rows.Next() {
		var role Role
		if err = rows.Scan(&role.ID, &role.Key, &role.Name, &role.Description, &role.Scope, &role.IsSystem, &role.Permissions, &role.UserCount); err != nil {
			databaseError(w, err)
			return
		}
		result = append(result, role)
	}
	if err = rows.Err(); err != nil {
		databaseError(w, err)
		return
	}
	httpresponse.Data(w, 200, result)
}

func (s *Service) createRole(w http.ResponseWriter, r *http.Request, actor User) {
	var input struct {
		Name        string    `json:"name"`
		Description string    `json:"description"`
		Scope       RoleScope `json:"scope"`
		Permissions []string  `json:"permissions"`
	}
	if !httpresponse.Decode(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if !validName(input.Name) || len([]rune(input.Description)) > 500 || !validScope(input.Scope) || !rolePermissionsValid(input.Permissions) {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Data peran atau hak akses tidak valid")
		return
	}
	tx, err := s.adminTransaction(r.Context(), actor)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	var id string
	key, err := roleKey(input.Name)
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO roles(school_id,key,name,description,scope,is_system)
			VALUES($1,$2,$3,$4,$5,false) RETURNING id::text`, s.schoolID, key, input.Name, input.Description, input.Scope).Scan(&id)
	}
	if err == nil {
		if err == nil {
			for _, permission := range normalizeRolePermissions(input.Permissions) {
				if _, err = tx.Exec(r.Context(), `INSERT INTO role_permissions(school_id,role_id,permission) VALUES($1,$2,$3)`, s.schoolID, id, permission); err != nil {
					break
				}
			}
		}
		if err == nil {
			var role Role
			role, err = loadRole(r.Context(), tx, s.schoolID, id)
			if err == nil {
				err = auditRole(r.Context(), tx, s.schoolID, actor.ID, "roles.created", id, w.Header().Get("X-Request-ID"), nil, role)
			}
			if err == nil {
				err = tx.Commit(r.Context())
			}
			if err == nil {
				httpresponse.Data(w, 201, role)
				return
			}
		}
	}
	writeRoleError(w, err)
}

func (s *Service) updateRole(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "PATCH") {
		return
	}
	actor, ok := s.roleActor(w, r, true)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !httpresponse.ValidID(id) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Peran tidak ditemukan")
		return
	}
	var input struct {
		Name        *string    `json:"name"`
		Description *string    `json:"description"`
		Scope       *RoleScope `json:"scope"`
		Permissions *[]string  `json:"permissions"`
	}
	if !httpresponse.Decode(w, r, &input) {
		return
	}
	if input.Name == nil && input.Description == nil && input.Scope == nil && input.Permissions == nil {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Tidak ada perubahan")
		return
	}
	tx, err := s.adminTransaction(r.Context(), actor)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	previous, err := loadRoleForUpdate(r.Context(), tx, s.schoolID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Peran tidak ditemukan")
		return
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	next := previous
	if input.Name != nil {
		next.Name = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		next.Description = strings.TrimSpace(*input.Description)
	}
	if input.Scope != nil {
		next.Scope = *input.Scope
	}
	if input.Permissions != nil {
		next.Permissions = normalizeRolePermissions(*input.Permissions)
	}
	if !validName(next.Name) || len([]rune(next.Description)) > 500 || !validScope(next.Scope) || !rolePermissionsValid(next.Permissions) {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Data peran atau hak akses tidak valid")
		return
	}
	if next.Key == "super_admin" && (next.Scope != ScopeSchool || !containsAll(next.Permissions, []string{"users.read", "users.write", "roles.manage"})) {
		httpresponse.Error(w, 400, "VALIDATION_ERROR", "Peran Superadmin wajib berscope sekolah dan memiliki hak akses inti")
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE roles SET name=$3,description=$4,scope=$5,updated_at=now()
		WHERE school_id=$1 AND id=$2`, s.schoolID, id, next.Name, next.Description, next.Scope)
	if err == nil && input.Permissions != nil {
		_, err = tx.Exec(r.Context(), "DELETE FROM role_permissions WHERE school_id=$1 AND role_id=$2", s.schoolID, id)
		for _, permission := range next.Permissions {
			if err != nil {
				break
			}
			_, err = tx.Exec(r.Context(), `INSERT INTO role_permissions(school_id,role_id,permission) VALUES($1,$2,$3)`, s.schoolID, id, permission)
		}
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), "DELETE FROM user_sessions WHERE school_id=$1 AND user_id IN (SELECT id FROM users WHERE school_id=$1 AND role=$2)", s.schoolID, previous.Key)
	}
	if err == nil {
		next, err = loadRole(r.Context(), tx, s.schoolID, id)
	}
	if err == nil {
		err = auditRole(r.Context(), tx, s.schoolID, actor.ID, "roles.updated", id, w.Header().Get("X-Request-ID"), &previous, next)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeRoleError(w, err)
		return
	}
	httpresponse.Data(w, 200, next)
}

func (s *Service) deleteRole(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "DELETE") {
		return
	}
	actor, ok := s.roleActor(w, r, true)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !httpresponse.ValidID(id) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Peran tidak ditemukan")
		return
	}
	tx, err := s.adminTransaction(r.Context(), actor)
	if err != nil {
		databaseError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	role, err := loadRoleForUpdate(r.Context(), tx, s.schoolID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Peran tidak ditemukan")
		return
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	if role.IsSystem || role.UserCount != 0 {
		httpresponse.Error(w, 409, "CONFLICT", "Peran bawaan atau peran yang masih dipakai tidak dapat dihapus")
		return
	}
	if _, err = tx.Exec(r.Context(), "DELETE FROM roles WHERE school_id=$1 AND id=$2", s.schoolID, id); err == nil {
		err = auditRole(r.Context(), tx, s.schoolID, actor.ID, "roles.deleted", id, w.Header().Get("X-Request-ID"), &role, Role{})
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeRoleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validScope(scope RoleScope) bool { return scope == ScopeSchool || scope == ScopeAssignedClasses }

func containsAll(actual, required []string) bool {
	for _, key := range required {
		if !slices.Contains(actual, key) {
			return false
		}
	}
	return true
}

func (s *Service) roleByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "PATCH":
		s.updateRole(w, r)
	case "DELETE":
		s.deleteRole(w, r)
	default:
		httpresponse.Method(w, r, "PATCH", "DELETE")
	}
}

type roleQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadRole(ctx context.Context, q roleQuerier, schoolID, id string) (Role, error) {
	var role Role
	err := q.QueryRow(ctx, `SELECT r.id::text,r.key,r.name,r.description,r.scope,r.is_system,
		COALESCE(array_agg(rp.permission ORDER BY rp.permission) FILTER (WHERE rp.permission IS NOT NULL),ARRAY[]::text[]),
		(SELECT count(*)::int FROM users u WHERE u.school_id=r.school_id AND u.role=r.key)
		FROM roles r LEFT JOIN role_permissions rp ON rp.school_id=r.school_id AND rp.role_id=r.id
		WHERE r.school_id=$1 AND r.id=$2 GROUP BY r.id`, schoolID, id).Scan(
		&role.ID, &role.Key, &role.Name, &role.Description, &role.Scope, &role.IsSystem, &role.Permissions, &role.UserCount)
	return role, err
}

func loadRoleForUpdate(ctx context.Context, tx pgx.Tx, schoolID, id string) (Role, error) {
	var role Role
	err := tx.QueryRow(ctx, `SELECT r.id::text,r.key,r.name,r.description,r.scope,r.is_system,
		COALESCE(array(SELECT permission FROM role_permissions rp WHERE rp.school_id=r.school_id AND rp.role_id=r.id ORDER BY permission),ARRAY[]::text[]),
		(SELECT count(*)::int FROM users u WHERE u.school_id=r.school_id AND u.role=r.key)
		FROM roles r WHERE r.school_id=$1 AND r.id=$2 FOR UPDATE`, schoolID, id).Scan(
		&role.ID, &role.Key, &role.Name, &role.Description, &role.Scope, &role.IsSystem, &role.Permissions, &role.UserCount)
	return role, err
}

func auditRole(ctx context.Context, tx pgx.Tx, schoolID, actorID, action, id, requestID string, previous *Role, next Role) error {
	var oldScope, newScope RoleScope
	var oldPermissions, newPermissions []string
	var oldName, newName string
	if previous != nil {
		oldScope, oldPermissions, oldName = previous.Scope, previous.Permissions, previous.Name
	}
	newScope, newPermissions, newName = next.Scope, next.Permissions, next.Name
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs(school_id,actor_id,action,target_type,target_id,request_id,metadata)
		VALUES($1,$2,$3,'role',$4,$5,jsonb_build_object(
		'name_before',NULLIF($6,''),'name_after',NULLIF($7,''),
		'scope_before',NULLIF($8,''),'scope_after',NULLIF($9,''),
		'permissions_before',$10::text[],'permissions_after',$11::text[]))`,
		schoolID, actorID, action, id, requestID, oldName, newName, oldScope, newScope, oldPermissions, newPermissions)
	return err
}

func writeRoleError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			httpresponse.Error(w, 409, "CONFLICT", "Nama peran sudah digunakan")
			return
		case "23503":
			httpresponse.Error(w, 409, "CONFLICT", "Peran masih digunakan atau referensi tidak valid")
			return
		}
	}
	databaseError(w, err)
}
