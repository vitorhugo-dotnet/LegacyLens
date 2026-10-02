package jrxml

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

func analyzeFixture(t *testing.T, name string) application.AnalysisResult {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "fixtures", "static", "reports", name)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	artifactPath := "reports/" + name
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "project", RevisionID: "revision",
		Artifacts: []domain.Artifact{{ID: "artifact", Path: artifactPath, Language: "jrxml"}},
		Sources:   map[string][]byte{artifactPath: source},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestAnalyzerExtractsReportQueryFieldsExpressionsAndSubreport(t *testing.T) {
	result := analyzeFixture(t, "orders.jrxml")
	counts := map[string]int{}
	for _, symbol := range result.Symbols {
		counts[symbol.Kind]++
	}
	for kind, want := range map[string]int{"report": 1, "query": 1, "table": 2, "field": 2, "expression": 1, "subreport": 1} {
		if counts[kind] != want {
			t.Errorf("%s symbols = %d, want %d; symbols=%#v", kind, counts[kind], want, result.Symbols)
		}
	}
	if len(result.Relations) < 6 || len(result.Evidence) < 7 {
		t.Fatalf("relations/evidence not extracted: %d/%d", len(result.Relations), len(result.Evidence))
	}
	for _, symbol := range result.Symbols {
		if symbol.Location == nil || symbol.Location.Line < 1 {
			t.Errorf("symbol has no source line: %#v", symbol)
		}
	}
}

func TestAnalyzerReportsDynamicSubreportExpression(t *testing.T) {
	result := analyzeFixture(t, "summary.jrxml")
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "jrxml.subreport_dynamic" && diagnostic.Location != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("dynamic subreport diagnostic with location missing: %#v", result.Diagnostics)
	}
	for _, relation := range result.Relations {
		if relation.Kind != "report-subreport" {
			continue
		}
		if relation.Resolution != domain.ResolutionDynamic {
			t.Fatalf("dynamic subreport relation resolution = %q, want %q", relation.Resolution, domain.ResolutionDynamic)
		}
		if len(relation.EvidenceIDs) != 1 {
			t.Fatalf("dynamic subreport relation lost its source evidence: relation=%#v evidence=%#v", relation, result.Evidence)
		}
		for _, evidence := range result.Evidence {
			if evidence.ID == relation.EvidenceIDs[0] && evidence.Source == `$P{REPORT_DIR} + "/detail.jasper"` {
				return
			}
		}
		t.Fatalf("dynamic subreport source evidence missing: relation=%#v evidence=%#v", relation, result.Evidence)
	}
	t.Fatal("report-subreport relation missing")
}

func TestAnalyzerDoesNotResolveExternalDTD(t *testing.T) {
	result, err := NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{
		ProjectID: "p", RevisionID: "r",
		Artifacts: []domain.Artifact{{ID: "a", Path: "unsafe.jrxml", Language: "jrxml"}},
		Sources:   map[string][]byte{"unsafe.jrxml": []byte(`<?xml version="1.0"?><!DOCTYPE jasperReport SYSTEM "file:///etc/passwd"><jasperReport><queryString>select '&external;'</queryString></jasperReport>`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, evidence := range result.Evidence {
		if strings.Contains(evidence.Source, "root:") || strings.Contains(evidence.Source, "external;") {
			t.Fatalf("external DTD content was read or emitted: %#v", evidence)
		}
	}
}

func TestSQLTablesIgnoresCommentsStringsAndQuotedIdentifiers(t *testing.T) {
	sql := `select 'from fake_table join ghost on 1=1' as note from actual_orders /* join commented_table */ join real_customers c on c.id=1 -- from hidden_table
where note = 'join another_fake' and "from quoted_identifier" = 'x'`
	got := strings.Join(sqlTables(sql), ",")
	if got != "actual_orders,real_customers" {
		t.Fatalf("sqlTables = %q", got)
	}
}

func TestSQLTablesExtractsCommaSeparatedFromTables(t *testing.T) {
	sql := "SELECT * FROM orders o, customers c WHERE c.id = o.customer_id"
	got := strings.Join(sqlTables(sql), ",")
	if got != "orders,customers" {
		t.Fatalf("sqlTables = %q, want orders,customers", got)
	}
	subquery := "SELECT COALESCE(o.name, c.name) FROM orders o, customers c WHERE o.id IN (SELECT d.order_id FROM details d)"
	got = strings.Join(sqlTables(subquery), ",")
	if got != "orders,customers,details" {
		t.Fatalf("sqlTables with expression and subquery = %q, want orders,customers,details", got)
	}
}

func TestAnalyzerKeepsExtensionElementsOutOfJRXML(t *testing.T) {
	source := []byte(`<jasperReport xmlns="http://jasperreports.sourceforge.net/jasperreports" xmlns:x="urn:vendor"><queryString>select * from real_table</queryString><field name="real"/><x:queryString>select * from fake_table</x:queryString><x:field name="fake"/></jasperReport>`)
	result, err := analyzeSource(t, "namespaced.jrxml", source)
	if err != nil {
		t.Fatal(err)
	}
	if countKind(result.Symbols, "query") != 1 || countKind(result.Symbols, "field") != 1 || countKind(result.Symbols, "table") != 1 {
		t.Fatalf("extension elements were interpreted: %#v", result.Symbols)
	}
	legacy, err := analyzeSource(t, "legacy.jrxml", []byte(`<jasperReport><field name="legacy"/></jasperReport>`))
	if err != nil || countKind(legacy.Symbols, "field") != 1 {
		t.Fatalf("legacy namespace-free report rejected: result=%#v err=%v", legacy, err)
	}
}

func TestAnalyzerScopesSubDatasetContents(t *testing.T) {
	source := []byte(`<jasperReport><queryString>select * from report_table</queryString><field name="reportField"/><subDataset name="items"><queryString>select * from item_table</queryString><field name="itemField"/></subDataset></jasperReport>`)
	result, err := analyzeSource(t, "dataset.jrxml", source)
	if err != nil {
		t.Fatal(err)
	}
	if countKind(result.Symbols, "subdataset") != 1 || countKind(result.Symbols, "query") != 2 || countKind(result.Symbols, "field") != 2 || countKind(result.Symbols, "table") != 2 {
		t.Fatalf("dataset contents missing or mis-scoped: %#v", result.Symbols)
	}
	for _, symbol := range result.Symbols {
		if symbol.Kind == "field" && strings.HasSuffix(symbol.QualifiedName, "#itemField") && !strings.Contains(symbol.QualifiedName, "items") {
			t.Fatalf("dataset field attributed to main report: %#v", symbol)
		}
	}
}

func TestAnalyzerParseDiagnosticUsesErrorPosition(t *testing.T) {
	result, err := analyzeSource(t, "broken.jrxml", []byte("<jasperReport>\n  <field>\n</jasperReport>\nignored tail\nmore tail"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Location == nil || result.Diagnostics[0].Location.Line != 3 {
		t.Fatalf("expected malformed close tag location on line 3, got %#v", result.Diagnostics)
	}
}

func analyzeSource(t *testing.T, path string, source []byte) (application.AnalysisResult, error) {
	t.Helper()
	return NewAnalyzer().Analyze(context.Background(), application.AnalysisInput{ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "a", Path: path, Language: "jrxml"}}, Sources: map[string][]byte{path: source}})
}
func countKind(symbols []domain.Symbol, kind string) int {
	count := 0
	for _, s := range symbols {
		if s.Kind == kind {
			count++
		}
	}
	return count
}
