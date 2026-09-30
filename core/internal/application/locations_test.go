package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"legacylens/core/internal/domain"
)

type locationProjectStore struct{ project domain.Project }

func (s locationProjectStore) SaveProject(context.Context, domain.Project) error { return nil }
func (s locationProjectStore) LoadProject(_ context.Context, id domain.ID) (domain.Project, error) {
	if id != s.project.ID {
		return domain.Project{}, os.ErrNotExist
	}
	return s.project, nil
}

type locationEditorFunc func(context.Context, domain.Location) (OpenResult, error)

func (f locationEditorFunc) Open(ctx context.Context, location domain.Location) (OpenResult, error) {
	return f(ctx, location)
}

func TestOpenLocationResolvesVerifiedAbsolutePath(t *testing.T) {
	root := t.TempDir()
	relative := filepath.Join("src", "ação com espaço.java")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, relative)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, relative), []byte("class Example {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got domain.Location
	service := NewLocationService(locationProjectStore{domain.Project{ID: "p1", Root: root}}, locationEditorFunc(func(_ context.Context, loc domain.Location) (OpenResult, error) {
		got = loc
		return OpenResult{Opened: true}, nil
	}))
	if _, err := service.Open(context.Background(), "p1", domain.Location{Path: filepath.ToSlash(relative), Line: 42}); err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got.Path) || got.Path != filepath.Join(root, relative) || got.Line != 42 {
		t.Fatalf("editor location = %#v; want absolute project file at line 42", got)
	}
}

func TestOpenLocationRejectsOutsideProject(t *testing.T) {
	root := t.TempDir()
	called := false
	service := NewLocationService(locationProjectStore{domain.Project{ID: "p1", Root: root}}, locationEditorFunc(func(context.Context, domain.Location) (OpenResult, error) {
		called = true
		return OpenResult{}, nil
	}))
	if _, err := service.Open(context.Background(), "p1", domain.Location{Path: "../outside.java", Line: 1}); err == nil {
		t.Fatal("expected traversal path rejection")
	}
	if called {
		t.Fatal("editor was called for a path outside the project")
	}
}

func TestRevisionMissingIsUnconfirmed(t *testing.T) {
	if got := CompareRevision(domain.Revision{}, domain.Revision{}); got == "confirmed" {
		t.Fatal("missing revisions must never be confirmed")
	}
}
