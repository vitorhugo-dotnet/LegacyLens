package javaworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
	"unicode/utf8"

	"legacylens/core/internal/application"
)

// WorkerConfig identifies the Java worker executable and its bounded run time.
// For a shaded jar, use Command="java" and Args=[]string{"-jar", jarPath}.
type WorkerConfig struct {
	Command     string
	Args        []string
	Timeout     time.Duration
	SourceRoots []string
	Classpath   []string
}

type Analyzer struct{ config WorkerConfig }

var _ application.Analyzer = (*Analyzer)(nil)

func New(config WorkerConfig) *Analyzer { return &Analyzer{config: config} }

func (a *Analyzer) Capabilities() []application.Capability {
	return []application.Capability{
		{Name: "java-calls", Supported: true, Description: "JavaParser AST and configured symbol roots through Java 21."},
		{Name: "java-syntax-after-21", Supported: false, Description: "Parser language level is Java 21."},
		{Name: "sql-dependencies", Supported: true, Description: "JSqlParser statement AST."},
	}
}

func (a *Analyzer) Analyze(ctx context.Context, input application.AnalysisInput) (application.AnalysisResult, error) {
	if err := ctx.Err(); err != nil {
		return application.AnalysisResult{}, err
	}
	if a.config.Command == "" {
		return application.AnalysisResult{}, errors.New("Java analyzer command is missing")
	}
	sources := make(map[string]string, len(input.Sources))
	for path, data := range input.Sources {
		if !utf8.Valid(data) {
			return application.AnalysisResult{}, fmt.Errorf("invalid UTF-8 source at %q", path)
		}
		sources[path] = string(data)
	}
	envelope := struct {
		ProjectID   string            `json:"projectId"`
		RevisionID  string            `json:"revisionId"`
		Root        string            `json:"root"`
		Artifacts   any               `json:"artifacts"`
		Sources     map[string]string `json:"sources"`
		SourceRoots []string          `json:"sourceRoots,omitempty"`
		Classpath   []string          `json:"classpath,omitempty"`
	}{string(input.ProjectID), string(input.RevisionID), input.Root, input.Artifacts, sources, a.config.SourceRoots, a.config.Classpath}
	request, err := json.Marshal(envelope)
	if err != nil {
		return application.AnalysisResult{}, err
	}
	request = append(request, '\n')
	runCtx := ctx
	cancel := func() {}
	if a.config.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, a.config.Timeout)
	}
	defer cancel()
	command := exec.CommandContext(runCtx, a.config.Command, a.config.Args...)
	command.Stdin = bytes.NewReader(request)
	var output bytes.Buffer
	command.Stdout = &boundedWriter{writer: &output, limit: 16 << 20}
	// Worker stderr may contain source literals or parser fragments; do not return it.
	command.Stderr = io.Discard
	err = command.Run()
	if runCtx.Err() != nil {
		return application.AnalysisResult{}, runCtx.Err()
	}
	if err != nil {
		return application.AnalysisResult{}, fmt.Errorf("Java analyzer process failed: %w", err)
	}
	line, rest, ok := bytes.Cut(output.Bytes(), []byte{'\n'})
	if !ok || len(bytes.TrimSpace(rest)) != 0 {
		return application.AnalysisResult{}, errors.New("Java analyzer returned invalid JSONL framing")
	}
	var result application.AnalysisResult
	if err := json.Unmarshal(line, &result); err != nil {
		return application.AnalysisResult{}, errors.New("Java analyzer returned invalid result JSON")
	}
	return result, nil
}

type boundedWriter struct {
	writer *bytes.Buffer
	limit  int
}

func (w *boundedWriter) Write(value []byte) (int, error) {
	if w.writer.Len()+len(value) > w.limit {
		return 0, errors.New("Java analyzer response exceeded limit")
	}
	return w.writer.Write(value)
}
