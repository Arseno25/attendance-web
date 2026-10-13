package httpresponse

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

func Method(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	if slices.Contains(allowed, r.Method) {
		return true
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	Error(w, 405, "METHOD_NOT_ALLOWED", "Method tidak diizinkan")
	return false
}

func Decode(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		Error(w, 400, "VALIDATION_ERROR", "Gunakan JSON yang valid")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(target); err == nil {
		err = d.Decode(new(any))
		if err == io.EOF {
			return true
		}
	}
	Error(w, 400, "VALIDATION_ERROR", "Request tidak valid atau terlalu besar")
	return false
}

func Pagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	page, size := 1, 20
	var err error
	if q := r.URL.Query().Get("page"); q != "" {
		page, err = strconv.Atoi(q)
		if err != nil {
			page = 0
		}
	}
	if q := r.URL.Query().Get("page_size"); q != "" {
		size, err = strconv.Atoi(q)
		if err != nil {
			size = 0
		}
	}
	if page < 1 || page > 100000 || size < 1 || size > 100 {
		Error(w, 400, "VALIDATION_ERROR", "Pagination tidak valid")
		return 0, 0, false
	}
	return page, size, true
}

func ValidID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	b, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil && len(b) == 16
}
