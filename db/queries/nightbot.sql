-- name: GetNightbotToken :one
SELECT * FROM nightbot_tokens WHERE user_email = ? AND channel_name = ? LIMIT 1;

-- name: GetNightbotTokensByUser :many
SELECT * FROM nightbot_tokens WHERE user_email = ? ORDER BY channel_display_name;

-- name: UpsertNightbotToken :exec
INSERT INTO nightbot_tokens (user_email, channel_name, channel_display_name, access_token, refresh_token, expires_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(user_email, channel_name) DO UPDATE SET
    channel_display_name = excluded.channel_display_name,
    access_token = excluded.access_token,
    refresh_token = excluded.refresh_token,
    expires_at = excluded.expires_at,
    updated_at = CURRENT_TIMESTAMP;

-- name: DeleteNightbotToken :exec
DELETE FROM nightbot_tokens WHERE user_email = ? AND channel_name = ?;

-- name: DeleteAllNightbotTokens :exec
DELETE FROM nightbot_tokens WHERE user_email = ?;
