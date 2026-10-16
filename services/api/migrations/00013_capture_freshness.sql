-- +goose Up
CREATE TABLE kiosk_capture_challenges (
 school_id uuid NOT NULL REFERENCES schools(id), event_id uuid NOT NULL DEFAULT gen_random_uuid(),
 session_token_hash bytea NOT NULL CHECK(octet_length(session_token_hash)=32),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '10 seconds',
 consumed_at timestamptz, image_hash bytea CHECK(image_hash IS NULL OR octet_length(image_hash)=32),
 PRIMARY KEY(school_id,event_id), UNIQUE(school_id,image_hash)
);
CREATE INDEX kiosk_capture_session ON kiosk_capture_challenges(school_id,session_token_hash) WHERE consumed_at IS NULL;
CREATE INDEX kiosk_capture_expiry ON kiosk_capture_challenges(expires_at);
CREATE INDEX device_session_operator ON device_sessions(school_id,paired_by);
-- +goose Down
DROP INDEX device_session_operator;
DROP TABLE kiosk_capture_challenges;
