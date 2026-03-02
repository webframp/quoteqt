-- User tracking queries

-- name: UpsertUser :exec
-- Record a user visit (insert or update last_seen and visit_count)
INSERT INTO users (user_id, email, first_seen_at, last_seen_at, visit_count)
VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 1)
ON CONFLICT (user_id) DO UPDATE SET
    email = excluded.email,
    last_seen_at = CURRENT_TIMESTAMP,
    visit_count = visit_count + 1;

-- name: GetAllUsers :many
SELECT * FROM users ORDER BY last_seen_at DESC;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = ?;

-- name: GetUserByID :one
SELECT * FROM users WHERE user_id = ?;

-- name: GetRecentUsers :many
SELECT * FROM users ORDER BY last_seen_at DESC LIMIT ?;

-- name: GetUserCount :one
SELECT COUNT(*) FROM users;
