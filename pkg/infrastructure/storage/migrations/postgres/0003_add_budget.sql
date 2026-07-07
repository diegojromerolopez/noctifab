CREATE TABLE IF NOT EXISTS budget_usage (
    date DATE NOT NULL,
    provider VARCHAR(255) NOT NULL,
    cost_usd NUMERIC(10, 5) NOT NULL DEFAULT 0.0,
    PRIMARY KEY (date, provider)
);
