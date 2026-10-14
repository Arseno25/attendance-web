-- +goose Up
CREATE TABLE roles (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  school_id uuid NOT NULL REFERENCES schools(id),
  key text NOT NULL,
  name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
  description text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
  scope text NOT NULL CHECK (scope IN ('school', 'assigned_classes')),
  is_system boolean NOT NULL DEFAULT false,
  legacy_role_key text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (school_id, id),
  UNIQUE (school_id, key)
);
CREATE UNIQUE INDEX roles_school_name_ci ON roles(school_id, lower(name));
CREATE INDEX roles_school_scope ON roles(school_id, scope);

CREATE TABLE role_permissions (
  school_id uuid NOT NULL,
  role_id uuid NOT NULL,
  permission text NOT NULL CHECK (permission IN (
    'users.read','users.write','roles.manage','audit.read','school.write',
    'students.read','students.write','academics.read','academics.write',
    'attendance.read','attendance.manual','attendance.approve','corrections.request',
    'reports.export','devices.manage','devices.health','biometrics.enroll',
    'biometrics.revoke','consents.manage'
  )),
  PRIMARY KEY (school_id, role_id, permission),
  FOREIGN KEY (school_id, role_id) REFERENCES roles(school_id, id) ON DELETE CASCADE
);
CREATE INDEX role_permissions_lookup ON role_permissions(school_id, permission, role_id);

-- The application owns the catalog. Seed the four current static role sets and
-- the new security-only permission; super_admin gets the complete catalog.
INSERT INTO roles (school_id, key, name, description, scope, is_system)
SELECT s.id, b.key, b.name, b.description, b.scope, b.is_system
FROM schools s
CROSS JOIN (VALUES
  ('super_admin','Superadmin','Pengelola penuh sekolah','school',true),
  ('admin','Admin','Administrator sekolah','school',false),
  ('teacher','Guru','Pengguna dengan penugasan kelas','assigned_classes',false),
  ('operator','Operator','Operator layanan sekolah','school',false)
) AS b(key,name,description,scope,is_system);

INSERT INTO role_permissions (school_id, role_id, permission)
SELECT r.school_id, r.id, p.permission
FROM roles r
CROSS JOIN LATERAL unnest(
  CASE r.key
    WHEN 'super_admin' THEN ARRAY[
      'users.read','users.write','roles.manage','audit.read','school.write',
      'students.read','students.write','academics.read','academics.write',
      'attendance.read','attendance.manual','attendance.approve','corrections.request',
      'reports.export','devices.manage','devices.health','biometrics.enroll',
      'biometrics.revoke','consents.manage'
    ]::text[]
    WHEN 'admin' THEN ARRAY[
      'academics.read','academics.write','attendance.manual','attendance.read',
      'corrections.request','devices.health','devices.manage','reports.export',
      'students.read','students.write'
    ]::text[]
    WHEN 'teacher' THEN ARRAY[
      'academics.read','attendance.read','corrections.request','reports.export','students.read'
    ]::text[]
    WHEN 'operator' THEN ARRAY['devices.health']::text[]
  END
) AS p(permission);

-- Preserve each non-superadmin user's exact old effective permission set.
-- Roles are shared only by an identical (school, legacy role, scope, set) tuple.
CREATE TEMP TABLE _rbac_legacy_groups ON COMMIT DROP AS
WITH legacy_users AS (
  SELECT u.school_id, u.id AS user_id, u.role AS legacy_role_key,
    CASE WHEN u.role='teacher' THEN 'assigned_classes' ELSE 'school' END AS scope,
    ARRAY(
      SELECT DISTINCT p.permission
      FROM (
        SELECT unnest(CASE u.role
          WHEN 'admin' THEN ARRAY[
            'academics.read','academics.write','attendance.manual','attendance.read',
            'corrections.request','devices.health','devices.manage','reports.export',
            'students.read','students.write'
          ]::text[]
          WHEN 'teacher' THEN ARRAY[
            'academics.read','attendance.read','corrections.request','reports.export','students.read'
          ]::text[]
          WHEN 'operator' THEN ARRAY['devices.health']::text[]
        END) AS permission
        UNION
        SELECT g.permission FROM permission_grants g
        WHERE g.school_id=u.school_id AND g.user_id=u.id
          AND CASE u.role
            WHEN 'admin' THEN g.permission IN (
              'biometrics.enroll','biometrics.revoke','consents.manage',
              'attendance.manual','attendance.approve','reports.export'
            )
            WHEN 'operator' THEN g.permission IN ('biometrics.enroll','attendance.manual')
            WHEN 'teacher' THEN g.permission='reports.export'
            ELSE false
          END
      ) AS p
      ORDER BY p.permission
    ) AS permissions
  FROM users u
  WHERE u.role <> 'super_admin'
), grouped AS (
  SELECT DISTINCT school_id, legacy_role_key, scope, permissions
  FROM legacy_users
), non_builtin AS (
  SELECT g.*,
    md5(g.legacy_role_key || E'\n' || g.scope || E'\n' || array_to_string(g.permissions, E'\n')) AS signature
  FROM grouped g
  WHERE NOT (
    (g.legacy_role_key='admin' AND g.permissions=ARRAY[
      'academics.read','academics.write','attendance.manual','attendance.read',
      'corrections.request','devices.health','devices.manage','reports.export',
      'students.read','students.write'
    ]::text[])
    OR (g.legacy_role_key='teacher' AND g.permissions=ARRAY[
      'academics.read','attendance.read','corrections.request','reports.export','students.read'
    ]::text[])
    OR (g.legacy_role_key='operator' AND g.permissions=ARRAY['devices.health']::text[])
  )
), with_base AS (
  SELECT n.*,
    'migration_' || n.legacy_role_key || '_' || n.scope || '_' || n.signature AS base_key
  FROM non_builtin n
), keyed AS (
  SELECT b.*,
    row_number() OVER (
      PARTITION BY b.school_id, b.base_key
      ORDER BY b.permissions::text, b.legacy_role_key, b.scope
    ) AS collision_ordinal
  FROM with_base b
)
SELECT school_id, legacy_role_key, scope, permissions,
  CASE WHEN collision_ordinal=1 THEN base_key ELSE base_key || '_' || collision_ordinal::text END AS role_key,
  'Migrasi · ' || legacy_role_key || ' · ' || signature ||
    CASE WHEN collision_ordinal=1 THEN '' ELSE '_' || collision_ordinal::text END AS role_name
FROM keyed;

INSERT INTO roles (school_id, key, name, description, scope, legacy_role_key)
SELECT school_id, role_key, role_name,
  'Role hasil migrasi otomatis; permission mempertahankan akses efektif sebelum RBAC dinamis.',
  scope, legacy_role_key
FROM _rbac_legacy_groups;

INSERT INTO role_permissions (school_id, role_id, permission)
SELECT g.school_id, r.id, p.permission
FROM _rbac_legacy_groups g
JOIN roles r ON r.school_id=g.school_id AND r.key=g.role_key
CROSS JOIN LATERAL unnest(g.permissions) AS p(permission);

ALTER TABLE users DROP CONSTRAINT users_role_check;

UPDATE users u SET role=g.role_key
FROM _rbac_legacy_groups g
WHERE u.school_id=g.school_id AND u.id IN (
  SELECT legacy.user_id FROM (
    SELECT lu.user_id, lu.school_id, lu.legacy_role_key, lu.scope, lu.permissions
    FROM (
      SELECT u2.school_id, u2.id AS user_id, u2.role AS legacy_role_key,
        CASE WHEN u2.role='teacher' THEN 'assigned_classes' ELSE 'school' END AS scope,
        ARRAY(
          SELECT DISTINCT p.permission
          FROM (
            SELECT unnest(CASE u2.role
              WHEN 'admin' THEN ARRAY[
                'academics.read','academics.write','attendance.manual','attendance.read',
                'corrections.request','devices.health','devices.manage','reports.export',
                'students.read','students.write'
              ]::text[]
              WHEN 'teacher' THEN ARRAY[
                'academics.read','attendance.read','corrections.request','reports.export','students.read'
              ]::text[]
              WHEN 'operator' THEN ARRAY['devices.health']::text[]
            END) AS permission
            UNION
            SELECT pg.permission FROM permission_grants pg
            WHERE pg.school_id=u2.school_id AND pg.user_id=u2.id
              AND CASE u2.role
                WHEN 'admin' THEN pg.permission IN (
                  'biometrics.enroll','biometrics.revoke','consents.manage',
                  'attendance.manual','attendance.approve','reports.export'
                )
                WHEN 'operator' THEN pg.permission IN ('biometrics.enroll','attendance.manual')
                WHEN 'teacher' THEN pg.permission='reports.export'
                ELSE false
              END
          ) AS p        ORDER BY p.permission
      ) AS permissions
      FROM users u2 WHERE u2.role <> 'super_admin'
    ) lu
  ) legacy
  WHERE legacy.school_id=g.school_id
    AND legacy.legacy_role_key=g.legacy_role_key
    AND legacy.scope=g.scope
    AND legacy.permissions=g.permissions
);

-- Move the old teacher-only assignment rows without changing identity/history.
DROP TRIGGER assignment_interval ON teacher_assignments;
ALTER TABLE teacher_assignments RENAME TO user_class_assignments;
ALTER TABLE user_class_assignments RENAME COLUMN teacher_id TO user_id;
ALTER INDEX assignments_school_teacher RENAME TO user_class_assignments_school_user;

-- The interval guard now accepts only active users whose live role has
-- assigned_classes scope. Student enrollment validation stays unchanged.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_master_interval() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE lower_bound timestamptz; upper_bound timestamptz; interval_start timestamptz; interval_end timestamptz; target_user_id uuid;
BEGIN
 SELECT starts_on::timestamp AT TIME ZONE timezone,(ends_on+1)::timestamp AT TIME ZONE timezone
 INTO lower_bound,upper_bound FROM academic_years WHERE school_id=NEW.school_id AND id=NEW.academic_year_id;
 IF TG_TABLE_NAME='student_enrollments' THEN
  interval_start:=NEW.joined_at; interval_end:=NEW.left_at;
 ELSE
  interval_start:=NEW.starts_at; interval_end:=NEW.ends_at;
  IF TG_TABLE_NAME='user_class_assignments' THEN target_user_id:=NEW.user_id;
  ELSE target_user_id:=NEW.teacher_id;
  END IF;
  IF (TG_OP='INSERT' OR NEW.is_active) AND NOT EXISTS(
    SELECT 1 FROM users u JOIN roles r ON r.school_id=u.school_id AND r.key=u.role
    WHERE u.id=target_user_id AND u.school_id=NEW.school_id AND u.is_active AND r.scope='assigned_classes'
  ) THEN
   RAISE EXCEPTION 'assignment requires active assigned-class user' USING ERRCODE='23514';
  END IF;
 END IF;
 IF lower_bound IS NULL OR interval_start<lower_bound OR interval_end>upper_bound THEN
  RAISE EXCEPTION 'interval outside academic period' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER assignment_interval BEFORE INSERT OR UPDATE ON user_class_assignments FOR EACH ROW EXECUTE FUNCTION check_master_interval();
CREATE INDEX user_class_assignments_school_user_class ON user_class_assignments(school_id,user_id,class_id) WHERE is_active;

ALTER TABLE users ADD CONSTRAINT users_school_role_fk FOREIGN KEY (school_id, role) REFERENCES roles(school_id, key);
DROP TABLE permission_grants;

-- +goose Down
DO $$
BEGIN
  RAISE EXCEPTION '00008_dynamic_rbac is forward-only; restore from a pre-migration backup or add an explicit reverse migration';
END $$;
