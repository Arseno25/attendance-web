package attendance

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"time"
)

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		job, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := s.Tick(job)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("attendance lifecycle deferred", "school_id", s.schoolID)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) Tick(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var active bool
	err = tx.QueryRow(ctx, `SELECT is_active FROM schools WHERE id=$1 FOR UPDATE`, s.schoolID).Scan(&active)
	if err != nil {
		return err
	}
	if !active {
		return tx.Commit(ctx)
	}
	schedules, err := rows(ctx, tx, `SELECT a.id::text AS id,(statement_timestamp() AT TIME ZONE a.timezone)::date::text AS date FROM attendance_schedules a JOIN classes c ON c.school_id=a.school_id AND c.id=a.class_id JOIN academic_years y ON y.school_id=c.school_id AND y.id=c.academic_year_id WHERE a.school_id=$1 AND a.is_active AND c.is_active AND y.is_active AND (statement_timestamp() AT TIME ZONE a.timezone)::date BETWEEN y.starts_on AND y.ends_on`, s.schoolID)
	if err != nil {
		return err
	}
	for _, a := range schedules {
		createdID, createErr := s.createSession(ctx, tx, a["id"].(string), a["date"].(string))
		err = createErr
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO audit_logs(school_id,action,target_type,target_id,request_id,metadata) VALUES($1,'attendance.sessions.created','attendance_session',$2,$3,'{"source":"lifecycle_worker"}'::jsonb)`, s.schoolID, createdID, rand.Text())
		}
		if err != nil && !errors.Is(err, errState) {
			return err
		}
	}
	sessions, err := rows(ctx, tx, `SELECT se.id::text AS id,se.status,se.starts_at,se.closes_at,statement_timestamp() AS clock,`+sessionEligibility+` AS eligible FROM attendance_sessions se JOIN attendance_schedules a ON a.school_id=se.school_id AND a.id=se.schedule_id JOIN classes c ON c.school_id=se.school_id AND c.id=se.class_id JOIN academic_years y ON y.school_id=c.school_id AND y.id=c.academic_year_id LEFT JOIN school_calendar_days cd ON cd.school_id=se.school_id AND cd.date=se.date WHERE se.school_id=$1 AND se.status IN('scheduled','open') FOR UPDATE OF se`, s.schoolID)
	if err != nil {
		return err
	}
	for _, se := range sessions {
		id := se["id"].(string)
		now := se["clock"].(time.Time)
		action := ""
		if se["status"] == "scheduled" {
			if !now.Before(se["closes_at"].(time.Time)) {
				_, err = tx.Exec(ctx, `UPDATE attendance_sessions SET status='cancelled',closed_at=clock_timestamp() WHERE school_id=$1 AND id=$2`, s.schoolID, id)
				action = "cancel"
			} else if se["eligible"].(bool) && !now.Before(se["starts_at"].(time.Time)) {
				err = s.openSession(ctx, tx, id)
				action = "open"
			}
		} else if !now.Before(se["closes_at"].(time.Time)) {
			_, err = tx.Exec(ctx, `UPDATE attendance_sessions SET status='closed',closed_at=clock_timestamp() WHERE school_id=$1 AND id=$2`, s.schoolID, id)
			action = "close"
		}
		if err != nil {
			return err
		}
		if action != "" {
			_, err = tx.Exec(ctx, `INSERT INTO audit_logs(school_id,action,target_type,target_id,request_id,metadata) VALUES($1,$2,'attendance_session',$3,$4,'{"source":"lifecycle_worker"}'::jsonb)`, s.schoolID, "attendance.sessions."+action, id, rand.Text())
			if err != nil {
				return err
			}
		}
	}
	for _, query := range []string{
		`UPDATE import_batches SET payload=NULL WHERE school_id=$1 AND expires_at<=clock_timestamp() AND payload IS NOT NULL`,
		`DELETE FROM device_sessions WHERE school_id=$1 AND expires_at<=clock_timestamp()`,
		`DELETE FROM device_pairing_tokens WHERE school_id=$1 AND expires_at<=clock_timestamp()`,
	} {
		if _, err = tx.Exec(ctx, query, s.schoolID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
