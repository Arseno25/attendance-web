package masterdata

import (
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type importRow struct {
	NIS  string `json:"nis"`
	NISN string `json:"nisn"`
	Name string `json:"full_name"`
}
type importError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

func parseImport(body string) ([]importRow, []importError, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(body, "\ufeff")))
	reader.FieldsPerRecord = 3
	header, err := reader.Read()
	if err != nil || strings.Join(header, ",") != "nis,nisn,full_name" {
		return nil, nil, errors.New("Gunakan header CSV nis,nisn,full_name")
	}
	data := []importRow{}
	issues := []importError{}
	seen := map[string]bool{}
	for n := 2; ; n++ {
		cells, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, errors.New("CSV tidak valid atau jumlah kolom berbeda")
		}
		if len(data) >= 500 {
			return nil, nil, errors.New("Maksimal 500 siswa per import")
		}
		row := importRow{strings.TrimSpace(cells[0]), strings.TrimSpace(cells[1]), strings.TrimSpace(cells[2])}
		data = append(data, row)
		if !text(row.NIS, 30) || len([]rune(row.NISN)) > 32 || !text(row.Name, 120) || strings.ContainsRune(row.NIS+row.NISN+row.Name, '\x00') {
			issues = append(issues, importError{n, "NIS/nama wajib diisi dan batas panjang harus sesuai"})
		}
		if seen[row.NIS] {
			issues = append(issues, importError{n, "NIS berulang di file"})
		}
		seen[row.NIS] = true
	}
	if len(data) == 0 {
		return nil, nil, errors.New("CSV kosong")
	}
	return data, issues, nil
}
func (s *Service) importDry(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	sc, ok := s.begin(w, r, "students.write", true)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	var in struct {
		CSV   string `json:"csv"`
		Class string `json:"class_id"`
	}
	if !httpresponse.DecodeLimit(w, r, &in, 128<<10) {
		return
	}
	if !httpresponse.ValidID(in.Class) {
		invalid(w, "Kelas tidak valid")
		return
	}
	in.Class = strings.ToLower(in.Class)
	if _, err := s.classBounds(r, sc, in.Class); err != nil {
		dbError(w, err)
		return
	}
	data, issues, err := parseImport(in.CSV)
	if err != nil {
		invalid(w, err.Error())
		return
	}
	for i, row := range data {
		var exists bool
		err = sc.tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM students WHERE school_id=$1 AND nis=$2)`, s.schoolID, row.NIS).Scan(&exists)
		if err != nil {
			dbError(w, err)
			return
		}
		if exists {
			issues = append(issues, importError{i + 2, "NIS sudah digunakan di sekolah"})
		}
	}
	payload, _ := json.Marshal(data)
	canonical, _ := json.Marshal(map[string]any{"class_id": in.Class, "rows": data})
	sum := sha256.Sum256(canonical)
	hash := hex.EncodeToString(sum[:])
	_, err = sc.tx.Exec(r.Context(), `UPDATE import_batches SET payload=NULL WHERE school_id=$1 AND expires_at<=clock_timestamp() AND payload IS NOT NULL`, s.schoolID)
	var entity map[string]any
	if err == nil {
		entity, err = one(r.Context(), sc.tx, `INSERT INTO import_batches(school_id,actor_id,class_id,payload,fingerprint,valid,row_count) VALUES($1,$2,$3,$4::jsonb,$5,$6,$7) RETURNING id::text AS id,expires_at`, s.schoolID, sc.user.ID, in.Class, string(payload), hash, len(issues) == 0, len(data))
	}
	if err != nil {
		dbError(w, err)
		return
	}
	entity["hash"] = hash
	entity["valid"] = len(issues) == 0
	entity["row_count"] = len(data)
	entity["errors"] = issues
	entity["preview"] = data
	s.finish(w, r, sc, 201, entity, "students.import.previewed", "import_batch", map[string]any{"row_count": len(data), "valid": len(issues) == 0})
}
func (s *Service) importCommit(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sc, ok := s.begin(w, r, "students.write", true)
	if !ok {
		return
	}
	defer sc.tx.Rollback(context.Background())
	var in struct {
		Hash string `json:"hash"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	var hash, class string
	var payload []byte
	var valid bool
	var expires time.Time
	var committed *time.Time
	var count int
	err := sc.tx.QueryRow(r.Context(), `SELECT fingerprint,class_id::text,payload,valid,expires_at,committed_at,row_count FROM import_batches WHERE school_id=$1 AND id=$2 AND actor_id=$3 FOR UPDATE`, s.schoolID, id, sc.user.ID).Scan(&hash, &class, &payload, &valid, &expires, &committed, &count)
	if err != nil {
		dbError(w, err)
		return
	}
	if hash != in.Hash {
		conflict(w, "Hash preview berubah; buat preview baru")
		return
	}
	if committed != nil {
		httpresponse.Data(w, 200, map[string]any{"id": id, "imported": count, "committed_at": committed})
		return
	}
	if !valid || !expires.After(time.Now()) || payload == nil {
		conflict(w, "Preview invalid atau kedaluwarsa; buat preview baru")
		return
	}
	if _, err = s.classBounds(r, sc, class); err != nil {
		dbError(w, err)
		return
	}
	var data []importRow
	if err = json.Unmarshal(payload, &data); err != nil {
		dbError(w, err)
		return
	}
	for _, row := range data {
		var student string
		err = sc.tx.QueryRow(r.Context(), `INSERT INTO students(school_id,nis,nisn,full_name,status) VALUES($1,$2,$3,$4,'active') RETURNING id::text`, s.schoolID, row.NIS, nullable(row.NISN), row.Name).Scan(&student)
		if err == nil {
			_, err = s.insertPlacement(r, sc, student, class, "", "Import siswa")
		}
		if err != nil {
			dbError(w, err)
			return
		}
	}
	entity, err := one(r.Context(), sc.tx, `UPDATE import_batches SET committed_at=clock_timestamp(),payload=NULL WHERE school_id=$1 AND id=$2 RETURNING id::text AS id,row_count AS imported,committed_at`, s.schoolID, id)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, sc, 200, entity, "students.import.committed", "import_batch", map[string]any{"row_count": count})
}
