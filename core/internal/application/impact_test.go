package application

import (
	"context"
	"strings"
	"testing"

	"legacylens/core/internal/domain"
)

type impactIndexFixture struct {
	snapshot InvestigationIndex
	search   SearchResult
	query    SearchQuery
}

func (s *impactIndexFixture) CommitIndex(context.Context, IndexResult) error { return nil }
func (s *impactIndexFixture) Search(_ context.Context, query SearchQuery) (SearchResult, error) {
	s.query = query
	return s.search, nil
}
func (*impactIndexFixture) Explore(context.Context, GraphQuery) (GraphResult, error) {
	return GraphResult{}, nil
}
func (s *impactIndexFixture) LoadInvestigationIndex(context.Context, domain.ID) (InvestigationIndex, error) {
	return s.snapshot, nil
}

type impactProjectFixture struct{ project domain.Project }

func (s impactProjectFixture) SaveProject(context.Context, domain.Project) error { return nil }
func (s impactProjectFixture) LoadProject(_ context.Context, id domain.ID) (domain.Project, error) {
	if id != s.project.ID {
		return domain.Project{}, context.Canceled
	}
	return s.project, nil
}

func TestSearchServiceUsesIndexStoreAndReportsPage(t *testing.T) {
	store := &impactIndexFixture{snapshot: InvestigationIndex{RevisionID: "rev-current"}, search: SearchResult{Symbols: []domain.Symbol{{ID: "s2"}}, Total: 3}}
	service := NewSearchService(store, impactProjectFixture{project: domain.Project{ID: "p1"}})
	got, err := service.Search(context.Background(), SearchQuery{ProjectID: "p1", Text: "save", Offset: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if store.query.RevisionID != "rev-current" || store.query.Offset != 1 || store.query.Limit != 1 || got.Offset != 1 || got.Limit != 1 || !got.HasMore || len(got.Symbols) != 1 || got.Symbols[0].ID != "s2" {
		t.Fatalf("Search() query=%+v result=%+v", store.query, got)
	}
}

func TestImpactServiceWalksBackToEvidenceBackedSameNameTables(t *testing.T) {
	target, reportTable, method, dao, view, report, query, subreport := domain.ID("jrxml-orders"), domain.ID("java-orders"), domain.ID("loadOrders"), domain.ID("orders-dao"), domain.ID("orders-view"), domain.ID("orders-report"), domain.ID("orders-query"), domain.ID("dynamic-subreport")
	to := func(id domain.ID) *domain.ID { return &id }
	fixture := &impactIndexFixture{snapshot: InvestigationIndex{
		RevisionID: "rev-1",
		Symbols: []domain.Symbol{
			{ID: target, ProjectID: "p1", RevisionID: "rev-1", Path: "reports/orders.jrxml", QualifiedName: "orders", Kind: "table"},
			{ID: reportTable, ProjectID: "p1", RevisionID: "rev-1", Path: "src/OrdersDao.java", QualifiedName: "ORDERS", Kind: "table"},
			{ID: method, ProjectID: "p1", RevisionID: "rev-1", Path: "src/OrdersDao.java", QualifiedName: "OrdersDao.loadOrders", Kind: "method"},
			{ID: dao, ProjectID: "p1", RevisionID: "rev-1", Path: "src/OrdersDao.java", QualifiedName: "OrdersDao", Kind: "class"},
			{ID: view, ProjectID: "p1", RevisionID: "rev-1", Path: "views/orders.xhtml", QualifiedName: "orders.xhtml", Kind: "view"},
			{ID: report, ProjectID: "p1", RevisionID: "rev-1", Path: "reports/orders.jrxml", QualifiedName: "ordersReport", Kind: "report"},
			{ID: query, ProjectID: "p1", RevisionID: "rev-1", Path: "reports/orders.jrxml", QualifiedName: "ordersReport#query", Kind: "query"},
			{ID: subreport, ProjectID: "p1", RevisionID: "rev-1", Path: "reports/orders.jrxml", QualifiedName: "${subreportPath}", Kind: "subreport"},
		},
		Relations: []domain.Relation{
			{ID: "java-sql", FromID: method, ToID: to(reportTable), Kind: "sql-table", EvidenceIDs: []domain.ID{"java-evidence"}, Layer: domain.LayerStatic, Resolution: domain.ResolutionResolved},
			{ID: "dao-method", FromID: dao, ToID: to(method), Kind: "calls", EvidenceIDs: []domain.ID{"dao-evidence"}, Layer: domain.LayerStatic, Resolution: domain.ResolutionResolved},
			{ID: "view-report", FromID: view, ToID: to(report), Kind: "report-reference", EvidenceIDs: []domain.ID{"view-evidence"}, Layer: domain.LayerStatic, Resolution: domain.ResolutionResolved},
			{ID: "report-query", FromID: report, ToID: to(query), Kind: "report-query", EvidenceIDs: []domain.ID{"report-evidence"}, Layer: domain.LayerStatic, Resolution: domain.ResolutionResolved},
			{ID: "query-table", FromID: query, ToID: to(target), Kind: "query-table", EvidenceIDs: []domain.ID{"table-evidence"}, Layer: domain.LayerStatic, Resolution: domain.ResolutionResolved},
			{ID: "dynamic-subreport-edge", FromID: report, ToID: to(subreport), Kind: "report-subreport", EvidenceIDs: []domain.ID{"subreport-evidence"}, Layer: domain.LayerStatic, Resolution: domain.ResolutionDynamic},
			{ID: "missing-subreport-edge", FromID: report, Kind: "report-subreport", EvidenceIDs: []domain.ID{"subreport-evidence"}, Layer: domain.LayerStatic, Resolution: domain.ResolutionUnresolved},
			// This back edge closes a cycle in the Java dependency graph.
			{ID: "cycle-edge", FromID: reportTable, ToID: to(method), Kind: "uses", EvidenceIDs: []domain.ID{"java-evidence"}, Layer: domain.LayerStatic, Resolution: domain.ResolutionResolved},
		},
		Evidence:    []domain.Evidence{{ID: "java-evidence", Kind: "source", Source: "src/OrdersDao.java"}, {ID: "dao-evidence", Kind: "source", Source: "src/OrdersDao.java"}, {ID: "view-evidence", Kind: "source", Source: "views/orders.xhtml"}, {ID: "report-evidence", Kind: "source", Source: "reports/orders.jrxml"}, {ID: "table-evidence", Kind: "source", Source: "reports/orders.jrxml"}, {ID: "subreport-evidence", Kind: "source", Source: "reports/orders.jrxml"}},
		Diagnostics: []domain.Diagnostic{{ID: "dynamic-diagnostic", Code: "jrxml.subreport_dynamic", Message: "Subreport path is dynamic", Severity: "info"}},
	}}
	service := NewImpactService(fixture)
	got, err := service.Query(context.Background(), GraphQuery{ProjectID: "p1", SymbolIDs: []domain.ID{target}, Depth: 8, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if got.RevisionID != "rev-1" || got.Depth != 8 || got.Offset != 0 || got.Limit != 20 || got.Total < 4 {
		t.Fatalf("impact metadata = %+v", got)
	}
	foundInferred := false
	foundJavaPath, foundDaoPath, foundViewPath := false, false, false
	for _, relation := range got.Relations {
		if relation.Kind == "impact.table_name_match" && relation.Resolution == domain.ResolutionDynamic && len(relation.EvidenceIDs) >= 2 {
			foundInferred = true
		}
	}
	for _, path := range got.Paths {
		ids := make([]string, len(path.SymbolIDs))
		for i, id := range path.SymbolIDs {
			ids[i] = string(id)
		}
		joined := strings.Join(ids, ",")
		if strings.Contains(joined, string(method)) && strings.Contains(joined, string(target)) {
			foundJavaPath = path.Inferred
		}
		foundDaoPath = foundDaoPath || strings.Contains(joined, string(dao)) && strings.Contains(joined, string(target))
		foundViewPath = foundViewPath || strings.Contains(joined, string(view)) && strings.Contains(joined, string(target))
	}
	if !foundInferred || !foundJavaPath || !foundDaoPath || !foundViewPath {
		t.Fatalf("same-name cross-artifact link was not explicit and evidence-backed: relations=%+v paths=%+v", got.Relations, got.Paths)
	}
	if len(got.Evidence) < 4 {
		t.Fatalf("source evidence missing from impact: %+v", got.Evidence)
	}
	foundCycle, foundDynamic := false, false
	for _, diagnostic := range got.Diagnostics {
		foundCycle = foundCycle || diagnostic.Code == "impact.cycle"
		foundDynamic = foundDynamic || diagnostic.Code == "impact.dynamic_subreport"
	}
	if !foundCycle || !foundDynamic {
		t.Fatalf("cycle or dynamic-subreport diagnostics missing: %+v", got.Diagnostics)
	}
}

func TestImpactServiceBoundsDepthAndPaginatesPaths(t *testing.T) {
	target := domain.ID("table")
	to := func(id domain.ID) *domain.ID { return &id }
	fixture := &impactIndexFixture{snapshot: InvestigationIndex{
		Symbols:   []domain.Symbol{{ID: target, Kind: "table", QualifiedName: "orders"}, {ID: "method-a", Kind: "method", QualifiedName: "A"}, {ID: "method-b", Kind: "method", QualifiedName: "B"}, {ID: "method-c", Kind: "method", QualifiedName: "C"}, {ID: "dao", Kind: "class", QualifiedName: "Dao"}},
		Relations: []domain.Relation{{ID: "a", FromID: "method-a", ToID: to(target), Kind: "query-table", EvidenceIDs: []domain.ID{"e1"}, Layer: domain.LayerStatic}, {ID: "b", FromID: "method-b", ToID: to(target), Kind: "query-table", EvidenceIDs: []domain.ID{"e2"}, Layer: domain.LayerStatic}, {ID: "unbacked", FromID: "method-c", ToID: to(target), Kind: "query-table", Layer: domain.LayerStatic}, {ID: "c", FromID: "dao", ToID: to(domain.ID("method-a")), Kind: "calls", EvidenceIDs: []domain.ID{"e3"}, Layer: domain.LayerStatic}},
		Evidence:  []domain.Evidence{{ID: "e1", Source: "a.java"}, {ID: "e2", Source: "b.java"}, {ID: "e3", Source: "dao.java"}},
	}}
	service := NewImpactService(fixture)
	first, err := service.Query(context.Background(), GraphQuery{ProjectID: "p1", SymbolIDs: []domain.ID{target}, Depth: 1, Offset: 0, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.Depth != 1 || first.Offset != 0 || first.Limit != 1 || first.Total != 2 || !first.HasMore || !first.Truncated || len(first.Paths) != 1 {
		t.Fatalf("first bounded impact page = %+v", first)
	}
	second, err := service.Query(context.Background(), GraphQuery{ProjectID: "p1", SymbolIDs: []domain.ID{target}, Depth: 1, Offset: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if second.Offset != 1 || len(second.Paths) != 1 || second.Paths[0].SymbolIDs[0] == first.Paths[0].SymbolIDs[0] {
		t.Fatalf("second impact page = %+v; first=%+v", second, first)
	}
	if _, err := service.Query(context.Background(), GraphQuery{ProjectID: "p1", SymbolIDs: []domain.ID{target}, Depth: 17, Limit: 1}); err == nil {
		t.Fatal("depth above 16 was accepted")
	}
}

func TestImpactServiceReportsAmbiguousSameNameTablesWithoutJoining(t *testing.T) {
	to := func(id domain.ID) *domain.ID { return &id }
	fixture := &impactIndexFixture{snapshot: InvestigationIndex{
		RevisionID: "rev1",
		Symbols:    []domain.Symbol{{ID: "table-a", ProjectID: "p1", RevisionID: "rev1", Path: "a/report.jrxml", QualifiedName: "orders", Kind: "table"}, {ID: "table-b", ProjectID: "p1", RevisionID: "rev1", Path: "b/report.jrxml", QualifiedName: "ORDERS", Kind: "table"}, {ID: "table-c", ProjectID: "p1", RevisionID: "rev1", Path: "src/OrdersDao.java", QualifiedName: "orders", Kind: "table"}},
		Relations:  []domain.Relation{{ID: "source-a", FromID: "table-a", ToID: to(domain.ID("table-c")), Kind: "reference", EvidenceIDs: []domain.ID{"e1"}, Layer: domain.LayerStatic}, {ID: "source-b", FromID: "table-b", ToID: to(domain.ID("table-c")), Kind: "reference", EvidenceIDs: []domain.ID{"e2"}, Layer: domain.LayerStatic}},
		Evidence:   []domain.Evidence{{ID: "e1", Source: "a/report.jrxml"}, {ID: "e2", Source: "b/report.jrxml"}},
	}}
	got, err := NewImpactService(fixture).Query(context.Background(), GraphQuery{ProjectID: "p1", SymbolIDs: []domain.ID{"table-a"}, Depth: 2, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range got.Relations {
		if relation.Kind == "impact.table_name_match" {
			t.Fatalf("ambiguous source names were joined: %+v", relation)
		}
	}
	found := false
	for _, diagnostic := range got.Diagnostics {
		found = found || diagnostic.Code == "impact.table_name_ambiguous"
	}
	if !found {
		t.Fatalf("ambiguous table names lack a diagnostic: %+v", got.Diagnostics)
	}
}

func TestImpactServiceRejectsMissingIndexOrTarget(t *testing.T) {
	if _, err := NewImpactService(nil).Query(context.Background(), GraphQuery{ProjectID: "p1", SymbolIDs: []domain.ID{"x"}, Depth: 1}); err == nil {
		t.Fatal("missing index store was accepted")
	}
	if _, err := NewImpactService(&impactIndexFixture{}).Query(context.Background(), GraphQuery{ProjectID: "p1", Depth: 1}); err == nil {
		t.Fatal("missing target symbol was accepted")
	}
}

func TestImpactServiceDoesNotInferTableIdentityFromAmbiguousArtifactPath(t *testing.T) {
	fixture := &impactIndexFixture{snapshot: InvestigationIndex{
		RevisionID: "rev1",
		Symbols: []domain.Symbol{
			{ID: "table-a", ProjectID: "p1", RevisionID: "rev1", Path: "reports/orders.jrxml", QualifiedName: "orders", Kind: "table"},
			{ID: "table-b", ProjectID: "p1", RevisionID: "rev1", Path: "reports/orders.jrxml", QualifiedName: "orders", Kind: "table"},
			{ID: "table-java", ProjectID: "p1", RevisionID: "rev1", Path: "src/OrdersDao.java", QualifiedName: "orders", Kind: "table"},
		},
		Relations: []domain.Relation{
			{ID: "source-a", FromID: "table-a", Kind: "reference", EvidenceIDs: []domain.ID{"e1"}, Layer: domain.LayerStatic},
			{ID: "source-b", FromID: "table-b", Kind: "reference", EvidenceIDs: []domain.ID{"e2"}, Layer: domain.LayerStatic},
			{ID: "source-java", FromID: "table-java", Kind: "reference", EvidenceIDs: []domain.ID{"e3"}, Layer: domain.LayerStatic},
		},
		Evidence: []domain.Evidence{{ID: "e1"}, {ID: "e2"}, {ID: "e3"}},
	}}
	got, err := NewImpactService(fixture).Query(context.Background(), GraphQuery{ProjectID: "p1", SymbolIDs: []domain.ID{"table-a"}, Depth: 2, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range got.Relations {
		if relation.Kind == "impact.table_name_match" {
			t.Fatalf("ambiguous artifact path was joined: %+v", relation)
		}
	}
	found := false
	for _, diagnostic := range got.Diagnostics {
		found = found || diagnostic.Code == "impact.table_name_ambiguous"
	}
	if !found {
		t.Fatalf("ambiguous table candidates were not diagnosed: %+v", got.Diagnostics)
	}
}

func TestImpactServicePreservesConvergingPathsInDiamondDAG(t *testing.T) {
	to := func(id domain.ID) *domain.ID { return &id }
	fixture := &impactIndexFixture{snapshot: InvestigationIndex{
		Symbols: []domain.Symbol{{ID: "target"}, {ID: "left"}, {ID: "right"}, {ID: "source"}},
		Relations: []domain.Relation{
			{ID: "left-target", FromID: "left", ToID: to("target"), Kind: "uses", EvidenceIDs: []domain.ID{"e-left-target"}, Layer: domain.LayerStatic},
			{ID: "right-target", FromID: "right", ToID: to("target"), Kind: "uses", EvidenceIDs: []domain.ID{"e-right-target"}, Layer: domain.LayerStatic},
			{ID: "source-left", FromID: "source", ToID: to("left"), Kind: "uses", EvidenceIDs: []domain.ID{"e-source-left"}, Layer: domain.LayerStatic},
			{ID: "source-right", FromID: "source", ToID: to("right"), Kind: "uses", EvidenceIDs: []domain.ID{"e-source-right"}, Layer: domain.LayerStatic},
		},
		Evidence: []domain.Evidence{{ID: "e-left-target"}, {ID: "e-right-target"}, {ID: "e-source-left"}, {ID: "e-source-right"}},
	}}
	got, err := NewImpactService(fixture).Query(context.Background(), GraphQuery{ProjectID: "p1", SymbolIDs: []domain.ID{"target"}, Depth: 3, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var sourcePaths []GraphPath
	for _, path := range got.Paths {
		if path.SourceID == "source" {
			sourcePaths = append(sourcePaths, path)
		}
	}
	if len(sourcePaths) != 2 {
		t.Fatalf("expected both converging paths from source, got %+v", sourcePaths)
	}
	for _, id := range []domain.ID{"e-left-target", "e-right-target", "e-source-left", "e-source-right"} {
		found := false
		for _, path := range sourcePaths {
			for _, evidenceID := range path.EvidenceIDs {
				found = found || evidenceID == id
			}
		}
		if !found {
			t.Errorf("evidence %q is missing from converging paths: %+v", id, sourcePaths)
		}
	}
	for _, diagnostic := range got.Diagnostics {
		if diagnostic.Code == "impact.cycle" {
			t.Fatalf("acyclic convergence was diagnosed as a cycle: %+v", got.Diagnostics)
		}
	}
}
