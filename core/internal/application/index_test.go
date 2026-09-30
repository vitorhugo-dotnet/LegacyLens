package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"legacylens/core/internal/adapters/static/xhtml"
	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

type sourceStub struct {
	files map[string][]byte
}

func (s sourceStub) List(_ context.Context, project domain.Project, _ []string) ([]domain.Artifact, error) {
	artifacts := make([]domain.Artifact, 0, len(s.files))
	for path := range s.files {
		artifacts = append(artifacts, domain.Artifact{ID: domain.ID(path), ProjectID: project.ID, Path: path, Language: "xhtml", Origin: "filesystem"})
	}
	return artifacts, nil
}
func (s sourceStub) Read(_ context.Context, _ domain.Project, artifact domain.Artifact) ([]byte, error) {
	return s.files[artifact.Path], nil
}

type storeStub struct {
	result application.IndexResult
	search application.SearchResult
	graph  application.GraphResult
}

func (s *storeStub) CommitIndex(_ context.Context, result application.IndexResult) error {
	s.result = result
	return nil
}
func (s *storeStub) Search(context.Context, application.SearchQuery) (application.SearchResult, error) {
	return s.search, nil
}
func (s *storeStub) Explore(context.Context, application.GraphQuery) (application.GraphResult, error) {
	return s.graph, nil
}

func TestIndexerExposesSearchAndGraphUseCases(t *testing.T) {
	project := domain.Project{ID: "p1"}
	projects := projectStoreStub{project: &project}
	wantSearch := application.SearchResult{Total: 1, Symbols: []domain.Symbol{{ID: "symbol-1"}}}
	wantGraph := application.GraphResult{Symbols: []domain.Symbol{{ID: "symbol-1"}}}
	store := &storeStub{search: wantSearch, graph: wantGraph}
	indexer := application.NewIndexer(store, projects, sourceStub{}, nil)
	gotSearch, err := indexer.Search(context.Background(), application.SearchQuery{ProjectID: "p1", Text: "save"})
	if err != nil || gotSearch.Total != 1 || gotSearch.Symbols[0].ID != "symbol-1" {
		t.Fatalf("Search() = %+v, %v", gotSearch, err)
	}
	gotGraph, err := indexer.Explore(context.Background(), application.GraphQuery{ProjectID: "p1", SymbolIDs: []domain.ID{"symbol-1"}})
	if err != nil || len(gotGraph.Symbols) != 1 || gotGraph.Symbols[0].ID != "symbol-1" {
		t.Fatalf("Explore() = %+v, %v", gotGraph, err)
	}
}

type projectStoreStub struct{ project *domain.Project }

func (s projectStoreStub) SaveProject(_ context.Context, p domain.Project) error {
	*s.project = p
	return nil
}
func (s projectStoreStub) LoadProject(_ context.Context, id domain.ID) (domain.Project, error) {
	if s.project.ID != id {
		return domain.Project{}, errors.New("project not found")
	}
	return *s.project, nil
}

func TestProjectPathWithSpacesAndAccents(t *testing.T) {
	var saved domain.Project
	projects := projectStoreStub{project: &saved}
	root := filepath.Join(t.TempDir(), "Projetos", "Gestão Legada")
	if runtime.GOOS == "windows" {
		root = `C:\Projetos\Gestão Legada`
	}
	project, err := application.NewProjectService(projects).RegisterProject(context.Background(), application.ProjectConfig{Root: root, Includes: []string{"**/*.xhtml"}, Excludes: []string{"vendor"}})
	if err != nil {
		t.Fatal(err)
	}
	if project.Root != root {
		t.Fatalf("project path changed: got %q, want %q", project.Root, root)
	}
	if saved.ID != project.ID || saved.Root != project.Root {
		t.Fatalf("project was not persisted: %#v", saved)
	}
	if len(saved.Includes) != 1 || saved.Includes[0] != "**/*.xhtml" || len(saved.Excludes) != 1 || saved.Excludes[0] != "vendor" {
		t.Fatalf("path policy not preserved: %#v", saved)
	}
}

func TestIndexXHTMLActionEvidence(t *testing.T) {
	body := "<html>\n  <h:commandButton action=\"#{orders.save}\"/>\n</html>"
	project := domain.Project{ID: "p1", Root: t.TempDir()}
	projects := projectStoreStub{project: &project}
	store := &storeStub{}
	source := sourceStub{files: map[string][]byte{"view.xhtml": []byte(body)}}
	indexer := application.NewIndexer(store, projects, source, []application.Analyzer{xhtml.NewAnalyzer()})
	result, err := indexer.Index(context.Background(), application.IndexRequest{ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Relations) != 1 {
		t.Fatalf("expected one action relation, got %d", len(result.Relations))
	}
	relation := result.Relations[0]
	if relation.Kind != "action" || relation.Resolution != domain.ResolutionDynamic || relation.ToID == nil {
		t.Fatalf("unexpected relation: %#v", relation)
	}
	if relation.Location == nil || relation.Location.Line != 2 {
		t.Fatalf("action line not preserved: %#v", relation.Location)
	}
	if len(relation.EvidenceIDs) != 1 || len(result.Evidence) != 1 {
		t.Fatalf("action evidence missing: %#v", result.Evidence)
	}
	if store.result.RevisionID != result.RevisionID {
		t.Fatal("indexer did not commit result")
	}
	digest := sha256.Sum256([]byte(body))
	if len(result.Artifacts) != 1 || result.Artifacts[0].ContentHash != hex.EncodeToString(digest[:]) {
		t.Fatalf("source hash missing or incorrect: %#v", result.Artifacts)
	}
}
