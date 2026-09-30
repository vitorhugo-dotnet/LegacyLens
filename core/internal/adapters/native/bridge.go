package native

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrUnknownOrigin = errors.New("native messaging extension origin is not provisioned")

var extensionIDPattern = regexp.MustCompile(`^[a-p]{32}$`)

type Bridge struct {
	Origin         string
	AllowedOrigins []string
	APIAddress     string
	HostToken      string
}

func (b Bridge) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	origin, ok := normalizeExtensionOrigin(b.Origin)
	if !ok || !isProvisionedOrigin(origin, b.AllowedOrigins) {
		return ErrUnknownOrigin
	}
	endpoint, err := apiEndpoint(b.APIAddress)
	if err != nil || b.HostToken == "" {
		return errors.New("local core discovery is unavailable")
	}
	transport := newLoopbackTransport()
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 30 * time.Second, Transport: transport}
	for {
		request, err := ReadFrame(in, MaxFrameSize)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		body, err := json.Marshal(request)
		if err != nil {
			return errors.New("native message could not be forwarded")
		}
		httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return errors.New("local core request could not be created")
		}
		httpRequest.Header.Set("Authorization", "Bearer "+b.HostToken)
		httpRequest.Header.Set("Origin", origin)
		httpRequest.Header.Set("Content-Type", "application/json")
		response, err := client.Do(httpRequest)
		if err != nil {
			return errors.New("local core is unavailable")
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, int64(MaxFrameSize)+1))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil || len(responseBody) > int(MaxFrameSize) {
			return errors.New("local core response is invalid")
		}
		var result Envelope
		decoder := json.NewDecoder(bytes.NewReader(responseBody))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&result); err != nil {
			return errors.New("local core response is invalid")
		}
		if err := WriteFrame(out, result); err != nil {
			return err
		}
	}
}

func isProvisionedOrigin(origin string, allowed []string) bool {
	if strings.ContainsAny(origin, "\r\n") {
		return false
	}
	for _, candidate := range allowed {
		allowedOrigin, ok := normalizeExtensionOrigin(candidate)
		if ok && subtle.ConstantTimeCompare([]byte(origin), []byte(allowedOrigin)) == 1 {
			return true
		}
	}
	return false
}

func normalizeExtensionOrigin(value string) (string, bool) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "chrome-extension" || !extensionIDPattern.MatchString(u.Host) || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Host, "/:\r\n") {
		return "", false
	}
	return "chrome-extension://" + u.Host, true
}

func apiEndpoint(address string) (string, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid local core address")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("local core address must be loopback")
	}
	if _, err := strconv.Atoi(u.Port()); err != nil || u.Port() == "" {
		return "", errors.New("local core port is invalid")
	}
	return strings.TrimRight(address, "/") + "/v1/commands", nil
}

func newLoopbackTransport() *http.Transport {
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
				return nil, errors.New("local core address must be loopback")
			}
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(host, port))
		},
	}
}
