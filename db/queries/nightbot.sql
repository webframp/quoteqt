-- name: GetNightbotToken :one
SELECT * FROM nightbot_tokens WHERE user_email = ? LIMIT 1;

-- name: UpsertNightbotToken :exec
INSERT INTO nightbot_tokens (user_email, access_token, refresh_token, expires_at, channel_name, channel_display_name, updated_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(user_email) DO UPDATE SET
    access_token = excluded.access_token,
    refresh_token = excluded.refresh_token,
    expires_at = excluded.expires_at,
    channel_name = excluded.channel_name,
    channel_display_name = excluded.channel_display_name,
    updated_at = CURRENT_TIMESTAMP;

-- name: DeleteNightbotToken :exec
DELETE FROM nightbot_tokens WHERE user_email = ?;
