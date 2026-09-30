package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"legacylens/core/internal/domain"
)

type memoryCaptureStore struct {
	projects map[domain.ID]domain.Project
	traces   map[domain.ID]domain.Trace
	events   map[domain.ID][]domain.Event
}

func (s *memoryCaptureStore) LoadProject(_ context.Context, id domain.ID) (domain.Project, error) {
	return s.projects[id], nil
}
func (s *memoryCaptureStore) StartCapture(_ context.Context, trace domain.Trace, _ string) error {
	s.traces[trace.ID] = trace
	return nil
}
func (s *memoryCaptureStore) StopCapture(_ context.Context, projectID, traceID domain.ID, at time.Time) error {
	trace := s.traces[traceID]
	if trace.ProjectID != projectID {
		return nil
	}
	trace.EndedAt = &at
	s.traces[traceID] = trace
	return nil
}
func (s *memoryCaptureStore) MarkCaptureIncomplete(_ context.Context, projectID, traceID domain.ID, _ domain.Diagnostic) error {
	trace := s.traces[traceID]
	if trace.ProjectID == projectID {
		trace.Incomplete = true
		s.traces[traceID] = trace
	}
	return nil
}
func (s *memoryCaptureStore) Append(_ context.Context, events []domain.Event) (IngestResult, error) {
	result := IngestResult{}
	for _, event := range events {
		for _, old := range s.events[event.TraceID] {
			if old.ProjectID == event.ProjectID && old.ProducerID == event.ProducerID && old.EventID == event.EventID {
				result.Duplicate++
				goto next
			}
		}
		s.events[event.TraceID] = append(s.events[event.TraceID], event)
		result.Accepted++
	next:
	}
	return result, nil
}
func (s *memoryCaptureStore) AppendBounded(ctx context.Context, events []domain.Event, _ int, diagnostics []domain.Diagnostic) (IngestResult, error) {
	result, err := s.Append(ctx, events)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	return result, err
}
func (s *memoryCaptureStore) Load(_ context.Context, projectID, traceID domain.ID) (Investigation, error) {
	return Investigation{Trace: s.traces[traceID], Events: s.events[traceID]}, nil
}

func TestIngestIdempotentAndOutOfOrder(t *testing.T) {
	store := &memoryCaptureStore{projects: map[domain.ID]domain.Project{"p": {ID: "p"}}, traces: map[domain.ID]domain.Trace{}, events: map[domain.ID][]domain.Event{}}
	service := NewCaptureService(store, CaptureConfig{Now: func() time.Time { return time.Unix(100, 0).UTC() }})
	session, err := service.Start(context.Background(), CaptureRequest{ProjectID: "p", TabID: "tab"})
	if err != nil {
		t.Fatal(err)
	}
	parent := domain.ID("parent")
	child := domain.Event{ProjectID: "p", TraceID: session.ID, ProducerID: "agent", Sequence: 2, EventID: "child", ParentEventID: &parent, OccurredAt: time.Unix(102, 0), Kind: "http.request"}
	dup := child
	sameEventIDFromOtherProducer := domain.Event{ProjectID: "p", TraceID: session.ID, ProducerID: "other-agent", Sequence: 1, EventID: "child", OccurredAt: time.Unix(102, 0), Kind: "http.request"}
	result, err := service.Ingest(context.Background(), []domain.Event{child, dup, sameEventIDFromOtherProducer})
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 2 || result.Duplicate != 1 || len(result.Diagnostics) == 0 {
		t.Fatalf("child before parent should be accepted once with a gap diagnostic: %#v", result)
	}
	parentEvent := domain.Event{ProjectID: "p", TraceID: session.ID, ProducerID: "agent", Sequence: 1, EventID: parent, OccurredAt: time.Unix(101, 0), Kind: "http.request"}
	result, err = service.Ingest(context.Background(), []domain.Event{parentEvent})
	if err != nil || result.Accepted != 1 {
		t.Fatalf("parent ingest: result=%#v err=%v", result, err)
	}
	if store.events[session.ID][0].ParentEventID == nil || *store.events[session.ID][0].ParentEventID != "parent" {
		t.Fatal("out-of-order event lost its parent reference")
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "capture.sequence_gap" || diagnostic.Code == "capture.parent_missing" {
			t.Fatalf("filled causal and sequence gaps should no longer be reported: %#v", result.Diagnostics)
		}
	}
}

func TestCaptureServiceLoadsInvestigationForProjectAndTrace(t *testing.T) {
	store := &memoryCaptureStore{projects: map[domain.ID]domain.Project{"p": {ID: "p"}}, traces: map[domain.ID]domain.Trace{"t": {ID: "t", ProjectID: "p"}}, events: map[domain.ID][]domain.Event{"t": {{ProjectID: "p", TraceID: "t", EventID: "e"}}}}
	service := NewCaptureService(store, CaptureConfig{})
	got, err := service.Investigation(context.Background(), "p", "t")
	if err != nil || got.Trace.ID != "t" || got.Trace.ProjectID != "p" || len(got.Events) != 1 {
		t.Fatalf("Investigation() = %+v, %v", got, err)
	}
}

func TestCaptureLimitMarksIncomplete(t *testing.T) {
	store := &memoryCaptureStore{projects: map[domain.ID]domain.Project{"p": {ID: "p"}}, traces: map[domain.ID]domain.Trace{}, events: map[domain.ID][]domain.Event{}}
	service := NewCaptureService(store, CaptureConfig{Now: func() time.Time { return time.Unix(100, 0).UTC() }})
	session, err := service.Start(context.Background(), CaptureRequest{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]domain.Event, 10_001)
	for i := range events {
		sequence := uint64(i + 1)
		events[i] = domain.Event{ProjectID: "p", TraceID: session.ID, ProducerID: "agent", Sequence: sequence, EventID: domain.ID(fmt.Sprintf("e%d", sequence)), OccurredAt: time.Unix(int64(101+i), 0), Kind: "http.request"}
	}
	result, err := service.Ingest(context.Background(), events)
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 10_000 || len(result.Diagnostics) == 0 {
		t.Fatalf("limit must preserve accepted events and return a diagnostic: %#v", result)
	}
	if !store.traces[session.ID].Incomplete {
		t.Fatal("limit must persist incomplete session state")
	}
}

func TestCaptureConfiguredLimit(t *testing.T) {
	store := &memoryCaptureStore{projects: map[domain.ID]domain.Project{"p": {ID: "p"}}, traces: map[domain.ID]domain.Trace{}, events: map[domain.ID][]domain.Event{}}
	service := NewCaptureService(store, CaptureConfig{MaxEvents: 1, Now: func() time.Time { return time.Unix(100, 0).UTC() }})
	session, err := service.Start(context.Background(), CaptureRequest{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	events := []domain.Event{
		{ProjectID: "p", TraceID: session.ID, ProducerID: "agent", Sequence: 1, EventID: "one", OccurredAt: time.Unix(101, 0), Kind: "http.request"},
		{ProjectID: "p", TraceID: session.ID, ProducerID: "agent", Sequence: 2, EventID: "two", OccurredAt: time.Unix(102, 0), Kind: "http.request"},
	}
	result, err := service.Ingest(context.Background(), events)
	if err != nil || result.Accepted != 1 || len(result.Diagnostics) == 0 {
		t.Fatalf("configured limit was not applied: %#v, %v", result, err)
	}
}

func TestCaptureExpiryAndStopAreIsolated(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	store := &memoryCaptureStore{projects: map[domain.ID]domain.Project{"p": {ID: "p"}, "other": {ID: "other"}}, traces: map[domain.ID]domain.Trace{}, events: map[domain.ID][]domain.Event{}}
	service := NewCaptureService(store, CaptureConfig{MaxDuration: time.Minute, Now: func() time.Time { return now }})
	first, err := service.Start(context.Background(), CaptureRequest{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Start(context.Background(), CaptureRequest{ProjectID: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Stop(context.Background(), "other", first.ID); err == nil {
		t.Fatal("stopping another project's trace must fail")
	}
	if store.traces[second.ID].EndedAt != nil {
		t.Fatal("stopping one capture must not stop another")
	}
	now = now.Add(2 * time.Minute)
	event := domain.Event{ProjectID: "p", TraceID: first.ID, ProducerID: "agent", Sequence: 1, EventID: "expired", OccurredAt: now, Kind: "http.request"}
	result, err := service.Ingest(context.Background(), []domain.Event{event})
	if err != nil || result.Accepted != 0 || len(result.Diagnostics) == 0 {
		t.Fatalf("expired capture accepted data: %#v, %v", result, err)
	}
	if len(store.events[second.ID]) != 0 {
		t.Fatal("events crossed capture sessions")
	}
}
