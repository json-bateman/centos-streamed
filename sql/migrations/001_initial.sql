-- +goose Up
CREATE TABLE events (
    id         INTEGER PRIMARY KEY,
    kind       TEXT NOT NULL DEFAULT 'visit',
    ip         TEXT NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- +goose Down
DROP TABLE events;
