package mdf

import (
	"bytes"
	"testing"
)

func TestValidateInputRejectsInvalidUTF8(t *testing.T) {
	data := []byte{0xff, 0xfe, 0xfd}
	if err := ValidateInput(data); err != ErrInvalidUTF8 {
		t.Fatalf("expected ErrInvalidUTF8, got %v", err)
	}
}

func TestValidateInputRejectsBinary(t *testing.T) {
	data := append([]byte("hello"), 0x00)
	if err := ValidateInput(data); err != ErrBinaryInput {
		t.Fatalf("expected ErrBinaryInput, got %v", err)
	}
}

func TestStreamSimulateSkipsBinary(t *testing.T) {
	reader := bytes.NewReader([]byte{0x00, 0x01, 0x02, 0x03, 0x04})
	var out bytes.Buffer
	err := StreamSimulate(StreamSimulateRequest{
		Reader:    reader,
		Writer:    &out,
		Width:     10,
		ChunkSize: 1,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty output, got %q", out.String())
	}
}
