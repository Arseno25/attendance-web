-- +goose Up
CREATE TABLE school_calendar_days (
 school_id uuid NOT NULL REFERENCES schools(id), date date NOT NULL, is_school_day boolean NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 500),
 updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(school_id,date)
);
CREATE TABLE attendance_schedules (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, class_id uuid NOT NULL, name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120),
 timezone text NOT NULL, weekdays integer[] NOT NULL CHECK(cardinality(weekdays) BETWEEN 1 AND 7 AND weekdays <@ ARRAY[1,2,3,4,5,6,7]),
 checkin_start time NOT NULL, checkin_cutoff time NOT NULL, grace_minutes integer NOT NULL DEFAULT 0 CHECK(grace_minutes BETWEEN 0 AND 120), close_time time NOT NULL,
 is_active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(checkin_start<checkin_cutoff AND checkin_cutoff + grace_minutes * interval '1 minute'<close_time),
 FOREIGN KEY(school_id,class_id) REFERENCES classes(school_id,id), UNIQUE(school_id,id), UNIQUE(school_id,id,class_id)
);
CREATE UNIQUE INDEX one_active_schedule_class ON attendance_schedules(school_id,class_id) WHERE is_active;
CREATE TABLE attendance_sessions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, schedule_id uuid NOT NULL, class_id uuid NOT NULL, date date NOT NULL,
 name text NOT NULL, timezone text NOT NULL, starts_at timestamptz NOT NULL, late_after timestamptz NOT NULL, closes_at timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'scheduled' CHECK(status IN('scheduled','open','closed','cancelled')),
 opened_at timestamptz, closed_at timestamptz, reconciled_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(starts_at<late_after AND late_after<closes_at), CHECK((status='scheduled' AND opened_at IS NULL AND closed_at IS NULL) OR (status='open' AND opened_at IS NOT NULL AND closed_at IS NULL) OR (status='closed' AND opened_at IS NOT NULL AND closed_at IS NOT NULL) OR (status='cancelled' AND closed_at IS NOT NULL)),
 CHECK(reconciled_at IS NULL OR status='closed'), FOREIGN KEY(school_id,schedule_id,class_id) REFERENCES attendance_schedules(school_id,id,class_id),
 UNIQUE(school_id,id), UNIQUE(school_id,id,class_id), UNIQUE(schedule_id,date)
);
CREATE INDEX attendance_sessions_school_date ON attendance_sessions(school_id,date,class_id);
ALTER TABLE student_enrollments ADD UNIQUE(school_id,id,student_id,class_id);
CREATE TABLE attendance_session_participants (
 school_id uuid NOT NULL, session_id uuid NOT NULL, student_id uuid NOT NULL, class_id uuid NOT NULL, enrollment_id uuid NOT NULL,
 nis text NOT NULL, full_name text NOT NULL, roster_reason text NOT NULL DEFAULT 'Roster saat pembukaan' CHECK(length(btrim(roster_reason)) BETWEEN 1 AND 500), created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(session_id,student_id), UNIQUE(school_id,session_id,student_id),
 FOREIGN KEY(school_id,session_id,class_id) REFERENCES attendance_sessions(school_id,id,class_id),
 FOREIGN KEY(school_id,enrollment_id,student_id,class_id) REFERENCES student_enrollments(school_id,id,student_id,class_id)
);
CREATE TABLE attendance_records (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, session_id uuid NOT NULL, student_id uuid NOT NULL,
 status text NOT NULL CHECK(status IN('present','late','excused','sick','absent','pending_review')),
 check_in_at timestamptz, check_out_at timestamptz, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 500), actor_id uuid NOT NULL,
 version integer NOT NULL DEFAULT 1 CHECK(version>0), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK((status IN('present','late') AND check_in_at IS NOT NULL) OR (status NOT IN('present','late') AND check_in_at IS NULL)),
 FOREIGN KEY(school_id,session_id,student_id) REFERENCES attendance_session_participants(school_id,session_id,student_id),
 FOREIGN KEY(school_id,actor_id) REFERENCES users(school_id,id), UNIQUE(session_id,student_id), UNIQUE(school_id,id), UNIQUE(school_id,id,session_id,student_id)
);
CREATE TABLE attendance_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, record_id uuid NOT NULL, actor_id uuid NOT NULL,
 event_type text NOT NULL CHECK(event_type IN('check_in','manual_adjustment','reconciliation','correction')),
 previous_status text, new_status text NOT NULL, reason text NOT NULL, occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(school_id,record_id) REFERENCES attendance_records(school_id,id), FOREIGN KEY(school_id,actor_id) REFERENCES users(school_id,id)
);
CREATE TABLE attendance_corrections (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, session_id uuid NOT NULL, student_id uuid NOT NULL,
 requested_by uuid NOT NULL, previous_version integer NOT NULL CHECK(previous_version>=0), previous_status text NOT NULL,
 new_status text NOT NULL CHECK(new_status IN('present','late','excused','sick','absent')), check_in_at timestamptz, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 500),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','approved','rejected')), decided_by uuid, decision_reason text, decided_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK((new_status IN('present','late') AND check_in_at IS NOT NULL) OR (new_status NOT IN('present','late') AND check_in_at IS NULL)),
 CHECK(decided_by IS NULL OR decided_by<>requested_by), CHECK((status='pending' AND decided_by IS NULL AND decided_at IS NULL) OR (status!='pending' AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND length(btrim(decision_reason)) BETWEEN 1 AND 500)),
 FOREIGN KEY(school_id,session_id,student_id) REFERENCES attendance_session_participants(school_id,session_id,student_id),
 FOREIGN KEY(school_id,requested_by) REFERENCES users(school_id,id), FOREIGN KEY(school_id,decided_by) REFERENCES users(school_id,id), UNIQUE(school_id,id)
);
CREATE UNIQUE INDEX pending_correction_student ON attendance_corrections(session_id,student_id) WHERE status='pending';
CREATE TABLE request_ledger (
 school_id uuid NOT NULL, actor_id uuid NOT NULL, event_id uuid NOT NULL, operation text NOT NULL, fingerprint text NOT NULL, response jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(school_id,actor_id,event_id), FOREIGN KEY(school_id,actor_id) REFERENCES users(school_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION attendance_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'attendance history is append-only' USING ERRCODE='23514'; END $$;
-- +goose StatementEnd
CREATE TRIGGER attendance_events_immutable BEFORE UPDATE OR DELETE ON attendance_events FOR EACH ROW EXECUTE FUNCTION attendance_append_only();
CREATE TRIGGER attendance_roster_immutable BEFORE UPDATE OR DELETE ON attendance_session_participants FOR EACH ROW EXECUTE FUNCTION attendance_append_only();
CREATE TRIGGER attendance_ledger_immutable BEFORE UPDATE OR DELETE ON request_ledger FOR EACH ROW EXECUTE FUNCTION attendance_append_only();
-- +goose Down
DROP TABLE request_ledger;
DROP TABLE attendance_corrections;
DROP TABLE attendance_events;
DROP TABLE attendance_records;
DROP TABLE attendance_session_participants;
ALTER TABLE student_enrollments DROP CONSTRAINT student_enrollments_school_id_id_student_id_class_id_key;
DROP TABLE attendance_sessions;
DROP TABLE attendance_schedules;
DROP TABLE school_calendar_days;
DROP FUNCTION attendance_append_only();
