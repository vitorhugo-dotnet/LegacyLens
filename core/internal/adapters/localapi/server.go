package localapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

const (
	protocolVersion = 1
	maxRequestBytes = 512 * 1024
	maxPageSize     = 200
	maxEventBatch   = 10_000
)

type Services struct {
	Projects       *application.ProjectService
	Indexer        *application.Indexer
	Search         *application.SearchService
	Impact         *application.ImpactService
	Captures       *application.CaptureService
	Presence       *application.AgentPresence
	Investigations *application.InvestigationService
	Locations      *application.LocationService
}

type AuthConfig struct {
	HostToken        string
	AgentToken       string
	ExtensionOrigins []string
}

type envelope struct {
	ProtocolVersion int             `json:"protocolVersion"`
	RequestID       string          `json:"requestId"`
	Command         string          `json:"command"`
	Payload         json.RawMessage `json:"payload"`
}

type errorPayload struct {
	Code          string   `json:"code"`
	Message       string   `json:"message"`
	DiagnosticIDs []string `json:"diagnosticIds"`
}

type response struct {
	ProtocolVersion int           `json:"protocolVersion"`
	RequestID       string        `json:"requestId"`
	Result          any           `json:"result,omitempty"`
	Error           *errorPayload `json:"error,omitempty"`
}

type server struct {
	services  Services
	auth      AuthConfig
	validAuth bool
}

func NewServer(services Services, auth AuthConfig) http.Handler {
	return &server{services: services, auth: auth, validAuth: auth.HostToken != "" && auth.AgentToken != "" && auth.HostToken != auth.AgentToken}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	origin := r.Header.Get("Origin")
	if origin != "" && !s.allowedOrigin(origin) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Max-Age", "300")
	}
	if r.Method == http.MethodOptions {
		if origin == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.URL.Path == "/v1/health" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
		return
	}
	if r.URL.Path != "/v1/commands" && r.URL.Path != "/v1/events" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/v1/events" {
		if !s.validAuth || !tokenMatches(r.Header.Get("Authorization"), s.auth.AgentToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	} else if !s.validAuth || !tokenMatches(r.Header.Get("Authorization"), s.auth.HostToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "INVALID_ENVELOPE", "Request exceeds the supported size.", "")
		return
	}
	request, parseErr := decodeEnvelope(body)
	if parseErr != nil {
		writeError(w, parseErr.status, parseErr.code, parseErr.message, requestIDFromBody(body))
		return
	}
	if r.URL.Path == "/v1/events" && request.Command != "trace.ingest" && request.Command != "agent.heartbeat" {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "The events endpoint accepts agent commands only.", request.RequestID)
		return
	}
	if request.Command == "agent.heartbeat" {
		if r.URL.Path != "/v1/events" || s.services.Presence == nil {
			writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "Agent heartbeat is unavailable.", request.RequestID)
			return
		}
		var payload struct {
			ProjectID  domain.ID `json:"projectId"`
			ProducerID domain.ID `json:"producerId"`
		}
		decoder := json.NewDecoder(bytes.NewReader(request.Payload))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&payload) != nil || decoder.Decode(new(any)) != io.EOF || !safePresenceID.MatchString(string(payload.ProjectID)) || !safePresenceID.MatchString(string(payload.ProducerID)) {
			writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "Agent heartbeat identity is invalid.", request.RequestID)
			return
		}
		if err := s.services.Presence.Heartbeat(payload.ProjectID, payload.ProducerID); err != nil {
			writeError(w, http.StatusServiceUnavailable, "AGENT_PRESENCE_LIMIT", "Agent presence is unavailable.", request.RequestID)
			return
		}
		writeResponse(w, http.StatusOK, response{ProtocolVersion: protocolVersion, RequestID: request.RequestID, Result: map[string]bool{"accepted": true}})
		return
	}
	result, failure := s.execute(r.Context(), request)
	if failure != nil {
		writeError(w, failure.status, failure.code, failure.message, request.RequestID)
		return
	}
	writeResponse(w, http.StatusOK, response{ProtocolVersion: protocolVersion, RequestID: request.RequestID, Result: result})
}

var safePresenceID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type apiError struct {
	status  int
	code    string
	message string
}

func decodeEnvelope(body []byte) (envelope, *apiError) {
	if !utf8.Valid(body) {
		return envelope{}, &apiError{http.StatusBadRequest, "INVALID_ENVELOPE", "Request envelope is invalid."}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request envelope
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF {
		return envelope{}, &apiError{http.StatusBadRequest, "INVALID_ENVELOPE", "Request envelope is invalid."}
	}
	if request.ProtocolVersion != protocolVersion {
		return request, &apiError{http.StatusBadRequest, "UNSUPPORTED_PROTOCOL_VERSION", "The requested protocol version is not supported."}
	}
	if request.RequestID == "" || len(request.RequestID) > 128 || request.Command == "" || len(request.Command) > 100 {
		return request, &apiError{http.StatusBadRequest, "INVALID_ENVELOPE", "Request envelope is invalid."}
	}
	trimmed := bytes.TrimSpace(request.Payload)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return request, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Command payload must be an object."}
	}
	request.Payload = trimmed
	return request, nil
}

func requestIDFromBody(body []byte) string {
	var value struct {
		RequestID string `json:"requestId"`
	}
	_ = json.Unmarshal(body, &value)
	if len(value.RequestID) > 128 {
		return ""
	}
	return value.RequestID
}

func (s *server) execute(ctx context.Context, request envelope) (any, *apiError) {
	decode := func(target any) *apiError {
		decoder := json.NewDecoder(bytes.NewReader(request.Payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil || decoder.Decode(new(any)) != io.EOF {
			return &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Command payload is invalid."}
		}
		return nil
	}
	unsupported := func() (any, *apiError) {
		return nil, &apiError{http.StatusNotImplemented, "UNSUPPORTED_COMMAND", "This command is not supported by the local core."}
	}
	failed := func() (any, *apiError) {
		return nil, &apiError{http.StatusUnprocessableEntity, "COMMAND_FAILED", "The command could not be completed."}
	}

	switch request.Command {
	case "project.register":
		var payload struct {
			ProjectID domain.ID `json:"projectId"`
			Root      string    `json:"root"`
			Name      string    `json:"name"`
			Includes  []string  `json:"includes"`
			Excludes  []string  `json:"excludes"`
		}
		if err := decode(&payload); err != nil || strings.TrimSpace(payload.Root) == "" || s.services.Projects == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Project registration requires a root path."}
		}
		result, err := s.services.Projects.RegisterProject(ctx, application.ProjectConfig{ProjectID: payload.ProjectID, Root: payload.Root, Name: payload.Name, Includes: payload.Includes, Excludes: payload.Excludes})
		if err != nil {
			return failed()
		}
		return result, nil
	case "project.list":
		var payload struct {
			Offset int `json:"offset"`
			Limit  int `json:"limit"`
		}
		if err := decode(&payload); err != nil || s.services.Projects == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Project listing payload is invalid."}
		}
		offset, limit, pageErr := pageParams(payload.Offset, payload.Limit)
		if pageErr != nil {
			return nil, pageErr
		}
		result, err := s.services.Projects.ListProjectsPage(ctx, offset, limit)
		if err != nil {
			return failed()
		}
		return result, nil
	case "project.status":
		var payload struct {
			ProjectID domain.ID `json:"projectId"`
		}
		if err := decode(&payload); err != nil || payload.ProjectID == "" || s.services.Projects == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Project status requires a project id."}
		}
		result, err := s.services.Projects.GetProject(ctx, payload.ProjectID)
		if err != nil {
			return failed()
		}
		return result, nil
	case "project.index":
		var payload struct {
			ProjectID domain.ID `json:"projectId"`
			Paths     []string  `json:"paths"`
			Offset    int       `json:"offset"`
			Limit     int       `json:"limit"`
		}
		if err := decode(&payload); err != nil || payload.ProjectID == "" || s.services.Indexer == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Indexing requires a project id."}
		}
		offset, limit, pageErr := pageParams(payload.Offset, payload.Limit)
		if pageErr != nil {
			return nil, pageErr
		}
		result, err := s.services.Indexer.Index(ctx, application.IndexRequest{ProjectID: payload.ProjectID, Paths: payload.Paths})
		if err != nil {
			return failed()
		}
		return application.PageIndexResult(result, offset, limit), nil
	case "symbol.search":
		var payload struct {
			ProjectID  domain.ID `json:"projectId"`
			RevisionID domain.ID `json:"revisionId"`
			Text       string    `json:"text"`
			Kinds      []string  `json:"kinds"`
			Limit      int       `json:"limit"`
			Offset     int       `json:"offset"`
		}
		if err := decode(&payload); err != nil || payload.ProjectID == "" || payload.Text == "" || s.services.Search == nil && s.services.Indexer == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Symbol search requires a project id and text."}
		}
		offset, limit, pageErr := pageParams(payload.Offset, payload.Limit)
		if pageErr != nil {
			return nil, pageErr
		}
		query := application.SearchQuery{ProjectID: payload.ProjectID, RevisionID: payload.RevisionID, Text: payload.Text, Kinds: payload.Kinds, Limit: limit, Offset: offset}
		var result application.SearchResult
		var err error
		if s.services.Search != nil {
			result, err = s.services.Search.Search(ctx, query)
		} else {
			result, err = s.services.Indexer.Search(ctx, query)
		}
		if err != nil {
			return failed()
		}
		return result, nil
	case "impact.query":
		var payload struct {
			ProjectID  domain.ID `json:"projectId"`
			RevisionID domain.ID `json:"revisionId"`
			SymbolID   domain.ID `json:"symbolId"`
			Depth      *int      `json:"depth"`
			Offset     *int      `json:"offset"`
			Limit      *int      `json:"limit"`
		}
		if err := decode(&payload); err != nil || payload.ProjectID == "" || payload.SymbolID == "" || payload.Depth == nil || payload.Offset == nil || payload.Limit == nil || s.services.Impact == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Impact query requires a project, symbol, depth, offset, and limit."}
		}
		if *payload.Depth < 0 || *payload.Depth > 16 {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Impact depth must be between 0 and 16."}
		}
		offset, limit, pageErr := pageParams(*payload.Offset, *payload.Limit)
		if pageErr != nil {
			return nil, pageErr
		}
		result, err := s.services.Impact.Query(ctx, application.GraphQuery{ProjectID: payload.ProjectID, RevisionID: payload.RevisionID, SymbolIDs: []domain.ID{payload.SymbolID}, Depth: *payload.Depth, Offset: offset, Limit: limit})
		if err != nil {
			return failed()
		}
		return result, nil
	case "graph.explore":
		var payload struct {
			ProjectID  domain.ID   `json:"projectId"`
			RevisionID domain.ID   `json:"revisionId"`
			SymbolIDs  []domain.ID `json:"symbolIds"`
			Depth      *int        `json:"depth"`
			Offset     int         `json:"offset"`
			Limit      int         `json:"limit"`
		}
		if err := decode(&payload); err != nil || payload.ProjectID == "" || s.services.Indexer == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Graph query requires a project id."}
		}
		if payload.Depth == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Graph query requires a depth."}
		}
		offset, limit, pageErr := pageParams(payload.Offset, payload.Limit)
		if pageErr != nil {
			return nil, pageErr
		}
		result, err := s.services.Indexer.Explore(ctx, application.GraphQuery{ProjectID: payload.ProjectID, RevisionID: payload.RevisionID, SymbolIDs: payload.SymbolIDs, Depth: *payload.Depth})
		if err != nil {
			return failed()
		}
		return application.PageGraphResult(result, offset, limit), nil
	case "capture.start":
		var payload struct {
			ProjectID domain.ID `json:"projectId"`
			TabID     string    `json:"tabId"`
		}
		if err := decode(&payload); err != nil || payload.ProjectID == "" || s.services.Captures == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Capture start requires a project id."}
		}
		result, err := s.services.Captures.Start(ctx, application.CaptureRequest{ProjectID: payload.ProjectID, TabID: payload.TabID})
		if err != nil {
			return failed()
		}
		return result, nil
	case "capture.stop":
		var payload struct {
			ProjectID domain.ID `json:"projectId"`
			TraceID   domain.ID `json:"traceId"`
		}
		if err := decode(&payload); err != nil || payload.ProjectID == "" || !validTraceID(payload.TraceID) || s.services.Captures == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Capture stop requires project and trace ids."}
		}
		if err := s.services.Captures.Stop(ctx, payload.ProjectID, payload.TraceID); err != nil {
			return failed()
		}
		return map[string]bool{"stopped": true}, nil
	case "trace.ingest":
		var payload struct {
			ProjectID domain.ID      `json:"projectId"`
			Events    []domain.Event `json:"events"`
		}
		if err := decode(&payload); err != nil || payload.ProjectID == "" || payload.Events == nil || len(payload.Events) > maxEventBatch || s.services.Captures == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Event batch is invalid."}
		}
		for _, event := range payload.Events {
			if event.ProjectID != payload.ProjectID || !validTraceID(event.TraceID) || event.ProducerID == "" || event.EventID == "" || event.Sequence == 0 || strings.TrimSpace(event.Kind) == "" || event.OccurredAt.IsZero() {
				return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Event batch contains an invalid event."}
			}
		}
		result, err := s.services.Captures.Ingest(ctx, payload.Events)
		if err != nil {
			return failed()
		}
		return result, nil
	case "investigation.get":
		var payload struct {
			ProjectID domain.ID `json:"projectId"`
			TraceID   domain.ID `json:"traceId"`
			Offset    int       `json:"offset"`
			Limit     int       `json:"limit"`
		}
		if err := decode(&payload); err != nil || payload.ProjectID == "" || !validTraceID(payload.TraceID) || s.services.Investigations == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Investigation requires project and trace ids."}
		}
		offset, limit, pageErr := pageParams(payload.Offset, payload.Limit)
		if pageErr != nil {
			return nil, pageErr
		}
		investigation, err := s.services.Investigations.Get(ctx, payload.ProjectID, payload.TraceID)
		if err != nil {
			return failed()
		}
		return application.PageInvestigation(investigation, offset, limit), nil
	case "location.open":
		var payload struct {
			ProjectID domain.ID       `json:"projectId"`
			Location  domain.Location `json:"location"`
		}
		if err := decode(&payload); err != nil || payload.ProjectID == "" || payload.Location.Path == "" || payload.Location.Line < 1 || payload.Location.Column < 1 || s.services.Locations == nil {
			return nil, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Location requires a project, path, and one-based line and column."}
		}
		result, err := s.services.Locations.Open(ctx, payload.ProjectID, payload.Location)
		if err != nil {
			return failed()
		}
		return result, nil
	case "explanation.preview", "explanation.generate":
		return unsupported()
	default:
		return unsupported()
	}
}

func pageParams(offset, limit int) (int, int, *apiError) {
	if offset < 0 || offset > 1_000_000_000 || limit < 0 || limit > maxPageSize {
		return 0, 0, &apiError{http.StatusBadRequest, "INVALID_PAYLOAD", "Page offset or size is outside the supported range."}
	}
	if limit == 0 {
		limit = maxPageSize
	}
	return offset, limit, nil
}

func (s *server) allowedOrigin(origin string) bool {
	for _, allowed := range s.auth.ExtensionOrigins {
		if allowed != "" && subtle.ConstantTimeCompare([]byte(origin), []byte(allowed)) == 1 {
			return true
		}
	}
	return false
}

func tokenMatches(header, expected string) bool {
	if expected == "" || !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	provided := strings.TrimPrefix(header, "Bearer ")
	return len(provided) == len(expected) && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func validTraceID(id domain.ID) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(string(id))
	return err == nil
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return false
	}
	host = r.Host
	if strings.HasPrefix(host, "[") {
		host, _, err = net.SplitHostPort(host)
		if err != nil {
			return false
		}
	} else if parsed, _, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = parsed
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	writeResponse(w, status, response{ProtocolVersion: protocolVersion, RequestID: requestID, Error: &errorPayload{Code: code, Message: message, DiagnosticIDs: []string{}}})
}

func writeResponse(w http.ResponseWriter, status int, value response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
