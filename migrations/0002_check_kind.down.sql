DROP INDEX mark_check_latest_idx;
ALTER TABLE mark_check DROP COLUMN kind;
CREATE INDEX mark_check_latest_idx ON mark_check (service, file, checked_at DESC);
