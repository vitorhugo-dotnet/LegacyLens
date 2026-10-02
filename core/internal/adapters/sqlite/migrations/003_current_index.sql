CREATE TABLE current_index (
    project_id TEXT PRIMARY KEY REFERENCES projects(id),
    revision_id TEXT NOT NULL REFERENCES revisions(id)
);
INSERT INTO current_index(project_id, revision_id)
SELECT project_id, id FROM revisions AS r
WHERE id = (SELECT r2.id FROM revisions AS r2 WHERE r2.project_id = r.project_id ORDER BY r2.created_at DESC, r2.id DESC LIMIT 1);
