CREATE TABLE IF NOT EXISTS state (
    id TEXT PRIMARY KEY,
    project_path TEXT NOT NULL,
    version INTEGER NOT NULL,
    build_status TEXT NOT NULL,
    input_source TEXT,
    input_path TEXT,
    integration_branch TEXT,
    feature_name TEXT,
    base_branch TEXT,
    project_version TEXT,
    total_tokens_used INTEGER NOT NULL DEFAULT 0
);

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

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    state_id TEXT NOT NULL,
    story_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL,
    change_type TEXT NOT NULL,
    assigned_to TEXT NOT NULL DEFAULT '',
    depends_on TEXT NOT NULL, -- JSON array of parent task IDs or titles
    target_files TEXT, -- JSON array of paths
    partial_changelog TEXT, -- JSON array of changelog entries
    retries INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY(state_id) REFERENCES state(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS clarifications (
    id TEXT PRIMARY KEY,
    state_id TEXT NOT NULL,
    question TEXT NOT NULL,
    answer TEXT DEFAULT '',
    resolved INTEGER NOT NULL DEFAULT 0,
    asked_at DATETIME NOT NULL,
    FOREIGN KEY(state_id) REFERENCES state(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    action_id TEXT DEFAULT '',
    state_id TEXT NOT NULL,
    task_id TEXT,
    timestamp DATETIME NOT NULL,
    tool TEXT NOT NULL,
    args TEXT NOT NULL, -- JSON formatted string
    reasoning TEXT NOT NULL,
    result TEXT NOT NULL,
    success INTEGER NOT NULL,
    FOREIGN KEY(state_id) REFERENCES state(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_actions_action_id ON actions(action_id);
CREATE INDEX IF NOT EXISTS idx_actions_state_id_id ON actions(state_id, id);

CREATE TABLE IF NOT EXISTS workspace_files (
    path TEXT NOT NULL,
    state_id TEXT NOT NULL,
    size INTEGER NOT NULL,
    last_modified DATETIME NOT NULL,
    PRIMARY KEY(path, state_id),
    FOREIGN KEY(state_id) REFERENCES state(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS token_usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    task_id TEXT,
    agent_id TEXT,
    prompt_tokens INTEGER NOT NULL,
    completion_tokens INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
