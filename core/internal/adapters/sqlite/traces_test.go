package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

func TestCaptureServicePersistsSanitizationDiagnostics(t *testing.T) {
	path := t.TempDir() + "/sanitized-capture.db"
	ctx := context.Background()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProject(ctx, domain.Project{ID: "p", Name: "p", Root: t.TempDir(), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	service := application.NewCaptureService(store, application.CaptureConfig{Now: func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }})
	session, err := service.Start(ctx, application.CaptureRequest{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	secrets := []string{"url-token-secret", "cookie-secret", "exception-secret", "sql-secret"}
	event := domain.Event{ProjectID: "p", TraceID: session.ID, ProducerID: "agent", Sequence: 1, EventID: "event-1", OccurredAt: session.StartedAt.Add(time.Second), Kind: "http.request", Metadata: map[string]string{
		"url":       "https://example.test/orders?token=" + secrets[0],
		"cookie":    "session=" + secrets[1],
		"exception": "request failed: " + secrets[2],
		"sql":       "SELECT * FROM orders WHERE id = '" + secrets[3] + "'",
	}}
	result, err := service.Ingest(ctx, []domain.Event{event})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) == 0 {
		t.Fatal("ingest should report removed metadata")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.Load(ctx, "p", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, diagnostic := range loaded.Diagnostics {
		if diagnostic.Code == "capture.metadata_removed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sanitized metadata-removal diagnostic was not persisted: %#v", loaded.Diagnostics)
	}
	encoded, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range secrets {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("capture or diagnostic retained sensitive value %q: %s", secret, encoded)
		}
	}
}

func TestTraceCapturePersistsLifecycleEventsAndDiagnostics(t *testing.T) {
	path := t.TempDir() + "/traces.db"
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.SaveProject(ctx, domain.Project{ID: "p", Name: "p", Root: t.TempDir(), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	index := application.IndexResult{ProjectID: "p", RevisionID: "r1", Artifacts: []domain.Artifact{{ID: "a1", ProjectID: "p", RevisionID: "r1", Path: "orders.java", Language: "java", Origin: "filesystem", ContentHash: "hash"}}, Symbols: []domain.Symbol{{ID: "s1", ProjectID: "p", RevisionID: "r1", ArtifactID: "a1", Path: "orders.java", QualifiedName: "OrderService", Kind: "class"}}}
	if err := store.CommitIndex(ctx, index); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	trace := domain.Trace{ID: "t", ProjectID: "p", StartedAt: started}
	if err := store.StartCapture(ctx, trace, "tab"); err != nil {
		t.Fatal(err)
	}
	event := domain.Event{ProjectID: "p", TraceID: "t", ProducerID: "agent", Sequence: 2, EventID: "e", OccurredAt: started.Add(time.Second), Kind: "http.request", Metadata: map[string]string{"http.method": "GET", "cookie": "secret"}}
	result, err := store.Append(ctx, []domain.Event{event})
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 1 || len(result.Diagnostics) == 0 {
		t.Fatalf("expected accepted event plus gap/sanitization diagnostics: %#v", result)
	}
	duplicate, err := store.Append(ctx, []domain.Event{event})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Accepted != 0 || duplicate.Duplicate != 1 {
		t.Fatalf("persistent event identity is not idempotent: %#v", duplicate)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	search, err := store.Search(ctx, application.SearchQuery{ProjectID: "p", RevisionID: "r1", Text: "OrderService"})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Symbols) != 1 || search.Symbols[0].ID != "s1" {
		t.Fatalf("index data did not survive migration reopen: %#v", search)
	}
	loaded, err := store.Load(ctx, "p", "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 1 || loaded.Events[0].Metadata["cookie"] != "" {
		t.Fatalf("event not safely persisted: %#v", loaded.Events)
	}
	if len(loaded.Diagnostics) == 0 {
		t.Fatal("diagnostics were not persisted")
	}
	var persisted string
	if err := store.db.QueryRow(`SELECT value_json FROM trace_events WHERE trace_id=?`, "t").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(persisted, "secret") || strings.Contains(persisted, "cookie") {
		t.Fatalf("sensitive metadata reached SQLite: %s", persisted)
	}
	if loaded.Trace.EndedAt != nil || loaded.Trace.Incomplete {
		t.Fatalf("unexpected started state: %#v", loaded.Trace)
	}
	stopped := started.Add(2 * time.Minute)
	if err := store.StopCapture(ctx, "p", "t", stopped); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load(ctx, "p", "t")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Trace.EndedAt == nil || !loaded.Trace.EndedAt.Equal(stopped) {
		t.Fatalf("stop state not persisted: %#v", loaded.Trace)
	}
	limitTrace := domain.Trace{ID: "t-limit", ProjectID: "p", StartedAt: started}
	if err := store.StartCapture(ctx, limitTrace, ""); err != nil {
		t.Fatal(err)
	}
	limitEvent := domain.Event{ProjectID: "p", TraceID: "t-limit", ProducerID: "agent", Sequence: 1, EventID: "kept", OccurredAt: started.Add(time.Second), Kind: "http.request"}
	if _, err := store.Append(ctx, []domain.Event{limitEvent}); err != nil {
		t.Fatal(err)
	}
	limitDiagnostic := domain.Diagnostic{Code: "capture.event_limit", Message: "Capture event limit reached; accepted events were preserved.", Severity: "warning", CreatedAt: stopped}
	if err := store.MarkCaptureIncomplete(ctx, "p", "t-limit", limitDiagnostic); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load(ctx, "p", "t-limit")
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Trace.Incomplete || len(loaded.Events) != 1 || len(loaded.Diagnostics) == 0 {
		t.Fatalf("incomplete capture state did not survive reload: %#v", loaded)
	}
}
