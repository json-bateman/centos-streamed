-- +goose Up
CREATE TABLE messages (
    id         INTEGER PRIMARY KEY,
    author     TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- +goose Down
DROP TABLE messages;
