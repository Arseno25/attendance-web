-- +goose Up
ALTER TABLE face_templates
 ADD COLUMN reference_ciphertext bytea,
 ADD COLUMN reference_nonce bytea,
 ADD CONSTRAINT face_reference_photo_pair CHECK (
  (reference_ciphertext IS NULL AND reference_nonce IS NULL) OR
  (reference_ciphertext IS NOT NULL AND reference_nonce IS NOT NULL
   AND octet_length(reference_ciphertext) BETWEEN 17 AND 65552
   AND octet_length(reference_nonce)=12)
 );
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM face_templates WHERE reference_ciphertext IS NOT NULL) THEN
  RAISE EXCEPTION 'export encrypted reference photos before rollback';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE face_templates DROP CONSTRAINT face_reference_photo_pair,
 DROP COLUMN reference_nonce, DROP COLUMN reference_ciphertext;
