package attendance

import (
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

const deviceColumns = `id::text AS id,name,location,revoked_at,last_seen_at,created_at`

func deviceHash(v string) []byte { h := sha256.Sum256([]byte(v)); return h[:] }
func deviceToken() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func (s *Service) devices(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET", "POST") {
		return
	}
	perm := "devices.health"
	if r.Method == "POST" {
		perm = "devices.manage"
	}
	tx, u, ok := s.begin(w, r, perm, r.Method == "POST")
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if r.Method == "GET" {
		s.list(w, r, tx, `SELECT `+deviceColumns+` FROM camera_devices WHERE school_id=$1`, s.schoolID)
		return
	}
	var in struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Location = strings.TrimSpace(in.Location)
	if !text(in.Name, 120) || !text(in.Location, 120) {
		invalid(w, "Nama dan lokasi perangkat wajib diisi")
		return
	}
	data, err := one(r.Context(), tx, `INSERT INTO camera_devices(school_id,name,location) VALUES($1,$2,$3) RETURNING `+deviceColumns, s.schoolID, in.Name, in.Location)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, tx, u, 201, data, "devices.created", "device", nil)
}
func (s *Service) deviceRevoke(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tx, u, ok := s.begin(w, r, "devices.manage", true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	var in struct {
		Reason string `json:"reason"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if !text(in.Reason, 500) {
		invalid(w, "Alasan pencabutan wajib diisi")
		return
	}
	data, err := one(r.Context(), tx, `UPDATE camera_devices SET revoked_at=COALESCE(revoked_at,clock_timestamp()),revoked_by=COALESCE(revoked_by,$3),revoke_reason=COALESCE(revoke_reason,$4) WHERE school_id=$1 AND id=$2 RETURNING `+deviceColumns, s.schoolID, id, u.ID, in.Reason)
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM device_sessions WHERE school_id=$1 AND device_id=$2`, s.schoolID, id)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM device_pairing_tokens WHERE school_id=$1 AND device_id=$2`, s.schoolID, id)
	}
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, tx, u, 200, data, "devices.revoked", "device", nil)
}
func (s *Service) pairingToken(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tx, u, ok := s.begin(w, r, "devices.manage", true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	var revoked *time.Time
	err := tx.QueryRow(r.Context(), `SELECT revoked_at FROM camera_devices WHERE school_id=$1 AND id=$2 FOR UPDATE`, s.schoolID, id).Scan(&revoked)
	if err != nil {
		dbError(w, err)
		return
	}
	if revoked != nil {
		conflict(w, "Perangkat telah dicabut; daftarkan perangkat baru")
		return
	}
	token, err := deviceToken()
	if err != nil {
		dbError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `DELETE FROM device_pairing_tokens WHERE school_id=$1 AND (device_id=$2 OR expires_at<=clock_timestamp())`, s.schoolID, id)
	var expires time.Time
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO device_pairing_tokens(token_hash,school_id,device_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '5 minutes') RETURNING expires_at`, deviceHash(token), s.schoolID, id).Scan(&expires)
	}
	if err == nil {
		err = s.audit(r, tx, u.ID, "devices.pairing_issued", "device", id, nil)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 201, map[string]any{"pairing_token": token, "expires_at": expires})
}
func (s *Service) kioskOptions(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	tx, _, ok := s.begin(w, r, "devices.health", false)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	s.list(w, r, tx, `SELECT a.id::text AS id,a.name,c.name AS class_name,a.created_at FROM attendance_sessions a JOIN classes c ON c.school_id=a.school_id AND c.id=a.class_id WHERE a.school_id=$1 AND a.status='open' AND a.starts_at<=clock_timestamp() AND a.closes_at>clock_timestamp()`, s.schoolID)
}
func (s *Service) deviceCookieName() string {
	if s.auth.SecureCookies() {
		return "__Host-attendance_device"
	}
	return "attendance_device"
}
func (s *Service) kioskSession(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	tx, u, ok := s.begin(w, r, "devices.health", true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	var in struct {
		Token   string `json:"pairing_token"`
		Session string `json:"session_id"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	if len(in.Token) != 64 || !httpresponse.ValidID(in.Session) {
		invalid(w, "Token pairing dan sesi wajib valid")
		return
	}
	var device, deviceName, sessionName string
	err := tx.QueryRow(r.Context(), `DELETE FROM device_pairing_tokens p USING camera_devices d WHERE p.school_id=$1 AND p.token_hash=$2 AND p.expires_at>clock_timestamp() AND d.school_id=p.school_id AND d.id=p.device_id AND d.revoked_at IS NULL RETURNING p.device_id::text,d.name`, s.schoolID, deviceHash(in.Token)).Scan(&device, &deviceName)
	if err != nil {
		httpresponse.Error(w, 401, "INVALID_PAIRING_TOKEN", "Token tidak valid, kedaluwarsa atau sudah digunakan")
		return
	}
	var open bool
	err = tx.QueryRow(r.Context(), `SELECT status='open' AND starts_at<=clock_timestamp() AND closes_at>clock_timestamp(),name FROM attendance_sessions WHERE school_id=$1 AND id=$2`, s.schoolID, in.Session).Scan(&open, &sessionName)
	if err != nil {
		dbError(w, err)
		return
	}
	if !open {
		conflict(w, "Sesi absensi tidak terbuka")
		return
	}
	token, err := deviceToken()
	if err != nil {
		dbError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `DELETE FROM device_sessions WHERE school_id=$1 AND (device_id=$2 OR expires_at<=clock_timestamp())`, s.schoolID, device)
	var expires time.Time
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO device_sessions(token_hash,school_id,device_id,session_id,expires_at) VALUES($1,$2,$3,$4,clock_timestamp()+interval '10 minutes') RETURNING expires_at`, deviceHash(token), s.schoolID, device, in.Session).Scan(&expires)
	}
	if err == nil {
		err = s.auth.EndCurrentSession(r.Context(), tx, r)
	}
	if err == nil {
		err = s.audit(r, tx, u.ID, "devices.paired", "device", device, map[string]any{"session_id": in.Session, "operator_session_revoked": true})
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.deviceCookieName(), Value: token, Path: "/", HttpOnly: true, Secure: s.auth.SecureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: 600, Expires: expires})
	s.auth.ClearSessionCookie(w)
	httpresponse.Data(w, 201, map[string]any{"device_name": deviceName, "session_name": sessionName, "expires_at": expires, "csrf_token": hex.EncodeToString(deviceHash("device-csrf:" + token)), "recognition_enabled": false})
}
func (s *Service) kioskAuth(w http.ResponseWriter, r *http.Request, write bool) (pgx.Tx, map[string]any, bool) {
	cookie, err := r.Cookie(s.deviceCookieName())
	if err != nil || len(cookie.Value) != 64 {
		httpresponse.Error(w, 401, "DEVICE_UNAUTHENTICATED", "Perangkat belum dipasangkan atau sesi kedaluwarsa")
		return nil, nil, false
	}
	if write {
		expected := hex.EncodeToString(deviceHash("device-csrf:" + cookie.Value))
		if !s.auth.CheckOrigin(w, r) {
			return nil, nil, false
		}
		if subtle.ConstantTimeCompare([]byte(expected), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
			httpresponse.Error(w, 403, "FORBIDDEN", "Token CSRF tidak valid")
			return nil, nil, false
		}
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return nil, nil, false
	}
	lock := "FOR SHARE"
	if write {
		lock = "FOR UPDATE"
	}
	var active bool
	err = tx.QueryRow(r.Context(), `SELECT is_active FROM schools WHERE id=$1 `+lock, s.schoolID).Scan(&active)
	var data map[string]any
	if err == nil && active {
		data, err = one(r.Context(), tx, `SELECT d.id::text AS device_id,d.name AS device_name,a.id::text AS session_id,a.name AS session_name,ds.expires_at FROM device_sessions ds JOIN camera_devices d ON d.school_id=ds.school_id AND d.id=ds.device_id JOIN attendance_sessions a ON a.school_id=ds.school_id AND a.id=ds.session_id WHERE ds.school_id=$1 AND ds.token_hash=$2 AND ds.expires_at>clock_timestamp() AND d.revoked_at IS NULL AND a.status='open' AND a.starts_at<=clock_timestamp() AND a.closes_at>clock_timestamp()`, s.schoolID, deviceHash(cookie.Value))
	}
	if err != nil || !active {
		_ = tx.Rollback(context.Background())
		httpresponse.Error(w, 401, "DEVICE_UNAUTHENTICATED", "Perangkat atau sesi tidak aktif")
		return nil, nil, false
	}
	data["csrf_token"] = hex.EncodeToString(deviceHash("device-csrf:" + cookie.Value))
	data["recognition_enabled"] = false
	return tx, data, true
}
func (s *Service) kioskMe(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") {
		return
	}
	tx, data, ok := s.kioskAuth(w, r, false)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	httpresponse.Data(w, 200, data)
}
func (s *Service) heartbeat(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	tx, data, ok := s.kioskAuth(w, r, true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	_, err := tx.Exec(r.Context(), `UPDATE camera_devices SET last_seen_at=clock_timestamp() WHERE school_id=$1 AND id=$2`, s.schoolID, data["device_id"])
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	httpresponse.Data(w, 200, data)
}
func (s *Service) recognize(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	tx, _, ok := s.kioskAuth(w, r, true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	httpresponse.Error(w, 503, "RECOGNITION_DISABLED", "Recognition menunggu evaluasi model, liveness dan kalibrasi; gunakan absensi manual")
}
