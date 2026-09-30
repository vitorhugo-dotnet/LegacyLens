package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"legacylens/core/internal/domain"
	"path/filepath"
	"strings"
	"time"
)

type ProjectService struct{ store ProjectStore }

func NewProjectService(store ProjectStore) *ProjectService { return &ProjectService{store: store} }

func (s *ProjectService) RegisterProject(ctx context.Context, config ProjectConfig) (domain.Project, error) {
	if s == nil || s.store == nil {
		return domain.Project{}, errors.New("project store is required")
	}
	root := strings.TrimSpace(config.Root)
	if root == "" {
		return domain.Project{}, errors.New("project root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return domain.Project{}, errors.New("project root is invalid")
	}
	id := config.ProjectID
	if id == "" {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return domain.Project{}, errors.New("could not create project identity")
		}
		id = domain.ID("project-" + hex.EncodeToString(random[:]))
	}
	name := strings.TrimSpace(config.Name)
	if name == "" {
		name = filepath.Base(filepath.Clean(abs))
	}
	project := domain.Project{ID: id, Name: name, Root: filepath.Clean(abs), Includes: append([]string(nil), config.Includes...), Excludes: append([]string(nil), config.Excludes...), CreatedAt: time.Now().UTC()}
	if err := s.store.SaveProject(ctx, project); err != nil {
		return domain.Project{}, err
	}
	return project, nil
}

func (s *ProjectService) ListProjects(ctx context.Context) ([]domain.Project, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("project store is required")
	}
	catalog, ok := s.store.(ProjectCatalog)
	if !ok {
		return nil, errors.New("project listing is unavailable")
	}
	return catalog.ListProjects(ctx)
}

func (s *ProjectService) ListProjectsPage(ctx context.Context, offset, limit int) (Page[domain.Project], error) {
	if s == nil || s.store == nil {
		return Page[domain.Project]{}, errors.New("project store is required")
	}
	if offset < 0 || offset > 1_000_000_000 || limit < 1 || limit > 200 {
		return Page[domain.Project]{}, errors.New("project page is outside the supported range")
	}
	catalog, ok := s.store.(PagedProjectCatalog)
	if !ok {
		return Page[domain.Project]{}, errors.New("project pagination is unavailable")
	}
	return catalog.PageProjects(ctx, offset, limit)
}

func (s *ProjectService) GetProject(ctx context.Context, id domain.ID) (domain.Project, error) {
	if s == nil || s.store == nil {
		return domain.Project{}, errors.New("project store is required")
	}
	if id == "" {
		return domain.Project{}, errors.New("project id is required")
	}
	project, err := s.store.LoadProject(ctx, id)
	if err != nil {
		return domain.Project{}, err
	}
	if project.ID != id {
		return domain.Project{}, errors.New("registered project identity does not match")
	}
	return project, nil
}
