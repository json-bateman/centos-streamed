-- name: CreateMessage :one
INSERT INTO messages (author, body)
VALUES (?, ?)
RETURNING *;

-- name: GetRecentMessages :many
SELECT id, author, body, created_at FROM messages
ORDER BY id DESC
LIMIT ?;

-- name: CountMessages :one
SELECT count(*) FROM messages;
