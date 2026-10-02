package javascript

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

// Analyzer builds a bounded syntax tree of function declarations and calls in
// project JavaScript. It is embedded Go code and never launches a JS runtime.
type Analyzer struct{}

func NewAnalyzer() *Analyzer { return &Analyzer{} }
func (Analyzer) Capabilities() []application.Capability {
	return []application.Capability{{Name: "javascript-calls", Supported: true, Description: "Embedded JavaScript function and call syntax."}}
}

type token struct {
	text   string
	offset int
	kind   byte
}
type functionNode struct {
	name                      string
	start, bodyStart, bodyEnd int
}
type callNode struct {
	name   string
	offset int
	owner  string
	kind   string
	source string
}

func (Analyzer) Analyze(ctx context.Context, input application.AnalysisInput) (application.AnalysisResult, error) {
	var result application.AnalysisResult
	for _, artifact := range input.Artifacts {
		if err := ctx.Err(); err != nil {
			return application.AnalysisResult{}, err
		}
		if artifact.Language != "javascript" {
			continue
		}
		source, ok := input.Sources[artifact.Path]
		if !ok {
			continue
		}
		tokens, diagnostics := lex(source)
		for _, diagnostic := range diagnostics {
			diagnostic.ID = id(string(input.ProjectID), string(input.RevisionID), artifact.Path, diagnostic.Code)
			result.Diagnostics = append(result.Diagnostics, diagnostic)
		}
		functions := functionsIn(tokens)
		byName := make(map[string]domain.Symbol)
		for _, fn := range functions {
			location := at(artifact.Path, source, fn.start)
			symbol := domain.Symbol{ID: id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "function", fn.name, fmt.Sprint(fn.start)), ProjectID: input.ProjectID, RevisionID: input.RevisionID, ArtifactID: artifact.ID, Path: artifact.Path, QualifiedName: fn.name, Kind: "javascript-function", Location: location}
			byName[fn.name] = symbol
			result.Symbols = append(result.Symbols, symbol)
		}
		for _, call := range callsIn(tokens, functions) {
			owner, ok := byName[call.owner]
			if !ok {
				continue
			}
			location := at(artifact.Path, source, call.offset)
			evidenceID := id(string(input.ProjectID), string(input.RevisionID), artifact.Path, fmt.Sprint(call.offset), "call")
			result.Evidence = append(result.Evidence, domain.Evidence{ID: evidenceID, Kind: "source", Source: call.source, Location: location})
			var target domain.ID
			resolution := domain.ResolutionUnresolved
			if call.kind == "request" {
				target = id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "request", fmt.Sprint(call.offset))
				resolution = domain.ResolutionDynamic
			} else if destination, ok := byName[call.name]; ok {
				target, resolution = destination.ID, domain.ResolutionResolved
			} else {
				target = id(string(input.ProjectID), string(input.RevisionID), "javascript-call", call.name)
			}
			result.Relations = append(result.Relations, domain.Relation{ID: id(string(owner.ID), string(target), fmt.Sprint(call.offset)), FromID: owner.ID, ToID: &target, Kind: call.kind, EvidenceIDs: []domain.ID{evidenceID}, Resolution: resolution, Layer: domain.LayerStatic, Location: location})
		}
	}
	return result, nil
}

// lex keeps identifiers and delimiters while discarding comments and literal
// bodies. Literal tokens cannot become call nodes.
func lex(source []byte) ([]token, []domain.Diagnostic) {
	var out []token
	var diagnostics []domain.Diagnostic
	for i := 0; i < len(source); {
		c := source[i]
		if c <= ' ' {
			i++
			continue
		}
		if c == '/' && i+1 < len(source) && source[i+1] == '/' {
			i += 2
			for i < len(source) && source[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(source) && source[i+1] == '*' {
			i += 2
			for i+1 < len(source) && !(source[i] == '*' && source[i+1] == '/') {
				i++
			}
			if i+1 < len(source) {
				i += 2
			} else {
				i = len(source)
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			start, quote := i, c
			i++
			for i < len(source) {
				if source[i] == '\\' {
					i += 2
					continue
				}
				if source[i] == quote {
					i++
					break
				}
				i++
			}
			if i > len(source) {
				i = len(source)
			}
			out = append(out, token{text: string(source[start:i]), offset: start, kind: 's'})
			continue
		}
		if c == '/' && regexStart(out) {
			start := i
			i++
			for i < len(source) {
				if source[i] == '\\' {
					i += 2
					continue
				}
				if source[i] == '/' {
					i++
					break
				}
				i++
			}
			if i > len(source) {
				i = len(source)
			}
			out = append(out, token{text: string(source[start:i]), offset: start, kind: 's'})
			continue
		}
		if identStart(c) {
			start := i
			i++
			for i < len(source) && identPart(source[i]) {
				i++
			}
			out = append(out, token{text: string(source[start:i]), offset: start, kind: 'i'})
			continue
		}
		out = append(out, token{text: string(c), offset: i})
		i++
	}
	return out, diagnostics
}

func regexStart(tokens []token) bool {
	if len(tokens) == 0 {
		return true
	}
	switch tokens[len(tokens)-1].text {
	case "=", "(", ",", ":", "return", "[", "!", "?":
		return true
	}
	return false
}
func identStart(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
func identPart(c byte) bool { return identStart(c) || c >= '0' && c <= '9' }

func functionsIn(tokens []token) []functionNode {
	var nodes []functionNode
	for i := 0; i+3 < len(tokens); i++ {
		if tokens[i].text == "function" && tokens[i+1].kind == 'i' {
			body := findAfter(tokens, i+2, "{")
			if body < 0 {
				continue
			}
			end := matching(tokens, body, "{", "}")
			if end < 0 {
				continue
			}
			nodes = append(nodes, functionNode{name: tokens[i+1].text, start: tokens[i+1].offset, bodyStart: body + 1, bodyEnd: end})
		} else if (tokens[i].text == "const" || tokens[i].text == "let" || tokens[i].text == "var") && tokens[i+1].kind == 'i' && tokens[i+2].text == "=" {
			body := findAfter(tokens, i+3, "{")
			if body < 0 || body > i+12 {
				continue
			}
			arrow := false
			for j := i + 3; j < body; j++ {
				if j+1 < body && tokens[j].text == "=" && tokens[j+1].text == ">" {
					arrow = true
				}
			}
			if !arrow {
				continue
			}
			end := matching(tokens, body, "{", "}")
			if end < 0 {
				continue
			}
			nodes = append(nodes, functionNode{name: tokens[i+1].text, start: tokens[i+1].offset, bodyStart: body + 1, bodyEnd: end})
		}
	}
	return nodes
}
func findAfter(tokens []token, start int, value string) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].text == value {
			return i
		}
		if tokens[i].text == ";" {
			return -1
		}
	}
	return -1
}
func matching(tokens []token, start int, open, close string) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].text == open {
			depth++
		}
		if tokens[i].text == close {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
func callsIn(tokens []token, functions []functionNode) []callNode {
	var calls []callNode
	for _, fn := range functions {
		for i := fn.bodyStart; i+1 < fn.bodyEnd; i++ {
			if tokens[i].text == "addEventListener" && i > 0 && tokens[i-1].text == "." && tokens[i+1].text == "(" {
				end := matching(tokens, i+1, "(", ")")
				if end > i+3 {
					for j := i + 2; j+1 < end; j++ {
						if tokens[j].text == "," && tokens[j+1].kind == 'i' {
							name := tokens[j+1].text
							calls = append(calls, callNode{name: name, owner: fn.name, kind: "handler", offset: tokens[i].offset, source: "addEventListener(..., " + name + ")"})
							break
						}
					}
				}
				continue
			}
			if tokens[i].kind != 'i' || tokens[i+1].text != "(" || i > 0 && (tokens[i-1].text == "function" || tokens[i-1].text == "new") {
				continue
			}
			if i > 0 && tokens[i-1].text == "." && tokens[i].text != "ajax" && tokens[i].text != "fetch" {
				continue
			}
			switch tokens[i].text {
			case "if", "for", "while", "switch", "catch", "typeof", "await":
				continue
			}
			kind := "calls"
			if tokens[i].text == "fetch" || tokens[i].text == "ajax" {
				kind = "request"
			}
			calls = append(calls, callNode{name: tokens[i].text, owner: fn.name, kind: kind, offset: tokens[i].offset, source: tokens[i].text + "()"})
		}
	}
	return calls
}
func at(path string, source []byte, offset int) *domain.Location {
	if offset > len(source) {
		offset = len(source)
	}
	before := source[:offset]
	line := 1 + strings.Count(string(before), "\n")
	column := 1
	if n := strings.LastIndexByte(string(before), '\n'); n >= 0 {
		column += utf8.RuneCount(before[n+1:])
	} else {
		column += utf8.RuneCount(before)
	}
	return &domain.Location{Path: path, Line: line, Column: column}
}
func id(parts ...string) domain.ID {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return domain.ID(hex.EncodeToString(h.Sum(nil)))
}
