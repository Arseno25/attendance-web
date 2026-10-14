-- +goose Up
ALTER TABLE face_templates ALTER COLUMN consent_id DROP NOT NULL;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION face_template_eligible() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM students WHERE school_id=NEW.school_id AND id=NEW.student_id AND status='active') OR (NEW.consent_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM biometric_consents c JOIN students st ON st.school_id=c.school_id AND st.id=c.student_id WHERE c.school_id=NEW.school_id AND c.id=NEW.consent_id AND c.student_id=NEW.student_id AND st.status='active' AND c.withdrawn_at IS NULL AND c.valid_until>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM biometric_tombstones t WHERE t.school_id=c.school_id AND t.consent_id=c.id))) THEN
  RAISE EXCEPTION 'template requires active student and live consent' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose Down
-- Refuse rollback while local templates exist instead of silently deleting them.
ALTER TABLE face_templates ALTER COLUMN consent_id SET NOT NULL;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION face_template_eligible() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM biometric_consents c JOIN students st ON st.school_id=c.school_id AND st.id=c.student_id WHERE c.school_id=NEW.school_id AND c.id=NEW.consent_id AND c.student_id=NEW.student_id AND st.status='active' AND c.withdrawn_at IS NULL AND c.valid_until>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM biometric_tombstones t WHERE t.school_id=c.school_id AND t.consent_id=c.id)) THEN
  RAISE EXCEPTION 'template requires active student and live consent' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
