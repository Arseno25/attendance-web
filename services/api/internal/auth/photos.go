package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"attendance.local/api/internal/platform/httpresponse"
	"attendance.local/api/internal/platform/images"
	"github.com/jackc/pgx/v5"
)

func serveImage(w http.ResponseWriter, data []byte, logo bool) {
	mime, cache := "image/jpeg", "private, no-store"
	if logo {
		mime, cache = "image/png", "public, max-age=3600"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", cache)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
func (s *Service) photo(w http.ResponseWriter, r *http.Request) { s.updateImage(w, r, false) }
func (s *Service) logo(w http.ResponseWriter, r *http.Request)  { s.updateImage(w, r, true) }
func (s *Service) updateImage(w http.ResponseWriter, r *http.Request, logo bool) {
	if !httpresponse.Method(w, r, "GET", "PUT", "DELETE") {
		return
	}
	if logo && r.Method == "GET" {
		var data []byte
		err := s.pool.QueryRow(r.Context(), `SELECT logo FROM schools WHERE id=$1 AND is_active AND logo IS NOT NULL`, s.schoolID).Scan(&data)
		if errors.Is(err, pgx.ErrNoRows) {
			httpresponse.Error(w, 404, "NOT_FOUND", "Logo belum ditambahkan.")
			return
		}
		if err != nil {
			databaseError(w, err)
			return
		}
		serveImage(w, data, true)
		return
	}
	permission := ""
	if logo {
		permission = "school.write"
	}
	tx, actor, ok := s.BeginAuthorized(w, r, permission, r.Method != "GET")
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if r.Method == "GET" {
		var data []byte
		err := tx.QueryRow(r.Context(), `SELECT avatar FROM users WHERE school_id=$1 AND id=$2 AND avatar IS NOT NULL`, s.schoolID, actor.ID).Scan(&data)
		if errors.Is(err, pgx.ErrNoRows) {
			httpresponse.Error(w, 404, "NOT_FOUND", "Foto profil belum ditambahkan.")
			return
		}
		if err != nil {
			databaseError(w, err)
			return
		}
		serveImage(w, data, false)
		return
	}
	var data []byte
	if r.Method == "PUT" {
		var err error
		data, err = images.Normalize(r.Body, logo)
		if err != nil {
			status := 422
			if errors.Is(err, images.ErrTooLarge) {
				status = 413
			}
			httpresponse.Error(w, status, "INVALID_IMAGE", err.Error())
			return
		}
	}
	var err error
	var revision string
	action, path := "auth.photo_updated", "/auth/profile/photo"
	targetType, targetID := "user", actor.ID
	if logo {
		action, path = "school.logo_updated", "/school/logo"
		targetType, targetID = "school", s.schoolID
		err = tx.QueryRow(r.Context(), `UPDATE schools SET logo=$2 WHERE id=$1 RETURNING COALESCE(md5(logo),'')`, s.schoolID, data).Scan(&revision)
	} else {
		err = tx.QueryRow(r.Context(), `UPDATE users SET avatar=$3,updated_at=now() WHERE school_id=$1 AND id=$2 RETURNING COALESCE(md5(avatar),'')`, s.schoolID, actor.ID, data).Scan(&revision)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(school_id,actor_id,action,target_type,target_id,request_id,metadata) VALUES($1,$2,$3,$4,$5,$6,jsonb_build_object('removed',$7::boolean))`, s.schoolID, actor.ID, action, targetType, targetID, w.Header().Get("X-Request-ID"), r.Method == "DELETE")
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	httpresponse.Data(w, 200, map[string]any{"image_url": imageURL(path, revision)})
}
