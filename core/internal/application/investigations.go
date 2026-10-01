package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"legacylens/core/internal/domain"
)

// InvestigationService materializes only causal links carried by recorded events.
// Indexed relations retain their original layer and resolution.
type InvestigationService struct{ traces TraceStore; presence *AgentPresence }

func NewInvestigationService(traces TraceStore) *InvestigationService {
	return &InvestigationService{traces: traces}
}

func (s *InvestigationService) WithAgentPresence(p *AgentPresence) *InvestigationService { s.presence = p; return s }

func (s *InvestigationService) Get(ctx context.Context, projectID, traceID domain.ID) (Investigation, error) {
	if s == nil || s.traces == nil {
		return Investigation{}, errors.New("trace store is required")
	}
	if projectID == "" || traceID == "" {
		return Investigation{}, errors.New("project and trace ids are required")
	}
	value, err := s.traces.Load(ctx, projectID, traceID)
	if err != nil {
		return Investigation{}, err
	}
	if value.Trace.ID != traceID || value.Trace.ProjectID != projectID {
		return Investigation{}, errors.New("capture session not found")
	}
	if indexed, ok := s.traces.(InvestigationIndexStore); ok {
		snapshot, err := indexed.LoadInvestigationIndex(ctx, projectID)
		if err != nil {
			return Investigation{}, err
		}
		value.IndexedRevisionID = snapshot.RevisionID
		value.Symbols = append(value.Symbols, snapshot.Symbols...)
		value.Relations = append(value.Relations, snapshot.Relations...)
		value.Evidence = append(value.Evidence, snapshot.Evidence...)
		value.Diagnostics = append(value.Diagnostics, snapshot.Diagnostics...)
	}
	if value.AgentStatus.State == "" {
		value.AgentStatus.State = "unknown"
	}
	if s.presence != nil {
		expected := make([]domain.ID, 0)
		for _, event := range value.Events { if event.Kind == "http.server" { expected = append(expected,event.ProducerID) } }
		value.AgentStatus = s.presence.Status(projectID, expected)
		if value.AgentStatus.EvidenceDiagnosticID != "" { value.Diagnostics = append(value.Diagnostics,s.presence.Diagnostic(value.AgentStatus)) }
	}

	byEventID := map[domain.ID][]domain.Event{}
	ids := map[string]domain.ID{}
	sequences := map[domain.ID][]uint64{}
	for _, event := range value.Events {
		if event.ProjectID != projectID || event.TraceID != traceID {
			return Investigation{}, errors.New("event identity does not match investigation")
		}
		key := string(event.ProducerID) + "\x00" + string(event.EventID)
		if _, exists := ids[key]; exists {
			continue
		}
		id := stableID(string(projectID), string(traceID), string(event.ProducerID), string(event.EventID))
		ids[key] = id
		byEventID[event.EventID] = append(byEventID[event.EventID], event)
		sequences[event.ProducerID] = append(sequences[event.ProducerID], event.Sequence)
		name := event.Kind
		kind := "observed-event"
		if class, method := event.Metadata["code.class"], event.Metadata["code.method"]; class != "" && method != "" {
			name = class + "." + method + event.Metadata["code.descriptor"]
			kind = "observed-java"
		}
		name += fmt.Sprintf(" · producer %s · event %s", event.ProducerID, event.EventID)
		value.Symbols = append(value.Symbols, domain.Symbol{ID: id, ProjectID: projectID, QualifiedName: name, Kind: kind})
		observedAt := event.OccurredAt
		value.Evidence = append(value.Evidence, domain.Evidence{ID: id, Kind: "observed-event", Source: event.Kind, ObservedAt: &observedAt})
		if event.Kind == "extension.gap" || event.Kind == "agent.loss" {
			value.Trace.Incomplete = true
			code, message := "capture.reconnected_gap", "Capture transport reconnected after an unknown interval."
			if event.Kind == "agent.loss" {
				code, message = "capture.agent_loss", "Java agent reported dropped trace events."
			}
			addInvestigationDiagnostic(&value, code, message, event)
		}
		if event.Metadata["code.line_missing"] == "true" {
			addInvestigationDiagnostic(&value, "source.debug_absent", "Java event has no source line in its debug metadata.", event)
		}
		if event.ApplicationRevision != nil || kind == "observed-java" {
			addInvestigationDiagnostic(&value, "source.match_unconfirmed", "Observed Java source has no verified match to an indexed artifact and deployment revision.", event)
		}
	}
	for producer, numbers := range sequences {
		sort.Slice(numbers, func(i, j int) bool { return numbers[i] < numbers[j] })
		previous := uint64(0)
		for _, number := range numbers {
			if number > previous+1 && (previous == 0 || number > 1) {
				value.Trace.Incomplete = true
				event := domain.Event{ProjectID: projectID, TraceID: traceID, ProducerID: producer, EventID: domain.ID(fmt.Sprintf("sequence-%d", number))}
				addInvestigationDiagnostic(&value, "capture.sequence_gap", "Recorded producer sequence has a gap; causal evidence may be missing.", event)
			}
			if number > previous {
				previous = number
			}
		}
	}
	for _, event := range value.Events {
		if event.ParentEventID == nil {
			continue
		}
		child := ids[string(event.ProducerID)+"\x00"+string(event.EventID)]
		if child == "" {
			continue
		}
		matches := byEventID[*event.ParentEventID]
		unknownParent := stableID("unknown-parent", string(projectID), string(traceID), string(event.ProducerID), string(event.EventID))
		relation := domain.Relation{ID: stableID("observed-parent", string(projectID), string(traceID), string(event.ProducerID), string(event.EventID)), FromID: unknownParent, ToID: &child, Kind: "event.parent_missing", EvidenceIDs: []domain.ID{child}, Resolution: domain.ResolutionUnresolved, Layer: domain.LayerObserved}
		if len(matches) == 1 {
			parent := matches[0]
			if parentID := ids[string(parent.ProducerID)+"\x00"+string(parent.EventID)]; parentID != "" {
				relation.FromID, relation.Kind, relation.Resolution = parentID, "event.parent", domain.ResolutionResolved
			}
		} else {
			value.Trace.Incomplete = true
			code, message := "capture.parent_missing", "Parent event is absent from this investigation."
			if len(matches) > 1 {
				code, message = "capture.parent_ambiguous", "Parent event ID occurs in multiple producer epochs; its causal link is unconfirmed."
				relation.Kind = "event.parent_ambiguous"
			}
			addInvestigationDiagnostic(&value, code, message, event)
		}
		value.Relations = append(value.Relations, relation)
	}
	requestEvents := map[string][]domain.Event{}
	serverEvents := map[string][]domain.Event{}
	for _, event := range value.Events {
		if event.Kind == "browser.network" || event.Kind == "primefaces.ajax" {
			if validRequestSpan(event.Metadata["spanId"]) { requestEvents[event.Metadata["spanId"]] = append(requestEvents[event.Metadata["spanId"]],event) }
		}
		if event.Kind == "http.server" && validRequestSpan(event.Metadata["http.request_span"]) { serverEvents[event.Metadata["http.request_span"]] = append(serverEvents[event.Metadata["http.request_span"]],event) }
	}
	for span, servers := range serverEvents {
		browsers := requestEvents[span]
		if len(servers) != 1 || len(browsers) != 1 { continue }
		from := ids[string(browsers[0].ProducerID)+"\x00"+string(browsers[0].EventID)]
		to := ids[string(servers[0].ProducerID)+"\x00"+string(servers[0].EventID)]
		if from == "" || to == "" { continue }
		value.Relations = append(value.Relations,domain.Relation{ID:stableID("http.request",string(projectID),string(traceID),span),FromID:from,ToID:&to,Kind:"http.request",EvidenceIDs:[]domain.ID{from,to},Resolution:domain.ResolutionResolved,Layer:domain.LayerObserved})
	}
	return value, nil
}

func validRequestSpan(span string) bool {
	if len(span) != 16 { return false }
	nonzero := false
	for _, c := range span { if c != '0' { nonzero = true }; if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) { return false } }
	return nonzero
}

func addInvestigationDiagnostic(value *Investigation, code, message string, event domain.Event) {
	id := stableID(code, string(event.ProjectID), string(event.TraceID), string(event.ProducerID), string(event.EventID))
	for _, existing := range value.Diagnostics {
		if existing.ID == id || (existing.Code == code && code == "capture.sequence_gap") {
			return
		}
	}
	at := event.OccurredAt
	if at.IsZero() {
		at = value.Trace.StartedAt
	}
	if at.IsZero() {
		at = time.Unix(0, 0).UTC()
	}
	value.Diagnostics = append(value.Diagnostics, domain.Diagnostic{ID: id, Code: code, Message: message, Severity: "warning", CreatedAt: at})
}
