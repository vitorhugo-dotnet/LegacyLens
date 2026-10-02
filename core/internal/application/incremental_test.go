package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"legacylens/core/internal/adapters/filesystem"
	"legacylens/core/internal/adapters/sqlite"
	"legacylens/core/internal/adapters/static/el"
	"legacylens/core/internal/adapters/static/javascript"
	"legacylens/core/internal/adapters/static/xhtml"
	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

type javaMethodAnalyzer struct{}

func (javaMethodAnalyzer) Capabilities() []application.Capability { return nil }
func (javaMethodAnalyzer) Analyze(_ context.Context, input application.AnalysisInput) (application.AnalysisResult, error) {
	for _, artifact := range input.Artifacts {
		if artifact.Language == "java" {
			return application.AnalysisResult{Symbols: []domain.Symbol{{ID: domain.ID("method-" + string(input.RevisionID)), ProjectID: input.ProjectID, RevisionID: input.RevisionID, ArtifactID: artifact.ID, Path: artifact.Path, QualifiedName: "com.example.OrdersBean.save", Kind: "method"}}}, nil
		}
	}
	return application.AnalysisResult{}, nil
}

func TestResolveManagedBeanAndRemoteCommand(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	files := map[string]string{
		"handlers.js": `function first(){ second(); } function second(){ remoteSave(); } // remoteSave();
const ignored = "second()";`,
		"view.xhtml":      `<html xmlns:h="h" xmlns:p="p"><h:commandButton onclick="first()"/><p:remoteCommand name="remoteSave" action="#{orders.save}"/></html>`,
		"OrdersBean.java": `class OrdersBean { void save() {} }`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := domain.Project{ID: "p", Root: root}
	if err := store.SaveProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	indexer := application.NewIndexer(store, store, filesystem.NewSource(), []application.Analyzer{javascript.NewAnalyzer(), xhtml.NewAnalyzer(), javaMethodAnalyzer{}}).WithExpressionResolver(el.NewResolver())
	result, err := indexer.Update(ctx, application.IndexRequest{ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	symbols := map[domain.ID]string{}
	for _, symbol := range result.Symbols {
		symbols[symbol.ID] = symbol.QualifiedName
	}
	evidence := map[domain.ID]string{}
	for _, item := range result.Evidence {
		evidence[item.ID] = item.Source
	}
	steps := map[string]string{"first": "second", "second": "remoteSave", "remoteSave": "com.example.OrdersBean.save"}
	for from, to := range steps {
		found := false
		for _, relation := range result.Relations {
			if symbols[relation.FromID] != from || relation.ToID == nil || symbols[*relation.ToID] != to {
				continue
			}
			if relation.Resolution != domain.ResolutionResolved || len(relation.EvidenceIDs) != 1 || evidence[relation.EvidenceIDs[0]] == "" {
				t.Fatalf("unsupported hop: %#v", relation)
			}
			found = true
		}
		if !found {
			t.Fatalf("missing %s -> %s; relations = %#v", from, to, result.Relations)
		}
	}
	if len(result.Relations) != 4 {
		t.Fatalf("comment/string created a relation: %#v", result.Relations)
	}
}

func TestIncrementalDeleteAndChangedDependency(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	js := filepath.Join(root, "handlers.js")
	view := filepath.Join(root, "view.xhtml")
	for path, body := range map[string]string{js: "function first(){ second(); } function second(){ remoteSave(); }", view: `<html xmlns:p="p"><p:remoteCommand name="remoteSave" action="#{orders.save}"/></html>`} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := domain.Project{ID: "p", Root: root}
	if err := store.SaveProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	indexer := application.NewIndexer(store, store, filesystem.NewSource(), []application.Analyzer{javascript.NewAnalyzer(), xhtml.NewAnalyzer()})
	first, err := indexer.Update(ctx, application.IndexRequest{ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Relations) < 3 {
		t.Fatalf("incomplete browser chain: %#v", first.Relations)
	}
	for _, relation := range first.Relations {
		if len(relation.EvidenceIDs) == 0 {
			t.Fatalf("relation lacks evidence: %#v", relation)
		}
	}
	if err := store.StartCapture(ctx, domain.Trace{ID: "historic-trace", ProjectID: project.ID, StartedAt: time.Now().UTC()}, "tab"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(js, []byte("function first(){ renamed(); } function renamed(){}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(view); err != nil {
		t.Fatal(err)
	}
	second, err := indexer.Update(ctx, application.IndexRequest{ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if first.RevisionID == second.RevisionID {
		t.Fatal("changed files kept the old revision")
	}
	if len(second.ReplacedFiles) != 1 || second.ReplacedFiles[0] != "handlers.js" || len(second.ExcludedFiles) != 1 || second.ExcludedFiles[0] != "view.xhtml" {
		t.Fatalf("incremental file changes = replaced %#v, excluded %#v", second.ReplacedFiles, second.ExcludedFiles)
	}
	for _, symbol := range second.Symbols {
		if strings.Contains(symbol.QualifiedName, "second") || strings.Contains(symbol.QualifiedName, "remoteSave") {
			t.Fatalf("stale symbol: %#v", symbol)
		}
	}
	for _, relation := range second.Relations {
		if relation.Kind == "action" || relation.Kind == "remote-command" {
			t.Fatalf("stale relation: %#v", relation)
		}
	}
	old, err := store.Explore(ctx, application.GraphQuery{ProjectID: project.ID, RevisionID: first.RevisionID, SymbolIDs: []domain.ID{first.Symbols[0].ID}})
	if err != nil || len(old.Relations) == 0 {
		t.Fatalf("historic snapshot missing: %#v, %v", old, err)
	}
	current, err := store.LoadInvestigationIndex(ctx, project.ID)
	if err != nil || current.RevisionID != second.RevisionID || len(current.Relations) != len(second.Relations) {
		t.Fatalf("latest graph retained stale relations: %#v, %v", current, err)
	}
	trace, err := store.Load(ctx, project.ID, "historic-trace")
	if err != nil || trace.Trace.ID != "historic-trace" {
		t.Fatalf("trace history was lost: %#v, %v", trace, err)
	}
}

type changingSource struct{ reads int }

func (s *changingSource) List(context.Context, domain.Project, []string) ([]domain.Artifact, error) {
	h := sha256.Sum256([]byte("before"))
	return []domain.Artifact{{Path: "view.xhtml", Language: "xhtml", ContentHash: hex.EncodeToString(h[:])}}, nil
}
func (s *changingSource) Read(context.Context, domain.Project, domain.Artifact) ([]byte, error) {
	s.reads++
	return []byte("after"), nil
}

func TestIndexFileChangedDuringRead(t *testing.T) {
	project := domain.Project{ID: "p", Root: t.TempDir()}
	store := &storeStub{}
	source := &changingSource{}
	indexer := application.NewIndexer(store, projectStoreStub{project: &project}, source, nil)
	result, err := indexer.Update(context.Background(), application.IndexRequest{ProjectID: project.ID})
	if err == nil && len(result.Diagnostics) == 0 {
		t.Fatalf("inconsistent revision committed: %#v", result)
	}
	if store.result.RevisionID != "" {
		t.Fatalf("inconsistent revision persisted: %#v", store.result)
	}
}

func TestIncrementalWindowsJunctionCannotReadOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := "outside-project-only-content"
	if err := os.WriteFile(filepath.Join(outside, "secret.xhtml"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if runtime.GOOS == "windows" {
		output, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput()
		if err != nil {
			t.Skipf("PENDING: Windows junction fixture could not be created: %v (%s)", err, output)
		}
	} else if err := os.Symlink(outside, link); err != nil {
		t.Skipf("PENDING: symlink fixture could not be created: %v", err)
	}
	project := domain.Project{ID: "p", Root: root}
	source := filesystem.NewSource()
	artifacts, listErr := source.List(context.Background(), project, nil)
	for _, artifact := range artifacts {
		if strings.Contains(artifact.Path, "secret") {
			t.Fatalf("outside artifact enumerated: %#v", artifact)
		}
		content, err := source.Read(context.Background(), project, artifact)
		if err == nil && strings.Contains(string(content), secret) {
			t.Fatal("outside content read through enumeration")
		}
	}
	content, readErr := source.Read(context.Background(), project, domain.Artifact{Path: "linked/secret.xhtml"})
	if readErr == nil || strings.Contains(string(content), secret) {
		t.Fatalf("outside content read through junction: %q, %v (list error: %v)", content, readErr, listErr)
	}
}
