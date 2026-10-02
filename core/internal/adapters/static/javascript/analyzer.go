package javascript

import (
	"bytes"
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
	kind                      string
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
			position := ""
			if diagnostic.Location != nil {
				diagnostic.Location.Path = artifact.Path
				position = fmt.Sprintf("%d:%d", diagnostic.Location.Line, diagnostic.Location.Column)
			}
			diagnostic.ID = id(string(input.ProjectID), string(input.RevisionID), artifact.Path, diagnostic.Code, position)
			result.Diagnostics = append(result.Diagnostics, diagnostic)
		}
		functions := functionsIn(tokens, source)
		byName := make(map[string]domain.Symbol)
		for _, fn := range functions {
			location := at(artifact.Path, source, fn.start)
			kind := fn.kind
			if kind == "" {
				kind = "javascript-function"
			}
			symbol := domain.Symbol{ID: id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "function", fn.name, fmt.Sprint(fn.start)), ProjectID: input.ProjectID, RevisionID: input.RevisionID, ArtifactID: artifact.ID, Path: artifact.Path, QualifiedName: fn.name, Kind: kind, Location: location}
			byName[fn.name] = symbol
			result.Symbols = append(result.Symbols, symbol)
		}
		for _, call := range callsIn(tokens, functions, source) {
			owner, ok := byName[call.owner]
			if !ok && call.owner == "<module>" {
				owner = domain.Symbol{ID: id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "module"), ProjectID: input.ProjectID, RevisionID: input.RevisionID, ArtifactID: artifact.ID, Path: artifact.Path, QualifiedName: artifact.Path, Kind: "javascript-module", Location: at(artifact.Path, source, 0)}
				byName[call.owner] = owner
				result.Symbols = append(result.Symbols, owner)
				ok = true
			}
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
			interpolation := false
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
				if quote == '`' && source[i] == '$' && i+1 < len(source) && source[i+1] == '{' {
					interpolation = true
				}
				i++
			}
			if i > len(source) {
				i = len(source)
			}
			out = append(out, token{text: string(source[start:i]), offset: start, kind: 's'})
			if interpolation {
				diagnostics = append(diagnostics, domain.Diagnostic{Code: "javascript.unsupported_syntax", Message: "Template interpolation is not analyzed", Severity: "warning", Location: at("", source, start)})
			}
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

func functionsIn(tokens []token, source []byte) []functionNode {
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
			start, end, ok := arrowBody(tokens, i+3, source)
			if ok {
				nodes = append(nodes, functionNode{name: tokens[i+1].text, start: tokens[i+1].offset, bodyStart: start, bodyEnd: end})
			}
		}
	}
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].text != "addEventListener" || tokens[i+1].text != "(" {
			continue
		}
		argument := eventHandlerArgument(tokens, i+1)
		if argument < 0 {
			continue
		}
		start, end, ok := arrowBody(tokens, argument, source)
		if ok {
			nodes = append(nodes, functionNode{name: callbackName(tokens[argument].offset), start: tokens[argument].offset, bodyStart: start, bodyEnd: end, kind: "javascript-callback"})
		}
	}
	return nodes
}

func arrowBody(tokens []token, start int, source []byte) (int, int, bool) {
	if start >= len(tokens) {
		return 0, 0, false
	}
	arrow := start
	if tokens[arrow].text == "async" {
		arrow++
	}
	if arrow >= len(tokens) {
		return 0, 0, false
	}
	if tokens[arrow].text == "(" {
		end := matching(tokens, arrow, "(", ")")
		if end < 0 {
			return 0, 0, false
		}
		arrow = end + 1
	} else if tokens[arrow].kind == 'i' {
		arrow++
	} else {
		return 0, 0, false
	}
	if arrow+2 >= len(tokens) || tokens[arrow].text != "=" || tokens[arrow+1].text != ">" {
		return 0, 0, false
	}
	body := arrow + 2
	if tokens[body].text == "{" {
		end := matching(tokens, body, "{", "}")
		if end < 0 {
			return 0, 0, false
		}
		return body + 1, end, true
	}
	return body, expressionEnd(tokens, body, source), true
}

func callbackName(offset int) string { return fmt.Sprintf("callback@%d", offset) }

func eventHandlerArgument(tokens []token, open int) int {
	end := matching(tokens, open, "(", ")")
	if end < 0 {
		return -1
	}
	parens, brackets, braces := 0, 0, 0
	for i := open + 1; i < end; i++ {
		switch tokens[i].text {
		case "(":
			parens++
		case ")":
			parens--
		case "[":
			brackets++
		case "]":
			brackets--
		case "{":
			braces++
		case "}":
			braces--
		case ",":
			if parens == 0 && brackets == 0 && braces == 0 && i+1 < end {
				return i + 1
			}
		}
	}
	return -1
}

func expressionEnd(tokens []token, start int, source []byte) int {
	parens, brackets, braces := 0, 0, 0
	for i := start; i < len(tokens); i++ {
		value := tokens[i].text
		if parens == 0 && brackets == 0 && braces == 0 && (value == ";" || value == "," || value == "}" || value == ")") {
			return i
		}
		previousEnd := 0
		if i > start {
			previousEnd = tokens[i-1].offset + len(tokens[i-1].text)
		}
		if i > start && parens == 0 && brackets == 0 && braces == 0 && declarationToken(value) && bytes.IndexByte(source[previousEnd:tokens[i].offset], '\n') >= 0 {
			return i
		}
		switch value {
		case "(":
			parens++
		case ")":
			if parens > 0 {
				parens--
			}
		case "[":
			brackets++
		case "]":
			if brackets > 0 {
				brackets--
			}
		case "{":
			braces++
		case "}":
			if braces > 0 {
				braces--
			}
		}
	}
	return len(tokens)
}

func declarationToken(value string) bool {
	switch value {
	case "const", "let", "var", "function", "class", "export", "import":
		return true
	}
	return false
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
func callsIn(tokens []token, functions []functionNode, source []byte) []callNode {
	var calls []callNode
	callbacks := map[int]functionNode{}
	for _, fn := range functions {
		if fn.kind == "javascript-callback" {
			callbacks[fn.start] = fn
		}
	}
	for _, fn := range functions {
		for i := fn.bodyStart; i+1 < fn.bodyEnd; i++ {
			insideCallback := false
			for _, callback := range callbacks {
				if callback.name != fn.name && i >= callback.bodyStart && i < callback.bodyEnd {
					insideCallback = true
					break
				}
			}
			if insideCallback {
				continue
			}
			if tokens[i].text == "addEventListener" && i > 0 && tokens[i-1].text == "." && tokens[i+1].text == "(" {
				argument := eventHandlerArgument(tokens, i+1)
				if argument >= 0 {
					if callback, ok := callbacks[tokens[argument].offset]; ok {
						end := matching(tokens, i+1, "(", ")")
						calls = append(calls, callNode{name: callback.name, owner: fn.name, kind: "handler", offset: tokens[i].offset, source: string(source[tokens[i].offset : tokens[end].offset+1])})
					} else if tokens[argument].kind == 'i' {
						name := tokens[argument].text
						calls = append(calls, callNode{name: name, owner: fn.name, kind: "handler", offset: tokens[i].offset, source: "addEventListener(..., " + name + ")"})
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
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].text != "addEventListener" || tokens[i+1].text != "(" || i == 0 || tokens[i-1].text != "." {
			continue
		}
		insideFunction := false
		for _, fn := range functions {
			if i >= fn.bodyStart && i < fn.bodyEnd {
				insideFunction = true
				break
			}
		}
		if insideFunction {
			continue
		}
		argument := eventHandlerArgument(tokens, i+1)
		if argument < 0 {
			continue
		}
		if callback, ok := callbacks[tokens[argument].offset]; ok {
			end := matching(tokens, i+1, "(", ")")
			calls = append(calls, callNode{name: callback.name, owner: "<module>", kind: "handler", offset: tokens[i].offset, source: string(source[tokens[i].offset : tokens[end].offset+1])})
		} else if tokens[argument].kind == 'i' {
			name := tokens[argument].text
			calls = append(calls, callNode{name: name, owner: "<module>", kind: "handler", offset: tokens[i].offset, source: "addEventListener(..., " + name + ")"})
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
