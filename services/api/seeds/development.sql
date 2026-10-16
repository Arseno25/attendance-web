-- Dummy school only; preserve an existing school and never create default credentials.
BEGIN;
LOCK TABLE schools IN SHARE ROW EXCLUSIVE MODE;
INSERT INTO schools(name)
SELECT 'Sekolah Demo' WHERE NOT EXISTS(SELECT 1 FROM schools);
COMMIT;
