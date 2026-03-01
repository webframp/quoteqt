-- Managed channels queries for automated Nightbot backup

-- name: CreateManagedChannel :one
INSERT INTO nightbot_managed_channels (
    user_email, channel_id, channel_name, session_token_encrypted,
    sync_enabled, sync_interval_minutes
) VALUES (?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: GetManagedChannel :one
SELECT * FROM nightbot_managed_channels WHERE id = ?;

-- name: GetManagedChannelByChannelID :one
SELECT * FROM nightbot_managed_channels WHERE channel_id = ?;

-- name: GetManagedChannelsByUser :many
SELECT * FROM nightbot_managed_channels WHERE user_email = ? ORDER BY channel_name;

-- name: GetAllManagedChannels :many
SELECT * FROM nightbot_managed_channels ORDER BY channel_name;

-- name: GetManagedChannelsDueForSync :many
-- Returns channels that are enabled and due for sync based on their interval
SELECT * FROM nightbot_managed_channels
WHERE sync_enabled = 1
  AND (last_sync_at IS NULL 
       OR datetime(last_sync_at, '+' || sync_interval_minutes || ' minutes') <= datetime('now'))
ORDER BY last_sync_at ASC NULLS FIRST;

-- name: UpdateManagedChannelSyncStatus :exec
UPDATE nightbot_managed_channels
SET last_sync_at = CURRENT_TIMESTAMP,
    last_sync_status = ?,
    last_error = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: UpdateManagedChannelToken :exec
UPDATE nightbot_managed_channels
SET session_token_encrypted = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: UpdateManagedChannelSettings :exec
UPDATE nightbot_managed_channels
SET channel_name = ?,
    sync_enabled = ?,
    sync_interval_minutes = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DisableManagedChannelSync :exec
UPDATE nightbot_managed_channels
SET sync_enabled = 0,
    last_sync_status = 'disabled',
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: EnableManagedChannelSync :exec
UPDATE nightbot_managed_channels
SET sync_enabled = 1,
    last_sync_status = NULL,
    last_error = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteManagedChannel :exec
DELETE FROM nightbot_managed_channels WHERE id = ?;
