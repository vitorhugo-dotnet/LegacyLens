package application

import (
	"context"
	"reflect"
	"testing"
	"time"

	"legacylens/core/internal/domain"
)

type investigationStore struct{ value Investigation }

func (s investigationStore) Load(context.Context, domain.ID, domain.ID) (Investigation, error) {
	return s.value, nil
}
func (s investigationStore) Append(context.Context, []domain.Event) (IngestResult, error) {
	return IngestResult{}, nil
}

func TestInvestigationSeparatesLayers(t *testing.T) {
	project, trace := domain.ID("project"), domain.ID("0123456789abcdef0123456789abcdef")
	parent := domain.ID("click")
	static := domain.Relation{ID: "static-edge", FromID: "xhtml", Kind: "navigation", Layer: domain.LayerStatic, Resolution: domain.ResolutionDynamic, EvidenceIDs: []domain.ID{"source"}}
	events := []domain.Event{
		{ProjectID: project, TraceID: trace, ProducerID: "browser", Sequence: 1, EventID: parent, Kind: "jsf.click", OccurredAt: time.Now()},
		{ProjectID: project, TraceID: trace, ProducerID: "browser", Sequence: 3, EventID: "ajax", ParentEventID: &parent, Kind: "primefaces.ajax", OccurredAt: time.Now()},
	}
	got, err := NewInvestigationService(investigationStore{Investigation{Trace: domain.Trace{ID: trace, ProjectID: project}, Relations: []domain.Relation{static}, Events: events}}).Get(context.Background(), project, trace)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Relations[0], static) {
		t.Fatalf("static relation changed: %+v", got.Relations[0])
	}
	observed := 0
	for _, relation := range got.Relations {
		if relation.Layer == domain.LayerObserved {
			observed++
		}
	}
	if observed != 1 {
		t.Fatalf("observed relations = %d, want 1", observed)
	}
	if len(got.Symbols) != 2 {
		t.Fatalf("observed symbols = %d, want 2", len(got.Symbols))
	}
	if got.Symbols[0].Location != nil {
		t.Fatal("event-only symbol acquired an unverified source location")
	}
	if !got.Trace.Incomplete {
		t.Fatal("sequence gap did not mark trace incomplete")
	}
	found := false
	for _, diagnostic := range got.Diagnostics {
		if diagnostic.Code == "capture.sequence_gap" {
			found = true
		}
	}
	if !found {
		t.Fatal("sequence gap diagnostic missing")
	}
}

func TestInvestigationPreservesProducerIdentityAndMissingParent(t *testing.T) {
	project, trace := domain.ID("p"), domain.ID("0123456789abcdef0123456789abcdef")
	missing := domain.ID("not-loaded")
	ambiguous := domain.ID("same")
	events := []domain.Event{
		{ProjectID: project, TraceID: trace, ProducerID: "epoch-a", Sequence: 1, EventID: "same", Kind: "jsf.click"},
		{ProjectID: project, TraceID: trace, ProducerID: "epoch-b", Sequence: 1, EventID: "same", Kind: "jsf.click"},
		{ProjectID: project, TraceID: trace, ProducerID: "epoch-b", Sequence: 2, EventID: "child", ParentEventID: &missing, Kind: "browser.network"},
		{ProjectID: project, TraceID: trace, ProducerID: "epoch-b", Sequence: 3, EventID: "ambiguous-child", ParentEventID: &ambiguous, Kind: "browser.network"},
	}
	got, err := NewInvestigationService(investigationStore{Investigation{Trace: domain.Trace{ID: trace, ProjectID: project}, Events: events}}).Get(context.Background(), project, trace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Symbols) != 4 || got.Symbols[0].ID == got.Symbols[1].ID {
		t.Fatal("producer epochs collapsed")
	}
	if len(got.Relations) != 2 {
		t.Fatalf("parent relations = %+v", got.Relations)
	}
	for i, kind := range []string{"event.parent_missing", "event.parent_ambiguous"} {
		relation := got.Relations[i]
		if relation.Resolution != domain.ResolutionUnresolved || relation.Kind != kind || relation.ToID == nil || *relation.ToID != got.Symbols[i+2].ID || relation.FromID == *relation.ToID {
			t.Fatalf("unresolved parent relation points in the wrong direction: %+v", relation)
		}
	}
	if !got.Trace.Incomplete {
		t.Fatal("missing parent was treated as complete")
	}
}

func TestInvestigationMarksReconnectGap(t *testing.T) {
	project, trace := domain.ID("p"), domain.ID("0123456789abcdef0123456789abcdef")
	gap := domain.Event{ProjectID: project, TraceID: trace, ProducerID: "browser", Sequence: 1, EventID: "gap", Kind: "extension.gap", Metadata: map[string]string{"code": "CAPTURE_RECONNECTED"}}
	got, err := NewInvestigationService(investigationStore{Investigation{Trace: domain.Trace{ID: trace, ProjectID: project}, Events: []domain.Event{gap}}}).Get(context.Background(), project, trace)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Trace.Incomplete {
		t.Fatal("reconnect gap presented as a complete capture")
	}
	found := false
	for _, diagnostic := range got.Diagnostics {
		if diagnostic.Code == "capture.reconnected_gap" {
			found = true
		}
	}
	if !found {
		t.Fatal("reconnect diagnostic absent")
	}
}

func TestInvestigationMarksAgentLoss(t *testing.T) {
	project, trace := domain.ID("p"), domain.ID("0123456789abcdef0123456789abcdef")
	loss := domain.Event{ProjectID: project, TraceID: trace, ProducerID: "agent", Sequence: 1, EventID: "loss", Kind: "agent.loss", Metadata: map[string]string{"agent.dropped_count": "2"}}
	got, err := NewInvestigationService(investigationStore{Investigation{Trace: domain.Trace{ID: trace, ProjectID: project}, Events: []domain.Event{loss}}}).Get(context.Background(), project, trace)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Trace.Incomplete {
		t.Fatal("agent event loss presented as complete")
	}
}
