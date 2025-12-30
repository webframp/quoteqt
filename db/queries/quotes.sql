-- name: CreateQuote :exec
INSERT INTO quotes (user_id, created_by_email, text, author, civilization, opponent_civ, channel, requested_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListQuotesByUser :many
SELECT * FROM quotes
WHERE user_id = ?
ORDER BY created_at DESC;

-- name: GetRandomQuote :one
SELECT * FROM quotes
WHERE channel IS NULL OR channel = ?
ORDER BY RANDOM()
LIMIT 1;

-- name: GetRandomQuoteGlobal :one
SELECT * FROM quotes
ORDER BY RANDOM()
LIMIT 1;

-- name: GetRandomQuoteByCiv :one
SELECT * FROM quotes
WHERE civilization = ? AND (channel IS NULL OR channel = ?)
ORDER BY RANDOM()
LIMIT 1;

-- name: GetRandomQuoteByCivGlobal :one
SELECT * FROM quotes
WHERE civilization = ?
ORDER BY RANDOM()
LIMIT 1;

-- name: DeleteQuote :exec
DELETE FROM quotes WHERE id = ? AND user_id = ?;

-- name: DeleteQuoteByID :exec
DELETE FROM quotes WHERE id = ?;

-- name: GetQuoteByID :one
SELECT * FROM quotes WHERE id = ?;

-- name: UpdateQuote :exec
UPDATE quotes SET text = ?, author = ?, civilization = ?, opponent_civ = ?, channel = ? WHERE id = ?;

-- name: CountQuotes :one
SELECT COUNT(*) as count FROM quotes;

-- name: ListAllQuotes :many
SELECT * FROM quotes ORDER BY created_at DESC;

-- name: ListQuotesPaginated :many
SELECT * FROM quotes ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: GetRandomMatchupQuote :one
SELECT * FROM quotes
WHERE civilization = ? AND opponent_civ = ? AND (channel IS NULL OR channel = ?)
ORDER BY RANDOM()
LIMIT 1;

-- name: GetRandomMatchupQuoteGlobal :one
SELECT * FROM quotes
WHERE civilization = ? AND opponent_civ = ?
ORDER BY RANDOM()
LIMIT 1;

-- name: ListMatchupQuotes :many
SELECT * FROM quotes
WHERE civilization = ? AND opponent_civ = ?
ORDER BY created_at DESC;

-- name: ListCivilizations :many
SELECT DISTINCT civilization FROM quotes WHERE civilization IS NOT NULL ORDER BY civilization;

-- name: DeleteQuoteByText :exec
DELETE FROM quotes WHERE text = ?;

-- name: ListQuotesByChannel :many
SELECT * FROM quotes
WHERE channel = ? OR channel IS NULL
ORDER BY created_at DESC;

-- name: ListChannels :many
SELECT DISTINCT channel FROM quotes WHERE channel IS NOT NULL ORDER BY channel;

-- name: BulkUpdateChannel :exec
UPDATE quotes SET channel = ? WHERE id IN (sqlc.slice('ids'));

-- name: BulkUpdateCivilization :exec
UPDATE quotes SET civilization = ? WHERE id IN (sqlc.slice('ids'));

-- name: BulkDeleteQuotes :exec
DELETE FROM quotes WHERE id IN (sqlc.slice('ids'));

-- name: GetLastUpdated :one
SELECT created_at FROM quotes ORDER BY created_at DESC LIMIT 1;

-- name: ListQuotesByChannelOnly :many
SELECT * FROM quotes
WHERE channel = ?
ORDER BY created_at DESC;

-- name: ListQuotesByChannelPaginated :many
SELECT * FROM quotes
WHERE channel = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CountQuotesByChannel :one
SELECT COUNT(*) as count FROM quotes WHERE channel = ?;
