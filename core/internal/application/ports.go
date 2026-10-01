package application

import (
	"context"
	"time"

	"legacylens/core/internal/domain"
)

type Analyzer interface {
	Capabilities() []Capability
	Analyze(context.Context, AnalysisInput) (AnalysisResult, error)
}

type IndexStore interface {
	CommitIndex(context.Context, IndexResult) error
	Search(context.Context, SearchQuery) (SearchResult, error)
	Explore(context.Context, GraphQuery) (GraphResult, error)
}

// ProjectStore persists registered projects independently of the storage adapter.
type ProjectStore interface {
	SaveProject(context.Context, domain.Project) error
	LoadProject(context.Context, domain.ID) (domain.Project, error)
}

// ProjectCatalog adds discovery without requiring every ProjectStore consumer to implement it.
type ProjectCatalog interface {
	ListProjects(context.Context) ([]domain.Project, error)
}

type PagedProjectCatalog interface {
	PageProjects(context.Context, int, int) (Page[domain.Project], error)
}

// ArtifactSource lists and reads only files approved by a project's path policy.
type ArtifactSource interface {
	List(context.Context, domain.Project, []string) ([]domain.Artifact, error)
	Read(context.Context, domain.Project, domain.Artifact) ([]byte, error)
}

type TraceStore interface {
	Append(context.Context, []domain.Event) (IngestResult, error)
	Load(context.Context, domain.ID, domain.ID) (Investigation, error)
}

// InvestigationIndexStore reads one indexed snapshot only when a user opens an investigation.
type InvestigationIndexStore interface {
	LoadInvestigationIndex(context.Context, domain.ID) (InvestigationIndex, error)
}

type CaptureStore interface {
	LoadProject(context.Context, domain.ID) (domain.Project, error)
	StartCapture(context.Context, domain.Trace, string) error
	StopCapture(context.Context, domain.ID, domain.ID, time.Time) error
	MarkCaptureIncomplete(context.Context, domain.ID, domain.ID, domain.Diagnostic) error
	AppendBounded(context.Context, []domain.Event, int, []domain.Diagnostic) (IngestResult, error)
	TraceStore
}

type Editor interface {
	Open(context.Context, domain.Location) (OpenResult, error)
}

type Explainer interface {
	Generate(context.Context, ExplanationRequest) (ExplanationResult, error)
}
