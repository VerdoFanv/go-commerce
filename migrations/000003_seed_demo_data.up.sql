-- Demo catalog for local/lab learning (idempotent).
-- Passwords (bcrypt cost 10):
--   seller@golang-be.dev / seller123
--   buyer@golang-be.dev  / user123
-- Admin remains from 000002: admin@golang-be.dev / admin123

INSERT INTO users (name, email, password_hash, role)
SELECT 'Seller Demo', 'seller@golang-be.dev', '$2a$10$c0hY12EN2jCx2rLJSJ8aA.qKoORgryaZxN/Jo1BfTy6tE4ROeZuFu', 'user'
WHERE NOT EXISTS (SELECT 1 FROM users WHERE email = 'seller@golang-be.dev');

INSERT INTO users (name, email, password_hash, role)
SELECT 'Buyer Demo', 'buyer@golang-be.dev', '$2a$10$gwICUaTp1ymHTQPCuwv4iegTzfesuuvKYhc/EdDskUc1AhPxbPAnm', 'user'
WHERE NOT EXISTS (SELECT 1 FROM users WHERE email = 'buyer@golang-be.dev');

-- Seller catalog
INSERT INTO products (user_id, name, description, price, stock)
SELECT u.id, 'Kopi Susu Gula Aren', 'Cold brew with palm sugar — signature SKU for search demos', 28000, 120
FROM users u WHERE u.email = 'seller@golang-be.dev'
  AND NOT EXISTS (SELECT 1 FROM products p WHERE p.user_id = u.id AND p.name = 'Kopi Susu Gula Aren' AND p.deleted_at IS NULL);

INSERT INTO products (user_id, name, description, price, stock)
SELECT u.id, 'Matcha Latte', 'Ceremonial grade matcha with oat milk', 32000, 80
FROM users u WHERE u.email = 'seller@golang-be.dev'
  AND NOT EXISTS (SELECT 1 FROM products p WHERE p.user_id = u.id AND p.name = 'Matcha Latte' AND p.deleted_at IS NULL);

INSERT INTO products (user_id, name, description, price, stock)
SELECT u.id, 'Croissant Butter', 'Flaky French pastry, baked daily', 22000, 40
FROM users u WHERE u.email = 'seller@golang-be.dev'
  AND NOT EXISTS (SELECT 1 FROM products p WHERE p.user_id = u.id AND p.name = 'Croissant Butter' AND p.deleted_at IS NULL);

INSERT INTO products (user_id, name, description, price, stock)
SELECT u.id, 'Americano', 'Double shot espresso + hot water', 18000, 200
FROM users u WHERE u.email = 'seller@golang-be.dev'
  AND NOT EXISTS (SELECT 1 FROM products p WHERE p.user_id = u.id AND p.name = 'Americano' AND p.deleted_at IS NULL);

INSERT INTO products (user_id, name, description, price, stock)
SELECT u.id, 'Brown Sugar Boba', 'Milk tea with chewy pearls', 25000, 90
FROM users u WHERE u.email = 'seller@golang-be.dev'
  AND NOT EXISTS (SELECT 1 FROM products p WHERE p.user_id = u.id AND p.name = 'Brown Sugar Boba' AND p.deleted_at IS NULL);

-- Admin-owned SKUs (RBAC / multi-tenant list demos)
INSERT INTO products (user_id, name, description, price, stock)
SELECT u.id, 'Admin Merch Hoodie', 'Internal brand hoodie — only visible on admin product list', 199000, 15
FROM users u WHERE u.email = 'admin@golang-be.dev'
  AND NOT EXISTS (SELECT 1 FROM products p WHERE p.user_id = u.id AND p.name = 'Admin Merch Hoodie' AND p.deleted_at IS NULL);

INSERT INTO products (user_id, name, description, price, stock)
SELECT u.id, 'Admin Sticker Pack', 'Laptop stickers for the team', 35000, 500
FROM users u WHERE u.email = 'admin@golang-be.dev'
  AND NOT EXISTS (SELECT 1 FROM products p WHERE p.user_id = u.id AND p.name = 'Admin Sticker Pack' AND p.deleted_at IS NULL);
