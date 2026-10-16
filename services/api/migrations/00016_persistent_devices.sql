-- +goose Up
ALTER TABLE device_sessions ADD COLUMN authorization_ends_at timestamptz;
-- Existing short-lived credentials keep their original authorization boundary.
UPDATE device_sessions SET authorization_ends_at=expires_at;
-- +goose Down
ALTER TABLE device_sessions DROP COLUMN authorization_ends_at;
