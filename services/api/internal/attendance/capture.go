package attendance

import (
	"context"
	"net/http"

	"attendance.local/api/internal/platform/httpresponse"
	"github.com/jackc/pgx/v5"
)

func (s *Service) captureChallenge(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	tx, _, ok := s.kioskAuth(w, r, true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	cookie, _ := r.Cookie(s.deviceCookieName())
	hash := deviceHash(cookie.Value)
	// Keep spent image fingerprints across pairing/restarts for one day; never store photos.
	_, err := tx.Exec(r.Context(), `DELETE FROM kiosk_capture_challenges WHERE school_id=$1 AND (expires_at<clock_timestamp()-interval '24 hours' OR (session_token_hash=$2 AND consumed_at IS NULL))`, s.schoolID, hash)
	var data map[string]any
	if err == nil {
		data, err = one(r.Context(), tx, `INSERT INTO kiosk_capture_challenges(school_id,session_token_hash) VALUES($1,$2) RETURNING event_id::text,expires_at`, s.schoolID, hash)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 201, data)
}

func (s *Service) consumeCapture(r *http.Request, tx pgx.Tx, in capture) error {
	cookie, _ := r.Cookie(s.deviceCookieName())
	var id string
	return tx.QueryRow(r.Context(), `UPDATE kiosk_capture_challenges SET consumed_at=clock_timestamp(),image_hash=$4 WHERE school_id=$1 AND event_id=$2 AND session_token_hash=$3 AND expires_at>clock_timestamp() AND consumed_at IS NULL RETURNING event_id::text`, s.schoolID, in.EventID, deviceHash(cookie.Value), deviceHash(in.Image)).Scan(&id)
}
