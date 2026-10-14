-- +goose Up
-- Keep legacy consent IDs for encrypted template compatibility and retain historical records.
DROP TRIGGER consent_face_purge ON biometric_consents;
DROP TRIGGER tombstone_face_purge ON biometric_tombstones;
DELETE FROM role_permissions WHERE permission='consents.manage';
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION face_template_eligible() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM students WHERE school_id=NEW.school_id AND id=NEW.student_id AND status='active') THEN
  RAISE EXCEPTION 'template requires active student' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION purge_ineligible_templates() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status<>'active' THEN
  DELETE FROM face_templates WHERE school_id=NEW.school_id AND student_id=NEW.id;
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
-- +goose Down
-- Restore code/policy from backup deliberately; never delete enrolled faces or invent permissions.
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'restore the prior policy and permissions from a verified backup before rollback';
END $$;
-- +goose StatementEnd
