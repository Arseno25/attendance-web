package attendance

import (
	"attendance.local/api/internal/platform/holidays"
	"attendance.local/api/internal/platform/httpresponse"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
)

var errHolidayBusy = errors.New("holiday sync already running")

// The caller holds the school write lock; an unavailable year never replaces cached dates.
func (s *Service) saveHolidayYear(ctx context.Context, tx pgx.Tx, year int, days []holidays.Day) error {
	if len(days) == 0 {
		_, err := tx.Exec(ctx, `INSERT INTO school_holiday_sync(school_id,year,status) VALUES($1,$2,'unavailable') ON CONFLICT(school_id,year) DO UPDATE SET attempted_at=clock_timestamp(),status='unavailable'`, s.schoolID, year)
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM school_holidays WHERE school_id=$1 AND date>=make_date($2,1,1) AND date<make_date($2+1,1,1)`, s.schoolID, year)
	for _, day := range days {
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO school_holidays(school_id,date,name,type) VALUES($1,$2::date,$3,$4)`, s.schoolID, day.Date, day.Name, day.Type)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO school_holiday_sync(school_id,year,synced_at,status) VALUES($1,$2,clock_timestamp(),'ready') ON CONFLICT(school_id,year) DO UPDATE SET attempted_at=clock_timestamp(),synced_at=clock_timestamp(),status='ready'`, s.schoolID, year)
	}
	return err
}
func (s *Service) syncHolidayYear(ctx context.Context, year int) error {
	if s.holidays == nil {
		return holidays.ErrUnavailable
	}
	if !s.holidayMu.TryLock() {
		return errHolidayBusy
	}
	defer s.holidayMu.Unlock()
	var due bool
	err := s.pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM school_holiday_sync WHERE school_id=$1 AND year=$2 AND attempted_at>clock_timestamp()-CASE WHEN status='ready' THEN interval '24 hours' ELSE interval '1 hour' END)`, s.schoolID, year).Scan(&due)
	if err != nil {
		return err
	}
	if !due {
		return nil
	}
	days, fetchErr := s.holidays.Year(ctx, year)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var active bool
	if err = tx.QueryRow(ctx, `SELECT is_active FROM schools WHERE id=$1 FOR UPDATE`, s.schoolID).Scan(&active); err != nil {
		return err
	}
	if !active {
		return errState
	}
	if err = s.saveHolidayYear(ctx, tx, year, days); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return fetchErr
}
func (s *Service) syncCalendarYears(ctx context.Context) {
	if s.holidays == nil {
		return
	}
	var year int
	if s.pool.QueryRow(ctx, `SELECT extract(year FROM clock_timestamp() AT TIME ZONE timezone)::integer FROM schools WHERE id=$1 AND is_active`, s.schoolID).Scan(&year) != nil {
		return
	}
	for _, y := range []int{year, year + 1} {
		if y <= 2200 {
			_ = s.syncHolidayYear(ctx, y)
		}
	}
}
func (s *Service) calendarSync(w http.ResponseWriter, r *http.Request) {
	if !httpresponse.Method(w, r, "POST") {
		return
	}
	tx, u, ok := s.begin(w, r, "academics.write", true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if !schoolCalendarWrite(w, u) {
		return
	}
	var in struct {
		Year int `json:"year"`
	}
	if !httpresponse.Decode(w, r, &in) {
		return
	}
	if in.Year < 1900 || in.Year > 2200 {
		invalid(w, "Tahun harus antara 1900 dan 2200.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	if !s.holidayMu.TryLock() {
		conflict(w, "Kalender sedang diperbarui. Tunggu sebentar lalu coba lagi.")
		return
	}
	defer s.holidayMu.Unlock()
	var days []holidays.Day
	var fetchErr error = holidays.ErrUnavailable
	if s.holidays != nil {
		days, fetchErr = s.holidays.Year(r.Context(), in.Year)
	}
	tx, u, ok = s.begin(w, r, "academics.write", true)
	if !ok {
		return
	}
	defer tx.Rollback(context.Background())
	if !schoolCalendarWrite(w, u) {
		return
	}
	if err := s.saveHolidayYear(r.Context(), tx, in.Year, days); err != nil {
		dbError(w, err)
		return
	}
	meta := map[string]any{"year": in.Year, "source": holidays.Source}
	if fetchErr != nil {
		err := s.audit(r, tx, u.ID, "attendance.calendar.sync_failed", "school_calendar", "", meta)
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			dbError(w, err)
			return
		}
		httpresponse.Error(w, 503, "HOLIDAYS_UNAVAILABLE", "Data libur belum tersedia atau belum dapat diperbarui. Data yang sudah tersimpan tetap digunakan.")
		return
	}
	data, err := one(r.Context(), tx, `SELECT year,synced_at,(SELECT count(*) FROM school_holidays WHERE school_id=$1 AND extract(year FROM date)=$2) AS days FROM school_holiday_sync WHERE school_id=$1 AND year=$2`, s.schoolID, in.Year)
	if err != nil {
		dbError(w, err)
		return
	}
	s.finish(w, r, tx, u, 200, data, "attendance.calendar.synced", "school_calendar", meta)
}
