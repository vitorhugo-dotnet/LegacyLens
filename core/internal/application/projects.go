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
