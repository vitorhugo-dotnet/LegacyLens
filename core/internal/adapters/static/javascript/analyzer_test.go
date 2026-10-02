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
