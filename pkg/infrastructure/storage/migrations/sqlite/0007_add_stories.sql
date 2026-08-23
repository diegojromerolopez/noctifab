CREATE TABLE IF NOT EXISTS stories (
    id TEXT PRIMARY KEY,
    state_id TEXT NOT NULL,
    title TEXT NOT NULL,
    file_path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    started_at DATETIME,
    completed_at DATETIME,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY(state_id) REFERENCES state(id) ON DELETE CASCADE
);

ALTER TABLE tasks ADD COLUMN story_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN started_at DATETIME;
ALTER TABLE tasks ADD COLUMN completed_at DATETIME;
ALTER TABLE token_usage ADD COLUMN story_id TEXT DEFAULT '';
ALTER TABLE token_usage ADD COLUMN provider TEXT DEFAULT '';
ALTER TABLE token_usage ADD COLUMN model TEXT DEFAULT '';
ALTER TABLE actions ADD COLUMN story_id TEXT DEFAULT '';
