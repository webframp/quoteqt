-- name: CreateQuote :exec
INSERT INTO quotes (user_id, text, author, created_at)
VALUES (?, ?, ?, ?);

-- name: ListQuotesByUser :many
SELECT * FROM quotes
WHERE user_id = ?
ORDER BY created_at DESC;

-- name: GetRandomQuote :one
SELECT * FROM quotes
ORDER BY RANDOM()
LIMIT 1;

-- name: DeleteQuote :exec
DELETE FROM quotes WHERE id = ? AND user_id = ?;

-- name: CountQuotes :one
SELECT COUNT(*) as count FROM quotes;
