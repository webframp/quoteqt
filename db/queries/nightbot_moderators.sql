-- Nightbot channel moderator queries

-- name: AddChannelModerator :exec
INSERT INTO nightbot_channel_moderators (channel_name, user_email, added_by)
VALUES (?, ?, ?)
ON CONFLICT (channel_name, user_email) DO NOTHING;

-- name: AddChannelModeratorByTwitch :exec
INSERT INTO nightbot_channel_moderators (channel_name, user_email, twitch_username, added_by)
VALUES (?, NULL, ?, ?);

-- name: RemoveChannelModerator :exec
DELETE FROM nightbot_channel_moderators WHERE id = ?;

-- name: GetChannelModerators :many
SELECT * FROM nightbot_channel_moderators WHERE channel_name = ? ORDER BY added_at;

-- name: GetModeratorChannels :many
-- Returns channels a user can moderate
SELECT DISTINCT channel_name FROM nightbot_channel_moderators WHERE user_email = ? ORDER BY channel_name;

-- name: IsChannelModerator :one
SELECT COUNT(*) > 0 as is_moderator FROM nightbot_channel_moderators 
WHERE channel_name = ? AND user_email = ?;

-- name: GetAllModerators :many
SELECT * FROM nightbot_channel_moderators ORDER BY channel_name, user_email;

-- name: GetAllModeratorsByChannel :many
SELECT channel_name, GROUP_CONCAT(user_email) as emails
FROM nightbot_channel_moderators 
GROUP BY channel_name 
ORDER BY channel_name;
