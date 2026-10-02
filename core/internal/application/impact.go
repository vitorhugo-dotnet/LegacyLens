package application

import (
	"context"
	"errors"
	"sort"
	"strings"

	"legacylens/core/internal/domain"
)

const (
	maxImpactDepth = 16
	maxImpactPage  = 200
	maxImpactWalks = 10_000
)

// ImpactService derives reverse dependency paths from one persisted source index.
// It does not load, parse, or execute JRXML itself.
type ImpactService struct{ indexes InvestigationIndexStore }

func NewImpactService(indexes InvestigationIndexStore) *ImpactService {
	return &ImpactService{indexes: indexes}
}

type impactWalk struct {
	symbolIDs   []domain.ID // ordered from the seed toward the discovered source
	relationIDs []domain.ID // same order as the traversed relations
	inferred    bool
}

type impactWalkState struct {
	node domain.ID
	path impactWalk
}

func (s *ImpactService) Query(ctx context.Context, query GraphQuery) (GraphResult, error) {
	if s == nil || s.indexes == nil {
		return GraphResult{}, errors.New("investigation index store is required")
	}
	if query.ProjectID == "" || len(query.SymbolIDs) != 1 || query.SymbolIDs[0] == "" {
		return GraphResult{}, errors.New("project id and one target symbol id are required")
	}
	if query.Depth < 0 || query.Depth > maxImpactDepth {
		return GraphResult{}, errors.New("impact depth is outside the supported range")
	}
	if query.Offset < 0 || query.Offset > 1_000_000_000 {
		return GraphResult{}, errors.New("impact offset is outside the supported range")
	}
	if query.Limit <= 0 || query.Limit > maxImpactPage {
		query.Limit = maxImpactPage
	}

	snapshot, err := s.indexes.LoadInvestigationIndex(ctx, query.ProjectID)
	if err != nil {
		return GraphResult{}, err
	}
	if query.RevisionID != "" && query.RevisionID != snapshot.RevisionID {
		return GraphResult{}, errors.New("requested revision is not the loaded investigation index")
	}
	seed := query.SymbolIDs[0]
	symbols := make(map[domain.ID]domain.Symbol, len(snapshot.Symbols))
	for _, symbol := range snapshot.Symbols {
		if symbol.ProjectID != "" && symbol.ProjectID != query.ProjectID {
			continue
		}
		if snapshot.RevisionID != "" && symbol.RevisionID != "" && symbol.RevisionID != snapshot.RevisionID {
			continue
		}
		symbols[symbol.ID] = symbol
	}
	if _, ok := symbols[seed]; !ok {
		return GraphResult{}, errors.New("target symbol was not found in the loaded index")
	}

	evidence := make(map[domain.ID]domain.Evidence, len(snapshot.Evidence))
	for _, item := range snapshot.Evidence {
		evidence[item.ID] = item
	}
	relations := append([]domain.Relation(nil), snapshot.Relations...)
	diagnostics := make([]GraphDiagnostic, 0)
	addDiagnostic := func(code, message string, symbolIDs []domain.ID, relationID domain.ID, evidenceIDs []domain.ID, location *domain.Location) {
		id := stableID(string(query.ProjectID), string(snapshot.RevisionID), code, string(relationID), strings.Join(idsAsStrings(symbolIDs), ","))
		diagnostics = append(diagnostics, GraphDiagnostic{ID: id, Code: code, Message: message, SymbolIDs: nonNilIDs(symbolIDs), RelationID: relationID, EvidenceIDs: nonNilIDs(evidenceIDs), Location: location})
	}

	// Analyzer IDs for tables are artifact-specific. Join matching table names
	// only when both source sides carry evidence, and keep the inference visible.
	added, ambiguous := tableNameRelations(query.ProjectID, snapshot.RevisionID, seed, symbols, relations, evidence)
	relations = append(relations, added...)
	for _, match := range ambiguous {
		addDiagnostic("impact.table_name_ambiguous", "Multiple source-backed table symbols share this name; no inferred identity edge was added.", match.symbolIDs, "", match.evidenceIDs, nil)
	}
	for _, relation := range added {
		addDiagnostic("impact.table_name_inferred", "Table symbols in separate artifacts were connected by a case-insensitive source-name match; this identity is inferred.", []domain.ID{relation.FromID, *relation.ToID}, relation.ID, relation.EvidenceIDs, relation.Location)
	}

	incoming := make(map[domain.ID][]domain.Relation, len(symbols))
	outgoing := make(map[domain.ID][]domain.Relation, len(symbols))
	for _, relation := range relations {
		if relation.Layer != domain.LayerStatic || len(existingEvidence(relation.EvidenceIDs, evidence)) == 0 {
			continue
		}
		outgoing[relation.FromID] = append(outgoing[relation.FromID], relation)
		if relation.ToID != nil {
			incoming[*relation.ToID] = append(incoming[*relation.ToID], relation)
		}
	}
	for key := range incoming {
		sort.Slice(incoming[key], func(i, j int) bool { return incoming[key][i].ID < incoming[key][j].ID })
	}

	queue := []impactWalkState{{node: seed, path: impactWalk{symbolIDs: []domain.ID{seed}}}}
	walks := make([]impactWalk, 0)
	truncated := false
	cycleSeen := map[domain.ID]bool{}
	dynamicSeen := map[domain.ID]bool{}
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return GraphResult{}, err
		}
		state := queue[0]
		queue = queue[1:]
		current := state.node
		depth := len(state.path.relationIDs)
		for _, relation := range incoming[current] {
			if isSubreportRelation(relation.Kind) && relation.Resolution != domain.ResolutionResolved {
				if !dynamicSeen[relation.ID] {
					dynamicSeen[relation.ID] = true
					addDiagnostic("impact.dynamic_subreport", "A dynamic or unresolved subreport dependency may add impact paths that static analysis cannot confirm.", []domain.ID{relation.FromID, current}, relation.ID, relation.EvidenceIDs, relation.Location)
				}
			}
			if _, known := symbols[relation.FromID]; !known {
				continue
			}
			if containsID(state.path.symbolIDs, relation.FromID) {
				if !cycleSeen[relation.ID] {
					cycleSeen[relation.ID] = true
					addDiagnostic("impact.cycle", "A dependency cycle was detected; each symbol is included once in the returned impact paths.", []domain.ID{relation.FromID, current}, relation.ID, relation.EvidenceIDs, relation.Location)
				}
				continue
			}
			if depth >= query.Depth {
				truncated = true
				continue
			}
			if len(walks) >= maxImpactWalks {
				truncated = true
				continue
			}
			ids := append([]domain.ID{relation.FromID}, state.path.symbolIDs...)
			relationIDs := append([]domain.ID{relation.ID}, state.path.relationIDs...)
			path := impactWalk{symbolIDs: ids, relationIDs: relationIDs, inferred: state.path.inferred || relation.Kind == "impact.table_name_match"}
			walks = append(walks, path)
			queue = append(queue, impactWalkState{node: relation.FromID, path: path})
		}
		// Dynamic subreport relations with missing destinations cannot enter the
		// incoming index; diagnose them when their owning report is reached.
		for _, relation := range outgoing[current] {
			if isSubreportRelation(relation.Kind) && (relation.Resolution != domain.ResolutionResolved || relation.ToID == nil) && !dynamicSeen[relation.ID] {
				dynamicSeen[relation.ID] = true
				addDiagnostic("impact.dynamic_subreport", "A dynamic or unresolved subreport dependency may add impact paths that static analysis cannot confirm.", []domain.ID{current}, relation.ID, relation.EvidenceIDs, relation.Location)
			}
		}
	}

	paths := make([]GraphPath, 0, len(walks))
	for _, walk := range walks {
		id := walk.symbolIDs[0]
		symbolIDs := append([]domain.ID(nil), walk.symbolIDs...)
		relationIDs := append([]domain.ID(nil), walk.relationIDs...)
		evidenceIDs := pathEvidence(relationIDs, relations, evidence)
		paths = append(paths, GraphPath{SourceID: id, TargetID: seed, SymbolIDs: symbolIDs, RelationIDs: relationIDs, EvidenceIDs: evidenceIDs, Inferred: walk.inferred})
	}
	sort.Slice(paths, func(i, j int) bool {
		if paths[i].SourceID != paths[j].SourceID {
			return paths[i].SourceID < paths[j].SourceID
		}
		return strings.Join(idsAsStrings(paths[i].RelationIDs), ",") < strings.Join(idsAsStrings(paths[j].RelationIDs), ",")
	})
	total := len(paths)
	paths = pageSlice(paths, query.Offset, query.Limit)

	usedSymbols := map[domain.ID]bool{seed: true}
	usedRelations := map[domain.ID]bool{}
	usedEvidence := map[domain.ID]bool{}
	for _, path := range paths {
		for _, id := range path.SymbolIDs {
			usedSymbols[id] = true
		}
		for _, id := range path.RelationIDs {
			usedRelations[id] = true
		}
		for _, id := range path.EvidenceIDs {
			usedEvidence[id] = true
		}
	}
	for _, diagnostic := range diagnostics {
		for _, id := range diagnostic.SymbolIDs {
			usedSymbols[id] = true
		}
		for _, id := range diagnostic.EvidenceIDs {
			usedEvidence[id] = true
		}
		if diagnostic.RelationID != "" {
			usedRelations[diagnostic.RelationID] = true
		}
	}
	for id := range usedSymbols {
		if _, ok := symbols[id]; !ok {
			delete(usedSymbols, id)
		}
	}
	result := GraphResult{ProjectID: query.ProjectID, RevisionID: snapshot.RevisionID, Depth: query.Depth, Offset: query.Offset, Limit: query.Limit, Total: total, HasMore: query.Offset+len(paths) < total, Truncated: truncated, Paths: paths}
	for _, symbol := range snapshot.Symbols {
		if usedSymbols[symbol.ID] {
			result.Symbols = append(result.Symbols, symbol)
		}
	}
	for _, relation := range relations {
		if usedRelations[relation.ID] {
			result.Relations = append(result.Relations, relation)
		}
	}
	for _, item := range snapshot.Evidence {
		if usedEvidence[item.ID] {
			result.Evidence = append(result.Evidence, item)
		}
	}
	result.Diagnostics = diagnostics
	if result.Symbols == nil {
		result.Symbols = []domain.Symbol{}
	}
	if result.Relations == nil {
		result.Relations = []domain.Relation{}
	}
	if result.Evidence == nil {
		result.Evidence = []domain.Evidence{}
	}
	if result.Paths == nil {
		result.Paths = []GraphPath{}
	}
	if result.Diagnostics == nil {
		result.Diagnostics = []GraphDiagnostic{}
	}
	return result, nil
}

type ambiguousTableMatch struct{ symbolIDs, evidenceIDs []domain.ID }

func tableNameRelations(projectID, revisionID, seedID domain.ID, symbols map[domain.ID]domain.Symbol, relations []domain.Relation, evidence map[domain.ID]domain.Evidence) ([]domain.Relation, []ambiguousTableMatch) {
	groups := map[string][]domain.Symbol{}
	for _, symbol := range symbols {
		if strings.EqualFold(symbol.Kind, "table") && canonicalTableName(symbol.QualifiedName) != "" {
			groups[canonicalTableName(symbol.QualifiedName)] = append(groups[canonicalTableName(symbol.QualifiedName)], symbol)
		}
	}
	var inferred []domain.Relation
	var ambiguous []ambiguousTableMatch
	for name, group := range groups {
		containsSeed := false
		for _, symbol := range group {
			if symbol.ID == seedID {
				containsSeed = true
				break
			}
		}
		if !containsSeed {
			continue
		}
		byPath := map[string][]domain.Symbol{}
		for _, symbol := range group {
			byPath[symbol.Path] = append(byPath[symbol.Path], symbol)
		}
		if len(byPath) < 2 {
			continue
		}
		pathAmbiguous := false
		for _, candidates := range byPath {
			if len(candidates) != 1 {
				pathAmbiguous = true
				break
			}
		}
		if pathAmbiguous || len(byPath) > 2 {
			ids := make([]domain.ID, 0, len(group))
			evidenceIDs := []domain.ID{}
			for _, symbol := range group {
				ids = append(ids, symbol.ID)
				evidenceIDs = append(evidenceIDs, symbolEvidence(symbol.ID, relations)...)
			}
			ambiguous = append(ambiguous, ambiguousTableMatch{uniqueIDs(ids), uniqueIDs(evidenceIDs)})
			continue
		}
		ordered := make([]domain.Symbol, 0, len(byPath))
		for _, candidates := range byPath {
			ordered = append(ordered, candidates[0])
		}
		ordered = append([]domain.Symbol(nil), ordered...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
		left, right := ordered[0], ordered[1]
		leftEvidence := existingEvidence(symbolEvidence(left.ID, relations), evidence)
		rightEvidence := existingEvidence(symbolEvidence(right.ID, relations), evidence)
		if len(leftEvidence) == 0 || len(rightEvidence) == 0 {
			continue
		}
		evidenceIDs := uniqueIDs(append(leftEvidence, rightEvidence...))
		from, to := left.ID, right.ID
		if seedID == left.ID {
			from, to = right.ID, left.ID
		}
		source := left
		if source.ID != from {
			source = right
		}
		inferred = append(inferred, domain.Relation{ID: stableID(string(projectID), string(revisionID), "impact.table_name_match", string(from), string(to), name), FromID: from, ToID: &to, Kind: "impact.table_name_match", EvidenceIDs: evidenceIDs, Resolution: domain.ResolutionDynamic, Layer: domain.LayerStatic, Location: source.Location})
	}
	sort.Slice(inferred, func(i, j int) bool { return inferred[i].ID < inferred[j].ID })
	return inferred, ambiguous
}

func containsID(ids []domain.ID, target domain.ID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func canonicalTableName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.NewReplacer("\"", "", "`", "", "[", "", "]", "").Replace(name)
	return name
}

func symbolEvidence(id domain.ID, relations []domain.Relation) []domain.ID {
	var out []domain.ID
	for _, relation := range relations {
		if relation.FromID == id || relation.ToID != nil && *relation.ToID == id {
			if relation.Layer != domain.LayerStatic {
				continue
			}
			out = append(out, relation.EvidenceIDs...)
		}
	}
	return uniqueIDs(out)
}

func existingEvidence(ids []domain.ID, evidence map[domain.ID]domain.Evidence) []domain.ID {
	var out []domain.ID
	for _, id := range ids {
		if _, ok := evidence[id]; ok {
			out = append(out, id)
		}
	}
	return uniqueIDs(out)
}

func pathEvidence(relationIDs []domain.ID, relations []domain.Relation, evidence map[domain.ID]domain.Evidence) []domain.ID {
	byID := make(map[domain.ID]domain.Relation, len(relations))
	for _, relation := range relations {
		byID[relation.ID] = relation
	}
	var out []domain.ID
	for _, id := range relationIDs {
		if relation, ok := byID[id]; ok {
			out = append(out, existingEvidence(relation.EvidenceIDs, evidence)...)
		}
	}
	return uniqueIDs(out)
}

func isSubreportRelation(kind string) bool {
	return strings.Contains(strings.ToLower(kind), "subreport")
}

func idsAsStrings(values []domain.ID) []string {
	out := make([]string, len(values))
	for i, id := range values {
		out[i] = string(id)
	}
	return out
}

func uniqueIDs(values []domain.ID) []domain.ID {
	seen := map[domain.ID]bool{}
	out := make([]domain.ID, 0, len(values))
	for _, id := range values {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func nonNilIDs(values []domain.ID) []domain.ID {
	if values == nil {
		return []domain.ID{}
	}
	return values
}
