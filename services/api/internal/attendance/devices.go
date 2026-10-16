package attendance

import (
	"attendance.local/api/internal/auth"
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

const deviceLifetime = 30 * 24 * time.Hour

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
	tx, u, ok := s.begin(w, r, "devices.health", false)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	s.list(w, r, tx, `SELECT a.id::text AS id,a.name,c.name AS class_name,a.created_at FROM attendance_sessions a JOIN classes c ON c.school_id=a.school_id AND c.id=a.class_id WHERE a.school_id=$1 AND a.status='open' AND a.starts_at<=clock_timestamp() AND a.closes_at>clock_timestamp() AND (NOT $2::boolean OR a.class_id=ANY($3::uuid[]))`, s.schoolID, u.Scope == auth.ScopeAssignedClasses, u.ClassIDs)
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
	var device, deviceName, sessionName, timezone, className, classID string
	var serverTime, starts, late, closes time.Time
	var checkout *time.Time
	err := tx.QueryRow(r.Context(), `DELETE FROM device_pairing_tokens p USING camera_devices d WHERE p.school_id=$1 AND p.token_hash=$2 AND p.expires_at>clock_timestamp() AND d.school_id=p.school_id AND d.id=p.device_id AND d.revoked_at IS NULL RETURNING p.device_id::text,d.name`, s.schoolID, deviceHash(in.Token)).Scan(&device, &deviceName)
	if err != nil {
		httpresponse.Error(w, 401, "INVALID_PAIRING_TOKEN", "Kode penghubung tidak valid, sudah berakhir, atau telah digunakan.")
		return
	}
	var open bool
	err = tx.QueryRow(r.Context(), `SELECT a.status='open' AND a.starts_at<=clock_timestamp() AND a.closes_at>clock_timestamp(),a.name,a.timezone,c.name,clock_timestamp(),a.starts_at,a.late_after,a.closes_at,a.checkout_at,a.class_id::text FROM attendance_sessions a JOIN classes c ON c.school_id=a.school_id AND c.id=a.class_id WHERE a.school_id=$1 AND a.id=$2`, s.schoolID, in.Session).Scan(&open, &sessionName, &timezone, &className, &serverTime, &starts, &late, &closes, &checkout, &classID)
	if err != nil {
		dbError(w, err)
		return
	}
	if !u.Can("devices.health", s.schoolID, classID) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Sesi tidak ditemukan")
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
	expires := serverTime.Add(deviceLifetime)
	var authorizationEnds *time.Time
	if u.Scope == auth.ScopeAssignedClasses {
		var grantEnds time.Time
		err = tx.QueryRow(r.Context(), `SELECT ends_at FROM user_class_assignments WHERE school_id=$1 AND user_id=$2 AND class_id=$3 AND is_active AND starts_at<=clock_timestamp() AND ends_at>clock_timestamp()`, s.schoolID, u.ID, classID).Scan(&grantEnds)
		if err != nil {
			dbError(w, err)
			return
		}
		authorizationEnds = &grantEnds
		// A replacement assignment must never revive an old device credential.
		if grantEnds.Before(expires) {
			expires = grantEnds
		}
	}
	_, err = tx.Exec(r.Context(), `DELETE FROM device_sessions WHERE school_id=$1 AND (device_id=$2 OR expires_at<=clock_timestamp())`, s.schoolID, device)
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO device_sessions(token_hash,school_id,device_id,session_id,paired_by,expires_at,authorization_ends_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING expires_at`, deviceHash(token), s.schoolID, device, in.Session, u.ID, expires, authorizationEnds).Scan(&expires)
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
	s.setDeviceCookie(w, token, expires)
	s.auth.ClearSessionCookie(w)
	httpresponse.Data(w, 201, map[string]any{"device_name": deviceName, "session_id": in.Session, "session_ready": true, "connection_status": "ready", "session_name": sessionName, "timezone": timezone, "class_name": className, "server_time": serverTime, "starts_at": starts, "late_after": late, "closes_at": closes, "checkout_at": checkout, "expires_at": expires, "csrf_token": hex.EncodeToString(deviceHash("device-csrf:" + token)), "recognition_enabled": s.ai.Status(r.Context()).RecognitionEnabled})
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
			httpresponse.Error(w, 403, "FORBIDDEN", "Halaman sudah tidak berlaku. Muat ulang dan coba lagi.")
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
		data, err = one(r.Context(), tx, `SELECT d.id::text AS device_id,d.name AS device_name,
 a.id::text AS session_id,a.name AS session_name,COALESCE(a.timezone,anchor.timezone) AS timezone,c.name AS class_name,
 clock_timestamp() AS server_time,a.starts_at,a.late_after,a.closes_at,a.checkout_at,
 ds.expires_at,ds.paired_by::text AS operator_id,anchor.class_id::text AS class_id,
 COALESCE(candidates.total,0)::int AS active_sessions
 FROM device_sessions ds
 JOIN camera_devices d ON d.school_id=ds.school_id AND d.id=ds.device_id
 JOIN attendance_sessions anchor ON anchor.school_id=ds.school_id AND anchor.id=ds.session_id
 JOIN classes c ON c.school_id=anchor.school_id AND c.id=anchor.class_id
 LEFT JOIN LATERAL (
 SELECT count(*) AS total,(array_agg(current.id ORDER BY current.starts_at))[1] AS id
 FROM attendance_sessions current WHERE current.school_id=ds.school_id AND current.class_id=anchor.class_id
 AND current.status='open' AND current.starts_at<=clock_timestamp() AND current.closes_at>clock_timestamp()
 ) candidates ON true
 LEFT JOIN attendance_sessions a ON a.school_id=ds.school_id AND a.id=candidates.id AND candidates.total=1
 WHERE ds.school_id=$1 AND ds.token_hash=$2 AND ds.paired_by IS NOT NULL
 AND ds.expires_at>clock_timestamp() AND (ds.authorization_ends_at IS NULL OR ds.authorization_ends_at>clock_timestamp())
 AND d.revoked_at IS NULL`, s.schoolID, deviceHash(cookie.Value))
	}
	if err == nil && active {
		active, err = s.auth.UserCan(r.Context(), tx, data["operator_id"].(string), "devices.health", data["class_id"].(string))
		delete(data, "operator_id")
		delete(data, "class_id")
	}
	if err != nil || !active {
		if err == nil && !active {
			// An expired class assignment must not revive an old credential if access is granted again.
			if _, err = tx.Exec(r.Context(), `DELETE FROM device_sessions WHERE school_id=$1 AND token_hash=$2`, s.schoolID, deviceHash(cookie.Value)); err == nil {
				_ = tx.Commit(r.Context())
			}
		}
		_ = tx.Rollback(context.Background())
		httpresponse.Error(w, 401, "DEVICE_UNAUTHENTICATED", "Perangkat atau sesi tidak aktif")
		return nil, nil, false
	}
	ready := data["session_id"] != nil
	data["session_ready"] = ready
	data["connection_status"] = "waiting_session"
	if ready {
		data["connection_status"] = "ready"
		if write {
			_, err = tx.Exec(r.Context(), `UPDATE device_sessions SET session_id=$3 WHERE school_id=$1 AND token_hash=$2`, s.schoolID, deviceHash(cookie.Value), data["session_id"])
			if err != nil {
				_ = tx.Rollback(context.Background())
				dbError(w, err)
				return nil, nil, false
			}
		}
	} else if data["active_sessions"].(int32) > 1 {
		data["connection_status"] = "multiple_sessions"
	}
	delete(data, "active_sessions")
	if !ready && r.URL.Path != "/api/v1/kiosk/me" && r.URL.Path != "/api/v1/kiosk/heartbeat" {
		_ = tx.Rollback(context.Background())
		httpresponse.Error(w, 409, "SESSION_UNAVAILABLE", "Perangkat terhubung. Menunggu satu sesi absensi aktif untuk kelas ini.")
		return nil, nil, false
	}
	data["csrf_token"] = hex.EncodeToString(deviceHash("device-csrf:" + cookie.Value))
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
	data["recognition_enabled"] = s.ai.Status(r.Context()).RecognitionEnabled
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
	cookie, _ := r.Cookie(s.deviceCookieName())
	var expires time.Time
	err := tx.QueryRow(r.Context(), `UPDATE device_sessions SET expires_at=LEAST(clock_timestamp()+interval '30 days',authorization_ends_at) WHERE school_id=$1 AND token_hash=$2 RETURNING expires_at`, s.schoolID, deviceHash(cookie.Value)).Scan(&expires)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE camera_devices SET last_seen_at=clock_timestamp() WHERE school_id=$1 AND id=$2`, s.schoolID, data["device_id"])
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		dbError(w, err)
		return
	}
	data["expires_at"] = expires
	data["recognition_enabled"] = s.ai.Status(r.Context()).RecognitionEnabled
	s.setDeviceCookie(w, cookie.Value, expires)
	httpresponse.Data(w, 200, data)
}

func (s *Service) setDeviceCookie(w http.ResponseWriter, token string, expires time.Time) {
	maxAge := int(time.Until(expires).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{Name: s.deviceCookieName(), Value: token, Path: "/", HttpOnly: true, Secure: s.auth.SecureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: maxAge, Expires: expires})
}
