package auth_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRBACMigrationPreservesLegacyAccess(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run real database integration")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "rbac_migration_" + strings.ToLower(strings.ReplaceAll(randomSchemaSuffix(t), "-", ""))
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		root.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = root.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE")
		root.Close()
	})
	conf, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	conf.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	for version := 1; version <= 7; version++ {
		applyMigrationFixture(t, ctx, pool, fmt.Sprintf("../../migrations/%05d_*.sql", version))
	}
	insertLegacyRBACFixture(t, ctx, pool)
	if _, err := os.Stat("../../migrations/00008_dynamic_rbac.sql"); err == nil {
		applyMigrationFixture(t, ctx, pool, "../../migrations/00008_dynamic_rbac.sql")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	var roleTableExists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('roles') IS NOT NULL").Scan(&roleTableExists); err != nil {
		t.Fatal(err)
	}
	if !roleTableExists {
		t.Fatal("dynamic RBAC migration did not create roles")
	}

	const schoolA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const schoolB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	assertRolePermissions(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000001", []string{
		"users.read", "users.write", "roles.manage", "audit.read", "school.write",
		"students.read", "students.write", "academics.read", "academics.write",
		"attendance.read", "attendance.manual", "attendance.approve", "corrections.request",
		"reports.export", "devices.manage", "devices.health", "biometrics.enroll",
		"biometrics.revoke", "consents.manage",
	})
	assertRolePermissions(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000002", []string{
		"students.read", "students.write", "academics.read", "academics.write", "attendance.read",
		"attendance.manual", "corrections.request", "reports.export", "devices.manage", "devices.health",
	})
	assertRolePermissions(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000003", []string{
		"students.read", "academics.read", "attendance.read", "corrections.request", "reports.export",
	})
	assertRolePermissions(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000004", []string{"devices.health"})
	assertRolePermissions(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000005", []string{
		"students.read", "students.write", "academics.read", "academics.write", "attendance.read",
		"attendance.manual", "corrections.request", "reports.export", "devices.manage", "devices.health", "biometrics.enroll",
	})
	assertRolePermissions(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000007", []string{
		"students.read", "students.write", "academics.read", "academics.write", "attendance.read",
		"attendance.manual", "corrections.request", "reports.export", "devices.manage", "devices.health", "attendance.approve",
	})

	assertSameRole(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000005", "10000000-0000-4000-8000-000000000006", true)
	assertSameRole(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000005", "10000000-0000-4000-8000-000000000007", false)
	assertRoleKey(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000002", "admin")
	assertRoleKey(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000003", "teacher")
	assertRoleKey(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000004", "operator")
	assertRoleScope(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000002", "school")
	assertRoleScope(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000003", "assigned_classes")
	assertRoleScope(t, ctx, pool, schoolA, "10000000-0000-4000-8000-000000000004", "school")

	assertAssignmentsPreserved(t, ctx, pool)
	assertSameSchoolRoleForeignKey(t, ctx, pool, schoolA, schoolB)
	var grantsExists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('permission_grants') IS NOT NULL").Scan(&grantsExists); err != nil {
		t.Fatal(err)
	}
	if grantsExists {
		t.Fatal("legacy permission_grants table remains after conversion")
	}
}

func randomSchemaSuffix(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%d", os.Getpid())
}

func applyMigrationFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pattern string) {
	t.Helper()
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("migration glob %q matched %d files: %v", pattern, len(files), files)
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(body), "-- +goose Down")[0]
	lines := make([]string, 0, strings.Count(up, "\n"))
	for _, line := range strings.Split(up, "\n") {
		if !strings.HasPrefix(line, "-- +goose") {
			lines = append(lines, line)
		}
	}
	if _, err := pool.Exec(ctx, strings.Join(lines, "\n")); err != nil {
		t.Fatalf("apply %s: %v", files[0], err)
	}
}

func insertLegacyRBACFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	const (
		schoolA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		schoolB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	)
	for _, school := range []struct{ id, name string }{{schoolA, "School A"}, {schoolB, "School B"}} {
		if _, err := pool.Exec(ctx, "INSERT INTO schools(id,name) VALUES($1,$2)", school.id, school.name); err != nil {
			t.Fatal(err)
		}
	}
	users := []struct {
		id, school, role, email string
		active                  bool
	}{
		{"10000000-0000-4000-8000-000000000001", schoolA, "super_admin", "root@a.test", true},
		{"10000000-0000-4000-8000-000000000002", schoolA, "admin", "admin@a.test", true},
		{"10000000-0000-4000-8000-000000000003", schoolA, "teacher", "teacher@a.test", true},
		{"10000000-0000-4000-8000-000000000004", schoolA, "operator", "operator@a.test", true},
		{"10000000-0000-4000-8000-000000000005", schoolA, "admin", "same-one@a.test", true},
		{"10000000-0000-4000-8000-000000000006", schoolA, "admin", "same-two@a.test", false},
		{"10000000-0000-4000-8000-000000000007", schoolA, "admin", "different@a.test", true},
		{"20000000-0000-4000-8000-000000000001", schoolB, "super_admin", "root@b.test", true},
	}
	for _, user := range users {
		if _, err := pool.Exec(ctx, `INSERT INTO users(id,school_id,email,full_name,password_hash,role,is_active)
			VALUES($1,$2,$3,$3,'test-hash',$4,$5)`, user.id, user.school, user.email, user.role, user.active); err != nil {
			t.Fatal(err)
		}
	}
	for _, grant := range []struct{ user, permission string }{
		{"10000000-0000-4000-8000-000000000005", "biometrics.enroll"},
		{"10000000-0000-4000-8000-000000000006", "biometrics.enroll"},
		{"10000000-0000-4000-8000-000000000007", "attendance.approve"},
		{"10000000-0000-4000-8000-000000000004", "reports.export"},
		{"10000000-0000-4000-8000-000000000003", "attendance.manual"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO permission_grants(school_id,user_id,permission,granted_by)
			VALUES($1,$2,$3,$4)`, schoolA, grant.user, grant.permission, "10000000-0000-4000-8000-000000000001"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO academic_years(id,school_id,name,starts_on,ends_on,timezone,is_active)
		VALUES('30000000-0000-4000-8000-000000000001',$1,'2026','2026-01-01','2026-12-31','Asia/Jakarta',true)`, schoolA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO classes(id,school_id,academic_year_id,code,name)
		VALUES('40000000-0000-4000-8000-000000000001',$1,'30000000-0000-4000-8000-000000000001','A','Class A')`, schoolA); err != nil {
		t.Fatal(err)
	}
	for _, assignment := range []struct {
		id, starts, ends string
		active           bool
	}{
		{"50000000-0000-4000-8000-000000000001", "2026-01-02 00:00:00+07", "2026-06-01 00:00:00+07", true},
		{"50000000-0000-4000-8000-000000000002", "2026-06-02 00:00:00+07", "2026-12-31 23:59:59+07", false},
	} {
		revokedAt := any(nil)
		if !assignment.active {
			revokedAt = "2026-06-01 12:00:00+07"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO teacher_assignments(id,school_id,teacher_id,class_id,academic_year_id,starts_at,ends_at,is_active,revoked_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, assignment.id, schoolA, "10000000-0000-4000-8000-000000000003", "40000000-0000-4000-8000-000000000001", "30000000-0000-4000-8000-000000000001", assignment.starts, assignment.ends, assignment.active, revokedAt); err != nil {
			t.Fatal(err)
		}
	}
}

func assertRolePermissions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schoolID, userID string, expected []string) {
	t.Helper()
	var roleID string
	if err := pool.QueryRow(ctx, "SELECT r.id::text FROM users u JOIN roles r ON r.school_id=u.school_id AND r.key=u.role WHERE u.school_id=$1 AND u.id=$2", schoolID, userID).Scan(&roleID); err != nil {
		t.Fatalf("find migrated role for user %s: %v", userID, err)
	}
	rows, err := pool.Query(ctx, "SELECT permission FROM role_permissions WHERE school_id=$1 AND role_id=$2", schoolID, roleID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	actual := make([]string, 0)
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			t.Fatal(err)
		}
		actual = append(actual, permission)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("user %s permissions=%v want=%v", userID, actual, expected)
	}
}

func assertSameRole(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schoolID, leftID, rightID string, wantSame bool) {
	t.Helper()
	var same bool
	err := pool.QueryRow(ctx, `SELECT l.role=r.role FROM users l JOIN users r ON r.school_id=l.school_id
		WHERE l.school_id=$1 AND l.id=$2 AND r.id=$3`, schoolID, leftID, rightID).Scan(&same)
	if err != nil {
		t.Fatal(err)
	}
	if same != wantSame {
		t.Fatalf("users %s and %s share role=%v want=%v", leftID, rightID, same, wantSame)
	}
}

func assertRoleKey(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schoolID, userID, want string) {
	t.Helper()
	var actual string
	if err := pool.QueryRow(ctx, "SELECT role FROM users WHERE school_id=$1 AND id=$2", schoolID, userID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != want {
		t.Fatalf("user %s role=%q want=%q", userID, actual, want)
	}
}

func assertRoleScope(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schoolID, userID, want string) {
	t.Helper()
	var actual string
	if err := pool.QueryRow(ctx, `SELECT r.scope FROM users u JOIN roles r ON r.school_id=u.school_id AND r.key=u.role
		WHERE u.school_id=$1 AND u.id=$2`, schoolID, userID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != want {
		t.Fatalf("user %s scope=%q want=%q", userID, actual, want)
	}
}

func assertAssignmentsPreserved(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT id::text,school_id::text,user_id::text,class_id::text,academic_year_id::text,
		starts_at,ends_at,is_active,revoked_at FROM user_class_assignments ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type assignment struct {
		id, school, user, classID, year string
		starts, ends, revoked           any
		active                          bool
	}
	actual := make([]assignment, 0)
	for rows.Next() {
		var item assignment
		if err := rows.Scan(&item.id, &item.school, &item.user, &item.classID, &item.year, &item.starts, &item.ends, &item.active, &item.revoked); err != nil {
			t.Fatal(err)
		}
		actual = append(actual, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(actual) != 2 || actual[0].id != "50000000-0000-4000-8000-000000000001" || actual[1].id != "50000000-0000-4000-8000-000000000002" {
		t.Fatalf("assignment identities not preserved: %+v", actual)
	}
	if actual[0].user != "10000000-0000-4000-8000-000000000003" || !actual[0].active || actual[0].revoked != nil || actual[1].active || actual[1].revoked == nil {
		t.Fatalf("assignment status/history changed: %+v", actual)
	}
}

func assertSameSchoolRoleForeignKey(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schoolA, schoolB string) {
	t.Helper()
	var migrationRoleKey string
	if err := pool.QueryRow(ctx, "SELECT role FROM users WHERE school_id=$1 AND id=$2", schoolA, "10000000-0000-4000-8000-000000000005").Scan(&migrationRoleKey); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET role=$1 WHERE school_id=$2 AND id=$3", migrationRoleKey, schoolB, "20000000-0000-4000-8000-000000000001"); err == nil {
		t.Fatal("cross-school role reference was accepted")
	}
}
