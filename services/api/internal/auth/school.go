package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrSchoolNotConfigured = errors.New("no school configured; run bootstrap after migrations")

// ResolveSchoolID refuses ambiguous databases instead of silently selecting another tenant.
func ResolveSchoolID(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (string, error) {
	var ids []string
	if err := db.QueryRow(ctx, `SELECT ARRAY(SELECT id::text FROM schools ORDER BY id LIMIT 2)`).Scan(&ids); err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", ErrSchoolNotConfigured
	}
	if len(ids) != 1 {
		return "", errors.New("database contains multiple schools; this installation supports one school")
	}
	return ids[0], nil
}
