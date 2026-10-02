package javascript

import (
	"context"
	"strings"
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

func TestResolveSemicolonlessArrowOwnership(t *testing.T) {
	source := []byte("const first = () => second()\nconst second = () => fetch('/x')\n")
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "js", Path: "handlers.js", Language: "javascript"}},
		Sources: map[string][]byte{"handlers.js": source},
	})
	if err != nil {
		t.Fatal(err)
	}
	names := map[domain.ID]string{}
	for _, symbol := range result.Symbols {
		names[symbol.ID] = symbol.QualifiedName
	}
	if len(result.Relations) != 2 {
		t.Fatalf("ASI created false edges: %#v", result.Relations)
	}
	for _, relation := range result.Relations {
		switch names[relation.FromID] {
		case "first":
			if relation.Kind != "calls" || relation.ToID == nil || names[*relation.ToID] != "second" {
				t.Fatalf("first owns wrong edge: %#v", relation)
			}
		case "second":
			if relation.Kind != "request" {
				t.Fatalf("second owns wrong edge: %#v", relation)
			}
		default:
			t.Fatalf("unexpected owner: %#v", relation)
		}
	}
}

func TestResolveAnonymousEventCallback(t *testing.T) {
	source := []byte("function wire() { button.addEventListener('click', () => save()); }\nfunction save() {}")
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "js", Path: "handlers.js", Language: "javascript"}},
		Sources: map[string][]byte{"handlers.js": source},
	})
	if err != nil {
		t.Fatal(err)
	}
	names := map[domain.ID]string{}
	kinds := map[domain.ID]string{}
	evidence := map[domain.ID]string{}
	for _, symbol := range result.Symbols {
		names[symbol.ID], kinds[symbol.ID] = symbol.QualifiedName, symbol.Kind
	}
	for _, item := range result.Evidence {
		evidence[item.ID] = item.Source
	}
	if len(result.Relations) != 2 {
		t.Fatalf("anonymous callback edges = %#v", result.Relations)
	}
	var callback domain.ID
	for _, relation := range result.Relations {
		if relation.Kind == "handler" && names[relation.FromID] == "wire" && relation.ToID != nil && kinds[*relation.ToID] == "javascript-callback" && relation.Resolution == domain.ResolutionResolved && len(relation.EvidenceIDs) == 1 && strings.Contains(evidence[relation.EvidenceIDs[0]], "() => save()") {
			callback = *relation.ToID
		}
	}
	if callback == "" {
		t.Fatalf("handler has no source-backed callback target: %#v", result.Relations)
	}
	for _, relation := range result.Relations {
		if relation.Kind == "calls" && relation.FromID == callback && relation.ToID != nil && names[*relation.ToID] == "save" && len(relation.EvidenceIDs) == 1 {
			return
		}
	}
	t.Fatalf("callback call was attributed to the wrong owner: %#v", result.Relations)
}

func TestResolveTopLevelAnonymousEventCallback(t *testing.T) {
	source := []byte("button.addEventListener('click', () => save())\nfunction save() {}")
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "js", Path: "handlers.js", Language: "javascript"}},
		Sources: map[string][]byte{"handlers.js": source},
	})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[domain.ID]string{}
	for _, symbol := range result.Symbols {
		kinds[symbol.ID] = symbol.Kind
	}
	for _, relation := range result.Relations {
		if relation.Kind == "handler" && kinds[relation.FromID] == "javascript-module" && relation.ToID != nil && kinds[*relation.ToID] == "javascript-callback" && len(relation.EvidenceIDs) == 1 {
			return
		}
	}
	t.Fatalf("top-level handler registration was lost: symbols=%#v relations=%#v", result.Symbols, result.Relations)
}

func TestResolveTemplateInterpolationDiagnostic(t *testing.T) {
	source := []byte("const first = () => `result ${fetch('/x')}`\n")
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "js", Path: "handlers.js", Language: "javascript"}},
		Sources: map[string][]byte{"handlers.js": source},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "javascript.unsupported_syntax" && diagnostic.Location != nil && diagnostic.Location.Path == "handlers.js" && diagnostic.Location.Line == 1 {
			return
		}
	}
	t.Fatalf("template interpolation was silently omitted: %#v", result.Diagnostics)
}
