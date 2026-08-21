CREATE TABLE story_contracts (
    state_id TEXT NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    story_id TEXT NOT NULL,
    source_path TEXT NOT NULL,
    source_sha256 TEXT NOT NULL,
    public_contracts TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (state_id, story_id)
);

CREATE TABLE review_phases (
    id TEXT PRIMARY KEY,
    state_id TEXT NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    story_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    role TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    artifact_manifest TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    status TEXT NOT NULL,
    terminal_reason TEXT NOT NULL DEFAULT '',
    started_at DATETIME NOT NULL,
    deadline_at DATETIME NOT NULL,
    completed_at DATETIME,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    UNIQUE (story_id, task_id, role, artifact_id, attempt)
);

CREATE TABLE qa_scenarios (
    id TEXT PRIMARY KEY,
    state_id TEXT NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    review_phase_id TEXT NOT NULL REFERENCES review_phases(id) ON DELETE CASCADE,
    public_contract_id TEXT NOT NULL,
    name TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    steps TEXT NOT NULL,
    status TEXT NOT NULL,
    evidence TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (review_phase_id, fingerprint)
);

CREATE TABLE qa_findings (
    id TEXT PRIMARY KEY,
    state_id TEXT NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    review_phase_id TEXT NOT NULL REFERENCES review_phases(id) ON DELETE CASCADE,
    task_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    scenario_fingerprint TEXT NOT NULL,
    public_contract_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    expected TEXT NOT NULL,
    actual TEXT NOT NULL,
    evidence TEXT NOT NULL,
    disposition TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (artifact_id, scenario_fingerprint)
);

CREATE INDEX review_phases_state_task_idx ON review_phases(state_id, task_id);
CREATE INDEX review_phases_story_status_idx ON review_phases(story_id, status);
CREATE INDEX review_phases_artifact_idx ON review_phases(artifact_id);
CREATE INDEX qa_scenarios_state_task_idx ON qa_scenarios(state_id, review_phase_id);
CREATE INDEX qa_findings_state_task_idx ON qa_findings(state_id, task_id);
CREATE INDEX qa_findings_artifact_idx ON qa_findings(artifact_id);
