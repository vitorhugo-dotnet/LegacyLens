CREATE TABLE traces (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    started_at TEXT NOT NULL,
    ended_at TEXT,
    incomplete INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX traces_project_started ON traces(project_id, started_at DESC);
CREATE TABLE trace_events (
    project_id TEXT NOT NULL,
    trace_id TEXT NOT NULL REFERENCES traces(id),
    producer_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    parent_event_id TEXT,
    occurred_at TEXT NOT NULL,
    value_json TEXT NOT NULL,
    PRIMARY KEY(project_id, trace_id, producer_id, event_id)
);
CREATE INDEX trace_events_order ON trace_events(project_id, trace_id, producer_id, sequence);
CREATE TABLE trace_diagnostics (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    trace_id TEXT NOT NULL REFERENCES traces(id),
    value_json TEXT NOT NULL
);
CREATE INDEX trace_diagnostics_trace ON trace_diagnostics(project_id, trace_id);
