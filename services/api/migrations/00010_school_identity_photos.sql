-- +goose Up
ALTER TABLE schools ADD COLUMN app_name text NOT NULL DEFAULT 'Absensi Sekolah' CHECK(length(btrim(app_name)) BETWEEN 1 AND 80);
ALTER TABLE schools ADD COLUMN address text NOT NULL DEFAULT '' CHECK(length(address)<=300);
ALTER TABLE schools ADD COLUMN phone text NOT NULL DEFAULT '' CHECK(length(phone)<=30);
ALTER TABLE schools ADD COLUMN email text NOT NULL DEFAULT '' CHECK(length(email)<=254);
ALTER TABLE schools ADD COLUMN logo bytea CHECK(octet_length(logo)<=524288);
ALTER TABLE users ADD COLUMN avatar bytea CHECK(octet_length(avatar)<=524288);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM schools WHERE logo IS NOT NULL OR address<>'' OR phone<>'' OR email<>'' OR app_name<>'Absensi Sekolah') OR EXISTS(SELECT 1 FROM users WHERE avatar IS NOT NULL) THEN
  RAISE EXCEPTION 'export school settings and photos before rollback';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE users DROP COLUMN avatar;
ALTER TABLE schools DROP COLUMN logo,DROP COLUMN email,DROP COLUMN phone,DROP COLUMN address,DROP COLUMN app_name;
