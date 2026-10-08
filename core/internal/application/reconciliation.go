package application

import (
	"context"
	"errors"

	"legacylens/core/internal/domain"
)

// Reconciler adds links only when an observed method identity uniquely matches
// a method in the investigation's exact indexed revision.
type Reconciler struct {
	traces TraceStore
}

func NewReconciler(traces TraceStore) *Reconciler {
	return &Reconciler{traces: traces}
}

func (r *Reconciler) Reconcile(ctx context.Context, projectID, traceID domain.ID) (Investigation, error) {
	if r == nil || r.traces == nil {
		return Investigation{}, errors.New("trace store is required")
	}
	return NewInvestigationService(r.traces).Get(ctx, projectID, traceID)
}

func reconcileInvestigation(ctx context.Context, projectID, traceID domain.ID, value Investigation) (Investigation, error) {
	if err := ctx.Err(); err != nil {
		return Investigation{}, err
	}
	if value.IndexedRevisionID == "" {
		return value, nil
	}

	methods := make(map[string][]domain.Symbol)
	for _, symbol := range value.Symbols {
		if symbol.ID == "" || symbol.ProjectID != projectID || symbol.RevisionID != value.IndexedRevisionID || symbol.Kind != "method" || symbol.Descriptor == "" {
			continue
		}
		key := string(symbol.RevisionID) + "\x00" + symbol.QualifiedName + "\x00" + symbol.Descriptor
		methods[key] = append(methods[key], symbol)
	}

	existing := make(map[domain.ID]struct{}, len(value.Relations))
	for _, relation := range value.Relations {
		existing[relation.ID] = struct{}{}
	}
	seenEvents := make(map[string]struct{}, len(value.Events))
	for _, event := range value.Events {
		if event.ProjectID != projectID || event.TraceID != traceID || event.ApplicationRevision == nil || *event.ApplicationRevision != value.IndexedRevisionID {
			continue
		}
		class, method, descriptor := event.Metadata["code.class"], event.Metadata["code.method"], event.Metadata["code.descriptor"]
		if class == "" || method == "" || descriptor == "" {
			continue
		}
		identity := string(event.ProducerID) + "\x00" + string(event.EventID)
		if _, duplicate := seenEvents[identity]; duplicate {
			continue
		}
		seenEvents[identity] = struct{}{}

		key := string(value.IndexedRevisionID) + "\x00" + class + "." + method + "\x00" + descriptor
		matches := methods[key]
		if len(matches) != 1 {
			continue
		}
		observedID := stableID(string(projectID), string(traceID), string(event.ProducerID), string(event.EventID))
		linkID := stableID("observed-static", string(projectID), string(traceID), string(event.ProducerID), string(event.EventID), string(matches[0].ID))
		if _, duplicate := existing[linkID]; duplicate {
			continue
		}
		to := matches[0].ID
		value.Relations = append(value.Relations, domain.Relation{
			ID:          linkID,
			FromID:      observedID,
			ToID:        &to,
			Kind:        "observed.matches_static",
			EvidenceIDs: []domain.ID{observedID},
			Resolution:  domain.ResolutionResolved,
			Layer:       domain.LayerObserved,
		})
		existing[linkID] = struct{}{}
	}
	return value, nil
}
