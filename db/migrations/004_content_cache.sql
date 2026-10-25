-- +goose Up
-- +goose StatementBegin
-- The cache is shared by every external feed (videos, blog posts, contact
-- subjects), so it is no longer media-specific. Rename the pre-existing table
-- when present and preserve its contents.
DO $$
BEGIN
    IF to_regclass('nixie.media_cache') IS NOT NULL
       AND to_regclass('nixie.content_cache') IS NULL THEN
        ALTER TABLE nixie.media_cache RENAME TO content_cache;
    END IF;
END
$$;

CREATE TABLE IF NOT EXISTS nixie.content_cache (
    key        TEXT PRIMARY KEY,
    payload    JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS nixie.content_cache;
-- +goose StatementEnd
