CREATE TABLE IF NOT EXISTS middleman_author_groups (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    members_json TEXT NOT NULL DEFAULT '[]',
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
