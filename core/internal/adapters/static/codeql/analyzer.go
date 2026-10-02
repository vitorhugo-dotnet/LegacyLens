// Package codeql provides an experimental, explicitly enabled CodeQL adapter.
package codeql

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

type ExtractionMode string

const (
	ExtractionExisting ExtractionMode = "existing"
	ExtractionCreate   ExtractionMode = "create"
	maxOutputBytes                    = 32 << 20
)

// Config requires both Enabled and Authorized before the external CLI can run.
// BuildArgs is passed as argv to CodeQL without shell interpretation.
type Config struct {
	Enabled        bool
	Authorized     bool
	Executable     string
	ExtractionMode ExtractionMode
	DatabasePath   string
	QueryDirectory string
	BuildArgs      []string
	Timeout        time.Duration
}

type Analyzer struct{ config Config }

var _ application.Analyzer = (*Analyzer)(nil)

func New(config Config) *Analyzer {
	if config.Timeout <= 0 {
		config.Timeout = 2 * time.Minute
	}
	return &Analyzer{config: config}
}

func (a *Analyzer) Capabilities() []application.Capability {
	active := a != nil && a.config.Enabled && a.config.Authorized
	return []application.Capability{{Name: "codeql-java-calls", Supported: active, Description: "Experimental direct Java call graph from an explicitly configured CodeQL CLI."}}
}

func (a *Analyzer) Analyze(ctx context.Context, input application.AnalysisInput) (application.AnalysisResult, error) {
	if err := ctx.Err(); err != nil {
		return application.AnalysisResult{}, err
	}
	if a == nil || !a.config.Enabled || !a.config.Authorized {
		return diagnosed("codeql.explicit_authorization_required", "CodeQL requires explicit enablement and authorization."), nil
	}
	java := false
	for _, artifact := range input.Artifacts {
		if strings.EqualFold(artifact.Language, "java") {
			java = true
			break
		}
	}
	if !java {
		return application.AnalysisResult{}, nil
	}
	if a.config.Executable == "" {
		return diagnosed("codeql.cli_unavailable", "CodeQL CLI is not configured."), nil
	}
	executable, err := exec.LookPath(a.config.Executable)
	if err != nil {
		return diagnosed("codeql.cli_unavailable", "CodeQL CLI is unavailable; local analysis remains available."), nil
	}
	runCtx, cancel := context.WithTimeout(ctx, a.config.Timeout)
	defer cancel()
	if _, err := runOutput(runCtx, executable, "version"); err != nil {
		return processFailure(ctx, runCtx, err)
	}
	languages, err := runOutput(runCtx, executable, "resolve", "languages")
	if err != nil {
		return processFailure(ctx, runCtx, err)
	}
	if !hasJavaExtractor(languages) {
		return diagnosed("codeql.java_extractor_unavailable", "CodeQL Java extractor is unavailable; local analysis remains available."), nil
	}
	if a.config.QueryDirectory == "" {
		return diagnosed("codeql.configuration_invalid", "CodeQL query directory is not configured."), nil
	}

	database, cleanup, err := a.prepareDatabase(runCtx, executable, input)
	if err != nil {
		return processFailure(ctx, runCtx, err)
	}
	defer cleanup()

	calls, err := a.runQuery(runCtx, executable, database, filepath.Join(a.config.QueryDirectory, "calls.ql"))
	if err != nil {
		return processFailure(ctx, runCtx, err)
	}
	result, err := decodeCalls(calls, input)
	if err != nil {
		return diagnosed("codeql.result_invalid", "CodeQL returned an unsupported call result; local analysis remains available."), nil
	}
	endpoints, err := a.runQuery(runCtx, executable, database, filepath.Join(a.config.QueryDirectory, "endpoints.ql"))
	if err != nil {
		return mergeDiagnostic(result, timeoutOrFailure(runCtx)), nil
	}
	endpointResult, err := decodeEndpoints(endpoints, input)
	if err != nil {
		return mergeDiagnostic(result, "CodeQL returned an unsupported endpoint result."), nil
	}
	result.Symbols = append(result.Symbols, endpointResult.Symbols...)
	result.Evidence = append(result.Evidence, endpointResult.Evidence...)
	return result, nil
}

func (a *Analyzer) prepareDatabase(ctx context.Context, executable string, input application.AnalysisInput) (string, func(), error) {
	switch a.config.ExtractionMode {
	case ExtractionExisting:
		if a.config.DatabasePath == "" {
			return "", func() {}, errors.New("database is not configured")
		}
		return a.config.DatabasePath, func() {}, nil
	case ExtractionCreate:
		if len(a.config.BuildArgs) == 0 {
			return "", func() {}, errors.New("authorized build argv is required")
		}
		directory, err := os.MkdirTemp("", "legacylens-codeql-")
		if err != nil {
			return "", func() {}, err
		}
		database := filepath.Join(directory, "database")
		cleanup := func() { _ = os.RemoveAll(directory) }
		if err := run(ctx, executable, "database", "init", "--language=java", "--source-root="+input.Root, "--", database); err != nil {
			cleanup()
			return "", func() {}, err
		}
		traceArgs := []string{"database", "trace-command", "--", database, "--"}
		traceArgs = append(traceArgs, a.config.BuildArgs...)
		if err := run(ctx, executable, traceArgs...); err != nil {
			cleanup()
			return "", func() {}, err
		}
		if err := run(ctx, executable, "database", "finalize", "--", database); err != nil {
			cleanup()
			return "", func() {}, err
		}
		return database, cleanup, nil
	default:
		return "", func() {}, errors.New("unsupported extraction mode")
	}
}

func (a *Analyzer) runQuery(ctx context.Context, executable, database, query string) ([]byte, error) {
	temp, err := os.MkdirTemp("", "legacylens-codeql-result-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	bqrs := filepath.Join(temp, "result.bqrs")
	jsonPath := filepath.Join(temp, "result.json")
	if err := run(ctx, executable, "query", "run", "--database="+database, "--output="+bqrs, "--", query); err != nil {
		return nil, err
	}
	if err := run(ctx, executable, "bqrs", "decode", "--format=json", "--output="+jsonPath, "--", bqrs); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, errors.New("could not read CodeQL result")
	}
	if len(data) > maxOutputBytes {
		return nil, errors.New("CodeQL result exceeded limit")
	}
	return data, nil
}

func run(ctx context.Context, executable string, args ...string) error {
	command := exec.CommandContext(ctx, executable, args...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return errors.New("CodeQL command failed")
	}
	return nil
}

func runOutput(ctx context.Context, executable string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...)
	var output limitedBuffer
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return nil, errors.New("CodeQL command failed")
	}
	return output.Bytes(), nil
}

func hasJavaExtractor(output []byte) bool {
	for _, field := range strings.Fields(strings.ToLower(string(output))) {
		field = strings.Trim(field, ",:[]{}\"'`")
		if field == "java" || field == "java-kotlin" || field == "java_and_kotlin" {
			return true
		}
	}
	return false
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > maxOutputBytes {
		return 0, errors.New("CodeQL output exceeded limit")
	}
	return b.Buffer.Write(data)
}

func processFailure(parent, runCtx context.Context, err error) (application.AnalysisResult, error) {
	if parent.Err() != nil {
		return application.AnalysisResult{}, parent.Err()
	}
	if runCtx.Err() != nil {
		return diagnosed("codeql.execution_failed", "CodeQL analysis timed out or was cancelled; local analysis remains available."), nil
	}
	_ = err // Do not expose executable arguments, paths, stderr, or source fragments.
	return diagnosed("codeql.execution_failed", "CodeQL analysis failed; local analysis remains available."), nil
}
func timeoutOrFailure(ctx context.Context) string {
	if ctx.Err() != nil {
		return "CodeQL analysis timed out or was cancelled."
	}
	return "CodeQL endpoint analysis failed."
}
func diagnosed(code, message string) application.AnalysisResult {
	return application.AnalysisResult{Diagnostics: []domain.Diagnostic{{Code: code, Message: message, Severity: "warning"}}}
}
func mergeDiagnostic(result application.AnalysisResult, message string) application.AnalysisResult {
	result.Diagnostics = append(result.Diagnostics, domain.Diagnostic{Code: "codeql.execution_failed", Message: message, Severity: "warning"})
	return result
}

type resultSet struct {
	Select struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
		Rows [][]json.RawMessage `json:"rows"`
	} `json:"#select"`
}

func readRows(data []byte, expected int) ([][]json.RawMessage, error) {
	var result resultSet
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if result.Select.Rows == nil {
		return nil, errors.New("missing result rows")
	}
	if len(result.Select.Columns) != expected {
		return nil, errors.New("unexpected result columns")
	}
	for _, row := range result.Select.Rows {
		if len(row) != expected {
			return nil, errors.New("unexpected result row width")
		}
	}
	return result.Select.Rows, nil
}
func scalarString(value json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", err
	}
	return text, nil
}
func scalarInt(value json.RawMessage) (int, error) {
	var n int
	if err := json.Unmarshal(value, &n); err == nil {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(value, &s); err != nil {
		return 0, err
	}
	return strconv.Atoi(s)
}
func decodeCalls(data []byte, input application.AnalysisInput) (application.AnalysisResult, error) {
	rows, err := readRows(data, 11)
	if err != nil {
		return application.AnalysisResult{}, err
	}
	paths := artifactMap(input.Artifacts)
	result := application.AnalysisResult{}
	for _, row := range rows {
		callerName, e := scalarString(row[0])
		if e != nil {
			return result, e
		}
		callerPath, e := scalarString(row[1])
		if e != nil {
			return result, e
		}
		callerLine, e := scalarInt(row[2])
		if e != nil {
			return result, e
		}
		callerCol, e := scalarInt(row[3])
		if e != nil {
			return result, e
		}
		calleeName, e := scalarString(row[4])
		if e != nil {
			return result, e
		}
		calleePath, e := scalarString(row[5])
		if e != nil {
			return result, e
		}
		calleeLine, e := scalarInt(row[6])
		if e != nil {
			return result, e
		}
		calleeCol, e := scalarInt(row[7])
		if e != nil {
			return result, e
		}
		callPath, e := scalarString(row[8])
		if e != nil {
			return result, e
		}
		callLine, e := scalarInt(row[9])
		if e != nil {
			return result, e
		}
		callCol, e := scalarInt(row[10])
		if e != nil {
			return result, e
		}
		callerID, ok := paths[callerPath]
		if !ok {
			return result, fmt.Errorf("unindexed caller path")
		}
		calleeID, ok := paths[calleePath]
		if !ok {
			return result, fmt.Errorf("unindexed callee path")
		}
		if _, ok = paths[callPath]; !ok {
			return result, fmt.Errorf("unindexed call path")
		}
		if !validLocation(callerLine, callerCol) || !validLocation(calleeLine, calleeCol) || !validLocation(callLine, callCol) {
			return result, errors.New("invalid source location")
		}
		from := makeSymbol(input, callerID, callerPath, callerName, "codeql-method", callerLine, callerCol)
		to := makeSymbol(input, calleeID, calleePath, calleeName, "codeql-method", calleeLine, calleeCol)
		callLocation := &domain.Location{Path: callPath, Line: callLine, Column: callCol}
		evidenceID := stableID("codeql-evidence", string(input.RevisionID), callerPath, strconv.Itoa(callLine), strconv.Itoa(callCol), calleeName)
		result.Symbols = append(result.Symbols, from, to)
		result.Evidence = append(result.Evidence, domain.Evidence{ID: domain.ID(evidenceID), Kind: "static-query", Source: "CodeQL direct call query", Location: callLocation})
		result.Relations = append(result.Relations, domain.Relation{ID: domain.ID(stableID("codeql-call", string(input.RevisionID), evidenceID)), FromID: from.ID, ToID: &to.ID, Kind: "calls", EvidenceIDs: []domain.ID{domain.ID(evidenceID)}, Resolution: domain.ResolutionResolved, Layer: domain.LayerStatic, Location: callLocation})
	}
	return result, nil
}
func decodeEndpoints(data []byte, input application.AnalysisInput) (application.AnalysisResult, error) {
	rows, err := readRows(data, 5)
	if err != nil {
		return application.AnalysisResult{}, err
	}
	paths := artifactMap(input.Artifacts)
	result := application.AnalysisResult{}
	for _, row := range rows {
		class, e := scalarString(row[0])
		if e != nil {
			return result, e
		}
		method, e := scalarString(row[1])
		if e != nil {
			return result, e
		}
		path, e := scalarString(row[2])
		if e != nil {
			return result, e
		}
		line, e := scalarInt(row[3])
		if e != nil {
			return result, e
		}
		col, e := scalarInt(row[4])
		if e != nil {
			return result, e
		}
		artifactID, ok := paths[path]
		if !ok {
			return result, errors.New("unindexed endpoint path")
		}
		if !validLocation(line, col) {
			return result, errors.New("invalid endpoint location")
		}
		symbol := makeSymbol(input, artifactID, path, class+"."+method, "endpoint", line, col)
		loc := symbol.Location
		result.Symbols = append(result.Symbols, symbol)
		result.Evidence = append(result.Evidence, domain.Evidence{ID: domain.ID(stableID("codeql-endpoint-evidence", string(input.RevisionID), path, strconv.Itoa(line), method)), Kind: "static-query", Source: "CodeQL servlet endpoint query", Location: loc})
	}
	return result, nil
}
func artifactMap(artifacts []domain.Artifact) map[string]domain.ID {
	result := make(map[string]domain.ID, len(artifacts))
	for _, item := range artifacts {
		result[filepath.ToSlash(item.Path)] = item.ID
	}
	return result
}
func makeSymbol(input application.AnalysisInput, artifactID domain.ID, path, name, kind string, line, column int) domain.Symbol {
	path = filepath.ToSlash(path)
	return domain.Symbol{ID: domain.ID(stableID("codeql-symbol", string(input.ProjectID), string(input.RevisionID), path, name, strconv.Itoa(line), strconv.Itoa(column))), ProjectID: input.ProjectID, RevisionID: input.RevisionID, ArtifactID: artifactID, Path: path, QualifiedName: name, Descriptor: "", Kind: kind, Location: &domain.Location{Path: path, Line: line, Column: column}}
}
func validLocation(line, column int) bool { return line > 0 && column > 0 }
func stableID(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = io.WriteString(h, part)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
