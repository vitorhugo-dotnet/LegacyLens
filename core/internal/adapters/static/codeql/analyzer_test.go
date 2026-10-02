package codeql

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

type indexStore struct{ result application.IndexResult }

func (s *indexStore) CommitIndex(_ context.Context, result application.IndexResult) error {
	s.result = result
	return nil
}
func (s *indexStore) Search(context.Context, application.SearchQuery) (application.SearchResult, error) {
	return application.SearchResult{}, nil
}
func (s *indexStore) Explore(context.Context, application.GraphQuery) (application.GraphResult, error) {
	return application.GraphResult{}, nil
}

type projectStore struct{ project domain.Project }

func (s projectStore) SaveProject(context.Context, domain.Project) error { return nil }
func (s projectStore) LoadProject(context.Context, domain.ID) (domain.Project, error) {
	return s.project, nil
}

type source struct{ body []byte }

func (s source) List(_ context.Context, project domain.Project, _ []string) ([]domain.Artifact, error) {
	return []domain.Artifact{{ProjectID: project.ID, Path: "src/Main.java", Language: "java", Origin: "filesystem"}}, nil
}
func (s source) Read(context.Context, domain.Project, domain.Artifact) ([]byte, error) {
	return s.body, nil
}

type basicAnalyzer struct{}

func (basicAnalyzer) Capabilities() []application.Capability { return nil }
func (basicAnalyzer) Analyze(_ context.Context, input application.AnalysisInput) (application.AnalysisResult, error) {
	return application.AnalysisResult{Symbols: []domain.Symbol{{ID: "basic-symbol", ProjectID: input.ProjectID, RevisionID: input.RevisionID, Path: "src/Main.java", QualifiedName: "Main", Kind: "class"}}}, nil
}

func TestCodeQLUnavailableLeavesBasicAnalysisUsable(t *testing.T) {
	project := domain.Project{ID: "p", Root: t.TempDir()}
	store := &indexStore{}
	codeqlAnalyzer := New(Config{Enabled: true, Authorized: true, Executable: filepath.Join(t.TempDir(), "missing-codeql")})
	indexer := application.NewIndexer(store, projectStore{project}, source{body: []byte("class Main {}")}, []application.Analyzer{basicAnalyzer{}, codeqlAnalyzer})
	result, err := indexer.Index(context.Background(), application.IndexRequest{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("indexing failed when optional analyzer was unavailable: %v", err)
	}
	if len(result.Symbols) == 0 || result.Symbols[0].ID != "basic-symbol" {
		t.Fatalf("basic analysis result was lost: %+v", result.Symbols)
	}
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "codeql.cli_unavailable" {
		t.Fatalf("missing unavailable diagnostic: %+v", result.Diagnostics)
	}
	if strings.Contains(strings.ToLower(result.Diagnostics[0].Message), "missing-codeql") {
		t.Fatalf("diagnostic disclosed executable path: %q", result.Diagnostics[0].Message)
	}
	if len(store.result.Symbols) == 0 {
		t.Fatal("index was not committed")
	}
}

func TestCodeQLRequiresExplicitEnablement(t *testing.T) {
	for name, config := range map[string]Config{
		"disabled":     {Executable: filepath.Join(t.TempDir(), "must-not-run")},
		"unauthorized": {Enabled: true, Executable: filepath.Join(t.TempDir(), "must-not-run")},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := New(config).Analyze(context.Background(), application.AnalysisInput{})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "codeql.explicit_authorization_required" {
				t.Fatalf("expected opt-in diagnostic: %+v", result.Diagnostics)
			}
		})
	}
}

func TestDecodeCallRowsKeepsLocation(t *testing.T) {
	data, err := os.ReadFile("testdata/calls.bqrs.json")
	if err != nil {
		t.Fatal(err)
	}
	input := application.AnalysisInput{ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "caller", Path: "src/OrderService.java"}, {ID: "callee", Path: "src/OrderDao.java"}}}
	result, err := decodeCalls(data, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Relations) != 1 || result.Relations[0].Kind != "calls" {
		t.Fatalf("expected one call relation: %+v", result.Relations)
	}
	location := result.Relations[0].Location
	if location == nil || location.Path != "src/OrderService.java" || location.Line != 27 || location.Column != 9 {
		t.Fatalf("call site location lost: %+v", location)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Location == nil || result.Evidence[0].Location.Line != 27 {
		t.Fatalf("call evidence location lost: %+v", result.Evidence)
	}
}

func TestDecodeCallRowsRejectsUnknownArtifactPath(t *testing.T) {
	data, err := os.ReadFile("testdata/calls.bqrs.json")
	if err != nil {
		t.Fatal(err)
	}
	// Path validation is exercised by a query result pointing outside the indexed source set.
	input := application.AnalysisInput{ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "caller", Path: "other.java"}}}
	if _, err := decodeCalls(data, input); err == nil {
		t.Fatal("expected unindexed source path to be rejected")
	}
}
