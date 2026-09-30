package native

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

const MaxFrameSize uint32 = 512 * 1024

var ErrFrameTooLarge = errors.New("native message exceeds the frame limit")

type ProtocolError struct {
	Code          string   `json:"code"`
	Message       string   `json:"message"`
	DiagnosticIDs []string `json:"diagnosticIds"`
}

type Envelope struct {
	ProtocolVersion int             `json:"protocolVersion"`
	RequestID       string          `json:"requestId"`
	Command         string          `json:"command,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	Error           *ProtocolError  `json:"error,omitempty"`
}

func ReadFrame(r io.Reader, maxBytes uint32) (Envelope, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Envelope{}, err
	}
	length := binary.LittleEndian.Uint32(header[:])
	if maxBytes > MaxFrameSize {
		maxBytes = MaxFrameSize
	}
	if length > maxBytes {
		return Envelope{}, ErrFrameTooLarge
	}
	if length == 0 {
		return Envelope{}, errors.New("native message frame is empty")
	}
	body := make([]byte, int(length))
	if _, err := io.ReadFull(r, body); err != nil {
		return Envelope{}, err
	}
	if !utf8.Valid(body) {
		return Envelope{}, errors.New("native message is not UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var envelope Envelope
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, errors.New("native message is invalid JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return Envelope{}, errors.New("native message contains trailing JSON")
	}
	if envelope.ProtocolVersion < 1 || envelope.RequestID == "" || len(envelope.RequestID) > 128 {
		return Envelope{}, errors.New("native message envelope is incomplete")
	}
	if envelope.Command == "" || len(envelope.Command) > 100 || envelope.Result != nil || envelope.Error != nil {
		return Envelope{}, errors.New("native message command is invalid")
	}
	if len(envelope.Payload) == 0 || !json.Valid(envelope.Payload) || bytes.TrimSpace(envelope.Payload)[0] != '{' {
		return Envelope{}, errors.New("native message payload must be an object")
	}
	return envelope, nil
}

func WriteFrame(w io.Writer, envelope Envelope) error {
	body, err := json.Marshal(envelope)
	if err != nil {
		return errors.New("native response could not be encoded")
	}
	if len(body) == 0 || len(body) > int(MaxFrameSize) {
		return ErrFrameTooLarge
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(body)))
	if err := writeAll(w, header[:]); err != nil {
		return fmt.Errorf("write native frame header: %w", err)
	}
	return writeAll(w, body)
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
