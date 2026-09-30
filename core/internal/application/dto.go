package application

import (
	"time"

	"legacylens/core/internal/domain"
)

type ProjectConfig struct {
	ProjectID domain.ID
	Root      string
	Name      string
	Includes  []string
	Excludes  []string
}

type IndexRequest struct {
	ProjectID domain.ID
	Root      string
	Paths     []string
}

type IndexResult struct {
	ProjectID     domain.ID
	RevisionID    domain.ID
	Artifacts     []domain.Artifact
	Symbols       []domain.Symbol
	Relations     []domain.Relation
	Evidence      []domain.Evidence
	Diagnostics   []domain.Diagnostic
	ReplacedFiles []string
	ExcludedFiles []string
}

type AnalysisInput struct {
	ProjectID  domain.ID
	RevisionID domain.ID
	Root       string
	Artifacts  []domain.Artifact
	Sources    map[string][]byte
}

type AnalysisResult struct {
	Symbols     []domain.Symbol
	Relations   []domain.Relation
	Evidence    []domain.Evidence
	Diagnostics []domain.Diagnostic
}

type Capability struct {
	Name        string
	Supported   bool
	Description string
}

type SearchQuery struct {
	ProjectID  domain.ID
	RevisionID domain.ID
	Text       string
	Kinds      []string
	Limit      int
}

type SearchResult struct {
	Symbols []domain.Symbol
	Total   int
}

type GraphQuery struct {
	ProjectID  domain.ID
	RevisionID domain.ID
	SymbolIDs  []domain.ID
	Depth      int
}

type GraphResult struct {
	Symbols   []domain.Symbol
	Relations []domain.Relation
}

type CaptureRequest struct {
	ProjectID domain.ID
	TabID     string
	StartedAt time.Time
}

type CaptureSession struct {
	ID        domain.ID
	ProjectID domain.ID
	StartedAt time.Time
	ExpiresAt time.Time
}

type IngestResult struct {
	Accepted    int
	Duplicate   int
	Diagnostics []domain.Diagnostic
}

type Investigation struct {
	Project   domain.Project
	Trace     domain.Trace
	Events    []domain.Event
	Symbols   []domain.Symbol
	Relations []domain.Relation
}

type OpenResult struct {
	Opened  bool
	Message string
	File    string
	Line    int
}

type ExplanationRequest struct {
	ProjectID domain.ID
	TraceID   domain.ID
	Question  string
}

type ExplanationResult struct {
	Text        string
	EvidenceIDs []domain.ID
}
