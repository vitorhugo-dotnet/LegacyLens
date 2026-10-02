package xhtml

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Analyzer struct{}

func NewAnalyzer() *Analyzer { return &Analyzer{} }
func (Analyzer) Capabilities() []application.Capability {
	return []application.Capability{{Name: "xhtml-action", Supported: true, Description: "Extracts JSF action expressions and browser handlers from XHTML."}}
}

func (Analyzer) Analyze(ctx context.Context, input application.AnalysisInput) (application.AnalysisResult, error) {
	var result application.AnalysisResult
	for _, artifact := range input.Artifacts {
		if err := ctx.Err(); err != nil {
			return application.AnalysisResult{}, err
		}
		if artifact.Language != "xhtml" {
			continue
		}
		source, ok := input.Sources[artifact.Path]
		if !ok {
			continue
		}
		decoder := xml.NewDecoder(bytes.NewReader(source))
		decoder.Strict = true
		for {
			token, err := decoder.Token()
			if err != nil {
				if err != io.EOF {
					result.Diagnostics = append(result.Diagnostics, domain.Diagnostic{ID: makeID(string(input.ProjectID), string(input.RevisionID), artifact.Path, "invalid-xml"), Code: "xhtml.invalid", Message: "XHTML could not be parsed safely", Severity: "warning"})
				}
				break
			}
			start, ok := token.(xml.StartElement)
			if !ok {
				continue
			}
			var remoteName string
			addedSymbol := false
			if strings.EqualFold(start.Name.Local, "remoteCommand") {
				for _, attr := range start.Attr {
					if strings.EqualFold(attr.Name.Local, "name") {
						remoteName = strings.TrimSpace(attr.Value)
					}
				}
			}
			for _, attr := range start.Attr {
				attribute := strings.ToLower(attr.Name.Local)
				if attribute != "action" && attribute != "actionlistener" && !strings.HasPrefix(attribute, "on") {
					continue
				}
				expression := strings.TrimSpace(attr.Value)
				isAction := attribute == "action" || attribute == "actionlistener"
				if isAction && !(strings.HasPrefix(expression, "#{") && strings.HasSuffix(expression, "}") || strings.HasPrefix(expression, "${") && strings.HasSuffix(expression, "}")) {
					continue
				}
				if !isAction && !strings.HasSuffix(expression, "()") {
					continue
				}
				offset := int(decoder.InputOffset())
				if offset > len(source) {
					offset = len(source)
				}
				startOffset := bytes.LastIndex(source[:offset], []byte("<"))
				if startOffset < 0 {
					startOffset = 0
				}
				line := 1 + bytes.Count(source[:startOffset], []byte("\n"))
				lineStart := bytes.LastIndex(source[:startOffset], []byte("\n")) + 1
				column := 1 + utf8.RuneCount(source[lineStart:startOffset])
				location := &domain.Location{Path: artifact.Path, Line: line, Column: column}
				elementOffset := strconv.Itoa(startOffset)
				symbolID := makeID(string(input.ProjectID), string(input.RevisionID), artifact.Path, elementOffset, start.Name.Local)
				name, kind := artifact.Path+"#"+start.Name.Local, "component"
				if remoteName != "" {
					name, kind = remoteName, "remote-command"
				}
				var destination domain.ID
				relationKind, resolution := "action", domain.ResolutionDynamic
				if isAction {
					destination = makeID(string(input.ProjectID), string(input.RevisionID), "dynamic-action", expression)
				} else {
					name := strings.TrimSuffix(expression, "()")
					if !simpleName(name) {
						continue
					}
					destination = makeID(string(input.ProjectID), string(input.RevisionID), "javascript-call", name)
					relationKind, resolution = "handler", domain.ResolutionUnresolved
				}
				evidenceID := makeID(string(input.ProjectID), string(input.RevisionID), artifact.Path, elementOffset, attribute, expression)
				if !addedSymbol {
					result.Symbols = append(result.Symbols, domain.Symbol{ID: symbolID, ProjectID: input.ProjectID, RevisionID: input.RevisionID, ArtifactID: artifact.ID, Path: artifact.Path, QualifiedName: name, Kind: kind, Location: location})
					addedSymbol = true
				}
				result.Evidence = append(result.Evidence, domain.Evidence{ID: evidenceID, Kind: "source", Source: expression, Location: location})
				target := destination
				result.Relations = append(result.Relations, domain.Relation{ID: makeID(string(symbolID), relationKind, attribute, string(target)), FromID: symbolID, ToID: &target, Kind: relationKind, EvidenceIDs: []domain.ID{evidenceID}, Resolution: resolution, Layer: domain.LayerStatic, Location: location})
			}
		}
	}
	return result, nil
}

func simpleName(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func makeID(parts ...string) domain.ID {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return domain.ID(hex.EncodeToString(hash.Sum(nil)))
}
