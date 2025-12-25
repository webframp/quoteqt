-- name: ListCivs :many
SELECT * FROM civilizations ORDER BY name;

-- name: GetCivByName :one
SELECT * FROM civilizations WHERE name = ?;

-- name: CountQuotesByCiv :one
SELECT COUNT(*) as count FROM quotes WHERE civilization = ?;
