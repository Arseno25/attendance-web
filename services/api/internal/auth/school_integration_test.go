package auth_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"attendance.local/api/internal/auth"
	"attendance.local/api/internal/platform/httpresponse"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func schoolFixture(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run real database integration")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := pgx.Identifier{"school_test_" + strings.ToLower(rand.Text()[:12])}.Sanitize()
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		root.Close()
		t.Fatal(err)
	}
	conf, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	conf.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); _, _ = root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); root.Close() })
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		applyMigrationFixture(t, ctx, pool, file)
	}
	return pool
}

func TestAutomaticSchoolBootstrapIsAtomicAndConcurrent(t *testing.T) {
	pool := schoolFixture(t)
	ctx := context.Background()
	if id, err := auth.ResolveSchoolID(ctx, pool); id != "" || !errors.Is(err, auth.ErrSchoolNotConfigured) {
		t.Fatal("empty database needs bootstrap", id, err)
	}
	svc, err := auth.New(pool, "", "http://localhost:3001")
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Bootstrap(ctx, "invalid@example.test", "Owner", "short", "School"); err == nil {
		t.Fatal("weak password accepted")
	}
	var schools, users int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM schools),(SELECT count(*) FROM users)`).Scan(&schools, &users); err != nil || schools != 0 || users != 0 {
		t.Fatal("invalid bootstrap wrote data", schools, users, err)
	}
	if _, err = pool.Exec(ctx, `CREATE FUNCTION reject_test_owner() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic registration rejection'; END $$; CREATE TRIGGER reject_test_owner BEFORE INSERT ON users FOR EACH ROW EXECUTE FUNCTION reject_test_owner()`); err != nil {
		t.Fatal(err)
	}
	if err = svc.Bootstrap(ctx, "rejected@example.test", "Owner", "ExamplePassword1!", "School"); err == nil {
		t.Fatal("synthetic database failure ignored")
	}
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM schools),(SELECT count(*) FROM roles)`).Scan(&schools, &users); err != nil || schools != 0 || users != 0 {
		t.Fatal("failed bootstrap left school or roles behind", schools, users, err)
	}
	if _, err = pool.Exec(ctx, `DROP TRIGGER reject_test_owner ON users; DROP FUNCTION reject_test_owner()`); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, email := range []string{"first@example.test", "second@example.test"} {
		go func() { results <- svc.Bootstrap(ctx, email, "Owner", "ExamplePassword1!", "School") }()
	}
	succeeded := 0
	for range 2 {
		if <-results == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatal("concurrent bootstrap must create one owner", succeeded)
	}
	var id string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM schools`).Scan(&id); err != nil || !httpresponse.ValidID(id) {
		t.Fatal("school ID not generated", err)
	}
	if resolved, err := auth.ResolveSchoolID(ctx, pool); err != nil || resolved != id {
		t.Fatal("school not resolved", resolved, err)
	}
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM schools),(SELECT count(*) FROM users)`).Scan(&schools, &users); err != nil || schools != 1 || users != 1 {
		t.Fatal("duplicate school or account", schools, users, err)
	}
	if err = svc.Bootstrap(ctx, "retry@example.test", "Owner", "ExamplePassword1!", "Changed School"); err == nil {
		t.Fatal("bootstrap repeated")
	}
	seed, err := os.ReadFile("../../seeds/development.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(seed)); err != nil {
		t.Fatal(err)
	}
	var name string
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM schools`).Scan(&schools); err != nil || schools != 1 {
		t.Fatal("seed created a second school", schools, err)
	}
	if err = pool.QueryRow(ctx, `SELECT name FROM schools WHERE id=$1`, id).Scan(&name); err != nil || name != "School" {
		t.Fatal("existing school renamed", name, err)
	}
}

func TestAutomaticBootstrapPreservesExistingSchoolAndRejectsAmbiguity(t *testing.T) {
	pool := schoolFixture(t)
	ctx := context.Background()
	var id, before, after string
	if err := pool.QueryRow(ctx, `INSERT INTO schools(name,app_name,address) VALUES('Existing school','School app','Existing address') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(s)::text FROM schools s WHERE id=$1`, id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	svc, err := auth.New(pool, "", "http://localhost:3001")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Bootstrap(ctx, "owner@example.test", "Owner", "ExamplePassword1!", "Ignored replacement name"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_jsonb(s)::text FROM schools s WHERE id=$1`, id).Scan(&after); err != nil || before != after {
		t.Fatal("existing school changed", err)
	}
	var ownerSchool string
	if err := pool.QueryRow(ctx, `SELECT school_id::text FROM users`).Scan(&ownerSchool); err != nil || ownerSchool != id {
		t.Fatal("owner attached to wrong school", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO schools(name,is_active) VALUES('Inactive second school',false)`); err != nil {
		t.Fatal(err)
	}
	if resolved, err := auth.ResolveSchoolID(ctx, pool); err == nil || resolved != "" {
		t.Fatal("ambiguous database selected a school", resolved, err)
	}
	if err := svc.Bootstrap(ctx, "another@example.test", "Owner", "ExamplePassword1!", "School"); err == nil {
		t.Fatal("ambiguous bootstrap accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 1 {
		t.Fatal("ambiguous bootstrap wrote an account", count, err)
	}
}

func TestAutomaticBootstrapRejectsInactiveSchool(t *testing.T) {
	pool := schoolFixture(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO schools(name,is_active) VALUES('Inactive school',false)`); err != nil {
		t.Fatal(err)
	}
	svc, err := auth.New(pool, "", "http://localhost:3001")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Bootstrap(ctx, "owner@example.test", "Owner", "ExamplePassword1!", "School"); err == nil {
		t.Fatal("inactive school bootstrapped")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatal("inactive school received an account", count, err)
	}
}
