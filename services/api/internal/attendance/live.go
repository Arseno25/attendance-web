package attendance

import (
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"github.com/gorilla/websocket"
	"net/http"
	"time"
)

// WebSocket carries invalidations only; REST remains the authorized source of rows.
func (s *Service) liveRevision(w http.ResponseWriter, r *http.Request, id string) (string, bool) {
	tx, u, ok := s.begin(w, r, "attendance.read", false)
	if !ok {
		return "", false
	}
	defer tx.Rollback(context.Background())
	se, err := s.getSession(r, tx, u, id, false)
	if err != nil {
		dbError(w, err)
		return "", false
	}
	var revision string
	err = tx.QueryRow(r.Context(), `SELECT concat(
 (SELECT count(*) FROM attendance_session_participants WHERE school_id=$1 AND session_id=$2),':',
 (SELECT max(created_at)::text FROM attendance_session_participants WHERE school_id=$1 AND session_id=$2),':',
 (SELECT max(updated_at)::text FROM attendance_records WHERE school_id=$1 AND session_id=$2),':',
 (SELECT count(*) FROM attendance_records WHERE school_id=$1 AND session_id=$2),':',
 (SELECT max(created_at)::text FROM attendance_corrections WHERE school_id=$1 AND session_id=$2),':',
 (SELECT max(decided_at)::text FROM attendance_corrections WHERE school_id=$1 AND session_id=$2),':',$3::text,':',$4::text)`, s.schoolID, id, se["status"], se["reconciled_at"]).Scan(&revision)
	if err != nil {
		dbError(w, err)
		return "", false
	}
	return revision, true
}

type liveResponse struct{ header http.Header }

func (w liveResponse) Header() http.Header         { return w.header }
func (w liveResponse) WriteHeader(int)             {}
func (w liveResponse) Write(b []byte) (int, error) { return len(b), nil }
func (s *Service) live(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "GET") || !s.auth.CheckOrigin(w, r) {
		return
	}
	id := r.URL.Query().Get("session_id")
	if !httpresponse.ValidID(id) {
		invalid(w, "Sesi live wajib valid")
		return
	}
	revision, ok := s.liveRevision(w, r, id)
	if !ok {
		return
	}
	select {
	case s.liveSlots <- struct{}{}:
		defer func() { <-s.liveSlots }()
	default:
		httpresponse.Error(w, 429, "RATE_LIMITED", "Koneksi live penuh; gunakan muat ulang REST")
		return
	}
	upgrader := websocket.Upgrader{HandshakeTimeout: 5 * time.Second, CheckOrigin: func(req *http.Request) bool { return req.Header.Get("Origin") == r.Header.Get("Origin") }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(256)
	_ = conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(45 * time.Second)) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	notify := func() error {
		_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		return conn.WriteJSON(map[string]string{"event": "attendance.changed"})
	}
	if notify() != nil {
		return
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	lifetime := time.NewTimer(30 * time.Minute)
	defer lifetime.Stop()
	for {
		select {
		case <-done:
			return
		case <-r.Context().Done():
			return
		case <-lifetime.C:
			return
		case <-ping.C:
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(3*time.Second)) != nil {
				return
			}
		case <-tick.C:
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			check := r.Clone(ctx)
			next, valid := s.liveRevision(liveResponse{make(http.Header)}, check, id)
			cancel()
			if !valid {
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Session unavailable"), time.Now().Add(time.Second))
				return
			}
			if next != revision {
				revision = next
				if notify() != nil {
					return
				}
			}
		}
	}
}
