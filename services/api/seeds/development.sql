-- Dummy school only; never creates a default admin password or biometric data.
INSERT INTO schools(id, name)
VALUES ('00000000-0000-4000-8000-000000000001', 'Sekolah Demo')
ON CONFLICT (id) DO NOTHING;
