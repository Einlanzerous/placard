-- Placard's Postgres is a cache of observations, never the source of truth
-- for a mark (that is the repo). Two tables:
--
-- mark_check: append-only results of verifying canonical URLs. The launcher
-- accepts a 404 logo_url silently; these rows are what notices.
CREATE TABLE mark_check (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    service        TEXT        NOT NULL,
    file           TEXT        NOT NULL, -- repo-relative path, e.g. argosy/argosy-mark-light.png
    url            TEXT        NOT NULL, -- the exact URL fetched
    ok             BOOLEAN     NOT NULL,
    http_status    INT,
    content_type   TEXT,
    content_length BIGINT,
    error          TEXT,
    checked_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Latest-per-file lookups (DISTINCT ON service, file ORDER BY checked_at DESC).
CREATE INDEX mark_check_latest_idx ON mark_check (service, file, checked_at DESC);

-- staged_upload: marks dropped on the front page, held for a human to review
-- and commit to the repo. Bytes live here (bytea) — small images, bounded by
-- PLACARD_MAX_UPLOAD_BYTES, gated by PLACARD_UPLOAD_TOKEN.
CREATE TABLE staged_upload (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    service      TEXT        NOT NULL,
    filename     TEXT        NOT NULL,
    content_type TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL,
    sha256       TEXT        NOT NULL,
    data         BYTEA       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX staged_upload_service_idx ON staged_upload (service, created_at DESC);
