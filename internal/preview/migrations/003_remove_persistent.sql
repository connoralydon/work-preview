INSERT INTO preview_events (preview_id, event_type, occurred_at, details)
SELECT id, 'expired', CURRENT_TIMESTAMP, '{"reason":"persistent previews removed"}'
FROM previews
WHERE persistent = 1 AND status = 'active';

UPDATE previews
SET status = 'expired', ended_at = CURRENT_TIMESTAMP
WHERE persistent = 1 AND status = 'active';

ALTER TABLE previews DROP COLUMN persistent;
ALTER TABLE previews DROP COLUMN boot_id;
