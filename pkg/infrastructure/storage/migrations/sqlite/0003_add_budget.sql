CREATE TABLE IF NOT EXISTS budget_usage (
    date TEXT NOT NULL,
    provider TEXT NOT NULL,
    cost_usd REAL NOT NULL DEFAULT 0.0,
    PRIMARY KEY (date, provider)
);
