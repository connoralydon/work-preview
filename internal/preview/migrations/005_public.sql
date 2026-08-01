ALTER TABLE previews ADD COLUMN public INTEGER NOT NULL DEFAULT 0 CHECK (public IN (0, 1));

DROP INDEX previews_active_prefix;
CREATE UNIQUE INDEX previews_active_prefix
ON previews(prefix, public) WHERE status = 'active';
