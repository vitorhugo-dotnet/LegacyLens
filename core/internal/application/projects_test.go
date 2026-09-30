package application

import (
	"context"
	"testing"

	"legacylens/core/internal/domain"
)

type projectCatalogTestStore struct{ projects []domain.Project }

func (s projectCatalogTestStore) SaveProject(context.Context, domain.Project) error { return nil }
func (s projectCatalogTestStore) LoadProject(_ context.Context, id domain.ID) (domain.Project, error) {
	for _, project := range s.projects {
		if project.ID == id {
			return project, nil
		}
	}
	return domain.Project{}, context.Canceled
}

func TestProjectServiceLoadsProjectForStatus(t *testing.T) {
	want := domain.Project{ID: "p1", Name: "demo", Root: `C:\source`}
	service := NewProjectService(projectCatalogTestStore{projects: []domain.Project{want}})
	got, err := service.GetProject(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Root != want.Root {
		t.Fatalf("GetProject() = %+v, want %+v", got, want)
	}
}
func (s projectCatalogTestStore) ListProjects(context.Context) ([]domain.Project, error) {
	return s.projects, nil
}

func TestProjectServiceListsRegisteredProjects(t *testing.T) {
	want := []domain.Project{{ID: "p1", Name: "demo", Root: `C:\source`}}
	service := NewProjectService(projectCatalogTestStore{projects: want})
	got, err := service.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "p1" || got[0].Root != `C:\source` {
		t.Fatalf("ListProjects() = %+v, want project p1", got)
	}
}
