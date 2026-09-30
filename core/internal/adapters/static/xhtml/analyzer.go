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
	return []application.Capability{{Name: "xhtml-action", Supported: true, Description: "Extracts JSF action expressions from XHTML."}}
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
					result.Diagnostics = append(result.Diagnostics, domain.Diagnostic{ID: makeID(string(input.ProjectID), artifact.Path, "invalid-xml"), Code: "xhtml.invalid", Message: "XHTML could not be parsed safely", Severity: "warning"})
				}
				break
			}
			start, ok := token.(xml.StartElement)
			if !ok {
				continue
			}
			for _, attr := range start.Attr {
				if !strings.EqualFold(attr.Name.Local, "action") {
					continue
				}
				expression := strings.TrimSpace(attr.Value)
				if !(strings.HasPrefix(expression, "#{") && strings.HasSuffix(expression, "}") || strings.HasPrefix(expression, "${") && strings.HasSuffix(expression, "}")) {
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
				symbolID := makeID(string(input.ProjectID), artifact.Path, elementOffset, start.Name.Local)
				destination := makeID(string(input.ProjectID), "dynamic-action", expression)
				evidenceID := makeID(string(input.ProjectID), artifact.Path, elementOffset, "action-evidence", expression)
				result.Symbols = append(result.Symbols, domain.Symbol{ID: symbolID, ProjectID: input.ProjectID, RevisionID: input.RevisionID, ArtifactID: artifact.ID, Path: artifact.Path, QualifiedName: artifact.Path + "#" + start.Name.Local, Kind: "component", Location: location})
				result.Evidence = append(result.Evidence, domain.Evidence{ID: evidenceID, Kind: "source", Source: expression, Location: location})
				target := destination
				result.Relations = append(result.Relations, domain.Relation{ID: makeID(string(symbolID), "action", string(target)), FromID: symbolID, ToID: &target, Kind: "action", EvidenceIDs: []domain.ID{evidenceID}, Resolution: domain.ResolutionDynamic, Layer: domain.LayerStatic, Location: location})
			}
		}
	}
	return result, nil
}

func makeID(parts ...string) domain.ID {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return domain.ID(hex.EncodeToString(hash.Sum(nil)))
}
