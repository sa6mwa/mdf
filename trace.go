package mdf

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const writeTraceOpWrite = "write"

// WriteTraceEvent records one observable write to a rendered output sink.
type WriteTraceEvent struct {
	// Seq is the 1-based event sequence for a trace writer.
	Seq uint64 `json:"seq"`
	// Op identifies the event operation. Write events use "write".
	Op string `json:"op"`
	// Bytes is the number of bytes accepted by the wrapped sink.
	Bytes int `json:"bytes"`
	// DataB64 is the exact accepted byte payload encoded with standard base64.
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

type writeTraceWriter struct {
	sink  io.Writer
	trace WriteTraceEncoder
	seq   uint64
}

// NewWriteTraceWriter wraps sink and records each successful sink write.
//
// The returned writer preserves streaming behavior: each Write call forwards to
// sink immediately and records only the byte prefix accepted by sink. It does
// not buffer, replay, or materialize the rendered stream.
func NewWriteTraceWriter(sink io.Writer, trace WriteTraceEncoder) io.Writer {
	return &writeTraceWriter{sink: sink, trace: trace}
}

func (w *writeTraceWriter) Write(p []byte) (int, error) {
	if w.sink == nil {
		return 0, fmt.Errorf("write trace: sink is nil")
	}
	n, writeErr := w.sink.Write(p)
	if n > 0 && w.trace != nil {
		w.seq++
		event := WriteTraceEvent{
			Seq:     w.seq,
			Op:      writeTraceOpWrite,
			Bytes:   n,
			DataB64: base64.StdEncoding.EncodeToString(p[:n]),
		}
		if traceErr := w.trace.EncodeWriteTraceEvent(event); traceErr != nil {
			if writeErr != nil {
				return n, errors.Join(writeErr, fmt.Errorf("write trace: %w", traceErr))
			}
			return n, fmt.Errorf("write trace: %w", traceErr)
		}
	}
	return n, writeErr
}
