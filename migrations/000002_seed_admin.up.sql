-- Seed a default admin account for RBAC demos.
-- Email: admin@golang-be.dev / Password: admin123 (bcrypt cost 10) — change in production!
-- The hash is single-quoted with '' escaping of the literal $ characters (none present,
-- but keep the pattern for future edits).

INSERT INTO users (name, email, password_hash, role)
SELECT 'Admin', 'admin@golang-be.dev', '$2a$10$Oesk/CKm0Bhy6yZOGvzG8.yM.zZrzH.hyo4XmsN0wFA6WwBDlQryi', 'admin'
WHERE NOT EXISTS (SELECT 1 FROM users WHERE email = 'admin@golang-be.dev');
