package main

import (
	"bytes"
	"errors"
	"testing"

	"legacylens/core/internal/adapters/native"
)

func TestParentWindowArgumentIsNotAnExtensionOrigin(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"--parent-window=12345"}, bytes.NewReader(nil), &output)
	if !errors.Is(err, native.ErrUnknownOrigin) {
		t.Fatalf("host run() error = %v, want ErrUnknownOrigin", err)
	}
	if output.Len() != 0 {
		t.Fatalf("unprovisioned process wrote %d bytes to protocol stdout", output.Len())
	}
}
