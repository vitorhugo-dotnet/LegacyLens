package application

import (
	"context"
	"reflect"
	"testing"

	"legacylens/core/internal/domain"
)

type reconciliationStore struct {
	investigation Investigation
	index         InvestigationIndex
}

func (s reconciliationStore) Load(context.Context, domain.ID, domain.ID) (Investigation, error) {
	return s.investigation, nil
}

func (s reconciliationStore) Append(context.Context, []domain.Event) (IngestResult, error) {
	return IngestResult{}, nil
}

func (s reconciliationStore) LoadInvestigationIndex(context.Context, domain.ID) (InvestigationIndex, error) {
	return s.index, nil
}

func TestReconciliationLinksOnlyUniqueRevisionQualifiedMethodIdentity(t *testing.T) {
	project, trace := domain.ID("project"), domain.ID("trace")
	staticRelation := domain.Relation{ID: "static-call", FromID: "caller", Kind: "calls", ToID: idPtr("method"), EvidenceIDs: []domain.ID{"static-source"}, Resolution: domain.ResolutionResolved, Layer: domain.LayerStatic}
	revision := domain.ID("revision-1")
	event := domain.Event{ProjectID: project, TraceID: trace, ProducerID: "agent-epoch-1", Sequence: 1, EventID: "event-1", Kind: "method.start", ApplicationRevision: &revision, Metadata: map[string]string{"code.class": "com.example.OrderService", "code.method": "load", "code.descriptor": "(Ljava/lang/String;)V"}}
	store := reconciliationStore{
		investigation: Investigation{Trace: domain.Trace{ID: trace, ProjectID: project}, Events: []domain.Event{event}},
		index: InvestigationIndex{
			RevisionID: "revision-1",
			Symbols: []domain.Symbol{
				{ID: "method", ProjectID: project, RevisionID: "revision-1", QualifiedName: "com.example.OrderService.load", Descriptor: "(Ljava/lang/String;)V", Kind: "method"},
				{ID: "other-revision", ProjectID: project, RevisionID: "revision-2", QualifiedName: "com.example.OrderService.load", Descriptor: "(Ljava/lang/String;)V", Kind: "method"},
				{ID: "other-descriptor", ProjectID: project, RevisionID: "revision-1", QualifiedName: "com.example.OrderService.load", Descriptor: "()V", Kind: "method"},
			},
			Relations: []domain.Relation{staticRelation},
			Evidence:  []domain.Evidence{{ID: "static-source", Kind: "source", Source: "OrderService.java"}},
		},
	}

	got, err := NewReconciler(store).Reconcile(context.Background(), project, trace)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Relations[0], staticRelation) {
		t.Fatalf("static evidence relation changed: %+v", got.Relations[0])
	}
	var links []domain.Relation
	for _, relation := range got.Relations {
		if relation.Kind == "observed.matches_static" {
			links = append(links, relation)
		}
	}
	if len(links) != 1 || links[0].FromID == "" || links[0].ToID == nil || *links[0].ToID != "method" || links[0].Layer != domain.LayerObserved || links[0].Resolution != domain.ResolutionResolved {
		t.Fatalf("verified observed/static link missing or incorrect: %+v", links)
	}
	if len(got.Evidence) != 2 || got.Evidence[0].Kind != "source" || got.Evidence[1].Kind != "observed-event" {
		t.Fatalf("evidence types or count changed: %+v", got.Evidence)
	}
	served, err := NewInvestigationService(store).Get(context.Background(), project, trace)
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range served.Relations {
		if relation.Kind == "observed.matches_static" {
			return
		}
	}
	t.Fatal("investigation service did not expose verified observed/static relation")
}

func TestReconciliationLeavesAmbiguousAndUnversionedMethodsUnlinked(t *testing.T) {
	project, trace := domain.ID("p"), domain.ID("t")
	revision := domain.ID("rev")
	events := []domain.Event{
		{ProjectID: project, TraceID: trace, ProducerID: "agent", Sequence: 1, EventID: "e1", Kind: "method.start", ApplicationRevision: &revision, Metadata: map[string]string{"code.class": "pkg.Worker", "code.method": "run", "code.descriptor": "()V"}},
		{ProjectID: project, TraceID: trace, ProducerID: "agent", Sequence: 2, EventID: "e2", Kind: "method.start", Metadata: map[string]string{"code.class": "pkg.Other", "code.method": "run", "code.descriptor": "()V"}},
	}
	store := reconciliationStore{
		investigation: Investigation{Trace: domain.Trace{ID: trace, ProjectID: project}, Events: events},
		index: InvestigationIndex{Symbols: []domain.Symbol{
			{ID: "candidate-a", ProjectID: project, RevisionID: "rev", QualifiedName: "pkg.Worker.run", Descriptor: "()V", Kind: "method"},
			{ID: "candidate-b", ProjectID: project, RevisionID: "rev", QualifiedName: "pkg.Worker.run", Descriptor: "()V", Kind: "method"},
			{ID: "candidate-c", ProjectID: project, RevisionID: "rev", QualifiedName: "pkg.Other.run", Descriptor: "()V", Kind: "method"},
		}},
	}

	got, err := NewReconciler(store).Reconcile(context.Background(), project, trace)
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range got.Relations {
		if relation.Kind == "observed.matches_static" {
			t.Fatalf("ambiguous or unversioned identity was linked: %+v", relation)
		}
	}
}

func TestReconciliationPreservesDuplicateAndOutOfOrderEventsWithinProducerEpochs(t *testing.T) {
	project, trace := domain.ID("p"), domain.ID("t")
	parent := domain.ID("parent")
	events := []domain.Event{
		{ProjectID: project, TraceID: trace, ProducerID: "epoch-a", Sequence: 3, EventID: "child", ParentEventID: &parent, Kind: "browser.network"},
		{ProjectID: project, TraceID: trace, ProducerID: "epoch-a", Sequence: 1, EventID: parent, Kind: "jsf.click"},
		{ProjectID: project, TraceID: trace, ProducerID: "epoch-b", Sequence: 1, EventID: parent, Kind: "jsf.click"},
	}
	events = append(events, events[0])
	store := reconciliationStore{investigation: Investigation{Trace: domain.Trace{ID: trace, ProjectID: project}, Events: events}}
	got, err := NewReconciler(store).Reconcile(context.Background(), project, trace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 4 {
		t.Fatalf("input event order/page contents unexpectedly changed: %d events", len(got.Events))
	}
	if len(got.Symbols) != 3 {
		t.Fatalf("duplicate full event identity was materialized: %d observed symbols", len(got.Symbols))
	}
	if !got.Trace.Incomplete {
		t.Fatal("sequence/parent gaps were lost")
	}
	foundAmbiguous := false
	for _, relation := range got.Relations {
		if relation.Kind == "event.parent_ambiguous" {
			foundAmbiguous = true
		}
	}
	if !foundAmbiguous {
		t.Fatal("cross-producer parent ambiguity was treated as a total order")
	}
}

func idPtr(value domain.ID) *domain.ID { return &value }
