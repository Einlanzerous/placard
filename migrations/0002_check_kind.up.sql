-- Edge self-checks (PCAD-10): placard also fetches its own public hostname,
-- so an Access gate landing there surfaces as failing checks instead of being
-- invisible to everyone holding a session. Those rows need a kind — an edge
-- fetch of a mirror path must not collide with the canonical (jsDelivr) check
-- for the same file.
ALTER TABLE mark_check ADD COLUMN kind TEXT NOT NULL DEFAULT 'canonical';

DROP INDEX mark_check_latest_idx;
CREATE INDEX mark_check_latest_idx ON mark_check (kind, service, file, checked_at DESC);
