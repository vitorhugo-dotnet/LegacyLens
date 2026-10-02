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

func TestResolveRequestInsideTemplateInterpolation(t *testing.T) {
	source := []byte("const first = () => `result ${fetch('/x')}`\n")
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "js", Path: "handlers.js", Language: "javascript"}},
		Sources: map[string][]byte{"handlers.js": source},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Relations) != 1 || result.Relations[0].Kind != "request" || len(result.Evidence) != 1 {
		t.Fatalf("template expression call was omitted: relations=%#v evidence=%#v diagnostics=%#v", result.Relations, result.Evidence, result.Diagnostics)
	}
}

func TestResolveTopLevelRequest(t *testing.T) {
	source := []byte("fetch('/api')\n")
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "js", Path: "handlers.js", Language: "javascript"}},
		Sources: map[string][]byte{"handlers.js": source},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Relations) != 1 || result.Relations[0].Kind != "request" {
		t.Fatalf("top-level request missing: symbols=%#v relations=%#v", result.Symbols, result.Relations)
	}
	if len(result.Symbols) != 1 || result.Symbols[0].Kind != "javascript-module" {
		t.Fatalf("top-level request must belong to module: %#v", result.Symbols)
	}
}

func TestResolveFunctionExpressionAndNestedOwnership(t *testing.T) {
	source := []byte("function outer() { function inner() { save(); } fetch('/outer'); }\nconst load = function() { fetch('/inner'); };\nfunction save() {}")
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
	got := map[string][]string{}
	for _, relation := range result.Relations {
		got[names[relation.FromID]] = append(got[names[relation.FromID]], relation.Kind+":"+names[deref(relation.ToID)])
	}
	if len(got["inner"]) != 1 || !strings.Contains(strings.Join(got["inner"], ","), "calls:save") {
		t.Fatalf("nested function must own its call once: relations=%#v owners=%#v", result.Relations, got)
	}
	if len(got["outer"]) != 1 || got["outer"][0] != "request:" {
		t.Fatalf("outer function must not inherit nested calls: %#v", got)
	}
	if len(got["load"]) != 1 || got["load"][0] != "request:" {
		t.Fatalf("anonymous function expression request missing: %#v", got)
	}
}

func TestResolveSemicolonlessArrowBeforeExpressionStatement(t *testing.T) {
	source := []byte("const first = () => second()\nfetch('/later')\nfunction second() {}")
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
		t.Fatalf("ASI expression statement ownership: %#v", result.Relations)
	}
	var firstCalls, moduleRequests int
	for _, relation := range result.Relations {
		if names[relation.FromID] == "first" && relation.Kind == "calls" {
			firstCalls++
		}
		if names[relation.FromID] == "handlers.js" && relation.Kind == "request" {
			moduleRequests++
		}
	}
	if firstCalls != 1 || moduleRequests != 1 {
		t.Fatalf("request after semicolonless arrow assigned incorrectly: symbols=%#v relations=%#v", result.Symbols, result.Relations)
	}
}

func TestParseErrorIncludesSourceLocation(t *testing.T) {
	source := []byte("const valid = 1;\nfunction {\n")
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "js", Path: "handlers.js", Language: "javascript"}},
		Sources: map[string][]byte{"handlers.js": source},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "javascript.parse_error" || result.Diagnostics[0].Location == nil || result.Diagnostics[0].Location.Path != "handlers.js" || result.Diagnostics[0].Location.Line != 2 {
		t.Fatalf("parse error must identify source location: %#v", result.Diagnostics)
	}
}

func deref(id *domain.ID) domain.ID {
	if id == nil {
		return ""
	}
	return *id
}
