package httpresponse

import (
	"encoding/json"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}, "request_id": w.Header().Get("X-Request-ID")})
}

func Data(w http.ResponseWriter, status int, data any) {
	JSON(w, status, map[string]any{"data": data, "request_id": w.Header().Get("X-Request-ID")})
}
