package domain

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSymbolIdentityIncludesProjectAndDescriptor(t *testing.T) {
	projectA := ID("project-a")
	projectB := ID("project-b")
	revision := ID("revision-1")

	base := SymbolID(projectA, revision, "src/Loader.java", "Loader.load", "load(int)")
	if otherProject := SymbolID(projectB, revision, "src/Loader.java", "Loader.load", "load(int)"); base == otherProject {
		t.Fatal("symbol identity must include the project")
	}
	if overload := SymbolID(projectA, revision, "src/Loader.java", "Loader.load", "load(String)"); base == overload {
		t.Fatal("symbol identity must include the method descriptor")
	}
}

func TestEventsWithoutJavaDestinationAndRelationsWithoutLocationAreRepresentable(t *testing.T) {
	event := Event{ProjectID: "project-a", TraceID: "0123456789abcdef0123456789abcdef", ProducerID: "browser", EventID: "event-1", Kind: "interaction"}
	if event.JavaDestination != nil {
		t.Fatal("browser interaction must not require a Java destination")
	}

	relation := Relation{ID: "relation-1", FromID: "symbol-a", Kind: "calls", EvidenceIDs: []ID{"evidence-1"}, Resolution: ResolutionResolved, Layer: LayerStatic}
	if relation.ToID != nil || relation.Location != nil {
		t.Fatal("unresolved relation target and location must remain optional")
	}
}

func TestValidInteractionExampleDecodesTraceIngestEnvelope(t *testing.T) {
	data, err := os.ReadFile("../../../contracts/examples/valid-interaction.json")
	if err != nil {
		t.Fatal(err)
	}
	var example struct {
		ProtocolVersion int    `json:"protocolVersion"`
		RequestID       string `json:"requestId"`
		Command         string `json:"command"`
		Payload         struct {
			ProjectID ID     `json:"projectId"`
			Events    []Event `json:"events"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &example); err != nil {
		t.Fatal(err)
	}
	if example.ProtocolVersion != 1 || example.RequestID != "request-1" || example.Command != "trace.ingest" ||
		example.Payload.ProjectID != "project-1" || len(example.Payload.Events) != 1 {
		t.Fatalf("example was not decoded as expected: %+v", example.Payload)
	}
	if event := example.Payload.Events[0]; event.ProjectID != example.Payload.ProjectID || event.Kind != "interaction" {
		t.Fatalf("interaction event was not decoded as expected: %+v", event)
	} else if event.JavaDestination != nil {
		t.Fatal("browser event example must not invent a Java destination")
	}
}
