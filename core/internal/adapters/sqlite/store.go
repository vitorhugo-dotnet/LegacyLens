package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
	_ "modernc.org/sqlite"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) < 4 {
			continue
		}
		var version int
		if _, err := fmt.Sscanf(entry.Name()[:3], "%d", &version); err != nil {
			return fmt.Errorf("invalid migration name %q", entry.Name())
		}
		var applied bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)`, version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlText, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(sqlText)); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, version, time.Now().UTC().Format(time.RFC3339Nano))
		}
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SaveProject(ctx context.Context, project domain.Project) error {
	if project.ID == "" || project.Root == "" {
		return errors.New("project id and root are required")
	}
	includes, _ := json.Marshal(project.Includes)
	excludes, _ := json.Marshal(project.Excludes)
	_, err := s.db.ExecContext(ctx, `INSERT INTO projects(id,name,root,includes_json,excludes_json,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name, root=excluded.root, includes_json=excluded.includes_json, excludes_json=excluded.excludes_json`, project.ID, project.Name, project.Root, string(includes), string(excludes), project.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) LoadProject(ctx context.Context, id domain.ID) (domain.Project, error) {
	var project domain.Project
	var includes, excludes, created string
	err := s.db.QueryRowContext(ctx, `SELECT id,name,root,includes_json,excludes_json,created_at FROM projects WHERE id=?`, id).Scan(&project.ID, &project.Name, &project.Root, &includes, &excludes, &created)
	if err != nil {
		return domain.Project{}, err
	}
	if err := json.Unmarshal([]byte(includes), &project.Includes); err != nil {
		return domain.Project{}, err
	}
	if err := json.Unmarshal([]byte(excludes), &project.Excludes); err != nil {
		return domain.Project{}, err
	}
	project.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return project, err
}

func (s *Store) CommitIndex(ctx context.Context, result application.IndexResult) error {
	if result.ProjectID == "" || result.RevisionID == "" {
		return errors.New("project and revision ids are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM revisions WHERE id=?`, result.RevisionID).Scan(&exists)
	if err == nil {
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	digest := string(result.RevisionID)
	if _, err := tx.ExecContext(ctx, `INSERT INTO revisions(id,project_id,digest,created_at) VALUES(?,?,?,?)`, result.RevisionID, result.ProjectID, digest, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	for _, artifact := range result.Artifacts {
		if err := insertJSON(ctx, tx, `INSERT INTO artifacts(id,project_id,revision_id,path,content_hash,value_json) VALUES(?,?,?,?,?,?)`, artifact.ID, artifact.ProjectID, artifact.RevisionID, artifact.Path, artifact.ContentHash, artifact); err != nil {
			return err
		}
	}
	for _, symbol := range result.Symbols {
		if err := insertJSON(ctx, tx, `INSERT INTO symbols(id,project_id,revision_id,artifact_id,name,kind,value_json) VALUES(?,?,?,?,?,?,?)`, symbol.ID, symbol.ProjectID, symbol.RevisionID, symbol.ArtifactID, symbol.QualifiedName, symbol.Kind, symbol); err != nil {
			return err
		}
	}
	for _, relation := range result.Relations {
		toID := any(nil)
		if relation.ToID != nil {
			toID = *relation.ToID
		}
		if err := insertJSON(ctx, tx, `INSERT INTO relations(id,project_id,revision_id,from_id,to_id,value_json) VALUES(?,?,?,?,?,?)`, relation.ID, result.ProjectID, result.RevisionID, relation.FromID, toID, relation); err != nil {
			return err
		}
	}
	for _, evidence := range result.Evidence {
		if err := insertJSON(ctx, tx, `INSERT INTO evidence(id,project_id,revision_id,value_json) VALUES(?,?,?,?)`, evidence.ID, result.ProjectID, result.RevisionID, evidence); err != nil {
			return err
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if err := insertJSON(ctx, tx, `INSERT INTO diagnostics(id,project_id,revision_id,value_json) VALUES(?,?,?,?)`, diagnostic.ID, result.ProjectID, result.RevisionID, diagnostic); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertJSON(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	value, err := json.Marshal(args[len(args)-1])
	if err != nil {
		return err
	}
	args[len(args)-1] = string(value)
	_, err = tx.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) Search(ctx context.Context, query application.SearchQuery) (application.SearchResult, error) {
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	where := `project_id=? AND revision_id=? AND name LIKE ?`
	args := []any{query.ProjectID, query.RevisionID, "%" + query.Text + "%"}
	if len(query.Kinds) > 0 {
		where += ` AND kind IN (` + stringPlaceholders(len(query.Kinds)) + `)`
		for _, kind := range query.Kinds {
			args = append(args, kind)
		}
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM symbols WHERE `+where, args...).Scan(&total); err != nil {
		return application.SearchResult{}, err
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT value_json FROM symbols WHERE `+where+` LIMIT ?`, args...)
	if err != nil {
		return application.SearchResult{}, err
	}
	defer rows.Close()
	result := application.SearchResult{}
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return application.SearchResult{}, err
		}
		var symbol domain.Symbol
		if err := json.Unmarshal([]byte(encoded), &symbol); err != nil {
			return application.SearchResult{}, err
		}
		result.Symbols = append(result.Symbols, symbol)
	}
	result.Total = total
	return result, rows.Err()
}

func (s *Store) Explore(ctx context.Context, query application.GraphQuery) (application.GraphResult, error) {
	result := application.GraphResult{}
	if len(query.SymbolIDs) == 0 {
		return result, nil
	}
	marks, args := placeholders(query.SymbolIDs)
	args = append([]any{query.ProjectID, query.RevisionID}, args...)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT value_json FROM symbols WHERE project_id=? AND revision_id=? AND id IN (%s)`, marks), args...)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var encoded string
		var symbol domain.Symbol
		if err := rows.Scan(&encoded); err != nil {
			rows.Close()
			return result, err
		}
		if err := json.Unmarshal([]byte(encoded), &symbol); err != nil {
			rows.Close()
			return result, err
		}
		result.Symbols = append(result.Symbols, symbol)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	marks, ids := placeholders(query.SymbolIDs)
	relationArgs := []any{query.ProjectID, query.RevisionID}
	relationArgs = append(relationArgs, ids...)
	relationArgs = append(relationArgs, ids...)
	rows, err = s.db.QueryContext(ctx, fmt.Sprintf(`SELECT value_json FROM relations WHERE project_id=? AND revision_id=? AND (from_id IN (%s) OR to_id IN (%s))`, marks, marks), relationArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var encoded string
		var relation domain.Relation
		if err := rows.Scan(&encoded); err != nil {
			return result, err
		}
		if err := json.Unmarshal([]byte(encoded), &relation); err != nil {
			return result, err
		}
		result.Relations = append(result.Relations, relation)
	}
	return result, rows.Err()
}

func placeholders(ids []domain.ID) (string, []any) {
	values := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		values[i] = "?"
		args[i] = id
	}
	return strings.Join(values, ","), args
}

func stringPlaceholders(count int) string {
	values := make([]string, count)
	for i := range values {
		values[i] = "?"
	}
	return strings.Join(values, ",")
}
