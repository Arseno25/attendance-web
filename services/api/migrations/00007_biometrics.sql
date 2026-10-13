-- +goose Up
CREATE TABLE gallery_state (
 school_id uuid PRIMARY KEY REFERENCES schools(id), revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0), updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE face_templates (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, student_id uuid NOT NULL, consent_id uuid NOT NULL,
 ciphertext bytea NOT NULL CHECK(octet_length(ciphertext)=528), nonce bytea NOT NULL CHECK(octet_length(nonce)=12),
 key_id text NOT NULL CHECK(length(key_id) BETWEEN 1 AND 80), model_sha256 text NOT NULL CHECK(model_sha256 ~ '^[a-f0-9]{64}$'),
 frame_count integer NOT NULL CHECK(frame_count BETWEEN 3 AND 5), status text NOT NULL DEFAULT 'provisional' CHECK(status='provisional'),
 created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(school_id,student_id) REFERENCES students(school_id,id), FOREIGN KEY(school_id,consent_id) REFERENCES biometric_consents(school_id,id),
 FOREIGN KEY(school_id,created_by) REFERENCES users(school_id,id), UNIQUE(school_id,student_id), UNIQUE(school_id,id)
);
ALTER TABLE device_sessions ADD COLUMN paired_by uuid;
ALTER TABLE device_sessions ADD FOREIGN KEY(school_id,paired_by) REFERENCES users(school_id,id);
-- +goose StatementBegin
CREATE FUNCTION face_template_eligible() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM biometric_consents c JOIN students st ON st.school_id=c.school_id AND st.id=c.student_id WHERE c.school_id=NEW.school_id AND c.id=NEW.consent_id AND c.student_id=NEW.student_id AND st.status='active' AND c.withdrawn_at IS NULL AND c.valid_until>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM biometric_tombstones t WHERE t.school_id=c.school_id AND t.consent_id=c.id)) THEN
  RAISE EXCEPTION 'template requires active student and live consent' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION face_template_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO gallery_state(school_id,revision) VALUES(COALESCE(NEW.school_id,OLD.school_id),1) ON CONFLICT(school_id) DO UPDATE SET revision=gallery_state.revision+1,updated_at=clock_timestamp();
 RETURN NULL;
END $$;
CREATE FUNCTION purge_ineligible_templates() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='biometric_consents' THEN
  DELETE FROM face_templates WHERE school_id=NEW.school_id AND consent_id=NEW.id AND (NEW.withdrawn_at IS NOT NULL OR NEW.valid_until<=clock_timestamp());
 ELSIF TG_TABLE_NAME='biometric_tombstones' THEN
  DELETE FROM face_templates WHERE school_id=NEW.school_id AND consent_id=NEW.consent_id;
 ELSIF NEW.status<>'active' THEN
  DELETE FROM face_templates WHERE school_id=NEW.school_id AND student_id=NEW.id;
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER face_eligible BEFORE INSERT OR UPDATE ON face_templates FOR EACH ROW EXECUTE FUNCTION face_template_eligible();
CREATE TRIGGER face_revision AFTER INSERT OR UPDATE OR DELETE ON face_templates FOR EACH ROW EXECUTE FUNCTION face_template_revision();
CREATE TRIGGER consent_face_purge AFTER UPDATE ON biometric_consents FOR EACH ROW EXECUTE FUNCTION purge_ineligible_templates();
CREATE TRIGGER tombstone_face_purge AFTER INSERT ON biometric_tombstones FOR EACH ROW EXECUTE FUNCTION purge_ineligible_templates();
CREATE TRIGGER student_face_purge AFTER UPDATE OF status ON students FOR EACH ROW EXECUTE FUNCTION purge_ineligible_templates();
-- +goose Down
DROP TRIGGER student_face_purge ON students;
DROP TRIGGER tombstone_face_purge ON biometric_tombstones;
DROP TRIGGER consent_face_purge ON biometric_consents;
DROP TRIGGER face_revision ON face_templates;
DROP TRIGGER face_eligible ON face_templates;
DROP FUNCTION purge_ineligible_templates();
DROP FUNCTION face_template_revision();
DROP FUNCTION face_template_eligible();
ALTER TABLE device_sessions DROP COLUMN paired_by;
DROP TABLE face_templates;
DROP TABLE gallery_state;
