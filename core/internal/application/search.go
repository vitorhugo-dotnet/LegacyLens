package application

import (
	"context"
	"errors"

	"legacylens/core/internal/domain"
)

// SearchService exposes indexed symbol search independently from indexing.
type SearchService struct {
	store    IndexStore
	projects ProjectStore
}

func NewSearchService(store IndexStore, projects ProjectStore) *SearchService {
	return &SearchService{store: store, projects: projects}
}

func (s *SearchService) Search(ctx context.Context, query SearchQuery) (SearchResult, error) {
	if s == nil || s.store == nil || s.projects == nil {
		return SearchResult{}, errors.New("search dependencies are required")
	}
	if query.ProjectID == "" {
		return SearchResult{}, errors.New("project id is required")
	}
	if _, err := s.projects.LoadProject(ctx, query.ProjectID); err != nil {
		return SearchResult{}, err
	}
	if query.RevisionID == "" {
		index, ok := s.store.(InvestigationIndexStore)
		if !ok {
			return SearchResult{}, errors.New("current index revision is unavailable")
		}
		snapshot, err := index.LoadInvestigationIndex(ctx, query.ProjectID)
		if err != nil {
			return SearchResult{}, err
		}
		if snapshot.RevisionID == "" {
			return SearchResult{}, errors.New("current index revision is unavailable")
		}
		query.RevisionID = snapshot.RevisionID
	}
	if query.Offset < 0 || query.Offset > 1_000_000_000 {
		return SearchResult{}, errors.New("search offset is outside the supported range")
	}
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 200
	}
	result, err := s.store.Search(ctx, query)
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
