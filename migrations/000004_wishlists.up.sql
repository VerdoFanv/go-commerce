-- Wishlists: classic mid-tier feature (many-to-many user ↔ product).
-- Use case: Postgres relational integrity + ownership checks in service layer.

CREATE TABLE IF NOT EXISTS wishlists (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id),
    product_id BIGINT      NOT NULL REFERENCES products (id),
    note       VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_wishlists_user_id ON wishlists (user_id);
CREATE INDEX IF NOT EXISTS idx_wishlists_product_id ON wishlists (product_id);

-- Buyer demo wishlist rows (idempotent) — needs seller products from 000003.
INSERT INTO wishlists (user_id, product_id, note)
SELECT b.id, p.id, 'Saved for later — lab demo'
FROM users b
JOIN users s ON s.email = 'seller@golang-be.dev'
JOIN products p ON p.user_id = s.id AND p.name = 'Kopi Susu Gula Aren' AND p.deleted_at IS NULL
WHERE b.email = 'buyer@golang-be.dev'
  AND NOT EXISTS (
    SELECT 1 FROM wishlists w WHERE w.user_id = b.id AND w.product_id = p.id
  );

INSERT INTO wishlists (user_id, product_id, note)
SELECT b.id, p.id, 'Weekend treat'
FROM users b
JOIN users s ON s.email = 'seller@golang-be.dev'
JOIN products p ON p.user_id = s.id AND p.name = 'Matcha Latte' AND p.deleted_at IS NULL
WHERE b.email = 'buyer@golang-be.dev'
  AND NOT EXISTS (
    SELECT 1 FROM wishlists w WHERE w.user_id = b.id AND w.product_id = p.id
  );
