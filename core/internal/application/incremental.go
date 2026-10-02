package application

import (
	"context"
	"strings"

	"legacylens/core/internal/domain"
)

// Update indexes the complete current source set. A partial path list cannot
// establish deletions, so Update always enumerates the registered project.
func (i *Indexer) Update(ctx context.Context, request IndexRequest) (IndexResult, error) {
	return i.index(ctx, IndexRequest{ProjectID: request.ProjectID}, true)
}

func (i *Indexer) resolveBrowserRelations(result *IndexResult) {
	jsFunctions := map[string][]domain.Symbol{}
	remoteCommands := map[string][]domain.Symbol{}
	for _, symbol := range result.Symbols {
		switch symbol.Kind {
		case "javascript-function":
			jsFunctions[symbol.QualifiedName] = append(jsFunctions[symbol.QualifiedName], symbol)
		case "remote-command":
			remoteCommands[symbol.QualifiedName] = append(remoteCommands[symbol.QualifiedName], symbol)
		}
	}
	evidence := make(map[domain.ID]domain.Evidence, len(result.Evidence))
	for _, item := range result.Evidence {
		evidence[item.ID] = item
	}
	for n := range result.Relations {
		relation := &result.Relations[n]
		if len(relation.EvidenceIDs) == 0 {
			continue
		}
		call := strings.TrimSuffix(evidence[relation.EvidenceIDs[0]].Source, "()")
		if relation.Resolution == domain.ResolutionUnresolved && (relation.Kind == "calls" || relation.Kind == "handler") {
			candidates := jsFunctions[call]
			if relation.Kind == "calls" && len(candidates) == 0 {
				candidates = remoteCommands[call]
			}
			if len(candidates) == 1 {
				target := candidates[0].ID
				relation.ToID = &target
				relation.Resolution = domain.ResolutionResolved
			}
		}
		if relation.Kind == "action" && i.resolver != nil {
			resolved, diagnostics := i.resolver.Resolve(evidence[relation.EvidenceIDs[0]].Source, result.Symbols)
			if len(resolved) == 1 && resolved[0].ToID != nil {
				target := *resolved[0].ToID
				relation.ToID = &target
				relation.Resolution = domain.ResolutionResolved
			}
			for _, diagnostic := range diagnostics {
				diagnostic.ID = stableID(string(result.ProjectID), string(result.RevisionID), string(relation.ID), diagnostic.Code)
				diagnostic.Location = relation.Location
				result.Diagnostics = append(result.Diagnostics, diagnostic)
			}
		}
	}
}
