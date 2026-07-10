CREATE TABLE IF NOT EXISTS previews (
  id TEXT PRIMARY KEY,
  prefix TEXT NOT NULL,
  port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
  status TEXT NOT NULL CHECK (status IN ('active', 'deleted', 'expired')),
  created_at DATETIME NOT NULL,
  last_access_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  ended_at DATETIME NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS previews_active_prefix
ON previews(prefix) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS preview_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  preview_id TEXT NOT NULL REFERENCES previews(id),
  event_type TEXT NOT NULL,
  occurred_at DATETIME NOT NULL,
  details TEXT NULL CHECK (details IS NULL OR json_valid(details))
);

CREATE INDEX IF NOT EXISTS preview_events_preview_time
ON preview_events(preview_id, occurred_at);
