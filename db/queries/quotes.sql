-- name: CreateQuote :exec
INSERT INTO quotes (user_id, text, author, civilization, opponent_civ, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListQuotesByUser :many
SELECT * FROM quotes
WHERE user_id = ?
ORDER BY created_at DESC;

-- name: GetRandomQuote :one
SELECT * FROM quotes
ORDER BY RANDOM()
LIMIT 1;

-- name: GetRandomQuoteByCiv :one
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
UPDATE quotes SET text = ?, author = ?, civilization = ?, opponent_civ = ? WHERE id = ?;

-- name: CountQuotes :one
SELECT COUNT(*) as count FROM quotes;

-- name: ListAllQuotes :many
SELECT * FROM quotes ORDER BY created_at DESC;

-- name: ListQuotesPaginated :many
SELECT * FROM quotes ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: GetRandomMatchupQuote :one
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
