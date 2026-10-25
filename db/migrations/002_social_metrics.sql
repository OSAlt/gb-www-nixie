-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS nixie.social_metrics (
    platform TEXT PRIMARY KEY,
    followers BIGINT NOT NULL,
    refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed with the current best-known values so the llama chart renders
-- correctly before the background worker performs its first refresh.
-- Per-platform fetch failures fall back to these/last-known-good values.
--
-- refreshed_at is deliberately 'epoch' (never refreshed) so the worker's
-- boot-refresh guard treats seeds as stale and pulls live data immediately.
-- Values verified against Metrika on 2026-10-25.
INSERT INTO nixie.social_metrics (platform, followers, refreshed_at) VALUES
    -- Combined subscribers of @NixiePixelGB (102k) + youtube.com/user/nixiedoeslinux (202k).
    ('youtube', 304000, 'epoch'),
    ('twitter', 26000, 'epoch'),
    ('twitch', 2595, 'epoch'),
    -- discord.gg/geekbeacon
    ('discord', 5425, 'epoch'),
    ('facebook', 41655, 'epoch'),
    ('instagram', 18000, 'epoch'),
    ('tiktok', 3616, 'epoch')
ON CONFLICT DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS nixie.social_metrics;
-- +goose StatementEnd
