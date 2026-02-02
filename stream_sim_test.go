package mdf

import (
	"bytes"
	"strings"
	"testing"
)

func TestStreamSimulatePlainText(t *testing.T) {
	input := "alpha beta gamma"
	var out bytes.Buffer
	err := StreamSimulate(StreamSimulateRequest{
		Reader:    strings.NewReader(input),
		Writer:    &out,
		Width:     6,
		ChunkSize: 2,
	})
	if err != nil {
		t.Fatalf("stream simulate: %v", err)
	}
	got := out.String()
	want := "alpha\nbeta\ngamma\n"
	if got != want {
		t.Fatalf("unexpected output\nwant: %q\n got: %q", want, got)
	}
}
