CREATE TABLE IF NOT EXISTS budget_usage (
    date TEXT NOT NULL,
    provider TEXT NOT NULL,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (date, provider)
);

