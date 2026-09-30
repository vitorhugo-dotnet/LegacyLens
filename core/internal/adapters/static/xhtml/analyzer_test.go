package xhtml

import (
	"context"
	"strings"
	"testing"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

func TestIndexXHTMLActionEvidence(t *testing.T) {
	body := "<html>\n  <h:commandButton action=\"#{orders.save}\"/>\n</html>"
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "project", RevisionID: "revision", Root: ".",
		Artifacts: []domain.Artifact{{ID: "artifact", ProjectID: "project", RevisionID: "revision", Path: "view.xhtml", Language: "xhtml", Origin: "filesystem"}},
		Sources:   map[string][]byte{"view.xhtml": []byte(body)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Relations) != 1 {
		t.Fatalf("expected one action relation, got %d", len(result.Relations))
	}
	relation := result.Relations[0]
	if relation.Kind != "action" || relation.Resolution != domain.ResolutionDynamic || relation.ToID == nil {
		t.Fatalf("unexpected action relation: %#v", relation)
	}
	if relation.Location == nil || relation.Location.Line != 2 {
		t.Fatalf("action line not preserved: %#v", relation.Location)
	}
	if len(relation.EvidenceIDs) != 1 || len(result.Evidence) != 1 || result.Evidence[0].Source != "#{orders.save}" {
		t.Fatalf("action evidence missing: %#v", result.Evidence)
	}
}

func TestAnalyzerDoesNotResolveExternalEntities(t *testing.T) {
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "a", Path: "view.xhtml", Language: "xhtml"}},
		Sources: map[string][]byte{"view.xhtml": []byte(`<!DOCTYPE x [<!ENTITY e SYSTEM "file:///secret">]><x>&e;</x>`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range result.Evidence {
		if strings.Contains(e.Source, "secret") {
			t.Fatal("external entity content leaked")
		}
	}
}
