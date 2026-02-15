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
