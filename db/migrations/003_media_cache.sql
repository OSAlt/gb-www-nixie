-- +goose Up
-- +goose StatementBegin
-- One row per cached feed. The home page reads youtube:recent; the refresh job
-- writes it. Storing the payload as JSONB keeps the read path a single primary
-- key lookup and lets the shape evolve without another migration.
CREATE TABLE IF NOT EXISTS nixie.media_cache (
    key        TEXT PRIMARY KEY,
    payload    JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS nixie.media_cache;
-- +goose StatementEnd
