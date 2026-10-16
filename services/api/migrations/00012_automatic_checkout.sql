-- +goose Up
ALTER TABLE attendance_schedules ADD COLUMN checkout_start time;
ALTER TABLE attendance_schedules ADD CONSTRAINT checkout_schedule_order CHECK
 (checkout_start IS NULL OR (checkin_cutoff + grace_minutes * interval '1 minute' < checkout_start AND checkout_start < close_time));
ALTER TABLE attendance_sessions ADD COLUMN checkout_at timestamptz;
ALTER TABLE attendance_sessions ADD CONSTRAINT checkout_session_order CHECK
 (checkout_at IS NULL OR (late_after < checkout_at AND checkout_at < closes_at));

-- +goose Down
ALTER TABLE attendance_sessions DROP COLUMN checkout_at;
ALTER TABLE attendance_schedules DROP COLUMN checkout_start;
