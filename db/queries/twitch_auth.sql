-- Twitch authentication queries

-- name: CreateTwitchSession :exec
INSERT INTO twitch_sessions (id, twitch_id, twitch_username, display_name, expires_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetTwitchSession :one
SELECT * FROM twitch_sessions WHERE id = ? AND expires_at > datetime('now');

-- name: DeleteTwitchSession :exec
DELETE FROM twitch_sessions WHERE id = ?;

-- name: DeleteExpiredTwitchSessions :exec
DELETE FROM twitch_sessions WHERE expires_at <= datetime('now');

-- name: IsChannelModeratorByTwitch :one
SELECT EXISTS(
    SELECT 1 FROM nightbot_channel_moderators
    WHERE channel_name = ? AND twitch_username = ?
) AS is_moderator;

-- name: GetModeratorChannelsByTwitch :many
SELECT channel_name FROM nightbot_channel_moderators
WHERE twitch_username = ?
ORDER BY channel_name;
