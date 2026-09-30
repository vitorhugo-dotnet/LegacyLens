package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

func testServer() http.Handler {
	return NewServer(Services{}, AuthConfig{
		HostToken:        "host-secret-token",
		AgentToken:       "agent-secret-token",
		ExtensionOrigins: []string{"chrome-extension://abcdefghijklmnopabcdefghijklmnop"},
	})
}

func apiRequest(handler http.Handler, path, token, origin, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	r.RemoteAddr = "127.0.0.1:12345"
	r.Host = "127.0.0.1:12345"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, r)
	return recorder
}

func TestAPIRejectsMissingTokenAndUnknownVersion(t *testing.T) {
	handler := testServer()
	missing := apiRequest(handler, "/v1/commands", "", "", `{"protocolVersion":1,"requestId":"r1","command":"project.list","payload":{}}`)
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want 401", missing.Code)
	}

	unsupported := apiRequest(handler, "/v1/commands", "host-secret-token", "", `{"protocolVersion":2,"requestId":"r2","command":"project.list","payload":{}}`)
	if unsupported.Code != http.StatusBadRequest {
		t.Fatalf("unsupported version status = %d, want 400", unsupported.Code)
	}
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(unsupported.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != "UNSUPPORTED_PROTOCOL_VERSION" {
		t.Fatalf("unsupported version code = %q", response.Error.Code)
	}
	for _, secret := range []string{"host-secret-token", "agent-secret-token", "discovery.json"} {
		if bytes.Contains(unsupported.Body.Bytes(), []byte(secret)) {
			t.Fatalf("API response exposed %q", secret)
		}
	}
}

func TestAPIRejectsHostileOriginEvenWithToken(t *testing.T) {
	response := apiRequest(testServer(), "/v1/commands", "host-secret-token", "https://hostile.example", `{"protocolVersion":1,"requestId":"r1","command":"project.list","payload":{}}`)
	if response.Code != http.StatusForbidden {
		t.Fatalf("hostile Origin status = %d, want 403", response.Code)
	}
}

func TestAPIUsesSeparateAgentCredential(t *testing.T) {
	handler := testServer()
	body := `{"protocolVersion":1,"requestId":"r1","command":"trace.ingest","payload":{"events":[]}}`
	if got := apiRequest(handler, "/v1/events", "host-secret-token", "", body).Code; got != http.StatusUnauthorized {
		t.Fatalf("host token on events endpoint = %d, want 401", got)
	}
	if got := apiRequest(handler, "/v1/events", "agent-secret-token", "", body).Code; got == http.StatusUnauthorized {
		t.Fatal("agent token was rejected on events endpoint")
	}
}

func TestAPIRejectsInvalidPayloadAndSharedCredentials(t *testing.T) {
	invalid := apiRequest(testServer(), "/v1/commands", "host-secret-token", "", `{"protocolVersion":1,"requestId":"r1","command":"project.register","payload":{"root":42}}`)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), `"code":"INVALID_PAYLOAD"`) {
		t.Fatalf("invalid project payload = %d %s", invalid.Code, invalid.Body)
	}
	shared := NewServer(Services{}, AuthConfig{HostToken: "same-secret", AgentToken: "same-secret"})
	for _, path := range []string{"/v1/commands", "/v1/events"} {
		if got := apiRequest(shared, path, "same-secret", "", `{"protocolVersion":1,"requestId":"r1","command":"project.list","payload":{}}`).Code; got != http.StatusUnauthorized {
			t.Fatalf("shared credential on %s returned %d, want 401", path, got)
		}
	}
}

func TestAPIReportsFutureCommandAsUnsupported(t *testing.T) {
	response := apiRequest(testServer(), "/v1/commands", "host-secret-token", "", `{"protocolVersion":1,"requestId":"r1","command":"project.archive","payload":{}}`)
	if response.Code != http.StatusNotImplemented || !strings.Contains(response.Body.String(), `"code":"UNSUPPORTED_COMMAND"`) {
		t.Fatalf("unsupported command = %d %s", response.Code, response.Body)
	}
}

func TestAPIReportsUnimplementedImpactAsUnsupported(t *testing.T) {
	request := `{"protocolVersion":1,"requestId":"r1","command":"impact.query","payload":{"projectId":"p1","symbolId":"s1"}}`
	response := apiRequest(testServer(), "/v1/commands", "host-secret-token", "", request)
	if response.Code != http.StatusNotImplemented || !strings.Contains(response.Body.String(), `"code":"UNSUPPORTED_COMMAND"`) {
		t.Fatalf("unimplemented impact command = %d %s", response.Code, response.Body)
	}
}

type projectCatalogStub struct{ projects []domain.Project }

func (s projectCatalogStub) SaveProject(context.Context, domain.Project) error { return nil }
func (s projectCatalogStub) LoadProject(_ context.Context, id domain.ID) (domain.Project, error) {
	for _, project := range s.projects {
		if project.ID == id {
			return project, nil
		}
	}
	return domain.Project{}, context.Canceled
}
func (s projectCatalogStub) ListProjects(context.Context) ([]domain.Project, error) {
	return s.projects, nil
}

func TestAPIListsRegisteredProjects(t *testing.T) {
	projects := projectCatalogStub{projects: []domain.Project{{ID: "p1", Name: "demo", Root: `C:\src`}}}
	services := Services{Projects: application.NewProjectService(projects)}
	handler := NewServer(services, AuthConfig{HostToken: "host", AgentToken: "agent"})
	response := apiRequest(handler, "/v1/commands", "host", "", `{"protocolVersion":1,"requestId":"r1","command":"project.list","payload":{}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("project.list status = %d, body=%s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"id":"p1"`) {
		t.Fatalf("project.list response omitted the registered project: %s", response.Body)
	}
}
