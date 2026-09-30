package native

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBridgeRejectsUnknownOrigin(t *testing.T) {
	var output bytes.Buffer
	bridge := Bridge{Origin: "chrome-extension://abcdefghijklmnopabcdefghijklmnop", AllowedOrigins: []string{"chrome-extension://ponmlkjihgfedcbaponmlkjihgfedcba"}}
	err := bridge.Run(context.Background(), bytes.NewReader(nil), &output)
	if !errors.Is(err, ErrUnknownOrigin) {
		t.Fatalf("Bridge.Run() error = %v, want ErrUnknownOrigin", err)
	}
	if output.Len() != 0 {
		t.Fatalf("unknown origin produced %d bytes on protocol stdout", output.Len())
	}
}

func TestBridgeReportsOversizedResponseAndContinues(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request Envelope
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			_, _ = io.WriteString(w, `{"protocolVersion":1,"requestId":"`+request.RequestID+`","result":{"blob":"`+strings.Repeat("x", int(MaxFrameSize))+`"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"protocolVersion":1,"requestId":"`+request.RequestID+`","result":{"ok":true}}`)
	}))
	defer server.Close()
	input := append(nativeRequestFrame("r1"), nativeRequestFrame("r2")...)
	var output bytes.Buffer
	bridge := Bridge{Origin: "chrome-extension://abcdefghijklmnopabcdefghijklmnop", AllowedOrigins: []string{"chrome-extension://abcdefghijklmnopabcdefghijklmnop"}, APIAddress: server.URL, HostToken: "host"}
	if err := bridge.Run(context.Background(), bytes.NewReader(input), &output); err != nil {
		t.Fatalf("Bridge.Run() error = %v", err)
	}
	first, rest, err := readTestResponseFrame(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestID != "r1" || first.Error == nil || first.Error.Code != "RESULT_TOO_LARGE" {
		t.Fatalf("first frame = %+v", first)
	}
	second, _, err := readTestResponseFrame(rest)
	if err != nil {
		t.Fatal(err)
	}
	if second.RequestID != "r2" || second.Error != nil || string(second.Result) != `{"ok":true}` {
		t.Fatalf("second frame = %+v", second)
	}
	if requests != 2 {
		t.Fatalf("bridge forwarded %d requests, want 2", requests)
	}
}

func nativeRequestFrame(id string) []byte {
	body, _ := json.Marshal(Envelope{ProtocolVersion: 1, RequestID: id, Command: "project.list", Payload: json.RawMessage(`{}`)})
	frame := make([]byte, 4+len(body))
	binary.LittleEndian.PutUint32(frame[:4], uint32(len(body)))
	copy(frame[4:], body)
	return frame
}

func readTestResponseFrame(input []byte) (Envelope, []byte, error) {
	if len(input) < 4 {
		return Envelope{}, nil, fmt.Errorf("response header missing")
	}
	length := binary.LittleEndian.Uint32(input[:4])
	if uint32(len(input)-4) < length {
		return Envelope{}, nil, fmt.Errorf("response body truncated")
	}
	var response Envelope
	if err := json.Unmarshal(input[4:4+length], &response); err != nil {
		return Envelope{}, nil, err
	}
	return response, input[4+length:], nil
}
