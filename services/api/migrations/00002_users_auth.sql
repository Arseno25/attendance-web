-- +goose Up
CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  school_id uuid NOT NULL REFERENCES schools(id),
  email text NOT NULL CHECK (email = lower(btrim(email)) AND length(email) <= 254),
  full_name text NOT NULL CHECK (length(btrim(full_name)) BETWEEN 1 AND 120),
  password_hash text NOT NULL,
  role text NOT NULL CHECK (role IN ('super_admin','admin','teacher','operator')),
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (school_id, email),
  UNIQUE (school_id, id)
);

CREATE TABLE user_sessions (
  token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
  school_id uuid NOT NULL,
  user_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  FOREIGN KEY (school_id, user_id) REFERENCES users(school_id, id),
  CHECK (expires_at > created_at)
);
CREATE INDEX user_sessions_user_idx ON user_sessions(school_id, user_id);
CREATE INDEX user_sessions_expiry_idx ON user_sessions(expires_at);

CREATE TABLE permission_grants (
  school_id uuid NOT NULL,
  user_id uuid NOT NULL,
  permission text NOT NULL CHECK (permission IN (
    'biometrics.enroll','biometrics.revoke','consents.manage',
    'attendance.manual','attendance.approve','reports.export'
  )),
  granted_by uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (school_id, user_id, permission),
  FOREIGN KEY (school_id, user_id) REFERENCES users(school_id, id),
  FOREIGN KEY (school_id, granted_by) REFERENCES users(school_id, id)
);

CREATE TABLE audit_logs (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  school_id uuid NOT NULL REFERENCES schools(id),
  actor_id uuid,
  action text NOT NULL,
  target_type text NOT NULL,
  target_id uuid,
  request_id text NOT NULL,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (school_id, actor_id) REFERENCES users(school_id, id)
);
CREATE INDEX audit_logs_school_id_idx ON audit_logs(school_id, id DESC);

-- +goose StatementBegin
CREATE FUNCTION prevent_audit_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit rows are append-only';
END $$;
-- +goose StatementEnd
CREATE TRIGGER audit_append_only BEFORE UPDATE OR DELETE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION prevent_audit_rewrite();

-- +goose Down
DROP TABLE audit_logs;
DROP FUNCTION prevent_audit_rewrite();
DROP TABLE permission_grants;
DROP TABLE user_sessions;
DROP TABLE users;
