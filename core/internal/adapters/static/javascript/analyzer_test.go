package javascript

import (
	"context"
	"testing"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

func TestResolveManagedBeanAndRemoteCommand(t *testing.T) {
	source := []byte(`function first() { second(); }
function second() { remoteSave(); }
// remoteSave();
const example = "remoteSave()";
`)
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r",
		Artifacts: []domain.Artifact{{ID: "js", Path: "handlers.js", Language: "javascript"}},
		Sources:   map[string][]byte{"handlers.js": source},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Relations) != 2 {
		t.Fatalf("expected two real call edges, got %#v", result.Relations)
	}
	for _, relation := range result.Relations {
		if relation.Kind != "calls" || len(relation.EvidenceIDs) != 1 || relation.Location == nil {
			t.Fatalf("call lacks evidence: %#v", relation)
		}
	}
}

func TestResolveHandlerAndRequest(t *testing.T) {
	source := []byte(`function first() { button.addEventListener("click", second); fetch("/orders"); }
function second() {}
// fetch("/ignored")
const ignored = "button.addEventListener('click', second)";`)
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "js", Path: "handlers.js", Language: "javascript"}},
		Sources: map[string][]byte{"handlers.js": source},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Relations) != 2 {
		t.Fatalf("handler/request edges = %#v", result.Relations)
	}
	kinds := map[string]bool{}
	for _, relation := range result.Relations {
		kinds[relation.Kind] = true
		if len(relation.EvidenceIDs) != 1 {
			t.Fatalf("edge lacks evidence: %#v", relation)
		}
	}
	if !kinds["handler"] || !kinds["request"] {
		t.Fatalf("handler/request edge missing: %#v", result.Relations)
	}
}

func TestResolveExpressionBodiedArrow(t *testing.T) {
	source := []byte(`const first = () => second();
const second = () => fetch("/orders");
const wire = () => button.addEventListener("click", first);
// second();
const ignored = "fetch('/ignored')";`)
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "js", Path: "handlers.js", Language: "javascript"}},
		Sources: map[string][]byte{"handlers.js": source},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Symbols) != 3 || len(result.Relations) != 3 {
		t.Fatalf("expression arrows lost symbols or created false calls: symbols=%#v relations=%#v", result.Symbols, result.Relations)
	}
	names := map[domain.ID]string{}
	for _, symbol := range result.Symbols {
		names[symbol.ID] = symbol.QualifiedName
	}
	kinds := map[string]bool{}
	for _, relation := range result.Relations {
		if relation.ToID == nil || len(relation.EvidenceIDs) != 1 || relation.Location == nil {
			t.Fatalf("arrow edge lacks target/evidence: %#v", relation)
		}
		kinds[relation.Kind] = true
		if names[relation.FromID] == "first" && (names[*relation.ToID] != "second" || relation.Resolution != domain.ResolutionResolved) {
			t.Fatalf("first must call second: %#v", relation)
		}
	}
	if !kinds["calls"] || !kinds["request"] || !kinds["handler"] {
		t.Fatalf("arrow call classifications missing: %#v", result.Relations)
	}
}
