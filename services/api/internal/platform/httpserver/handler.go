package httpserver

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"

	"attendance.local/api/internal/platform/httpresponse"
	"attendance.local/api/internal/platform/weborigin"
)

func NewHandler(ping func(context.Context) error, webOrigin string, register func(*http.ServeMux)) http.Handler {
	return NewHandlerWithLocalOrigins(ping, webOrigin, false, register)
}

func NewHandlerWithLocalOrigins(ping func(context.Context) error, webOrigin string, allowLocalWebPortRange bool, register func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	if register != nil {
		register(mux)
	}
	mux.HandleFunc("/api/v1/health/live", func(w http.ResponseWriter, r *http.Request) {
		if !healthMethod(w, r) {
			return
		}
		httpresponse.JSON(w, 200, map[string]any{"status": "ok", "request_id": w.Header().Get("X-Request-ID")})
	})
	mux.HandleFunc("/api/v1/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if !healthMethod(w, r) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ping(ctx); err != nil {
			httpresponse.Error(w, 503, "DATABASE_UNAVAILABLE", "Layanan belum siap")
			return
		}
		httpresponse.JSON(w, 200, map[string]any{"status": "ok", "request_id": w.Header().Get("X-Request-ID")})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpresponse.Error(w, 404, "NOT_FOUND", "Endpoint tidak tersedia")
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
			if !weborigin.Allowed(origin, webOrigin, allowLocalWebPortRange) {
				httpresponse.Error(w, 403, "FORBIDDEN", "Origin tidak diizinkan")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PATCH, PUT, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")
				w.WriteHeader(204)
				return
			}
		}
		// Upgraded sockets have their own bounded deadlines and live authorization checks.
		if r.URL.Path == "/api/v1/ws/attendance" && r.Method == http.MethodGet {
			mux.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		mux.ServeHTTP(w, r)
	})
}

func healthMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	httpresponse.Error(w, 405, "METHOD_NOT_ALLOWED", "Tindakan ini tidak tersedia.")
	return false
}
