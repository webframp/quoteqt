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

-- name: CreateNightbotSnapshot :one
INSERT INTO nightbot_snapshots (channel_name, command_count, commands_json, created_by, note)
VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: GetNightbotSnapshots :many
SELECT * FROM nightbot_snapshots WHERE channel_name = ? AND deleted_at IS NULL ORDER BY snapshot_at DESC LIMIT ?;

-- name: GetNightbotSnapshot :one
SELECT * FROM nightbot_snapshots WHERE id = ?;

-- name: DeleteNightbotSnapshot :exec
DELETE FROM nightbot_snapshots WHERE id = ?;

-- name: SoftDeleteNightbotSnapshot :exec
UPDATE nightbot_snapshots SET deleted_at = CURRENT_TIMESTAMP, deleted_by = ? WHERE id = ?;

-- name: RestoreNightbotSnapshot :exec
UPDATE nightbot_snapshots SET deleted_at = NULL, deleted_by = NULL WHERE id = ?;

-- name: GetDeletedNightbotSnapshots :many
SELECT * FROM nightbot_snapshots WHERE channel_name = ? AND deleted_at IS NOT NULL ORDER BY deleted_at DESC LIMIT ?;

-- name: GetAllDeletedSnapshots :many
SELECT * FROM nightbot_snapshots WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC LIMIT ?;

-- name: PurgeOldDeletedSnapshots :exec
-- Permanently delete snapshots that were soft-deleted more than 14 days ago
DELETE FROM nightbot_snapshots WHERE deleted_at IS NOT NULL AND deleted_at < datetime('now', '-14 days');

-- name: UpdateSnapshotDiffCache :exec
UPDATE nightbot_snapshots
SET last_diff_added = ?, last_diff_removed = ?, last_diff_modified = ?, last_diff_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: GetImportedOnlyChannels :many
-- Returns channels that have snapshots but no OAuth tokens (imported via Tampermonkey)
SELECT DISTINCT channel_name FROM nightbot_snapshots
WHERE channel_name NOT IN (SELECT channel_name FROM nightbot_tokens)
  AND deleted_at IS NULL
ORDER BY channel_name;

-- name: GetChannelLastSnapshot :one
-- Returns the most recent snapshot date for a channel
SELECT snapshot_at FROM nightbot_snapshots 
WHERE channel_name = ? AND deleted_at IS NULL 
ORDER BY snapshot_at DESC LIMIT 1;

-- name: GetAllChannelsLastSnapshot :many
-- Returns the most recent snapshot date for all channels
SELECT channel_name, 
       (SELECT snapshot_at FROM nightbot_snapshots s2 
        WHERE s2.channel_name = s1.channel_name AND s2.deleted_at IS NULL 
        ORDER BY snapshot_at DESC LIMIT 1) as last_snapshot_at
FROM nightbot_snapshots s1 
WHERE deleted_at IS NULL
GROUP BY channel_name;

-- name: UpdateSnapshotNote :exec
UPDATE nightbot_snapshots SET note = ? WHERE id = ?;
