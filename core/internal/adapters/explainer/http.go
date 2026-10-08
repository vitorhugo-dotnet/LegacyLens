package explainer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"legacylens/core/internal/application"
)

const maxProviderResponseBytes = 256 * 1024

type HTTPConfig struct {
	Endpoint string
	Token    string
	Timeout  time.Duration
}

type HTTPExplainer struct {
	endpoint string
	token    string
	client   *http.Client
}

func NewHTTP(config HTTPConfig) (*HTTPExplainer, error) {
	endpoint, err := url.Parse(strings.TrimSpace(config.Endpoint))
	if err != nil || endpoint == nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("explanation endpoint must be an absolute URL without credentials or query data")
	}
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && isLoopbackHost(endpoint.Hostname())) {
		return nil, errors.New("explanation endpoint must use HTTPS or HTTP loopback")
	}
	token := strings.TrimSpace(config.Token)
	if len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("explanation provider credential is invalid")
	}
	timeout := config.Timeout
	if timeout <= 0 || timeout > 2*time.Minute { timeout = 20 * time.Second }
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &HTTPExplainer{endpoint: endpoint.String(), token: token, client: client}, nil
}

func (h *HTTPExplainer) Destination() string {
	if h == nil { return "" }
	return h.endpoint
}

func (h *HTTPExplainer) Explain(ctx context.Context, data application.ExplanationPackage) (application.ExplanationResult, error) {
	if h == nil || h.client == nil { return application.ExplanationResult{}, errors.New("explanation provider is unavailable") }
	requestID, err := newRequestID()
	if err != nil { return application.ExplanationResult{}, errors.New("provider request could not be created") }
	body, err := json.Marshal(struct {
		ProtocolVersion int                               `json:"protocolVersion"`
		RequestID       string                            `json:"requestId"`
		Data            application.ExplanationPackage  `json:"data"`
	}{ProtocolVersion: 1, RequestID: requestID, Data: data})
	if err != nil { return application.ExplanationResult{}, errors.New("provider request could not be encoded") }
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(body))
	if err != nil { return application.ExplanationResult{}, errors.New("provider request could not be created") }
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if h.token != "" { request.Header.Set("Authorization", "Bearer "+h.token) }
	response, err := h.client.Do(request)
	if err != nil { return application.ExplanationResult{}, errors.New("provider request failed") }
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 { return application.ExplanationResult{}, errors.New("provider returned an unsuccessful status") }
	limited := io.LimitReader(response.Body, maxProviderResponseBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil || len(responseBody) > maxProviderResponseBytes { return application.ExplanationResult{}, errors.New("provider response is unavailable or too large") }
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	var result application.ExplanationResult
	if err := decoder.Decode(&result); err != nil || decoder.Decode(new(any)) != io.EOF { return application.ExplanationResult{}, errors.New("provider response does not match the explanation contract") }
	return result, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") { return true }
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func newRequestID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil { return "", err }
	return hex.EncodeToString(value), nil
}
