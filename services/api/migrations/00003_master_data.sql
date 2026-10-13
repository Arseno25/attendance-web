-- +goose Up
CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public;
ALTER TABLE audit_logs ADD COLUMN metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata)='object');

CREATE TABLE academic_years (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL REFERENCES schools(id),
 name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
 starts_on date NOT NULL, ends_on date NOT NULL, timezone text NOT NULL,
 is_active boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK (starts_on <= ends_on), UNIQUE(school_id,id), UNIQUE(school_id,name),
 EXCLUDE USING gist(school_id public.gist_uuid_ops WITH =, daterange(starts_on,ends_on+1,'[)') WITH &&)
);
CREATE UNIQUE INDEX academic_years_one_active ON academic_years(school_id) WHERE is_active;
CREATE TABLE classes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, academic_year_id uuid NOT NULL,
 code text NOT NULL CHECK (code=btrim(code) AND length(code) BETWEEN 1 AND 30),
 name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120), level text NOT NULL DEFAULT '' CHECK(length(level)<=30),
 is_active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(school_id,academic_year_id) REFERENCES academic_years(school_id,id), UNIQUE(school_id,id), UNIQUE(school_id,id,academic_year_id)
);
CREATE UNIQUE INDEX classes_code_year ON classes(school_id,academic_year_id,lower(code));
CREATE TABLE students (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL REFERENCES schools(id),
 nis text NOT NULL CHECK (nis=btrim(nis) AND length(nis) BETWEEN 1 AND 30),
 nisn text CHECK (nisn=btrim(nisn) AND length(nisn) BETWEEN 1 AND 32),
 full_name text NOT NULL CHECK(length(btrim(full_name)) BETWEEN 1 AND 120),
 status text NOT NULL DEFAULT 'active' CHECK(status IN('active','inactive','graduated','transferred')),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(school_id,id), UNIQUE(school_id,nis), UNIQUE(school_id,nisn)
);
CREATE INDEX students_school_status ON students(school_id,status);
CREATE TABLE student_enrollments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, student_id uuid NOT NULL,
 class_id uuid NOT NULL, academic_year_id uuid NOT NULL,
 joined_at timestamptz NOT NULL, left_at timestamptz NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 500), created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(school_id,student_id) REFERENCES students(school_id,id),
 FOREIGN KEY(school_id,class_id,academic_year_id) REFERENCES classes(school_id,id,academic_year_id),
 CHECK(joined_at<left_at), UNIQUE(school_id,id),
 EXCLUDE USING gist(student_id public.gist_uuid_ops WITH =, tstzrange(joined_at,left_at,'[)') WITH &&)
);
CREATE INDEX enrollments_school_class ON student_enrollments(school_id,class_id,student_id);
CREATE TABLE teacher_assignments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, teacher_id uuid NOT NULL,
 class_id uuid NOT NULL, academic_year_id uuid NOT NULL, starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL,
 is_active boolean NOT NULL DEFAULT true, revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(school_id,teacher_id) REFERENCES users(school_id,id),
 FOREIGN KEY(school_id,class_id,academic_year_id) REFERENCES classes(school_id,id,academic_year_id),
 CHECK(starts_at<ends_at), CHECK((is_active AND revoked_at IS NULL) OR (NOT is_active AND revoked_at IS NOT NULL)),
 EXCLUDE USING gist(teacher_id public.gist_uuid_ops WITH =, class_id public.gist_uuid_ops WITH =, tstzrange(starts_at,ends_at,'[)') WITH &&) WHERE(is_active)
);
CREATE INDEX assignments_school_teacher ON teacher_assignments(school_id,teacher_id,class_id) WHERE is_active;

-- +goose StatementBegin
CREATE FUNCTION protect_academic_dates() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.starts_on,NEW.ends_on,NEW.timezone,NEW.school_id) IS DISTINCT FROM (OLD.starts_on,OLD.ends_on,OLD.timezone,OLD.school_id)
 AND EXISTS(SELECT 1 FROM classes WHERE academic_year_id=OLD.id) THEN
  RAISE EXCEPTION 'academic period with classes is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION check_master_interval() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE lower_bound timestamptz; upper_bound timestamptz; interval_start timestamptz; interval_end timestamptz;
BEGIN
 SELECT starts_on::timestamp AT TIME ZONE timezone,(ends_on+1)::timestamp AT TIME ZONE timezone
 INTO lower_bound,upper_bound FROM academic_years WHERE school_id=NEW.school_id AND id=NEW.academic_year_id;
 IF TG_TABLE_NAME='student_enrollments' THEN interval_start:=NEW.joined_at; interval_end:=NEW.left_at;
 ELSE
  interval_start:=NEW.starts_at; interval_end:=NEW.ends_at;
  IF (TG_OP='INSERT' OR NEW.is_active) AND NOT EXISTS(SELECT 1 FROM users WHERE id=NEW.teacher_id AND school_id=NEW.school_id AND role='teacher' AND is_active) THEN
   RAISE EXCEPTION 'assignment requires active teacher' USING ERRCODE='23514';
  END IF;
 END IF;
 IF lower_bound IS NULL OR interval_start<lower_bound OR interval_end>upper_bound THEN
  RAISE EXCEPTION 'interval outside academic period' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION protect_placement_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'placement history cannot be deleted' USING ERRCODE='23514'; END IF;
 IF (NEW.id,NEW.school_id,NEW.student_id,NEW.class_id,NEW.academic_year_id,NEW.joined_at,NEW.reason)
 IS DISTINCT FROM (OLD.id,OLD.school_id,OLD.student_id,OLD.class_id,OLD.academic_year_id,OLD.joined_at,OLD.reason)
 OR NEW.left_at>OLD.left_at THEN RAISE EXCEPTION 'placement identity is immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER academic_dates BEFORE UPDATE ON academic_years FOR EACH ROW EXECUTE FUNCTION protect_academic_dates();
CREATE TRIGGER enrollment_interval BEFORE INSERT OR UPDATE ON student_enrollments FOR EACH ROW EXECUTE FUNCTION check_master_interval();
CREATE TRIGGER assignment_interval BEFORE INSERT OR UPDATE ON teacher_assignments FOR EACH ROW EXECUTE FUNCTION check_master_interval();
CREATE TRIGGER placement_history BEFORE UPDATE OR DELETE ON student_enrollments FOR EACH ROW EXECUTE FUNCTION protect_placement_history();

-- +goose Down
DROP TABLE teacher_assignments;
DROP TABLE student_enrollments;
DROP TABLE students;
DROP TABLE classes;
DROP TABLE academic_years;
DROP FUNCTION protect_placement_history();
DROP FUNCTION check_master_interval();
DROP FUNCTION protect_academic_dates();
ALTER TABLE audit_logs DROP COLUMN metadata;
