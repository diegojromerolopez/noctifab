CREATE TABLE IF NOT EXISTS budget_usage (
    date DATE NOT NULL,
    provider VARCHAR(255) NOT NULL,
    tokens_used BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (date, provider)
);

