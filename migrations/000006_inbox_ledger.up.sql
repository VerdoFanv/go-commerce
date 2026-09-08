-- Consumer inbox (processed_events) + append-only stock ledger.
-- Inbox pairs with transactional outbox: at-least-once delivery + exactly-once side effects.
-- Ledger makes inventory mutations auditable (common in large commerce systems).

CREATE TABLE IF NOT EXISTS processed_events (
    event_id     UUID PRIMARY KEY,
    consumer     VARCHAR(64) NOT NULL,
    event_type   VARCHAR(64) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_processed_events_consumer_at
    ON processed_events (consumer, processed_at DESC);

CREATE TABLE IF NOT EXISTS stock_ledger (
    id           BIGSERIAL PRIMARY KEY,
    product_id   BIGINT NOT NULL REFERENCES products (id),
    order_id     BIGINT REFERENCES orders (id),
    delta        INTEGER NOT NULL,
    reason       VARCHAR(32) NOT NULL,
    balance_after INTEGER,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_stock_ledger_product_at
    ON stock_ledger (product_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_stock_ledger_order
    ON stock_ledger (order_id);
