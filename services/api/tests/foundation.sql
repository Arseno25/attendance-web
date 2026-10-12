\set ON_ERROR_STOP on
BEGIN;
DO $$
DECLARE test_school_id uuid;
BEGIN
  INSERT INTO schools(name) VALUES ('Sekolah Uji') RETURNING id INTO test_school_id;
  IF NOT EXISTS (
    SELECT 1 FROM schools
    WHERE id = test_school_id
      AND timezone = 'Asia/Jakarta' AND is_active AND created_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'school defaults do not match the contract';
  END IF;
  BEGIN
    INSERT INTO schools(name) VALUES ('  ');
    RAISE EXCEPTION 'blank school name accepted';
  EXCEPTION WHEN check_violation THEN NULL;
  END;
END $$;
ROLLBACK;
