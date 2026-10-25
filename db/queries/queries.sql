-- name: ListContacts :many
SELECT * FROM nixie.contact_form
WHERE domain = $1;

-- name: GetSocialAppByType :one
SELECT * FROM config.social_app
WHERE social_app_type = $1;

-- name: GetUserSocialByApp :one
SELECT * FROM config.user_social
WHERE app_id = $1;

-- name: ListSocialApps :many
SELECT * FROM config.social_app;

-- name: GetSocialCounts :many
SELECT * FROM nixie.social_metrics;

-- name: UpsertSocialCount :exec
INSERT INTO nixie.social_metrics (platform, followers, refreshed_at)
VALUES ($1, $2, now())
ON CONFLICT (platform) DO UPDATE
    SET followers = EXCLUDED.followers,
        refreshed_at = EXCLUDED.refreshed_at;

-- name: LastSocialRefresh :one
SELECT COALESCE(max(refreshed_at), 'epoch'::timestamptz)::timestamptz AS refreshed_at
FROM nixie.social_metrics;
