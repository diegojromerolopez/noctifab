CREATE TABLE IF NOT EXISTS state (
    id VARCHAR(255) PRIMARY KEY,
    project_path TEXT NOT NULL,
    version INT NOT NULL,
    build_status VARCHAR(50) NOT NULL,
    input_source TEXT,
    input_path TEXT,
    integration_branch VARCHAR(255),
    feature_name VARCHAR(255),
    base_branch VARCHAR(255),
    project_version VARCHAR(50),
    total_tokens_used BIGINT NOT NULL DEFAULT 0,
    total_cost_usd NUMERIC(10, 5) NOT NULL DEFAULT 0.0
);

CREATE TABLE IF NOT EXISTS tasks (
    id VARCHAR(255) PRIMARY KEY,
    state_id VARCHAR(255) NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    status VARCHAR(50) NOT NULL,
    change_type VARCHAR(50) NOT NULL,
    assigned_to VARCHAR(255) NOT NULL DEFAULT '',
    depends_on JSONB NOT NULL, -- JSON array of parent task IDs or titles
    target_files JSONB, -- JSON array of paths
    partial_changelog JSONB, -- JSON array of changelog entries
    retries INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE IF NOT EXISTS clarifications (
    id VARCHAR(255) PRIMARY KEY,
    state_id VARCHAR(255) NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    question TEXT NOT NULL,
    answer TEXT DEFAULT '',
    resolved INT NOT NULL DEFAULT 0,
    asked_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE IF NOT EXISTS actions (
    id SERIAL PRIMARY KEY,
    state_id VARCHAR(255) NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    task_id VARCHAR(255),
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    tool VARCHAR(100) NOT NULL,
    args JSONB NOT NULL,
    reasoning TEXT NOT NULL,
    result TEXT NOT NULL,
    success INT NOT NULL
);

CREATE TABLE IF NOT EXISTS workspace_files (
    path TEXT PRIMARY KEY,
    state_id VARCHAR(255) NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    size BIGINT NOT NULL,
    last_modified TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE IF NOT EXISTS token_usage (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    task_id VARCHAR(255),
    agent_id VARCHAR(255),
    prompt_tokens BIGINT NOT NULL,
    completion_tokens BIGINT NOT NULL,
    cost_usd NUMERIC(10, 5) NOT NULL
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INT PRIMARY KEY,
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
