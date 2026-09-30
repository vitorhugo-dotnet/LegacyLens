package application

import (
	"encoding/json"
	"strings"
	"testing"

	"legacylens/core/internal/domain"
)

func TestApplicationDTOsUseCamelCaseJSONFields(t *testing.T) {
	indexJSON, err := json.Marshal(IndexResult{ProjectID: "p1", RevisionID: "r1", ReplacedFiles: []string{"view.xhtml"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"projectId":"p1"`, `"revisionId":"r1"`, `"replacedFiles":["view.xhtml"]`} {
		if !strings.Contains(string(indexJSON), key) {
			t.Fatalf("IndexResult JSON %s is missing %s", indexJSON, key)
		}
	}
	if strings.Contains(string(indexJSON), `"ProjectID"`) {
		t.Fatalf("IndexResult JSON leaked Go field names: %s", indexJSON)
	}

	openJSON, err := json.Marshal(OpenResult{Opened: true, File: `C:\source\view.xhtml`, Line: 12})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(openJSON), `"file":"C:\\source\\view.xhtml"`) || !strings.Contains(string(openJSON), `"line":12`) {
		t.Fatalf("OpenResult JSON has unexpected keys: %s", openJSON)
	}
}

func TestIndexAndGraphPagesAreStableAndReportLaterPages(t *testing.T) {
	index := IndexResult{ProjectID: "p1", RevisionID: "r1", Symbols: []domain.Symbol{{ID: "s3"}, {ID: "s1"}, {ID: "s2"}}, ReplacedFiles: []string{"z.xhtml", "a.xhtml"}}
	page := PageIndexResult(index, 1, 1)
	if page.Symbols.Items[0].ID != "s2" || !page.Symbols.HasMore || page.Symbols.Total != 3 || page.Symbols.Offset != 1 {
		t.Fatalf("index symbol page = %+v", page.Symbols)
	}
	if page.ReplacedFiles.Items[0] != "z.xhtml" || page.ReplacedFiles.HasMore {
		t.Fatalf("index path page = %+v", page.ReplacedFiles)
	}
	graph := PageGraphResult(GraphResult{Symbols: index.Symbols, Relations: []domain.Relation{{ID: "r3"}, {ID: "r2"}, {ID: "r1"}}}, 1, 1)
	if graph.Symbols.Items[0].ID != "s2" || graph.Relations.Items[0].ID != "r2" || !graph.Relations.HasMore {
		t.Fatalf("graph pages = %+v", graph)
	}
}
