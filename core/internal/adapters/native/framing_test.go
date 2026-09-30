package native

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type chunkReader struct {
	reader *bytes.Reader
	chunk  int
	read   int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(p) > r.chunk {
		p = p[:r.chunk]
	}
	n, err := r.reader.Read(p)
	r.read += n
	return n, err
}

func TestFramePartialReadAndOversize(t *testing.T) {
	t.Run("reads a valid frame delivered in fragments", func(t *testing.T) {
		body := []byte(`{"protocolVersion":1,"requestId":"r1","command":"project.list","payload":{}}`)
		framed := make([]byte, 4+len(body))
		binary.LittleEndian.PutUint32(framed, uint32(len(body)))
		copy(framed[4:], body)
		reader := &chunkReader{reader: bytes.NewReader(framed), chunk: 2}

		got, err := ReadFrame(reader, 512*1024)
		if err != nil {
			t.Fatalf("ReadFrame() error = %v", err)
		}
		if got.RequestID != "r1" || got.Command != "project.list" {
			t.Fatalf("ReadFrame() = %+v", got)
		}
	})

	t.Run("rejects a length over the cap before reading its body", func(t *testing.T) {
		var header [4]byte
		binary.LittleEndian.PutUint32(header[:], 512*1024+1)
		reader := &chunkReader{reader: bytes.NewReader(append(header[:], bytes.Repeat([]byte{'x'}, 512*1024+1)...)), chunk: 4}

		_, err := ReadFrame(reader, 512*1024)
		if !errors.Is(err, ErrFrameTooLarge) {
			t.Fatalf("ReadFrame() error = %v, want ErrFrameTooLarge", err)
		}
		if reader.read != 4 {
			t.Fatalf("ReadFrame() consumed %d bytes, want only the 4-byte header", reader.read)
		}
	})
}

func TestWriteFrameProducesLittleEndianJSON(t *testing.T) {
	var output bytes.Buffer
	want, _ := json.Marshal(Envelope{ProtocolVersion: 1, RequestID: "r1", Command: "project.list", Payload: json.RawMessage(`{}`)})
	if err := WriteFrame(&output, Envelope{ProtocolVersion: 1, RequestID: "r1", Command: "project.list", Payload: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint32(output.Bytes()[:4]); got != uint32(len(want)) {
		t.Fatalf("frame length = %d, want %d", got, len(want))
	}
	if !bytes.Equal(output.Bytes()[4:], want) {
		t.Fatalf("frame payload = %q, want %q", output.Bytes()[4:], want)
	}
	if _, err := ReadFrame(io.NopCloser(bytes.NewReader(output.Bytes())), 512*1024); err != nil {
		t.Fatalf("round trip ReadFrame() error = %v", err)
	}
}

func TestReadFrameRejectsInvalidUTF8(t *testing.T) {
	body := []byte("{\"protocolVersion\":1,\"requestId\":\"r1\",\"command\":\"project.list\",\"payload\":{\"x\":\"")
	body = append(body, 0xff)
	body = append(body, []byte("\"}}")...)
	framed := make([]byte, 4+len(body))
	binary.LittleEndian.PutUint32(framed, uint32(len(body)))
	copy(framed[4:], body)
	if _, err := ReadFrame(bytes.NewReader(framed), 512*1024); err == nil {
		t.Fatal("ReadFrame accepted a non-UTF-8 JSON frame")
	}
}
