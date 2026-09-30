package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"legacylens/core/internal/domain"
)

func TestSourceIndexesPathWithSpacesAndAccents(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Projetos", "Gestão Legada")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "view.xhtml"), []byte("<html/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := domain.Project{ID: "p1", Root: root, Includes: []string{"**/*.xhtml"}}
	source := NewSource()
	artifacts, err := source.List(context.Background(), project, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Path != "view.xhtml" {
		t.Fatalf("unexpected artifacts: %#v", artifacts)
	}
	if artifacts[0].ContentHash == "" {
		t.Fatal("content hash missing")
	}
	if _, err := source.Read(context.Background(), project, artifacts[0]); err != nil {
		t.Fatal(err)
	}
}

func TestSourceRejectsTraversalAndExcludesDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "private"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "private", "secret.xhtml"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "view.xhtml"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := domain.Project{ID: "p1", Root: root, Excludes: []string{"private"}}
	artifacts, err := NewSource().List(context.Background(), project, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Path != "view.xhtml" {
		t.Fatalf("excluded file indexed: %#v", artifacts)
	}
	if _, err := NewSource().Read(context.Background(), project, domain.Artifact{Path: "../outside"}); err == nil {
		t.Fatal("expected traversal rejection")
	}
}
