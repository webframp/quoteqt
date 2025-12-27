-- name: GetChannelsByOwner :many
SELECT channel FROM channel_owners WHERE user_email = ?;

-- name: GetOwnersByChannel :many
SELECT user_email FROM channel_owners WHERE channel = ?;

-- name: IsChannelOwner :one
SELECT COUNT(*) > 0 as is_owner FROM channel_owners WHERE channel = ? AND user_email = ?;

-- name: AddChannelOwner :exec
INSERT INTO channel_owners (channel, user_email, invited_by) VALUES (?, ?, ?);

-- name: RemoveChannelOwner :exec
DELETE FROM channel_owners WHERE channel = ? AND user_email = ?;

-- name: ListAllChannelOwners :many
SELECT * FROM channel_owners ORDER BY channel, user_email;

-- name: CountChannelOwners :one
SELECT COUNT(*) as count FROM channel_owners;
