CREATE TABLE story_contracts (
    state_id VARCHAR(255) NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    story_id VARCHAR(255) NOT NULL,
    source_path TEXT NOT NULL,
    source_sha256 VARCHAR(64) NOT NULL,
    public_contracts JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (state_id, story_id)
);

CREATE TABLE review_phases (
    id VARCHAR(255) PRIMARY KEY,
    state_id VARCHAR(255) NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    story_id VARCHAR(255) NOT NULL,
    task_id VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL,
    artifact_id TEXT NOT NULL,
    artifact_manifest JSONB NOT NULL,
    attempt INT NOT NULL,
    status VARCHAR(50) NOT NULL,
    terminal_reason TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMP WITH TIME ZONE NOT NULL,
    deadline_at TIMESTAMP WITH TIME ZONE NOT NULL,
    completed_at TIMESTAMP WITH TIME ZONE,
    tokens_used BIGINT NOT NULL DEFAULT 0,
    UNIQUE (story_id, task_id, role, artifact_id, attempt)
);

CREATE TABLE qa_scenarios (
    id VARCHAR(255) PRIMARY KEY,
    state_id VARCHAR(255) NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    review_phase_id VARCHAR(255) NOT NULL REFERENCES review_phases(id) ON DELETE CASCADE,
    public_contract_id VARCHAR(255) NOT NULL,
    name TEXT NOT NULL,
    fingerprint VARCHAR(64) NOT NULL,
    steps JSONB NOT NULL,
    status VARCHAR(50) NOT NULL,
    evidence TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (review_phase_id, fingerprint)
);

CREATE TABLE qa_findings (
    id VARCHAR(255) PRIMARY KEY,
    state_id VARCHAR(255) NOT NULL REFERENCES state(id) ON DELETE CASCADE,
    review_phase_id VARCHAR(255) NOT NULL REFERENCES review_phases(id) ON DELETE CASCADE,
    task_id VARCHAR(255) NOT NULL,
    artifact_id TEXT NOT NULL,
    scenario_fingerprint VARCHAR(64) NOT NULL,
    public_contract_id VARCHAR(255) NOT NULL,
    severity VARCHAR(50) NOT NULL,
    expected TEXT NOT NULL,
    actual TEXT NOT NULL,
    evidence TEXT NOT NULL,
    disposition VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (artifact_id, scenario_fingerprint)
);

CREATE INDEX review_phases_state_task_idx ON review_phases(state_id, task_id);
CREATE INDEX review_phases_story_status_idx ON review_phases(story_id, status);
CREATE INDEX review_phases_artifact_idx ON review_phases(artifact_id);
CREATE INDEX qa_scenarios_state_task_idx ON qa_scenarios(state_id, review_phase_id);
CREATE INDEX qa_findings_state_task_idx ON qa_findings(state_id, task_id);
CREATE INDEX qa_findings_artifact_idx ON qa_findings(artifact_id);
