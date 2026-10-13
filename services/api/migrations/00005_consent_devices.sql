-- +goose Up
CREATE TABLE guardians (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, student_id uuid NOT NULL,
 full_name text NOT NULL CHECK(length(btrim(full_name)) BETWEEN 1 AND 120), relationship text NOT NULL CHECK(relationship IN('parent','guardian')),
 verification_ref text NOT NULL CHECK(length(btrim(verification_ref)) BETWEEN 1 AND 120), verified_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(school_id,student_id) REFERENCES students(school_id,id), FOREIGN KEY(school_id,verified_by) REFERENCES users(school_id,id), UNIQUE(school_id,id,student_id)
);
CREATE TABLE biometric_consents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, student_id uuid NOT NULL, guardian_id uuid NOT NULL,
 policy_version text NOT NULL CHECK(length(btrim(policy_version)) BETWEEN 1 AND 80), evidence_ref text NOT NULL CHECK(length(btrim(evidence_ref)) BETWEEN 1 AND 120),
 recorded_by uuid NOT NULL, valid_until timestamptz NOT NULL, withdrawn_at timestamptz, withdrawn_by uuid, withdrawal_reason text,
 created_at timestamptz NOT NULL DEFAULT now(), CHECK(valid_until>created_at),
 CHECK((withdrawn_at IS NULL AND withdrawn_by IS NULL AND withdrawal_reason IS NULL) OR (withdrawn_at IS NOT NULL AND withdrawn_by IS NOT NULL AND length(btrim(withdrawal_reason)) BETWEEN 1 AND 500)),
 FOREIGN KEY(school_id,guardian_id,student_id) REFERENCES guardians(school_id,id,student_id), FOREIGN KEY(school_id,recorded_by) REFERENCES users(school_id,id), FOREIGN KEY(school_id,withdrawn_by) REFERENCES users(school_id,id), UNIQUE(school_id,id)
);
CREATE INDEX consent_student ON biometric_consents(school_id,student_id,valid_until);
CREATE TABLE biometric_tombstones (
 school_id uuid NOT NULL, consent_id uuid NOT NULL, student_id uuid NOT NULL, withdrawn_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(school_id,consent_id), FOREIGN KEY(school_id,consent_id) REFERENCES biometric_consents(school_id,id), FOREIGN KEY(school_id,student_id) REFERENCES students(school_id,id)
);
CREATE TRIGGER consent_tombstones_immutable BEFORE UPDATE OR DELETE ON biometric_tombstones FOR EACH ROW EXECUTE FUNCTION attendance_append_only();
CREATE TABLE camera_devices (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL REFERENCES schools(id), name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120), location text NOT NULL CHECK(length(btrim(location)) BETWEEN 1 AND 120),
 revoked_at timestamptz, revoked_by uuid, revoke_reason text, last_seen_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(school_id,revoked_by) REFERENCES users(school_id,id), UNIQUE(school_id,id)
);
CREATE TABLE device_pairing_tokens (
 token_hash bytea PRIMARY KEY CHECK(octet_length(token_hash)=32), school_id uuid NOT NULL, device_id uuid NOT NULL, expires_at timestamptz NOT NULL,
 FOREIGN KEY(school_id,device_id) REFERENCES camera_devices(school_id,id)
);
CREATE TABLE device_sessions (
 token_hash bytea PRIMARY KEY CHECK(octet_length(token_hash)=32), school_id uuid NOT NULL, device_id uuid NOT NULL, session_id uuid NOT NULL,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(school_id,device_id) REFERENCES camera_devices(school_id,id), FOREIGN KEY(school_id,session_id) REFERENCES attendance_sessions(school_id,id)
);
-- +goose Down
DROP TABLE device_sessions;
DROP TABLE device_pairing_tokens;
DROP TABLE camera_devices;
DROP TABLE biometric_tombstones;
DROP TABLE biometric_consents;
DROP TABLE guardians;
