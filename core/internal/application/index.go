package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"legacylens/core/internal/domain"
	"sort"
)

type Indexer struct {
	store     IndexStore
	projects  ProjectStore
	source    ArtifactSource
	analyzers []Analyzer
	resolver  ExpressionResolver
}

func NewIndexer(store IndexStore, projects ProjectStore, source ArtifactSource, analyzers []Analyzer) *Indexer {
	return &Indexer{store: store, projects: projects, source: source, analyzers: append([]Analyzer(nil), analyzers...)}
}

func (i *Indexer) WithExpressionResolver(resolver ExpressionResolver) *Indexer {
	i.resolver = resolver
	return i
}

func (i *Indexer) Index(ctx context.Context, request IndexRequest) (IndexResult, error) {
	return i.index(ctx, request, len(request.Paths) == 0)
}

func (i *Indexer) index(ctx context.Context, request IndexRequest, incremental bool) (IndexResult, error) {
	if i == nil || i.store == nil || i.projects == nil || i.source == nil {
		return IndexResult{}, errors.New("indexer dependencies are required")
	}
	project, err := i.projects.LoadProject(ctx, request.ProjectID)
	if err != nil {
		return IndexResult{}, err
	}
	artifacts, err := i.source.List(ctx, project, request.Paths)
	if err != nil {
		return IndexResult{}, err
	}
	sources := make(map[string][]byte, len(artifacts))
	for n := range artifacts {
		content, err := i.source.Read(ctx, project, artifacts[n])
		if err != nil {
			return IndexResult{}, err
		}
		digest := sha256.Sum256(content)
		contentHash := hex.EncodeToString(digest[:])
		if artifacts[n].ContentHash != "" && artifacts[n].ContentHash != contentHash {
			return IndexResult{}, fmt.Errorf("source changed during indexing: %s", artifacts[n].Path)
		}
		artifacts[n].ContentHash = contentHash
		sources[artifacts[n].Path] = content
	}
	sort.Slice(artifacts, func(a, b int) bool { return artifacts[a].Path < artifacts[b].Path })
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00", project.ID)
	for _, artifact := range artifacts {
		fmt.Fprintf(h, "%s\x00%s\n", artifact.Path, artifact.ContentHash)
	}
	revisionID := domain.ID("revision-" + hex.EncodeToString(h.Sum(nil)))
	for n := range artifacts {
		artifacts[n].ProjectID = project.ID
		artifacts[n].RevisionID = revisionID
		artifacts[n].ID = stableID(string(project.ID), string(revisionID), artifacts[n].Path)
	}
	result := IndexResult{ProjectID: project.ID, RevisionID: revisionID, Artifacts: artifacts}
	for _, analyzer := range i.analyzers {
		analysis, err := analyzer.Analyze(ctx, AnalysisInput{ProjectID: project.ID, RevisionID: revisionID, Root: project.Root, Artifacts: artifacts, Sources: sources})
		if err != nil {
			return IndexResult{}, err
		}
		result.Symbols = append(result.Symbols, analysis.Symbols...)
		result.Relations = append(result.Relations, analysis.Relations...)
		result.Evidence = append(result.Evidence, analysis.Evidence...)
		result.Diagnostics = append(result.Diagnostics, analysis.Diagnostics...)
	}
	i.resolveBrowserRelations(&result)
	// Check once more after analyzer work, before publishing a revision. A file
	// changing between enumeration, read, and analysis cannot enter the index.
	for _, artifact := range artifacts {
		content, err := i.source.Read(ctx, project, artifact)
		if err != nil {
			return IndexResult{}, err
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != artifact.ContentHash {
			return IndexResult{}, fmt.Errorf("source changed during indexing: %s", artifact.Path)
		}
	}
	if incremental {
		if state, ok := i.store.(IndexStateStore); ok {
			previous, err := state.LatestArtifacts(ctx, project.ID)
			if err != nil {
				return IndexResult{}, err
			}
			current := make(map[string]string, len(artifacts))
			for _, artifact := range artifacts {
				current[artifact.Path] = artifact.ContentHash
			}
			for _, artifact := range previous {
				if hash, exists := current[artifact.Path]; !exists {
					result.ExcludedFiles = append(result.ExcludedFiles, artifact.Path)
				} else if hash != artifact.ContentHash {
					result.ReplacedFiles = append(result.ReplacedFiles, artifact.Path)
				}
			}
			sort.Strings(result.ExcludedFiles)
			sort.Strings(result.ReplacedFiles)
		}
	}
	if err := i.store.CommitIndex(ctx, result); err != nil {
		return IndexResult{}, err
	}
	return result, nil
}

func (i *Indexer) Search(ctx context.Context, query SearchQuery) (SearchResult, error) {
	if i == nil || i.store == nil || i.projects == nil {
		return SearchResult{}, errors.New("indexer dependencies are required")
	}
	if query.ProjectID == "" {
		return SearchResult{}, errors.New("project id is required")
	}
	if _, err := i.projects.LoadProject(ctx, query.ProjectID); err != nil {
		return SearchResult{}, err
	}
	if query.Offset < 0 || query.Offset > 1_000_000_000 {
		return SearchResult{}, errors.New("search offset is outside the supported range")
	}
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 200
	}
	result, err := i.store.Search(ctx, query)
	if err != nil {
		return SearchResult{}, err
	}
	result.Offset, result.Limit = query.Offset, query.Limit
	result.HasMore = query.Offset+len(result.Symbols) < result.Total
	if result.Symbols == nil {
		result.Symbols = []domain.Symbol{}
	}
	return result, nil
}

func (i *Indexer) Explore(ctx context.Context, query GraphQuery) (GraphResult, error) {
	if i == nil || i.store == nil || i.projects == nil {
		return GraphResult{}, errors.New("indexer dependencies are required")
	}
	if query.ProjectID == "" || len(query.SymbolIDs) == 0 {
		return GraphResult{}, errors.New("project and symbol ids are required")
	}
	if _, err := i.projects.LoadProject(ctx, query.ProjectID); err != nil {
		return GraphResult{}, err
	}
	if query.Depth < 0 || query.Depth > 16 {
		return GraphResult{}, errors.New("graph depth is outside the supported range")
	}
	return i.store.Explore(ctx, query)
}

func stableID(parts ...string) domain.ID {
	h := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(h, "%s\x00", part)
	}
	return domain.ID(hex.EncodeToString(h.Sum(nil)))
}
