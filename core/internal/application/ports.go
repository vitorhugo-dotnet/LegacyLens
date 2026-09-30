package application

import (
	"context"

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

type TraceStore interface {
	Append(context.Context, []domain.Event) (IngestResult, error)
	Load(context.Context, domain.ID, domain.ID) (Investigation, error)
}

type Editor interface {
	Open(context.Context, domain.Location) (OpenResult, error)
}

type Explainer interface {
	Generate(context.Context, ExplanationRequest) (ExplanationResult, error)
}
