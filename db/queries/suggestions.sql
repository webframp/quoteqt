-- name: CreateSuggestion :exec
INSERT INTO quote_suggestions (text, author, civilization, opponent_civ, channel, submitted_by_ip, submitted_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListPendingSuggestions :many
SELECT * FROM quote_suggestions
WHERE status = 'pending'
ORDER BY submitted_at DESC;

-- name: ListPendingSuggestionsByChannel :many
SELECT * FROM quote_suggestions
WHERE channel = ? AND status = 'pending'
ORDER BY submitted_at DESC;

-- name: GetSuggestionByID :one
SELECT * FROM quote_suggestions WHERE id = ?;

-- name: ApproveSuggestion :exec
UPDATE quote_suggestions
SET status = 'approved', reviewed_by = ?, reviewed_at = ?
WHERE id = ?;

-- name: RejectSuggestion :exec
UPDATE quote_suggestions
SET status = 'rejected', reviewed_by = ?, reviewed_at = ?
WHERE id = ?;

-- name: CountPendingSuggestions :one
SELECT COUNT(*) as count FROM quote_suggestions WHERE status = 'pending';

-- name: CountPendingSuggestionsByChannel :one
SELECT COUNT(*) as count FROM quote_suggestions WHERE channel = ? AND status = 'pending';

-- name: CountRecentSuggestionsByIP :one
SELECT COUNT(*) as count FROM quote_suggestions
WHERE submitted_by_ip = ? AND submitted_at > ?;

-- name: CountRecentSuggestionsByChannel :one
SELECT COUNT(*) as count FROM quote_suggestions
WHERE channel = ? AND submitted_at > ?;

-- name: DeleteSuggestion :exec
DELETE FROM quote_suggestions WHERE id = ?;
