package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"legacylens/core/internal/domain"
)

func (s *CaptureService) Ingest(ctx context.Context, events []domain.Event) (IngestResult, error) {
	if s == nil || s.store == nil {
		return IngestResult{}, errors.New("capture store is required")
	}
	if len(events) == 0 {
		return IngestResult{}, nil
	}
	groups := make(map[domain.ID][]domain.Event)
	for _, event := range events {
		if event.ProjectID == "" || event.TraceID == "" || event.ProducerID == "" || event.EventID == "" || event.Sequence == 0 {
			return IngestResult{}, errors.New("event identity and sequence are required")
		}
		groups[event.TraceID] = append(groups[event.TraceID], event)
	}
	traceIDs := make([]string, 0, len(groups))
	for id := range groups {
		traceIDs = append(traceIDs, string(id))
	}
	sort.Strings(traceIDs)
	total := IngestResult{}
	for _, rawID := range traceIDs {
		traceID := domain.ID(rawID)
		batch := groups[traceID]
		projectID := batch[0].ProjectID
		for _, event := range batch {
			if event.ProjectID != projectID {
				return IngestResult{}, errors.New("one trace cannot contain multiple projects")
			}
		}
		current, err := s.store.Load(ctx, projectID, traceID)
		if err != nil {
			return IngestResult{}, err
		}
		if current.Trace.ID != traceID || current.Trace.ProjectID != projectID {
			return IngestResult{}, errors.New("capture session not found")
		}
		if current.Trace.EndedAt != nil || current.Trace.Incomplete {
			total.Diagnostics = append(total.Diagnostics, makeDiagnostic("capture.closed", "Capture session is closed.", s.config.Now()))
			continue
		}
		now := s.config.Now().UTC()
		if !now.Before(current.Trace.StartedAt.Add(s.config.MaxDuration)) {
			diagnostic := makeDiagnostic("capture.expired", "Capture session expired before these events arrived.", now)
			if err := s.store.MarkCaptureIncomplete(ctx, projectID, traceID, diagnostic); err != nil {
				return total, err
			}
			total.Diagnostics = append(total.Diagnostics, diagnostic)
			continue
		}
		seen := make(map[eventIdentityKey]bool, len(current.Events)+len(batch))
		for _, old := range current.Events {
			seen[eventIdentity(old)] = true
		}
		remaining := s.config.MaxEvents - len(current.Events)
		acceptedBatch := make([]domain.Event, 0, len(batch))
		batchDuplicates := 0
		limitReached := false
		for _, event := range batch {
			if seen[eventIdentity(event)] {
				batchDuplicates++
				continue
			}
			seen[eventIdentity(event)] = true
			if remaining <= 0 {
				limitReached = true
				continue
			}
			metadata, diagnostics := domain.SanitizeMetadata(event.Metadata)
			event.Metadata = metadata
			for i := range diagnostics {
				diagnostics[i].CreatedAt = now
			}
			total.Diagnostics = append(total.Diagnostics, diagnostics...)
			acceptedBatch = append(acceptedBatch, event)
			remaining--
		}
		result, err := s.store.AppendBounded(ctx, acceptedBatch, s.config.MaxEvents)
		if err != nil {
			return total, err
		}
		total.Accepted += result.Accepted
		total.Duplicate += result.Duplicate + batchDuplicates
		total.Diagnostics = append(total.Diagnostics, result.Diagnostics...)
		after, err := s.store.Load(ctx, projectID, traceID)
		if err != nil {
			return total, err
		}
		total.Diagnostics = append(total.Diagnostics, eventGapDiagnostics(after.Events, now)...)
		if limitReached {
			diagnostic := makeDiagnostic("capture.event_limit", "Capture event limit reached; accepted events were preserved.", now)
			if err := s.store.MarkCaptureIncomplete(ctx, projectID, traceID, diagnostic); err != nil {
				return total, err
			}
			total.Diagnostics = append(total.Diagnostics, diagnostic)
		}
	}
	return total, nil
}

type eventIdentityKey struct {
	projectID  domain.ID
	traceID    domain.ID
	producerID domain.ID
	eventID    domain.ID
}

func eventIdentity(event domain.Event) eventIdentityKey {
	return eventIdentityKey{projectID: event.ProjectID, traceID: event.TraceID, producerID: event.ProducerID, eventID: event.EventID}
}

func eventGapDiagnostics(events []domain.Event, now time.Time) []domain.Diagnostic {
	sequences := map[domain.ID]map[uint64]bool{}
	ids := map[domain.ID]map[domain.ID]bool{}
	for _, event := range events {
		if sequences[event.ProducerID] == nil {
			sequences[event.ProducerID] = map[uint64]bool{}
			ids[event.ProducerID] = map[domain.ID]bool{}
		}
		sequences[event.ProducerID][event.Sequence] = true
		ids[event.ProducerID][event.EventID] = true
	}
	var out []domain.Diagnostic
	for _, values := range sequences {
		if uint64(len(values)) != maxSequence(values) {
			out = append(out, makeDiagnostic("capture.sequence_gap", "Capture event sequence contains a gap.", now))
		}
	}
	for _, event := range events {
		if event.ParentEventID != nil && !ids[event.ProducerID][*event.ParentEventID] {
			out = append(out, makeDiagnostic("capture.parent_missing", "A parent event has not arrived yet.", now))
		}
	}
	return out
}

func maxSequence(values map[uint64]bool) uint64 {
	var max uint64
	for sequence := range values {
		if sequence > max {
			max = sequence
		}
	}
	return max
}

func makeDiagnostic(code, message string, now time.Time) domain.Diagnostic {
	return domain.Diagnostic{ID: domain.ID(fmt.Sprintf("diag_%x", []byte(code))), Code: code, Message: message, Severity: "warning", CreatedAt: now.UTC()}
}
