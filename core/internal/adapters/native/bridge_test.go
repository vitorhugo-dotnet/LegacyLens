package native

import (
	"bytes"
	"context"
	"errors"
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
