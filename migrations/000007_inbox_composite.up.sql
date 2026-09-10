-- Inbox composite key (consumer, event_id) so multiple handlers can process
-- the same Kafka event independently — matches large-system inbox pattern.

ALTER TABLE processed_events DROP CONSTRAINT IF EXISTS processed_events_pkey;

-- Deduplicate if any accidental same event_id with different consumers already existed
-- (primary was event_id-only before). Keep earliest row per pair.
DELETE FROM processed_events a
USING processed_events b
WHERE a.ctid < b.ctid
  AND a.event_id = b.event_id
  AND a.consumer = b.consumer;

ALTER TABLE processed_events
    ADD PRIMARY KEY (event_id, consumer);
