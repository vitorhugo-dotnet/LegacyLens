package explainer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

func TestHTTPExplainerSendsOnlyConfirmedPackageWithCoreCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/explanations" { t.Fatalf("request = %s %s", r.Method, r.URL.Path) }
		if r.Header.Get("Authorization") != "Bearer core-only-secret" { t.Fatal("provider credential was absent") }
		var request struct { ProtocolVersion int `json:"protocolVersion"`; RequestID string `json:"requestId"`; Data application.ExplanationPackage `json:"data"` }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil { t.Fatal(err) }
		if request.ProtocolVersion != 1 || request.RequestID == "" || len(request.Data.Evidence) != 1 || request.Data.Evidence[0].ID != "e1" { t.Fatalf("provider request = %+v", request) }
		_ = json.NewEncoder(w).Encode(application.ExplanationResult{Claims: []application.ExplanationClaim{{Text: "DAO writes the row.", EvidenceIDs: []domain.ID{"e1"}, Confidence: "supported"}}, Limitations: []string{"Check against the deployed revision."}})
	}))
	defer server.Close()
	adapter, err := NewHTTP(HTTPConfig{Endpoint: server.URL + "/v1/explanations", Token: "core-only-secret"})
	if err != nil { t.Fatal(err) }
	result, err := adapter.Explain(context.Background(), application.ExplanationPackage{Question: "What writes this?", Evidence: []application.ExplanationEvidence{{ID: "e1", Kind: "java-method"}}})
	if err != nil { t.Fatal(err) }
	if len(result.Claims) != 1 || result.Claims[0].EvidenceIDs[0] != "e1" { t.Fatalf("result = %+v", result) }
}

func TestHTTPExplainerRejectsCredentialInURLAndRedirects(t *testing.T) {
	if _, err := NewHTTP(HTTPConfig{Endpoint: "https://user:secret@example.com/explain"}); err == nil { t.Fatal("endpoint userinfo was accepted") }
	if _, err := NewHTTP(HTTPConfig{Endpoint: "https://provider.example/explain?token=secret"}); err == nil { t.Fatal("endpoint query data was accepted") }
	if _, err := NewHTTP(HTTPConfig{Endpoint: "http://192.168.1.2/explain"}); err == nil { t.Fatal("non-loopback HTTP endpoint was accepted") }
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "https://other.example/collect", http.StatusFound) }))
	defer redirect.Close()
	adapter, err := NewHTTP(HTTPConfig{Endpoint: redirect.URL, Token: "secret"})
	if err != nil { t.Fatal(err) }
	if _, err := adapter.Explain(context.Background(), application.ExplanationPackage{Question: "q", Evidence: []application.ExplanationEvidence{{ID: "e1", Kind: "source"}}}); err == nil { t.Fatal("redirect response was accepted") }
}
