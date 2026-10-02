// Package jrxml extracts static structure from JasperReports XML without
// compiling reports or evaluating their expressions or queries.
package jrxml

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

type Analyzer struct{}

func NewAnalyzer() *Analyzer { return &Analyzer{} }

func (Analyzer) Capabilities() []application.Capability {
	return []application.Capability{{Name: "jrxml-report", Supported: true, Description: "Extracts report queries, tables, fields, expressions, and subreports from JRXML without executing them."}}
}

type node struct {
	name      string
	namespace string
	attrs     map[string]string
	text      strings.Builder
	children  []*node
	location  *domain.Location
}

func (Analyzer) Analyze(ctx context.Context, input application.AnalysisInput) (application.AnalysisResult, error) {
	var result application.AnalysisResult
	for _, artifact := range input.Artifacts {
		if err := ctx.Err(); err != nil {
			return application.AnalysisResult{}, err
		}
		if artifact.Language != "jrxml" {
			continue
		}
		source, ok := input.Sources[artifact.Path]
		if !ok {
			continue
		}
		root, parseLocation, err := parse(source, artifact.Path)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, domain.Diagnostic{ID: id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "invalid"), Code: "jrxml.invalid", Message: "JRXML could not be parsed safely", Severity: "warning", Location: parseLocation})
			continue
		}
		walk(root, func(n *node) {
			if n != root || !strings.EqualFold(n.name, "jasperReport") || !acceptedNamespace(n.namespace) {
				return
			}
			name := strings.TrimSpace(n.attrs["name"])
			if name == "" {
				name = artifact.Path
			}
			reportID := id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "report", name)
			result.Symbols = append(result.Symbols, makeSymbol(input, artifact, reportID, name, "report", "", n.location))
			queryNodes := descendants(n, "queryString")
			var queryID domain.ID
			for _, q := range queryNodes {
				sql := strings.TrimSpace(q.content())
				queryID = id(string(reportID), "query", sql)
				result.Symbols = append(result.Symbols, makeSymbol(input, artifact, queryID, name+"#query", "query", sql, q.location))
				addRelation(&result, input, artifact, reportID, queryID, "report-query", sql, q.location)
				seen := map[string]bool{}
				for _, table := range sqlTables(sql) {
					if seen[strings.ToLower(table)] {
						continue
					}
					seen[strings.ToLower(table)] = true
					tableID := id(string(queryID), "table", strings.ToLower(table))
					result.Symbols = append(result.Symbols, makeSymbol(input, artifact, tableID, table, "table", "", q.location))
					addRelation(&result, input, artifact, queryID, tableID, "query-table", table, q.location)
				}
			}
			for _, field := range descendants(n, "field") {
				fieldName := strings.TrimSpace(field.attrs["name"])
				if fieldName == "" {
					continue
				}
				fieldID := id(string(reportID), "field", fieldName)
				result.Symbols = append(result.Symbols, makeSymbol(input, artifact, fieldID, name+"#"+fieldName, "field", strings.TrimSpace(field.attrs["class"]), field.location))
				addRelation(&result, input, artifact, reportID, fieldID, "report-field", fieldName, field.location)
			}
			for _, expr := range descendants(n, "*Expression") {
				if isWithinSubreport(n, expr) {
					continue
				}
				sourceText := strings.TrimSpace(expr.content())
				if sourceText == "" {
					continue
				}
				exprID := id(string(reportID), "expression", expr.name, sourceText)
				result.Symbols = append(result.Symbols, makeSymbol(input, artifact, exprID, name+"#"+expr.name, "expression", sourceText, expr.location))
				addRelation(&result, input, artifact, reportID, exprID, "report-expression", sourceText, expr.location)
			}
			for _, sub := range descendants(n, "subreport") {
				expr := firstDescendant(sub, "subreportExpression")
				if expr == nil {
					continue
				}
				sourceText := strings.TrimSpace(expr.content())
				if sourceText == "" {
					continue
				}
				staticPath := quotedLiteral(sourceText)
				subName := staticPath
				if subName == "" {
					subName = sourceText
				}
				subID := id(string(reportID), "subreport", subName)
				result.Symbols = append(result.Symbols, makeSymbol(input, artifact, subID, subName, "subreport", "", sub.location))
				addRelation(&result, input, artifact, reportID, subID, "report-subreport", sourceText, expr.location)
				if staticPath == "" {
					result.Relations[len(result.Relations)-1].Resolution = domain.ResolutionDynamic
					result.Diagnostics = append(result.Diagnostics, domain.Diagnostic{ID: id(string(input.ProjectID), string(input.RevisionID), artifact.Path, "dynamic-subreport", sourceText), Code: "jrxml.subreport_dynamic", Message: "Subreport path is dynamic and requires runtime resolution", Severity: "info", Location: expr.location})
				}
			}
			for _, dataset := range descendants(n, "subDataset") {
				datasetName := strings.TrimSpace(dataset.attrs["name"])
				if datasetName == "" {
					datasetName = "anonymous"
				}
				datasetID := id(string(reportID), "subdataset", datasetName)
				qualifiedDataset := name + "#dataset:" + datasetName
				result.Symbols = append(result.Symbols, makeSymbol(input, artifact, datasetID, qualifiedDataset, "subdataset", "", dataset.location))
				for _, q := range descendants(dataset, "queryString") {
					sql := strings.TrimSpace(q.content())
					queryID := id(string(datasetID), "query", sql)
					result.Symbols = append(result.Symbols, makeSymbol(input, artifact, queryID, qualifiedDataset+"#query", "query", sql, q.location))
					addRelation(&result, input, artifact, datasetID, queryID, "dataset-query", sql, q.location)
					seen := map[string]bool{}
					for _, table := range sqlTables(sql) {
						if seen[strings.ToLower(table)] {
							continue
						}
						seen[strings.ToLower(table)] = true
						tableID := id(string(queryID), "table", strings.ToLower(table))
						result.Symbols = append(result.Symbols, makeSymbol(input, artifact, tableID, table, "table", "", q.location))
						addRelation(&result, input, artifact, queryID, tableID, "query-table", table, q.location)
					}
				}
				for _, field := range descendants(dataset, "field") {
					fieldName := strings.TrimSpace(field.attrs["name"])
					if fieldName == "" {
						continue
					}
					fieldID := id(string(datasetID), "field", fieldName)
					result.Symbols = append(result.Symbols, makeSymbol(input, artifact, fieldID, qualifiedDataset+"#"+fieldName, "field", strings.TrimSpace(field.attrs["class"]), field.location))
					addRelation(&result, input, artifact, datasetID, fieldID, "dataset-field", fieldName, field.location)
				}
			}
		})
	}
	return result, nil
}

func parse(source []byte, path string) (*node, *domain.Location, error) {
	d := xml.NewDecoder(bytes.NewReader(source))
	d.Strict = true
	// encoding/xml does not resolve external entities. Ignore directives and
	// leave the entity table empty so declarations cannot supply replacement text.
	d.Entity = map[string]string{}
	var root *node
	var stack []*node
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, locationAt(source, path, int(d.InputOffset())), err
		}
		switch t := tok.(type) {
		case xml.Directive:
			// DTD content is never interpreted or passed to another parser.
		case xml.StartElement:
			offset := int(d.InputOffset())
			if offset > len(source) {
				offset = len(source)
			}
			start := bytes.LastIndex(source[:offset], []byte("<"))
			if start < 0 {
				start = 0
			}
			line := 1 + bytes.Count(source[:start], []byte("\n"))
			lineStart := bytes.LastIndex(source[:start], []byte("\n")) + 1
			loc := &domain.Location{Path: path, Line: line, Column: 1 + utf8.RuneCount(source[lineStart:start])}
			n := &node{name: t.Name.Local, namespace: t.Name.Space, attrs: map[string]string{}, location: loc}
			for _, a := range t.Attr {
				n.attrs[a.Name.Local] = a.Value
			}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.children = append(p.children, n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write([]byte(t))
			}
		}
	}
	return root, nil, nil
}

const jasperNamespace = "http://jasperreports.sourceforge.net/jasperreports"

func acceptedNamespace(namespace string) bool { return namespace == "" || namespace == jasperNamespace }

func locationAt(source []byte, path string, offset int) *domain.Location {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	prefix := source[:offset]
	lineStart := bytes.LastIndex(prefix, []byte("\n")) + 1
	return &domain.Location{Path: path, Line: 1 + bytes.Count(prefix, []byte("\n")), Column: 1 + utf8.RuneCount(prefix[lineStart:])}
}

func (n *node) content() string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(n.text.String())
	for _, c := range n.children {
		b.WriteString(c.content())
	}
	return b.String()
}
func walk(n *node, fn func(*node)) {
	if n == nil {
		return
	}
	fn(n)
	for _, c := range n.children {
		walk(c, fn)
	}
}
func descendants(n *node, suffix string) []*node {
	var out []*node
	walk(n, func(c *node) {
		if c != n && c.namespace == n.namespace && (suffix == "*Expression" && strings.HasSuffix(strings.ToLower(c.name), "expression") || strings.EqualFold(c.name, suffix)) && (!crossesDataset(n, c) || strings.EqualFold(suffix, "subDataset")) {
			out = append(out, c)
		}
	})
	return out
}

func crossesDataset(root, target *node) bool {
	var visit func(*node, bool) bool
	visit = func(current *node, inside bool) bool {
		if current != root && strings.EqualFold(current.name, "subDataset") {
			inside = true
		}
		if current == target {
			return inside
		}
		for _, child := range current.children {
			if visit(child, inside) {
				return true
			}
		}
		return false
	}
	return visit(root, false)
}
func firstDescendant(n *node, name string) *node {
	all := descendants(n, name)
	if len(all) == 0 {
		return nil
	}
	return all[0]
}

func isWithinSubreport(root, target *node) bool {
	var visit func(*node, bool) bool
	visit = func(current *node, inside bool) bool {
		inside = inside || strings.EqualFold(current.name, "subreport")
		if current == target {
			return inside
		}
		for _, child := range current.children {
			if visit(child, inside) {
				return true
			}
		}
		return false
	}
	return visit(root, false)
}

func sqlTables(sql string) []string {
	tokens := tokenizeSQL(sql)
	var out []string
	activeFrom := map[int]bool{}
	depth := 0
	for i, token := range tokens {
		if token.text == ")" {
			delete(activeFrom, depth)
			if depth > 0 {
				depth--
			}
			continue
		}
		if token.text == "(" {
			depth++
			continue
		}
		if token.kind == "word" {
			switch token.text {
			case "from":
				activeFrom[depth] = true
				if name, ok := sqlTableAt(tokens, i+1); ok {
					out = append(out, name)
				}
			case "join":
				if name, ok := sqlTableAt(tokens, i+1); ok {
					out = append(out, name)
				}
			case "where", "group", "order", "having", "union", "limit", "offset", "fetch":
				activeFrom[depth] = false
			}
		}
		if token.text == "," && activeFrom[depth] {
			if name, ok := sqlTableAt(tokens, i+1); ok {
				out = append(out, name)
			}
		}
	}
	return out
}

func sqlTableAt(tokens []sqlToken, index int) (string, bool) {
	if index >= len(tokens) {
		return "", false
	}
	if tokens[index].kind != "identifier" && (tokens[index].kind != "word" || sqlClause(tokens[index].text)) {
		return "", false
	}
	name := tokens[index].text
	for index+2 < len(tokens) && tokens[index+1].text == "." && (tokens[index+2].kind == "identifier" || tokens[index+2].kind == "word" && !sqlClause(tokens[index+2].text)) {
		name += "." + tokens[index+2].text
		index += 2
	}
	return name, true
}

type sqlToken struct{ kind, text string }

func tokenizeSQL(sql string) []sqlToken {
	var tokens []sqlToken
	for i := 0; i < len(sql); {
		c := sql[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			i++
			continue
		}
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			i += 2
			depth := 1
			for i < len(sql) && depth > 0 {
				if i+1 < len(sql) && sql[i] == '/' && sql[i+1] == '*' {
					depth++
					i += 2
				} else if i+1 < len(sql) && sql[i] == '*' && sql[i+1] == '/' {
					depth--
					i += 2
				} else {
					i++
				}
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			close := c
			kind := "identifier"
			if c == '\'' {
				close = '\''
				kind = "string"
			} else if c == '[' {
				close = ']'
			}
			i++
			var b strings.Builder
			for i < len(sql) {
				if sql[i] == close {
					if i+1 < len(sql) && sql[i+1] == close {
						b.WriteByte(close)
						i += 2
						continue
					}
					i++
					break
				}
				b.WriteByte(sql[i])
				i++
			}
			tokens = append(tokens, sqlToken{kind: kind, text: b.String()})
			continue
		}
		if isSQLIdent(c) {
			start := i
			for i < len(sql) && isSQLIdent(sql[i]) {
				i++
			}
			tokens = append(tokens, sqlToken{kind: "word", text: strings.ToLower(sql[start:i])})
			continue
		}
		if strings.ContainsRune(".,()", rune(c)) {
			tokens = append(tokens, sqlToken{kind: "punct", text: string(c)})
		}
		i++
	}
	return tokens
}

func isSQLIdent(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func sqlClause(word string) bool {
	switch word {
	case "select", "from", "join", "where", "on", "group", "order", "having", "union", "left", "right", "inner", "outer", "cross", "full", "limit", "offset", "fetch", "as":
		return true
	default:
		return false
	}
}

var literalPattern = regexp.MustCompile(`^\s*["']([^"']+\.(?:jasper|jrxml))["']\s*$`)

func quotedLiteral(s string) string {
	m := literalPattern.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
func makeSymbol(input application.AnalysisInput, a domain.Artifact, sid domain.ID, name, kind, descriptor string, loc *domain.Location) domain.Symbol {
	return domain.Symbol{ID: sid, ProjectID: input.ProjectID, RevisionID: input.RevisionID, ArtifactID: a.ID, Path: a.Path, QualifiedName: name, Kind: kind, Descriptor: descriptor, Location: loc}
}
func addRelation(r *application.AnalysisResult, input application.AnalysisInput, a domain.Artifact, from, to domain.ID, kind, source string, loc *domain.Location) {
	eid := id(string(input.ProjectID), string(input.RevisionID), a.Path, kind, source, itoa(loc))
	r.Evidence = append(r.Evidence, domain.Evidence{ID: eid, Kind: "source", Source: source, Location: loc})
	target := to
	r.Relations = append(r.Relations, domain.Relation{ID: id(string(from), kind, string(to)), FromID: from, ToID: &target, Kind: kind, EvidenceIDs: []domain.ID{eid}, Resolution: domain.ResolutionResolved, Layer: domain.LayerStatic, Location: loc})
}
func errorLocation(source []byte, path string) *domain.Location {
	line := 1 + bytes.Count(source, []byte("\n"))
	return &domain.Location{Path: path, Line: line, Column: 1}
}
func itoa(l *domain.Location) string {
	if l == nil {
		return ""
	}
	return strconv.Itoa(l.Line) + ":" + strconv.Itoa(l.Column)
}
func id(parts ...string) domain.ID {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return domain.ID(hex.EncodeToString(h.Sum(nil)))
}
