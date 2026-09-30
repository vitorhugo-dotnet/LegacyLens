CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    root TEXT NOT NULL,
    includes_json TEXT NOT NULL,
    excludes_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS revisions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    digest TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS revisions_project ON revisions(project_id, created_at DESC);
CREATE TABLE IF NOT EXISTS artifacts (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    revision_id TEXT NOT NULL REFERENCES revisions(id),
    path TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    value_json TEXT NOT NULL,
    UNIQUE(project_id, revision_id, path)
);
CREATE TABLE IF NOT EXISTS symbols (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    revision_id TEXT NOT NULL REFERENCES revisions(id),
    artifact_id TEXT NOT NULL REFERENCES artifacts(id),
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    value_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS symbols_search ON symbols(project_id, revision_id, kind, name);
CREATE TABLE IF NOT EXISTS relations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    revision_id TEXT NOT NULL REFERENCES revisions(id),
    from_id TEXT NOT NULL,
    to_id TEXT,
    value_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS relations_from ON relations(project_id, revision_id, from_id);
CREATE TABLE IF NOT EXISTS evidence (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    revision_id TEXT NOT NULL REFERENCES revisions(id),
    value_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS diagnostics (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    revision_id TEXT NOT NULL REFERENCES revisions(id),
    value_json TEXT NOT NULL
);
