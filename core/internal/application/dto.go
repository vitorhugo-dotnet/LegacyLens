package application

import (
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
}

type SearchResult struct {
	Symbols []domain.Symbol `json:"symbols"`
	Total   int             `json:"total"`
}

type GraphQuery struct {
	ProjectID  domain.ID   `json:"projectId"`
	RevisionID domain.ID   `json:"revisionId,omitempty"`
	SymbolIDs  []domain.ID `json:"symbolIds"`
	Depth      int         `json:"depth"`
}

type GraphResult struct {
	Symbols   []domain.Symbol   `json:"symbols"`
	Relations []domain.Relation `json:"relations"`
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
	Project     domain.Project      `json:"project"`
	Trace       domain.Trace        `json:"trace"`
	Events      []domain.Event      `json:"events"`
	Diagnostics []domain.Diagnostic `json:"diagnostics"`
	Symbols     []domain.Symbol     `json:"symbols"`
	Relations   []domain.Relation   `json:"relations"`
}

type OpenResult struct {
	Opened  bool   `json:"opened"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

type ExplanationRequest struct {
	ProjectID domain.ID `json:"projectId"`
	TraceID   domain.ID `json:"traceId"`
	Question  string    `json:"question"`
}

type ExplanationResult struct {
	Text        string      `json:"text"`
	EvidenceIDs []domain.ID `json:"evidenceIds"`
}
