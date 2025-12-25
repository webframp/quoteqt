-- name: ListCivs :many
SELECT * FROM civilizations ORDER BY name;

-- name: GetCivByID :one
SELECT * FROM civilizations WHERE id = ?;

-- name: GetCivByName :one
SELECT * FROM civilizations WHERE name = ?;

-- name: GetCivByShortname :one
SELECT * FROM civilizations WHERE shortname = ?;

-- name: ResolveCivName :one
SELECT name FROM civilizations WHERE shortname = ? OR LOWER(name) = LOWER(?);

-- name: CountQuotesByCiv :one
SELECT COUNT(*) as count FROM quotes WHERE civilization = ?;

-- name: CreateCiv :exec
INSERT INTO civilizations (name, variant_of, dlc, shortname) VALUES (?, ?, ?, ?);

-- name: UpdateCiv :exec
UPDATE civilizations SET name = ?, variant_of = ?, dlc = ?, shortname = ? WHERE id = ?;

-- name: DeleteCiv :exec
DELETE FROM civilizations WHERE id = ?;
