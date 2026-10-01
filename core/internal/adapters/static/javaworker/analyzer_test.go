package javaworker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

func TestJSONLWorkerHandlesPathWithSpaces(t *testing.T) {
	t.Setenv("GO_WANT_JAVA_WORKER_HELPER", "1")
	worker := New(WorkerConfig{Command: os.Args[0], Args: []string{"-test.run=TestWorkerHelper"}, Timeout: time.Second, Classpath: []string{"one" + string(os.PathListSeparator) + "two"}})
	result, err := worker.Analyze(context.Background(), application.AnalysisInput{ProjectID: "p", RevisionID: "r", Root: `C:\a project`, Artifacts: []domain.Artifact{{ID: "a", Path: "source with spaces.java", Language: "java"}}, Sources: map[string][]byte{"source with spaces.java": []byte("class Example {}")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Symbols) != 1 || result.Symbols[0].Path != "source with spaces.java" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestWorkerCancellation(t *testing.T) {
	t.Setenv("GO_WANT_JAVA_WORKER_HELPER", "1")
	worker := New(WorkerConfig{Command: os.Args[0], Args: []string{"-test.run=TestWorkerHelper"}, Timeout: 20 * time.Millisecond})
	_, err := worker.Analyze(context.Background(), application.AnalysisInput{Root: "sleep"})
	if err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected deadline, got %v", err)
	}
}

func TestWorkerHelper(t *testing.T) {
	if os.Getenv("GO_WANT_JAVA_WORKER_HELPER") != "1" {
		return
	}
	var input struct {
		Root      string            `json:"root"`
		Sources   map[string]string `json:"sources"`
		Classpath []string          `json:"classpath"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		os.Exit(2)
	}
	if input.Root == "sleep" {
		time.Sleep(time.Second)
		os.Exit(0)
	}
	if input.Sources["source with spaces.java"] != "class Example {}" {
		os.Exit(3)
	}
	if input.Classpath[0] != "one"+string(os.PathListSeparator)+"two" {
		os.Exit(4)
	}
	fmt.Fprintln(os.Stdout, `{"symbols":[{"id":"s","path":"source with spaces.java"}],"relations":[],"evidence":[],"diagnostics":[]}`)
	os.Exit(0)
}
