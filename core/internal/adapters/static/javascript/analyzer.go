package javascript

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

// Analyzer parses JavaScript into an embedded Go AST. It never launches a JS runtime.
type Analyzer struct{}

func NewAnalyzer() *Analyzer { return &Analyzer{} }
func (Analyzer) Capabilities() []application.Capability {
	return []application.Capability{{Name: "javascript-calls", Supported: true, Description: "Embedded JavaScript AST for functions, handlers, and requests."}}
}

type functionInfo struct {
	node  ast.Node
	name  string
	kind  string
	start int
	end   int
}

type callInfo struct {
	name       string
	kind       string
	start      int
	source     string
	owner      ast.Node
	target     ast.Node
	targetName string
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
		program, err := parser.ParseFile(nil, artifact.Path, string(source), 0)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, domain.Diagnostic{ID: id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "parse"), Code: "javascript.parse_error", Message: "JavaScript source could not be parsed", Severity: "warning", Location: parseErrorLocation(artifact.Path, source, err)})
			continue
		}

		functions := collectFunctions(program, source)
		functionByNode := make(map[ast.Node]functionInfo, len(functions))
		symbolByNode := make(map[ast.Node]domain.Symbol, len(functions))
		symbolByName := make(map[string]domain.Symbol, len(functions))
		for _, fn := range functions {
			functionByNode[fn.node] = fn
			kind := fn.kind
			if kind == "" {
				kind = "javascript-function"
			}
			symbol := domain.Symbol{ID: id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "function", fn.name, fmt.Sprint(fn.start)), ProjectID: input.ProjectID, RevisionID: input.RevisionID, ArtifactID: artifact.ID, Path: artifact.Path, QualifiedName: fn.name, Kind: kind, Location: at(artifact.Path, source, fn.start)}
			symbolByNode[fn.node] = symbol
			symbolByName[fn.name] = symbol
			result.Symbols = append(result.Symbols, symbol)
		}

		module := domain.Symbol{ID: id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "module"), ProjectID: input.ProjectID, RevisionID: input.RevisionID, ArtifactID: artifact.ID, Path: artifact.Path, QualifiedName: artifact.Path, Kind: "javascript-module", Location: at(artifact.Path, source, 0)}
		calls := collectCalls(program, source, functionByNode)
		moduleUsed := false
		for _, call := range calls {
			owner := module
			if call.owner != nil {
				owner, ok = symbolByNode[call.owner]
				if !ok {
					continue
				}
			} else {
				moduleUsed = true
			}

			var target domain.ID
			resolution := domain.ResolutionUnresolved
			if call.target != nil {
				if symbol, found := symbolByNode[call.target]; found {
					target, resolution = symbol.ID, domain.ResolutionResolved
				}
			} else if call.kind == "handler" {
				if symbol, found := symbolByName[call.targetName]; found {
					target, resolution = symbol.ID, domain.ResolutionResolved
				}
			} else if call.kind == "request" {
				target = id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "request", fmt.Sprint(call.start))
				resolution = domain.ResolutionDynamic
			} else if symbol, found := symbolByName[call.name]; found {
				target, resolution = symbol.ID, domain.ResolutionResolved
			} else {
				target = id(string(input.ProjectID), string(input.RevisionID), "javascript-call", call.name)
			}
			location := at(artifact.Path, source, call.start)
			evidenceID := id(string(input.ProjectID), string(input.RevisionID), artifact.Path, fmt.Sprint(call.start), "call")
			result.Evidence = append(result.Evidence, domain.Evidence{ID: evidenceID, Kind: "source", Source: call.source, Location: location})
			result.Relations = append(result.Relations, domain.Relation{ID: id(string(owner.ID), string(target), fmt.Sprint(call.start)), FromID: owner.ID, ToID: &target, Kind: call.kind, EvidenceIDs: []domain.ID{evidenceID}, Resolution: resolution, Layer: domain.LayerStatic, Location: location})
		}
		if moduleUsed {
			result.Symbols = append(result.Symbols, module)
		}
	}
	return result, nil
}

func collectFunctions(program *ast.Program, source []byte) []functionInfo {
	var nodes []ast.Node
	walkAST(program, func(node ast.Node) { nodes = append(nodes, node) })
	names := make(map[ast.Node]string)
	callbackNodes := make(map[ast.Node]bool)
	for _, node := range nodes {
		switch n := node.(type) {
		case *ast.FunctionLiteral:
			if n.Name != nil {
				names[n] = n.Name.Name.String()
			}
		case *ast.Binding:
			name := bindingName(n.Target)
			if name != "" {
				switch fn := n.Initializer.(type) {
				case *ast.FunctionLiteral:
					names[fn] = name
				case *ast.ArrowFunctionLiteral:
					names[fn] = name
				}
			}
		case *ast.CallExpression:
			if calleeName(n.Callee) == "addEventListener" && len(n.ArgumentList) > 1 {
				if fn := functionNode(n.ArgumentList[1]); fn != nil {
					callbackNodes[fn] = true
				}
			}
		}
	}
	var functions []functionInfo
	for _, node := range nodes {
		switch fn := node.(type) {
		case *ast.FunctionLiteral, *ast.ArrowFunctionLiteral:
			start, end := nodeOffsets(node, len(source))
			name := names[node]
			kind := ""
			if callbackNodes[node] {
				name = callbackName(start)
				kind = "javascript-callback"
			} else if name == "" {
				name = callbackName(start)
			}
			_ = fn
			functions = append(functions, functionInfo{node: node, name: name, kind: kind, start: start, end: end})
		}
	}
	return functions
}

func collectCalls(program *ast.Program, source []byte, functions map[ast.Node]functionInfo) []callInfo {
	var calls []callInfo
	var walk func(ast.Node, ast.Node)
	walk = func(node ast.Node, owner ast.Node) {
		if node == nil {
			return
		}
		if _, isFunction := functions[node]; isFunction {
			owner = node
		}
		if call, ok := node.(*ast.CallExpression); ok {
			name := calleeName(call.Callee)
			start, _ := nodeOffsets(call, len(source))
			if name == "addEventListener" && len(call.ArgumentList) > 1 {
				arg := call.ArgumentList[1]
				if target := functionNode(arg); target != nil {
					calls = append(calls, callInfo{name: name, kind: "handler", start: start, source: sourceForNode(source, call), owner: owner, target: target})
				} else if targetName := identifierName(arg); targetName != "" {
					calls = append(calls, callInfo{name: name, kind: "handler", start: start, source: sourceForNode(source, call), owner: owner, targetName: targetName})
				}
			} else if isRequest(name) {
				calls = append(calls, callInfo{name: name, kind: "request", start: start, source: sourceForNode(source, call), owner: owner})
			} else if name != "" {
				calls = append(calls, callInfo{name: name, kind: "calls", start: start, source: sourceForNode(source, call), owner: owner})
			}
		}
		forEachChild(node, func(child ast.Node) { walk(child, owner) })
	}
	walk(program, nil)
	return calls
}

func walkAST(root ast.Node, visit func(ast.Node)) {
	var walk func(ast.Node)
	walk = func(node ast.Node) {
		if node == nil {
			return
		}
		visit(node)
		forEachChild(node, walk)
	}
	walk(root)
}

func forEachChild(node ast.Node, visit func(ast.Node)) {
	value := reflect.ValueOf(node)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if !value.Type().Field(i).IsExported() {
			continue
		}
		collectASTNodes(field, visit)
	}
}

func collectASTNodes(value reflect.Value, visit func(ast.Node)) {
	if !value.IsValid() {
		return
	}
	if value.Kind() == reflect.Interface && value.IsNil() || value.Kind() == reflect.Ptr && value.IsNil() {
		return
	}
	if value.CanInterface() {
		if node, ok := value.Interface().(ast.Node); ok {
			visit(node)
			return
		}
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Ptr:
		collectASTNodes(value.Elem(), visit)
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			collectASTNodes(value.Index(i), visit)
		}
	case reflect.Struct:
		if value.Type().PkgPath() != "github.com/dop251/goja/ast" {
			return
		}
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).IsExported() {
				collectASTNodes(value.Field(i), visit)
			}
		}
	}
}

func functionNode(expression ast.Expression) ast.Node {
	switch expression.(type) {
	case *ast.FunctionLiteral, *ast.ArrowFunctionLiteral:
		return expression
	default:
		return nil
	}
}

func bindingName(target ast.BindingTarget) string {
	if identifier, ok := target.(*ast.Identifier); ok {
		return identifier.Name.String()
	}
	return ""
}

func identifierName(expression ast.Expression) string {
	if identifier, ok := expression.(*ast.Identifier); ok {
		return identifier.Name.String()
	}
	return ""
}

func calleeName(expression ast.Expression) string {
	switch callee := expression.(type) {
	case *ast.Identifier:
		return callee.Name.String()
	case *ast.DotExpression:
		return callee.Identifier.Name.String()
	case *ast.BracketExpression:
		if literal, ok := callee.Member.(*ast.StringLiteral); ok {
			return strings.Trim(literal.Literal, "'\"")
		}
	}
	return ""
}

func isRequest(name string) bool     { return name == "fetch" || name == "ajax" }
func callbackName(offset int) string { return fmt.Sprintf("callback@%d", offset) }

func nodeOffsets(node ast.Node, sourceLength int) (int, int) {
	start, end := int(node.Idx0())-1, int(node.Idx1())-1
	if start < 0 {
		start = 0
	}
	if start > sourceLength {
		start = sourceLength
	}
	if end < start {
		end = start
	}
	if end > sourceLength {
		end = sourceLength
	}
	return start, end
}

func sourceForNode(source []byte, node ast.Node) string {
	start, end := nodeOffsets(node, len(source))
	if end == start {
		return ""
	}
	return string(source[start:end])
}

func parseErrorLocation(path string, source []byte, err error) *domain.Location {
	var parseErrors parser.ErrorList
	if errors.As(err, &parseErrors) && len(parseErrors) > 0 {
		position := parseErrors[0].Position
		return &domain.Location{Path: path, Line: position.Line, Column: position.Column}
	}
	return at(path, source, 0)
}

func at(path string, source []byte, offset int) *domain.Location {
	if offset < 0 {
		offset = 0
	}
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
