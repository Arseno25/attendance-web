package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"attendance.local/api/internal/platform/httpresponse"
	"github.com/jackc/pgx/v5"
)

type schoolSettings struct {
	Name    string `json:"name"`
	AppName string `json:"app_name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	LogoURL string `json:"logo_url"`
}

func imageURL(path, revision string) string {
	if revision == "" {
		return ""
	}
	return path + "?v=" + revision
}

func (s *Service) branding(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	var name, appName, revision string
	err := s.pool.QueryRow(r.Context(), `SELECT name,app_name,COALESCE(md5(logo),'') FROM schools WHERE id=$1 AND is_active`, s.schoolID).Scan(&name, &appName, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Sekolah belum tersedia.")
		return
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpresponse.Data(w, 200, map[string]any{"name": name, "app_name": appName, "logo_url": imageURL("/school/logo", revision)})
}
func (s *Service) schoolSettings(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "PATCH") {
		return
	}
	tx, actor, ok := s.BeginAuthorized(w, r, "school.write", r.Method == "PATCH")
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	var input schoolSettings
	if r.Method == "PATCH" {
		var body struct {
			Name    string `json:"name"`
			AppName string `json:"app_name"`
			Address string `json:"address"`
			Phone   string `json:"phone"`
			Email   string `json:"email"`
		}
		if !httpresponse.Decode(w, r, &body) {
			return
		}
		input = schoolSettings{Name: strings.TrimSpace(body.Name), AppName: strings.TrimSpace(body.AppName), Address: strings.TrimSpace(body.Address), Phone: strings.TrimSpace(body.Phone), Email: strings.TrimSpace(body.Email)}
		if !validName(input.Name) || input.AppName == "" || utf8.RuneCountInString(input.AppName) > 80 || utf8.RuneCountInString(input.Address) > 300 || utf8.RuneCountInString(input.Phone) > 30 || strings.Trim(input.Phone, "+0123456789() .-") != "" {
			httpresponse.Error(w, 400, "VALIDATION_ERROR", "Periksa nama sekolah dan informasi kontak yang Anda isi.")
			return
		}
		if input.Email != "" {
			var err error
			input.Email, err = normalizeEmail(input.Email)
			if err != nil {
				httpresponse.Error(w, 400, "VALIDATION_ERROR", "Masukkan alamat email sekolah yang valid.")
				return
			}
		}
		_, err := tx.Exec(r.Context(), `UPDATE schools SET name=$2,app_name=$3,address=$4,phone=$5,email=$6 WHERE id=$1`, s.schoolID, input.Name, input.AppName, input.Address, input.Phone, input.Email)
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(school_id,actor_id,action,target_type,target_id,request_id,metadata) VALUES($1,$2,'school.settings_updated','school',$1,$3,'{}')`, s.schoolID, actor.ID, w.Header().Get("X-Request-ID"))
		}
		if err != nil {
			databaseError(w, err)
			return
		}
	}
	var revision string
	err := tx.QueryRow(r.Context(), `SELECT name,app_name,address,phone,email,COALESCE(md5(logo),'') FROM schools WHERE id=$1`, s.schoolID).Scan(&input.Name, &input.AppName, &input.Address, &input.Phone, &input.Email, &revision)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	input.LogoURL = imageURL("/school/logo", revision)
	httpresponse.Data(w, 200, input)
}
