package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

func NewHandler(ping func(context.Context) error, webOrigin string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health/live", func(w http.ResponseWriter, r *http.Request) {
		if !healthMethod(w, r) {
			return
		}
		writeJSON(w, 200, map[string]any{"status": "ok", "request_id": w.Header().Get("X-Request-ID")})
	})
	mux.HandleFunc("/api/v1/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if !healthMethod(w, r) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ping(ctx); err != nil {
			writeError(w, 503, "DATABASE_UNAVAILABLE", "Layanan belum siap")
			return
		}
		writeJSON(w, 200, map[string]any{"status": "ok", "request_id": w.Header().Get("X-Request-ID")})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, 404, "NOT_FOUND", "Endpoint tidak tersedia")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := rand.Text()
		defer func() {
			slog.Info("request_completed", "request_id", requestID, "method", r.Method, "route", r.Pattern, "duration_ms", time.Since(started).Milliseconds())
		}()
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origin != "" {
			if origin != webOrigin {
				writeError(w, 403, "FORBIDDEN", "Origin tidak diizinkan")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")
				w.WriteHeader(204)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func healthMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	writeError(w, 405, "METHOD_NOT_ALLOWED", "Method tidak diizinkan")
	return false
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error":      map[string]string{"code": code, "message": message},
		"request_id": w.Header().Get("X-Request-ID"),
	})
}
