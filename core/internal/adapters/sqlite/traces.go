package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

const captureEventLimit = 10_000

func (s *Store) StartCapture(ctx context.Context, trace domain.Trace, _ string) error {
	if trace.ID == "" || trace.ProjectID == "" || trace.StartedAt.IsZero() {
		return errors.New("trace id, project id and start time are required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO traces(id,project_id,started_at) VALUES(?,?,?)`, trace.ID, trace.ProjectID, trace.StartedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) StopCapture(ctx context.Context, projectID, traceID domain.ID, at time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE traces SET ended_at=COALESCE(ended_at,?) WHERE project_id=? AND id=?`, at.UTC().Format(time.RFC3339Nano), projectID, traceID)
	return requireOne(result, err, "capture session not found")
}

func (s *Store) MarkCaptureIncomplete(ctx context.Context, projectID, traceID domain.ID, diagnostic domain.Diagnostic) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE traces SET incomplete=1,ended_at=COALESCE(ended_at,?) WHERE project_id=? AND id=?`, diagnostic.CreatedAt.UTC().Format(time.RFC3339Nano), projectID, traceID)
	if err != nil {
		return err
	}
	if err = requireOne(result, nil, "capture session not found"); err != nil {
		return err
	}
	if err = insertTraceDiagnostic(ctx, tx, projectID, traceID, diagnostic); err != nil {
		return err
	}
	return tx.Commit()
}

func requireOne(result sql.Result, err error, message string) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New(message)
	}
	return nil
}

func (s *Store) Append(ctx context.Context, events []domain.Event) (application.IngestResult, error) {
	return s.AppendBounded(ctx, events, captureEventLimit, nil)
}

func (s *Store) AppendBounded(ctx context.Context, events []domain.Event, eventLimit int, diagnostics []domain.Diagnostic) (application.IngestResult, error) {
	result := application.IngestResult{}
	if len(events) == 0 {
		return result, nil
	}
	if eventLimit <= 0 {
		eventLimit = captureEventLimit
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var projectID, traceID domain.ID
	for i, event := range events {
		if event.ProjectID == "" || event.TraceID == "" || event.ProducerID == "" || event.EventID == "" || event.Sequence == 0 {
			return result, errors.New("event identity and sequence are required")
		}
		if i == 0 {
			projectID, traceID = event.ProjectID, event.TraceID
		} else if projectID != event.ProjectID || traceID != event.TraceID {
			return result, errors.New("append accepts one project trace at a time")
		}
	}
	var ended sql.NullString
	var incomplete int
	if err := tx.QueryRowContext(ctx, `SELECT ended_at,incomplete FROM traces WHERE project_id=? AND id=?`, projectID, traceID).Scan(&ended, &incomplete); err != nil {
		return result, err
	}
	if ended.Valid || incomplete != 0 {
		return result, errors.New("capture session is closed")
	}
	for _, diagnostic := range diagnostics {
		if err := insertTraceDiagnostic(ctx, tx, projectID, traceID, diagnostic); err != nil {
			return result, err
		}
	}
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM trace_events WHERE project_id=? AND trace_id=?`, projectID, traceID).Scan(&count); err != nil {
		return result, err
	}
	for _, event := range events {
		metadata, diagnostics := domain.SanitizeMetadata(event.Metadata)
		event.Metadata = metadata
		for i := range diagnostics {
			diagnostics[i].CreatedAt = event.OccurredAt.UTC()
			result.Diagnostics = append(result.Diagnostics, diagnostics...)
			if err := insertTraceDiagnostic(ctx, tx, projectID, traceID, diagnostics[i]); err != nil {
				return result, err
			}
		}
		var exists int
		err = tx.QueryRowContext(ctx, `SELECT 1 FROM trace_events WHERE project_id=? AND trace_id=? AND producer_id=? AND event_id=?`, event.ProjectID, event.TraceID, event.ProducerID, event.EventID).Scan(&exists)
		if err == nil {
			result.Duplicate++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return result, err
		}
		if count >= eventLimit {
			diagnostic := domain.Diagnostic{Code: "capture.event_limit", Message: "Capture event limit reached; accepted events were preserved.", Severity: "warning", CreatedAt: event.OccurredAt.UTC()}
			if err := markIncompleteTx(ctx, tx, projectID, traceID, diagnostic); err != nil {
				return result, err
			}
			result.Diagnostics = append(result.Diagnostics, diagnostic)
			break
		}
		var parent any
		if event.ParentEventID != nil {
			parent = *event.ParentEventID
		}
		inserted, err := tx.ExecContext(ctx, `INSERT INTO trace_events(project_id,trace_id,producer_id,event_id,sequence,parent_event_id,occurred_at,value_json) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(project_id,trace_id,producer_id,event_id) DO NOTHING`, event.ProjectID, event.TraceID, event.ProducerID, event.EventID, event.Sequence, parent, event.OccurredAt.UTC().Format(time.RFC3339Nano), string(encoded))
		if err != nil {
			return result, err
		}
		n, err := inserted.RowsAffected()
		if err != nil {
			return result, err
		}
		if n == 0 {
			result.Duplicate++
		} else {
			result.Accepted++
			count++
		}
	}
	stored, err := loadTraceEvents(ctx, tx, projectID, traceID)
	if err != nil {
		return result, err
	}
	if err = refreshGapDiagnostics(ctx, tx, projectID, traceID, stored); err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func markIncompleteTx(ctx context.Context, tx *sql.Tx, projectID, traceID domain.ID, diagnostic domain.Diagnostic) error {
	_, err := tx.ExecContext(ctx, `UPDATE traces SET incomplete=1,ended_at=COALESCE(ended_at,?) WHERE project_id=? AND id=?`, diagnostic.CreatedAt.UTC().Format(time.RFC3339Nano), projectID, traceID)
	if err != nil {
		return err
	}
	return insertTraceDiagnostic(ctx, tx, projectID, traceID, diagnostic)
}

func insertTraceDiagnostic(ctx context.Context, tx *sql.Tx, projectID, traceID domain.ID, diagnostic domain.Diagnostic) error {
	diagnostic.ID = domain.ID(fmt.Sprintf("diag_%s_%x_%d", traceID, []byte(diagnostic.Code), diagnostic.CreatedAt.UTC().UnixNano()))
	encoded, err := json.Marshal(diagnostic)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO trace_diagnostics(id,project_id,trace_id,value_json) VALUES(?,?,?,?)`, diagnostic.ID, projectID, traceID, string(encoded))
	return err
}

func (s *Store) Load(ctx context.Context, projectID, traceID domain.ID) (application.Investigation, error) {
	var investigation application.Investigation
	project, err := s.LoadProject(ctx, projectID)
	if err != nil {
		return investigation, err
	}
	investigation.Project = project
	var started string
	var ended sql.NullString
	var incomplete int
	err = s.db.QueryRowContext(ctx, `SELECT id,project_id,started_at,ended_at,incomplete FROM traces WHERE project_id=? AND id=?`, projectID, traceID).Scan(&investigation.Trace.ID, &investigation.Trace.ProjectID, &started, &ended, &incomplete)
	if err != nil {
		return investigation, err
	}
	investigation.Trace.StartedAt, err = time.Parse(time.RFC3339Nano, started)
	if err != nil {
		return investigation, err
	}
	if ended.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, ended.String)
		if parseErr != nil {
			return investigation, parseErr
		}
		investigation.Trace.EndedAt = &value
	}
	investigation.Trace.Incomplete = incomplete != 0
	rows, err := s.db.QueryContext(ctx, `SELECT value_json FROM trace_events WHERE project_id=? AND trace_id=? ORDER BY producer_id,sequence,event_id`, projectID, traceID)
	if err != nil {
		return investigation, err
	}
	for rows.Next() {
		var value string
		var event domain.Event
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return investigation, err
		}
		if err := json.Unmarshal([]byte(value), &event); err != nil {
			rows.Close()
			return investigation, err
		}
		investigation.Events = append(investigation.Events, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return investigation, err
	}
	rows.Close()
	diagRows, err := s.db.QueryContext(ctx, `SELECT value_json FROM trace_diagnostics WHERE project_id=? AND trace_id=? ORDER BY id`, projectID, traceID)
	if err != nil {
		return investigation, err
	}
	defer diagRows.Close()
	for diagRows.Next() {
		var value string
		var diagnostic domain.Diagnostic
		if err := diagRows.Scan(&value); err != nil {
			return investigation, err
		}
		if err := json.Unmarshal([]byte(value), &diagnostic); err != nil {
			return investigation, err
		}
		investigation.Diagnostics = append(investigation.Diagnostics, diagnostic)
	}
	if err := diagRows.Err(); err != nil {
		return investigation, err
	}
	if err := diagRows.Close(); err != nil {
		return investigation, err
	}
	return investigation, nil
}

func (s *Store) LoadInvestigationIndex(ctx context.Context, projectID domain.ID) (application.InvestigationIndex, error) {
	var snapshot application.InvestigationIndex
	// The indexed graph is a separate static layer. Use one revision as a unit;
	// its relation evidence never becomes proof that the trace executed it.
	err := s.db.QueryRowContext(ctx, `SELECT id FROM revisions WHERE project_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, projectID).Scan(&snapshot.RevisionID)
	if errors.Is(err, sql.ErrNoRows) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	collections := []struct {
		table       string
		appendValue func(string) error
	}{
		{"symbols", func(value string) error {
			var item domain.Symbol
			if err := json.Unmarshal([]byte(value), &item); err != nil {
				return err
			}
			snapshot.Symbols = append(snapshot.Symbols, item)
			return nil
		}},
		{"relations", func(value string) error {
			var item domain.Relation
			if err := json.Unmarshal([]byte(value), &item); err != nil {
				return err
			}
			snapshot.Relations = append(snapshot.Relations, item)
			return nil
		}},
		{"evidence", func(value string) error {
			var item domain.Evidence
			if err := json.Unmarshal([]byte(value), &item); err != nil {
				return err
			}
			snapshot.Evidence = append(snapshot.Evidence, item)
			return nil
		}},
		{"diagnostics", func(value string) error {
			var item domain.Diagnostic
			if err := json.Unmarshal([]byte(value), &item); err != nil {
				return err
			}
			snapshot.Diagnostics = append(snapshot.Diagnostics, item)
			return nil
		}},
	}
	for _, collection := range collections {
		rows, err := s.db.QueryContext(ctx, `SELECT value_json FROM `+collection.table+` WHERE project_id=? AND revision_id=? ORDER BY id`, projectID, snapshot.RevisionID)
		if err != nil {
			return snapshot, err
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return snapshot, err
			}
			if err := collection.appendValue(value); err != nil {
				rows.Close()
				return snapshot, err
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return snapshot, err
		}
		rows.Close()
	}
	return snapshot, nil
}

func loadTraceEvents(ctx context.Context, tx *sql.Tx, projectID, traceID domain.ID) ([]domain.Event, error) {
	rows, err := tx.QueryContext(ctx, `SELECT value_json FROM trace_events WHERE project_id=? AND trace_id=?`, projectID, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.Event
	for rows.Next() {
		var value string
		var event domain.Event
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(value), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func refreshGapDiagnostics(ctx context.Context, tx *sql.Tx, projectID, traceID domain.ID, events []domain.Event) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM trace_diagnostics WHERE project_id=? AND trace_id=? AND (value_json LIKE '%capture.sequence_gap%' OR value_json LIKE '%capture.parent_missing%')`, projectID, traceID); err != nil {
		return err
	}
	sequences := map[domain.ID]map[uint64]bool{}
	ids := map[domain.ID]map[domain.ID]bool{}
	var at time.Time
	for _, event := range events {
		if sequences[event.ProducerID] == nil {
			sequences[event.ProducerID] = map[uint64]bool{}
			ids[event.ProducerID] = map[domain.ID]bool{}
		}
		sequences[event.ProducerID][event.Sequence] = true
		ids[event.ProducerID][event.EventID] = true
		if event.OccurredAt.After(at) {
			at = event.OccurredAt
		}
	}
	for _, values := range sequences {
		var max uint64
		for n := range values {
			if n > max {
				max = n
			}
		}
		if uint64(len(values)) != max {
			diagnostic := domain.Diagnostic{Code: "capture.sequence_gap", Message: "Capture event sequence contains a gap.", Severity: "warning", CreatedAt: at}
			if err := insertTraceDiagnostic(ctx, tx, projectID, traceID, diagnostic); err != nil {
				return err
			}
		}
	}
	for _, event := range events {
		if event.ParentEventID != nil && !ids[event.ProducerID][*event.ParentEventID] {
			diagnostic := domain.Diagnostic{Code: "capture.parent_missing", Message: "A parent event has not arrived yet.", Severity: "warning", CreatedAt: at}
			if err := insertTraceDiagnostic(ctx, tx, projectID, traceID, diagnostic); err != nil {
				return err
			}
		}
	}
	return nil
}
