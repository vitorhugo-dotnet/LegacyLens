package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestAPIAgentHeartbeatRequiresAgentCredentialAndStrictIdentity(t *testing.T) {
 now := time.Date(2026,10,1,12,0,0,0,time.UTC)
 presence := application.NewAgentPresence(func() time.Time { return now },5*time.Second,2)
 handler := NewServer(Services{Presence:presence},AuthConfig{HostToken:"host-secret-token",AgentToken:"agent-secret-token"})
 body := `{"protocolVersion":1,"requestId":"beat","command":"agent.heartbeat","payload":{"projectId":"p1","producerId":"process-a"}}`
 if got := apiRequest(handler,"/v1/events","host-secret-token","",body).Code; got != http.StatusUnauthorized { t.Fatalf("host credential accepted: %d",got) }
 if got := apiRequest(handler,"/v1/commands","host-secret-token","",body).Code; got == http.StatusOK { t.Fatalf("host command accepted heartbeat: %d",got) }
 if got := apiRequest(handler,"/v1/events","agent-secret-token","",body).Code; got != http.StatusOK { t.Fatalf("agent heartbeat: %d",got) }
 extra := `{"protocolVersion":1,"requestId":"beat","command":"agent.heartbeat","payload":{"projectId":"p1","producerId":"process-a","token":"secret"}}`
 if got := apiRequest(handler,"/v1/events","agent-secret-token","",extra).Code; got != http.StatusBadRequest { t.Fatalf("extra field accepted: %d",got) }
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
func (s projectCatalogStub) PageProjects(_ context.Context, offset, limit int) (application.Page[domain.Project], error) {
	start := offset
	if start > len(s.projects) {
		start = len(s.projects)
	}
	end := start + limit
	if end > len(s.projects) {
		end = len(s.projects)
	}
	return application.NewPage(s.projects[start:end], offset, limit, len(s.projects)), nil
}

func TestAPIListsRegisteredProjects(t *testing.T) {
	projects := projectCatalogStub{projects: []domain.Project{{ID: "p1", Name: "demo", Root: `C:\src`}}}
	services := Services{Projects: application.NewProjectService(projects)}
	handler := NewServer(services, AuthConfig{HostToken: "host", AgentToken: "agent"})
	response := apiRequest(handler, "/v1/commands", "host", "", `{"protocolVersion":1,"requestId":"r1","command":"project.list","payload":{"offset":0,"limit":1}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("project.list status = %d, body=%s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"id":"p1"`) {
		t.Fatalf("project.list response omitted the registered project: %s", response.Body)
	}
}

func TestAPIProjectListReturnsRequestedLaterPage(t *testing.T) {
	projects := make([]domain.Project, 205)
	for i := range projects {
		projects[i] = domain.Project{ID: domain.ID(fmt.Sprintf("p%03d", i)), Name: fmt.Sprintf("p%03d", i), Root: "/projects"}
	}
	service := application.NewProjectService(projectCatalogStub{projects: projects})
	handler := NewServer(Services{Projects: service}, AuthConfig{HostToken: "host", AgentToken: "agent"})
	response := apiRequest(handler, "/v1/commands", "host", "", `{"protocolVersion":1,"requestId":"r1","command":"project.list","payload":{"offset":200,"limit":5}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("project.list = %d %s", response.Code, response.Body)
	}
	var wire struct {
		Result struct {
			Items   []domain.Project `json:"items"`
			Offset  int              `json:"offset"`
			Limit   int              `json:"limit"`
			Total   int              `json:"total"`
			HasMore bool             `json:"hasMore"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	result := wire.Result
	if len(result.Items) != 5 || result.Items[0].ID != "p200" || result.Offset != 200 || result.Limit != 5 || result.Total != 205 || result.HasMore {
		t.Fatalf("later project page = %+v", result)
	}
}

func TestAPIInvestigationReturnsLaterPagesForEachCollection(t *testing.T) {
	trace := domain.Trace{ID: "0123456789abcdef0123456789abcdef", ProjectID: "p1"}
	events := []domain.Event{
		{ProjectID: "p1", TraceID: trace.ID, ProducerID: "agent", Sequence: 1, EventID: "e1", Kind: "http.request"},
		{ProjectID: "p1", TraceID: trace.ID, ProducerID: "agent", Sequence: 2, EventID: "e2", Kind: "http.request"},
	}
	loaded := application.Investigation{Project: domain.Project{ID: "p1"}, Trace: trace, Events: events,
		Diagnostics: []domain.Diagnostic{{ID: "d1"}, {ID: "d2"}}, Symbols: []domain.Symbol{{ID: "s1"}, {ID: "s2"}}, Relations: []domain.Relation{{ID: "r1"}, {ID: "r2"}}, Evidence: []domain.Evidence{{ID: "v1"}, {ID: "v2"}}}
	service := application.NewCaptureService(captureStoreStub{investigation: loaded}, application.CaptureConfig{})
	handler := NewServer(Services{Captures: service, Investigations: application.NewInvestigationService(captureStoreStub{investigation: loaded})}, AuthConfig{HostToken: "host", AgentToken: "agent"})
	response := apiRequest(handler, "/v1/commands", "host", "", `{"protocolVersion":1,"requestId":"r1","command":"investigation.get","payload":{"projectId":"p1","traceId":"0123456789abcdef0123456789abcdef","offset":1,"limit":1}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("investigation.get = %d %s", response.Code, response.Body)
	}
	var wire struct {
		Result application.InvestigationPage `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	result := wire.Result
	if result.Events.Items[0].EventID != "e2" || result.Events.Total != 2 || result.Events.HasMore {
		t.Fatalf("event later page = %+v", result.Events)
	}
	if result.Diagnostics.Items[0].ID != "d2" || result.Symbols.Total != 4 || result.Relations.Items[0].ID != "r2" || result.Evidence.Total != 4 || len(result.Evidence.Items) != 1 {
		t.Fatalf("collections did not expose their later pages: %+v", result)
	}
}

type searchIndexStoreStub struct{ query application.SearchQuery }

func (searchIndexStoreStub) CommitIndex(context.Context, application.IndexResult) error { return nil }
func (s *searchIndexStoreStub) Search(_ context.Context, query application.SearchQuery) (application.SearchResult, error) {
	s.query = query
	return application.SearchResult{Symbols: []domain.Symbol{{ID: "s2"}}, Total: 3}, nil
}
func (searchIndexStoreStub) Explore(context.Context, application.GraphQuery) (application.GraphResult, error) {
	return application.GraphResult{}, nil
}

func TestAPISymbolSearchReturnsRequestedLaterPage(t *testing.T) {
	projects := projectCatalogStub{projects: []domain.Project{{ID: "p1"}}}
	store := &searchIndexStoreStub{}
	indexer := application.NewIndexer(store, projects, nil, nil)
	handler := NewServer(Services{Indexer: indexer}, AuthConfig{HostToken: "host", AgentToken: "agent"})
	response := apiRequest(handler, "/v1/commands", "host", "", `{"protocolVersion":1,"requestId":"r1","command":"symbol.search","payload":{"projectId":"p1","text":"save","offset":1,"limit":1}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("symbol.search = %d %s", response.Code, response.Body)
	}
	var wire struct {
		Result application.SearchResult `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if store.query.Offset != 1 || wire.Result.Offset != 1 || wire.Result.Limit != 1 || wire.Result.Total != 3 || !wire.Result.HasMore || len(wire.Result.Symbols) != 1 || wire.Result.Symbols[0].ID != "s2" {
		t.Fatalf("search later page query=%+v result=%+v", store.query, wire.Result)
	}
}

type captureStoreStub struct{ investigation application.Investigation }

func (s captureStoreStub) LoadProject(context.Context, domain.ID) (domain.Project, error) {
	return s.investigation.Project, nil
}
func (captureStoreStub) StartCapture(context.Context, domain.Trace, string) error { return nil }
func (captureStoreStub) StopCapture(context.Context, domain.ID, domain.ID, time.Time) error {
	return nil
}
func (captureStoreStub) MarkCaptureIncomplete(context.Context, domain.ID, domain.ID, domain.Diagnostic) error {
	return nil
}
func (captureStoreStub) AppendBounded(context.Context, []domain.Event, int, []domain.Diagnostic) (application.IngestResult, error) {
	return application.IngestResult{}, nil
}
func (s captureStoreStub) Append(context.Context, []domain.Event) (application.IngestResult, error) {
	return application.IngestResult{}, nil
}
func (s captureStoreStub) Load(context.Context, domain.ID, domain.ID) (application.Investigation, error) {
	return s.investigation, nil
}

func TestAPIRejectsEventMissingKindBeforeCallingCaptureService(t *testing.T) {
	traceID := domain.ID("0123456789abcdef0123456789abcdef")
	captures := application.NewCaptureService(captureStoreStub{investigation: application.Investigation{
		Project: domain.Project{ID: "p1"}, Trace: domain.Trace{ID: traceID, ProjectID: "p1"},
	}}, application.CaptureConfig{})
	handler := NewServer(Services{Captures: captures}, AuthConfig{HostToken: "host-secret-token", AgentToken: "agent-secret-token"})
	for _, event := range []string{
		`{"projectId":"p1","traceId":"0123456789abcdef0123456789abcdef","producerId":"agent","sequence":1,"eventId":"e1","occurredAt":"2026-09-30T12:00:00Z"}`,
		`{"projectId":"p1","traceId":"0123456789abcdef0123456789abcdef","producerId":"agent","sequence":0,"eventId":"e1","kind":"http.request","occurredAt":"2026-09-30T12:00:00Z"}`,
	} {
		body := `{"protocolVersion":1,"requestId":"r1","command":"trace.ingest","payload":{"projectId":"p1","events":[` + event + `]}}`
		response := apiRequest(handler, "/v1/commands", "host-secret-token", "", body)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"INVALID_PAYLOAD"`) {
			t.Fatalf("invalid event response = %d %s", response.Code, response.Body)
		}
	}
}

type recordingCaptureStore struct {
	captureStoreStub
	accepted []domain.Event
}

func (s *recordingCaptureStore) AppendBounded(_ context.Context, events []domain.Event, _ int, _ []domain.Diagnostic) (application.IngestResult, error) {
	s.accepted = append(s.accepted, events...)
	return application.IngestResult{Accepted: len(events)}, nil
}
func (s *recordingCaptureStore) Load(_ context.Context, projectID, traceID domain.ID) (application.Investigation, error) {
	return application.Investigation{Project: domain.Project{ID: projectID}, Trace: domain.Trace{ID: traceID, ProjectID: projectID, StartedAt: time.Now().UTC()}}, nil
}

func TestEventsEndpointIngestsAndSanitizesStructuredJavaIdentity(t *testing.T) {
	store := &recordingCaptureStore{}
	captures := application.NewCaptureService(store, application.CaptureConfig{})
	handler := NewServer(Services{Captures: captures}, AuthConfig{HostToken: "host-secret-token", AgentToken: "agent-secret-token"})
	metadata := `"metadata":{"code.class":"com.example.OrderService","code.method":"loadOrder","code.descriptor":"(Ljava/lang/String;)V","code.deployment":"deployment@loader-19af","code.line_missing":"true","code.arguments":"customer-secret","code.exception":"password=secret"}`
	event := `{"projectId":"p1","traceId":"0123456789abcdef0123456789abcdef","producerId":"java-agent","sequence":1,"eventId":"e1","kind":"method.error","occurredAt":"2026-09-30T12:00:00Z",` + metadata + `}`
	diagnostic := `{"projectId":"p1","traceId":"0123456789abcdef0123456789abcdef","producerId":"java-agent","sequence":2,"eventId":"e2","kind":"agent.loss","occurredAt":"2026-09-30T12:00:01Z","metadata":{"agent.dropped_count":"1","code.exception":"password=secret"}}`
	body := `{"protocolVersion":1,"requestId":"r1","command":"trace.ingest","payload":{"projectId":"p1","events":[` + event + `,` + diagnostic + `]}}`
	response := apiRequest(handler, "/v1/events", "agent-secret-token", "", body)
	if response.Code != http.StatusOK {
		t.Fatalf("event ingest response=%d %s", response.Code, response.Body)
	}
	if len(store.accepted) != 2 {
		t.Fatalf("accepted events=%d, want 2", len(store.accepted))
	}
	got := store.accepted[0].Metadata
	for key, want := range map[string]string{"code.class": "com.example.OrderService", "code.method": "loadOrder", "code.descriptor": "(Ljava/lang/String;)V", "code.deployment": "deployment@loader-19af", "code.line_missing": "true"} {
		if got[key] != want {
			t.Errorf("safe metadata %s=%q want %q", key, got[key], want)
		}
	}
	for _, key := range []string{"code.arguments", "code.exception"} {
		if _, ok := got[key]; ok {
			t.Errorf("unsafe field %s persisted: %#v", key, got)
		}
	}
	if store.accepted[1].Metadata["agent.dropped_count"] != "1" {
		t.Fatalf("safe loss diagnostic did not survive ingest sanitization: %#v", store.accepted[1].Metadata)
	}
	if _, ok := store.accepted[1].Metadata["code.exception"]; ok {
		t.Fatalf("loss event exception text persisted: %#v", store.accepted[1].Metadata)
	}
}
