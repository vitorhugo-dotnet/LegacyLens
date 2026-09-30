package application

import (
	"encoding/json"
	"strings"
	"testing"
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
