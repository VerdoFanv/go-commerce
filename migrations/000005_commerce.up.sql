-- Commerce lab: orders, inventory reservations, payments, idempotency, transactional outbox.
-- Stock is held (decremented) at order create; released on cancel/payment_failed; committed on paid.

CREATE TABLE IF NOT EXISTS orders (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT       NOT NULL REFERENCES users (id),
    status     VARCHAR(32)  NOT NULL,
    total      DOUBLE PRECISION NOT NULL DEFAULT 0,
    currency   VARCHAR(8)   NOT NULL DEFAULT 'IDR',
    version    INTEGER      NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders (user_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders (status);

CREATE TABLE IF NOT EXISTS order_items (
    id         BIGSERIAL PRIMARY KEY,
    order_id   BIGINT       NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id BIGINT       NOT NULL REFERENCES products (id),
    qty        INTEGER      NOT NULL CHECK (qty > 0),
    unit_price DOUBLE PRECISION NOT NULL CHECK (unit_price >= 0)
);

CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items (order_id);

CREATE TABLE IF NOT EXISTS inventory_reservations (
    id         BIGSERIAL PRIMARY KEY,
    order_id   BIGINT       NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id BIGINT       NOT NULL REFERENCES products (id),
    qty        INTEGER      NOT NULL CHECK (qty > 0),
    status     VARCHAR(16)  NOT NULL DEFAULT 'held',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (order_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_inventory_reservations_status ON inventory_reservations (status);

CREATE TABLE IF NOT EXISTS payments (
    id              BIGSERIAL PRIMARY KEY,
    order_id        BIGINT       NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    idempotency_key VARCHAR(128) NOT NULL,
    status          VARCHAR(32)  NOT NULL,
    amount          DOUBLE PRECISION NOT NULL,
    provider_ref    VARCHAR(128),
    attempt         INTEGER      NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (order_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments (order_id);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id              BIGSERIAL PRIMARY KEY,
    key             VARCHAR(128) NOT NULL,
    user_id         BIGINT       NOT NULL REFERENCES users (id),
    method          VARCHAR(16)  NOT NULL,
    path            VARCHAR(255) NOT NULL,
    request_hash    VARCHAR(64)  NOT NULL DEFAULT '',
    response_status INTEGER,
    response_body   JSONB,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (user_id, key)
);

CREATE TABLE IF NOT EXISTS outbox_events (
    id             BIGSERIAL PRIMARY KEY,
    event_id       UUID         NOT NULL UNIQUE,
    aggregate_type VARCHAR(64)  NOT NULL,
    aggregate_id   BIGINT       NOT NULL,
    event_type     VARCHAR(64)  NOT NULL,
    payload        JSONB        NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_outbox_unpublished
    ON outbox_events (created_at)
    WHERE published_at IS NULL;

-- Ensure demo catalog has enough stock for concurrency / load demos.
UPDATE products SET stock = GREATEST(stock, 100)
WHERE deleted_at IS NULL
  AND name IN (
    'Kopi Susu Gula Aren',
    'Matcha Latte',
    'Americano',
    'Brown Sugar Boba',
    'Admin Sticker Pack'
  );
