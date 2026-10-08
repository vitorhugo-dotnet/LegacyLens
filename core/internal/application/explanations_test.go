package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"legacylens/core/internal/domain"
)

type explanationTestProvider struct {
	calls   int
	request ExplanationPackage
	result  ExplanationResult
	err     error
}

func (p *explanationTestProvider) Destination() string {
	return "https://provider.example/v1/explanations"
}
func (p *explanationTestProvider) Explain(_ context.Context, request ExplanationPackage) (ExplanationResult, error) {
	p.calls++
	p.request = request
	return p.result, p.err
}

func explanationFixture() Investigation {
	traceID := domain.ID("0123456789abcdef0123456789abcdef")
	deployedRevision := domain.ID("revision-deployed")
	location := &domain.Location{Path: "src/OrdersDao.java", Line: 12, Column: 4}
	return Investigation{
		Project:           domain.Project{ID: "p1", Root: `C:\private\project`},
		Trace:             domain.Trace{ID: traceID, ProjectID: "p1"},
		IndexedRevisionID: "revision-indexed",
		Events:            []domain.Event{{ProjectID: "p1", TraceID: traceID, ApplicationRevision: &deployedRevision, Metadata: map[string]string{"sql": "SELECT * FROM orders WHERE note='secret-literal'"}}},
		Evidence: []domain.Evidence{
			{ID: "e1", Kind: "java-method", Source: "SELECT * FROM orders WHERE note='secret-literal'", Location: location},
			{ID: "e2", Kind: "sql-table", Source: "src/OrdersDao.java", Location: &domain.Location{Path: "src/OrdersDao.java", Line: 24, Column: 2}},
			{ID: "e3", Kind: "unselected", Source: "must-not-leave-core"},
		},
	}
}

func TestExplanationPreviewIsLocalAndContainsOnlySelectedSafeEvidence(t *testing.T) {
	provider := &explanationTestProvider{}
	service := NewExplanationService(NewInvestigationService(investigationStore{explanationFixture()}), provider)
	preview, err := service.Preview(context.Background(), ExplanationRequest{ProjectID: "p1", TraceID: explanationFixture().Trace.ID, Question: "What writes the order?", EvidenceIDs: []domain.ID{"e1", "e2"}})
	if err != nil {
		t.Fatal(err)
	}
	encodedBytes, err := json.Marshal(preview.Package)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(encodedBytes)
	if provider.calls != 0 {
		t.Fatal("preview contacted the external provider")
	}
	if preview.Destination != provider.Destination() || !preview.ProviderAvailable {
		t.Fatalf("preview destination/availability = %+v", preview)
	}
	if len(preview.Package.Evidence) != 2 || preview.Package.Evidence[0].ID != "e1" || preview.Package.Evidence[1].ID != "e2" {
		t.Fatalf("selected evidence = %+v", preview.Package.Evidence)
	}
	for _, secret := range []string{"secret-literal", "must-not-leave-core", `C:\private\project`} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("preview package exposed %q", secret)
		}
	}
}

func TestExplanationRequiresExplicitSendAndConsumesConsentOnce(t *testing.T) {
	provider := &explanationTestProvider{result: ExplanationResult{Claims: []ExplanationClaim{{Text: "The DAO writes orders.", EvidenceIDs: []domain.ID{"e1"}, Confidence: "supported"}}, Limitations: []string{}}}
	service := NewExplanationService(NewInvestigationService(investigationStore{explanationFixture()}), provider)
	preview, err := service.Preview(context.Background(), ExplanationRequest{ProjectID: "p1", TraceID: explanationFixture().Trace.ID, Question: "What writes the order?", EvidenceIDs: []domain.ID{"e1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Generate(context.Background(), ExplanationConsent{PreviewID: preview.PreviewID, Consented: false}); !errors.Is(err, ErrExplicitConsentRequired) {
		t.Fatalf("unconsented Generate error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatal("provider was called without explicit consent")
	}
	if _, err = service.Generate(context.Background(), ExplanationConsent{PreviewID: preview.PreviewID, Consented: true}); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d", provider.calls)
	}
	if _, err = service.Generate(context.Background(), ExplanationConsent{PreviewID: preview.PreviewID, Consented: true}); err == nil {
		t.Fatal("preview consent was reusable")
	}
	if provider.calls != 1 {
		t.Fatal("provider was retried with a consumed preview")
	}
}

func TestExplanationRejectsNullProviderCollections(t *testing.T) {
	for name, result := range map[string]ExplanationResult{
		"claims":      {Claims: nil, Limitations: []string{}},
		"limitations": {Claims: []ExplanationClaim{}, Limitations: nil},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &explanationTestProvider{result: result}
			service := NewExplanationService(NewInvestigationService(investigationStore{explanationFixture()}), provider)
			preview, err := service.Preview(context.Background(), ExplanationRequest{ProjectID: "p1", TraceID: explanationFixture().Trace.ID, Question: "Explain this", EvidenceIDs: []domain.ID{"e1"}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Generate(context.Background(), ExplanationConsent{PreviewID: preview.PreviewID, Consented: true})
			if err == nil || !strings.Contains(err.Error(), "explanation contract") {
				t.Fatalf("null provider collection error = %v", err)
			}
		})
	}
}

func TestUnknownEvidenceRejectedFromProviderClaims(t *testing.T) {
	provider := &explanationTestProvider{result: ExplanationResult{Claims: []ExplanationClaim{{Text: "Unsupported statement.", EvidenceIDs: []domain.ID{"not-in-preview"}, Confidence: "supported"}}, Limitations: []string{}}}
	service := NewExplanationService(NewInvestigationService(investigationStore{explanationFixture()}), provider)
	preview, err := service.Preview(context.Background(), ExplanationRequest{ProjectID: "p1", TraceID: explanationFixture().Trace.ID, Question: "Explain this", EvidenceIDs: []domain.ID{"e1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Generate(context.Background(), ExplanationConsent{PreviewID: preview.PreviewID, Consented: true}); !errors.Is(err, ErrUnknownExplanationEvidence) {
		t.Fatalf("unknown evidence error = %v", err)
	}
}
