CREATE TABLE IF NOT EXISTS reports (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT NOT NULL,
    day_of_month INT  NOT NULL DEFAULT 1 CHECK (day_of_month BETWEEN 1 AND 28),
    hour         INT  NOT NULL DEFAULT 8 CHECK (hour BETWEEN 0 AND 23),
    recipients   TEXT NOT NULL DEFAULT '',        -- comma-separated email addresses
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS report_executions (
    id            BIGSERIAL PRIMARY KEY,
    report_id     BIGINT NOT NULL REFERENCES reports(id) ON DELETE CASCADE,
    period_year   INT  NOT NULL,
    period_month  INT  NOT NULL,                  -- 1-12; the month the report is ABOUT
    trigger       TEXT NOT NULL DEFAULT 'scheduled',  -- 'scheduled' | 'manual'
    status        TEXT NOT NULL DEFAULT 'pending',    -- 'pending' | 'running' | 'success' | 'failed'
    event_id      BIGINT,                         -- id returned by the events service on enqueue
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    duration_ms   BIGINT,
    error_message TEXT,
    insights      JSONB,                          -- raw /report-insights data
    html          TEXT,                           -- browser-viewable report (img src = HTTP URLs)
    email_sent_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_report_executions_report_period
    ON report_executions (report_id, period_year, period_month);

CREATE TABLE IF NOT EXISTS report_charts (
    id           BIGSERIAL PRIMARY KEY,
    execution_id BIGINT NOT NULL REFERENCES report_executions(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,                   -- 'expenses-by-category', etc.
    image        BYTEA NOT NULL,                  -- PNG bytes
    UNIQUE (execution_id, name)
);

-- Seed a default report so the UI has something to edit on first boot.
INSERT INTO reports (name, day_of_month, hour, recipients, enabled)
SELECT 'Monthly report', 1, 8, '', TRUE
WHERE NOT EXISTS (SELECT 1 FROM reports);
