package application

import (
	"sort"
	"time"

	"legacylens/core/internal/domain"
)

type ProjectConfig struct {
	ProjectID domain.ID `json:"projectId,omitempty"`
	Root      string    `json:"root"`
	Name      string    `json:"name,omitempty"`
	Includes  []string  `json:"includes,omitempty"`
	Excludes  []string  `json:"excludes,omitempty"`
}

type IndexRequest struct {
	ProjectID domain.ID `json:"projectId"`
	Root      string    `json:"root,omitempty"`
	Paths     []string  `json:"paths,omitempty"`
}

type IndexResult struct {
	ProjectID     domain.ID           `json:"projectId"`
	RevisionID    domain.ID           `json:"revisionId"`
	Artifacts     []domain.Artifact   `json:"artifacts"`
	Symbols       []domain.Symbol     `json:"symbols"`
	Relations     []domain.Relation   `json:"relations"`
	Evidence      []domain.Evidence   `json:"evidence"`
	Diagnostics   []domain.Diagnostic `json:"diagnostics"`
	ReplacedFiles []string            `json:"replacedFiles"`
	ExcludedFiles []string            `json:"excludedFiles"`
}

type IndexResultPage struct {
	ProjectID     domain.ID               `json:"projectId"`
	RevisionID    domain.ID               `json:"revisionId"`
	Artifacts     Page[domain.Artifact]   `json:"artifacts"`
	Symbols       Page[domain.Symbol]     `json:"symbols"`
	Relations     Page[domain.Relation]   `json:"relations"`
	Evidence      Page[domain.Evidence]   `json:"evidence"`
	Diagnostics   Page[domain.Diagnostic] `json:"diagnostics"`
	ReplacedFiles Page[string]            `json:"replacedFiles"`
	ExcludedFiles Page[string]            `json:"excludedFiles"`
}

func PageIndexResult(value IndexResult, offset, limit int) IndexResultPage {
	return IndexResultPage{ProjectID: value.ProjectID, RevisionID: value.RevisionID,
		Artifacts:     sortedPage(value.Artifacts, offset, limit, func(a, b domain.Artifact) bool { return a.ID < b.ID }),
		Symbols:       sortedPage(value.Symbols, offset, limit, func(a, b domain.Symbol) bool { return a.ID < b.ID }),
		Relations:     sortedPage(value.Relations, offset, limit, func(a, b domain.Relation) bool { return a.ID < b.ID }),
		Evidence:      sortedPage(value.Evidence, offset, limit, func(a, b domain.Evidence) bool { return a.ID < b.ID }),
		Diagnostics:   sortedPage(value.Diagnostics, offset, limit, func(a, b domain.Diagnostic) bool { return a.ID < b.ID }),
		ReplacedFiles: sortedPage(value.ReplacedFiles, offset, limit, func(a, b string) bool { return a < b }),
		ExcludedFiles: sortedPage(value.ExcludedFiles, offset, limit, func(a, b string) bool { return a < b })}
}

type AnalysisInput struct {
	ProjectID  domain.ID         `json:"projectId"`
	RevisionID domain.ID         `json:"revisionId"`
	Root       string            `json:"root"`
	Artifacts  []domain.Artifact `json:"artifacts"`
	Sources    map[string][]byte `json:"-"`
}

type AnalysisResult struct {
	Symbols     []domain.Symbol     `json:"symbols"`
	Relations   []domain.Relation   `json:"relations"`
	Evidence    []domain.Evidence   `json:"evidence"`
	Diagnostics []domain.Diagnostic `json:"diagnostics"`
}

type Capability struct {
	Name        string `json:"name"`
	Supported   bool   `json:"supported"`
	Description string `json:"description"`
}

type SearchQuery struct {
	ProjectID  domain.ID `json:"projectId"`
	RevisionID domain.ID `json:"revisionId,omitempty"`
	Text       string    `json:"text"`
	Kinds      []string  `json:"kinds,omitempty"`
	Limit      int       `json:"limit,omitempty"`
	Offset     int       `json:"offset,omitempty"`
}

type SearchResult struct {
	Symbols []domain.Symbol `json:"symbols"`
	Total   int             `json:"total"`
	Offset  int             `json:"offset"`
	Limit   int             `json:"limit"`
	HasMore bool            `json:"hasMore"`
}

type Page[T any] struct {
	Items   []T  `json:"items"`
	Offset  int  `json:"offset"`
	Limit   int  `json:"limit"`
	Total   int  `json:"total"`
	HasMore bool `json:"hasMore"`
}

func NewPage[T any](items []T, offset, limit, total int) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Offset: offset, Limit: limit, Total: total, HasMore: offset+len(items) < total}
}

type InvestigationPage struct {
	Project           domain.Project          `json:"project"`
	Trace             domain.Trace            `json:"trace"`
	AgentStatus       AgentStatus             `json:"agentStatus"`
	IndexedRevisionID domain.ID               `json:"indexedRevisionId,omitempty"`
	Events            Page[domain.Event]      `json:"events"`
	Diagnostics       Page[domain.Diagnostic] `json:"diagnostics"`
	Symbols           Page[domain.Symbol]     `json:"symbols"`
	Relations         Page[domain.Relation]   `json:"relations"`
	Evidence          Page[domain.Evidence]   `json:"evidence"`
}

func PageInvestigation(value Investigation, offset, limit int) InvestigationPage {
	return InvestigationPage{
		Project: value.Project, Trace: value.Trace, AgentStatus: value.AgentStatus, IndexedRevisionID: value.IndexedRevisionID,
		Events: sortedPage(value.Events, offset, limit, func(a, b domain.Event) bool {
			if a.ProducerID == b.ProducerID {
				if a.Sequence == b.Sequence {
					return a.EventID < b.EventID
				}
				return a.Sequence < b.Sequence
			}
			return a.ProducerID < b.ProducerID
		}),
		Diagnostics: sortedPage(value.Diagnostics, offset, limit, func(a, b domain.Diagnostic) bool { return a.ID < b.ID }),
		Symbols:     sortedPage(value.Symbols, offset, limit, func(a, b domain.Symbol) bool { return a.ID < b.ID }),
		Relations:   sortedPage(value.Relations, offset, limit, func(a, b domain.Relation) bool { return a.ID < b.ID }),
		Evidence:    sortedPage(value.Evidence, offset, limit, func(a, b domain.Evidence) bool { return a.ID < b.ID }),
	}
}

func pageSlice[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return []T{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

func sortedPage[T any](items []T, offset, limit int, less func(T, T) bool) Page[T] {
	ordered := append([]T(nil), items...)
	sort.Slice(ordered, func(i, j int) bool { return less(ordered[i], ordered[j]) })
	return NewPage(pageSlice(ordered, offset, limit), offset, limit, len(ordered))
}

type GraphQuery struct {
	ProjectID  domain.ID   `json:"projectId"`
	RevisionID domain.ID   `json:"revisionId,omitempty"`
	SymbolIDs  []domain.ID `json:"symbolIds"`
	Depth      int         `json:"depth"`
	Offset     int         `json:"offset,omitempty"`
	Limit      int         `json:"limit,omitempty"`
}

type GraphResult struct {
	ProjectID   domain.ID         `json:"projectId,omitempty"`
	RevisionID  domain.ID         `json:"revisionId,omitempty"`
	Symbols     []domain.Symbol   `json:"symbols"`
	Relations   []domain.Relation `json:"relations"`
	Paths       []GraphPath       `json:"paths,omitempty"`
	Evidence    []domain.Evidence `json:"evidence,omitempty"`
	Diagnostics []GraphDiagnostic `json:"diagnostics,omitempty"`
	Depth       int               `json:"depth,omitempty"`
	Offset      int               `json:"offset,omitempty"`
	Limit       int               `json:"limit,omitempty"`
	Total       int               `json:"total,omitempty"`
	HasMore     bool              `json:"hasMore,omitempty"`
	Truncated   bool              `json:"truncated,omitempty"`
}

// GraphPath gives the UI an explicit source-to-target chain and its evidence.
// SymbolIDs and RelationIDs are ordered from the affected source toward target.
type GraphPath struct {
	SourceID    domain.ID   `json:"sourceId"`
	TargetID    domain.ID   `json:"targetId"`
	SymbolIDs   []domain.ID `json:"symbolIds"`
	RelationIDs []domain.ID `json:"relationIds"`
	EvidenceIDs []domain.ID `json:"evidenceIds"`
	Inferred    bool        `json:"inferred,omitempty"`
}

// GraphDiagnostic ties incomplete or inferred graph results back to indexed source.
type GraphDiagnostic struct {
	ID          domain.ID        `json:"id"`
	Code        string           `json:"code"`
	Message     string           `json:"message"`
	SymbolIDs   []domain.ID      `json:"symbolIds,omitempty"`
	RelationID  domain.ID        `json:"relationId,omitempty"`
	EvidenceIDs []domain.ID      `json:"evidenceIds,omitempty"`
	Location    *domain.Location `json:"location,omitempty"`
}

type GraphResultPage struct {
	Symbols   Page[domain.Symbol]   `json:"symbols"`
	Relations Page[domain.Relation] `json:"relations"`
}

func PageGraphResult(value GraphResult, offset, limit int) GraphResultPage {
	return GraphResultPage{Symbols: sortedPage(value.Symbols, offset, limit, func(a, b domain.Symbol) bool { return a.ID < b.ID }), Relations: sortedPage(value.Relations, offset, limit, func(a, b domain.Relation) bool { return a.ID < b.ID })}
}

type CaptureRequest struct {
	ProjectID domain.ID `json:"projectId"`
	TabID     string    `json:"tabId,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
}

type CaptureSession struct {
	ID        domain.ID `json:"id"`
	ProjectID domain.ID `json:"projectId"`
	StartedAt time.Time `json:"startedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type IngestResult struct {
	Accepted    int                 `json:"accepted"`
	Duplicate   int                 `json:"duplicate"`
	Diagnostics []domain.Diagnostic `json:"diagnostics"`
}

type Investigation struct {
	Project           domain.Project      `json:"project"`
	Trace             domain.Trace        `json:"trace"`
	AgentStatus       AgentStatus         `json:"agentStatus"`
	IndexedRevisionID domain.ID           `json:"indexedRevisionId,omitempty"`
	Events            []domain.Event      `json:"events"`
	Diagnostics       []domain.Diagnostic `json:"diagnostics"`
	Symbols           []domain.Symbol     `json:"symbols"`
	Relations         []domain.Relation   `json:"relations"`
	Evidence          []domain.Evidence   `json:"evidence"`
}

type InvestigationIndex struct {
	RevisionID  domain.ID
	Symbols     []domain.Symbol
	Relations   []domain.Relation
	Evidence    []domain.Evidence
	Diagnostics []domain.Diagnostic
}

// AgentStatus requires a diagnostic with an authenticated liveness signal.
// A missing signal is unknown, including when a trace has no Java events.
type AgentStatus struct {
	State                string    `json:"state"`
	EvidenceDiagnosticID domain.ID `json:"evidenceDiagnosticId,omitempty"`
}

type OpenResult struct {
	Opened  bool   `json:"opened"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

type ExplanationRequest struct {
	ProjectID   domain.ID   `json:"projectId"`
	TraceID     domain.ID   `json:"traceId"`
	Question    string      `json:"question"`
	EvidenceIDs []domain.ID `json:"evidenceIds"`
}

type ExplanationClaim struct {
	Text        string      `json:"text"`
	EvidenceIDs []domain.ID `json:"evidenceIds"`
	Confidence  string      `json:"confidence"`
}

type ExplanationResult struct {
	Claims      []ExplanationClaim `json:"claims"`
	Limitations []string           `json:"limitations"`
}
