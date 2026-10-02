package el

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"legacylens/core/internal/domain"
)

// Resolver binds a simple JSF method expression to an indexed Java method.
// Expressions with arguments, property chains, or ambiguous beans stay dynamic.
type Resolver struct{}

func NewResolver() *Resolver { return &Resolver{} }

func (Resolver) Resolve(expression string, symbols []domain.Symbol) ([]domain.Relation, []domain.Diagnostic) {
	value := strings.TrimSpace(expression)
	if len(value) < 5 || !(strings.HasPrefix(value, "#{") || strings.HasPrefix(value, "${")) || !strings.HasSuffix(value, "}") {
		return nil, []domain.Diagnostic{{Code: "el.unsupported", Message: "Expression is not a supported JSF method reference", Severity: "info"}}
	}
	parts := strings.Split(strings.TrimSpace(value[2:len(value)-1]), ".")
	if len(parts) != 2 || !identifier(parts[0]) || !identifier(parts[1]) {
		return nil, []domain.Diagnostic{{Code: "el.dynamic", Message: "Expression requires runtime resolution", Severity: "info"}}
	}
	bean, method := strings.ToLower(parts[0]), parts[1]
	var matches []domain.Symbol
	for _, symbol := range symbols {
		if symbol.Kind != "method" {
			continue
		}
		qualified := symbol.QualifiedName
		if !strings.HasSuffix(qualified, "."+method) {
			continue
		}
		owner := qualified[:len(qualified)-len(method)-1]
		if dot := strings.LastIndexByte(owner, '.'); dot >= 0 {
			owner = owner[dot+1:]
		}
		owner = strings.ToLower(strings.TrimSuffix(owner, "Bean"))
		if owner == bean {
			matches = append(matches, symbol)
		}
	}
	if len(matches) != 1 {
		code := "el.unresolved"
		if len(matches) > 1 {
			code = "el.ambiguous"
		}
		return nil, []domain.Diagnostic{{Code: code, Message: "JSF method reference has no unique indexed destination", Severity: "info"}}
	}
	from, to := expressionID(value), matches[0].ID
	return []domain.Relation{{ID: expressionID(value + string(to)), FromID: from, ToID: &to, Kind: "action", Resolution: domain.ResolutionResolved, Layer: domain.LayerStatic}}, nil
}

func identifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if r == '_' || r == '$' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func expressionID(value string) domain.ID {
	h := sha256.Sum256([]byte(value))
	return domain.ID(hex.EncodeToString(h[:]))
}
