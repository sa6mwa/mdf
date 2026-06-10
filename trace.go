package mdf

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

const (
	writeTraceOpEmit = "emit"

	// WriteTraceFormatANSI identifies ANSI renderer emission events.
	WriteTraceFormatANSI = "ansi"
	// WriteTraceFormatHTML identifies HTML renderer emission events.
	WriteTraceFormatHTML = "html"
)

// WriteTraceEvent records one renderer emission.
type WriteTraceEvent struct {
	// Seq is the 1-based event sequence for a trace emitter.
	Seq uint64 `json:"seq"`
	// Format identifies the rendered output format.
	Format string `json:"format"`
	// Op identifies the event operation. Emission events use "emit".
	Op string `json:"op"`
	// Bytes is the number of bytes emitted to the output sink.
	Bytes int `json:"bytes"`
	// DataB64 is the exact emitted byte payload encoded with standard base64.
	DataB64 string `json:"data_b64"`
}

// WriteTraceEncoder receives write trace events.
type WriteTraceEncoder interface {
	EncodeWriteTraceEvent(WriteTraceEvent) error
}

// NDJSONWriteTraceEncoder writes one JSON object per line.
type NDJSONWriteTraceEncoder struct {
	encoder *json.Encoder
}

// NewNDJSONWriteTraceEncoder creates a stable machine-readable trace encoder.
func NewNDJSONWriteTraceEncoder(w io.Writer) WriteTraceEncoder {
	if w == nil {
		return &NDJSONWriteTraceEncoder{}
	}
	return &NDJSONWriteTraceEncoder{encoder: json.NewEncoder(w)}
}

// EncodeWriteTraceEvent writes one trace event as newline-delimited JSON.
func (e *NDJSONWriteTraceEncoder) EncodeWriteTraceEvent(event WriteTraceEvent) error {
	if e == nil || e.encoder == nil {
		return fmt.Errorf("write trace: encoder is nil")
	}
	return e.encoder.Encode(event)
}

// WriteTraceEmitter emits sequenced write trace events for one renderer.
type WriteTraceEmitter struct {
	format string
	trace  WriteTraceEncoder
	seq    uint64
}

// NewWriteTraceEmitter creates an emission trace helper.
func NewWriteTraceEmitter(format string, trace WriteTraceEncoder) *WriteTraceEmitter {
	if trace == nil {
		return nil
	}
	return &WriteTraceEmitter{format: format, trace: trace}
}

// Emit records one renderer emission payload.
func (e *WriteTraceEmitter) Emit(p []byte) error {
	if e == nil || e.trace == nil || len(p) == 0 {
		return nil
	}
	e.seq++
	event := WriteTraceEvent{
		Seq:     e.seq,
		Format:  e.format,
		Op:      writeTraceOpEmit,
		Bytes:   len(p),
		DataB64: base64.StdEncoding.EncodeToString(p),
	}
	if err := e.trace.EncodeWriteTraceEvent(event); err != nil {
		return fmt.Errorf("write trace: %w", err)
	}
	return nil
}

// EmitString records one renderer emission payload.
func (e *WriteTraceEmitter) EmitString(text string) error {
	return e.Emit([]byte(text))
}
