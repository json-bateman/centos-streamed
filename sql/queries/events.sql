-- name: CreateEvent :one
INSERT INTO events (kind, ip)
VALUES (?, ?)
RETURNING *;

-- name: GetRecentEvents :many
SELECT id, kind, ip, created_at FROM events
ORDER BY id DESC
LIMIT ?;

-- name: CountEvents :one
SELECT count(*) FROM events;
