package mdf

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

type captureTraceEncoder struct {
	events []WriteTraceEvent
	err    error
}

func (e *captureTraceEncoder) EncodeWriteTraceEvent(event WriteTraceEvent) error {
	if e.err != nil {
		return e.err
	}
	e.events = append(e.events, event)
	return nil
}

type shortWriteSink struct {
	buf bytes.Buffer
	n   int
	err error
}

func (s *shortWriteSink) Write(p []byte) (int, error) {
	n := s.n
	if n > len(p) {
		n = len(p)
	}
	_, _ = s.buf.Write(p[:n])
	return n, s.err
}

func TestWriteTraceWriterRecordsEachAcceptedSinkWrite(t *testing.T) {
	var sink bytes.Buffer
	trace := &captureTraceEncoder{}
	writer := NewWriteTraceWriter(&sink, trace)

	if _, err := writer.Write([]byte("hello")); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	if _, err := writer.Write([]byte("\n")); err != nil {
		t.Fatalf("write newline: %v", err)
	}

	if got, want := sink.String(), "hello\n"; got != want {
		t.Fatalf("sink output got %q want %q", got, want)
	}
	if got, want := len(trace.events), 2; got != want {
		t.Fatalf("trace event count got %d want %d", got, want)
	}
	assertTraceEvent(t, trace.events[0], 1, "hello")
	assertTraceEvent(t, trace.events[1], 2, "\n")
}

func TestWriteTraceWriterRecordsOnlyAcceptedPrefixOnPartialWrite(t *testing.T) {
	sinkErr := io.ErrShortWrite
	sink := &shortWriteSink{n: 2, err: sinkErr}
	trace := &captureTraceEncoder{}
	writer := NewWriteTraceWriter(sink, trace)

	n, err := writer.Write([]byte("hello"))
	if !errors.Is(err, sinkErr) {
		t.Fatalf("write error got %v want %v", err, sinkErr)
	}
	if n != 2 {
		t.Fatalf("write count got %d want 2", n)
	}
	if got, want := sink.buf.String(), "he"; got != want {
		t.Fatalf("sink output got %q want %q", got, want)
	}
	if got, want := len(trace.events), 1; got != want {
		t.Fatalf("trace event count got %d want %d", got, want)
	}
	assertTraceEvent(t, trace.events[0], 1, "he")
}

func TestWriteTraceWriterDoesNotTraceZeroByteFailedWrite(t *testing.T) {
	sinkErr := io.ErrClosedPipe
	sink := &shortWriteSink{n: 0, err: sinkErr}
	trace := &captureTraceEncoder{}
	writer := NewWriteTraceWriter(sink, trace)

	n, err := writer.Write([]byte("hello"))
	if !errors.Is(err, sinkErr) {
		t.Fatalf("write error got %v want %v", err, sinkErr)
	}
	if n != 0 {
		t.Fatalf("write count got %d want 0", n)
	}
	if len(trace.events) != 0 {
		t.Fatalf("zero-byte failed write must not emit trace events")
	}
}

func TestWriteTraceWriterReturnsTraceErrorAfterSinkAcceptsBytes(t *testing.T) {
	var sink bytes.Buffer
	traceErr := errors.New("trace failed")
	writer := NewWriteTraceWriter(&sink, &captureTraceEncoder{err: traceErr})

	n, err := writer.Write([]byte("hello"))
	if n != 5 {
		t.Fatalf("write count got %d want 5", n)
	}
	if !errors.Is(err, traceErr) {
		t.Fatalf("write error got %v want trace error", err)
	}
	if got, want := sink.String(), "hello"; got != want {
		t.Fatalf("sink output got %q want %q", got, want)
	}
}

func TestWriteTraceWriterPreservesSinkAndTraceErrors(t *testing.T) {
	sinkErr := io.ErrShortWrite
	traceErr := errors.New("trace failed")
	sink := &shortWriteSink{n: 2, err: sinkErr}
	writer := NewWriteTraceWriter(sink, &captureTraceEncoder{err: traceErr})

	n, err := writer.Write([]byte("hello"))
	if n != 2 {
		t.Fatalf("write count got %d want 2", n)
	}
	if !errors.Is(err, sinkErr) {
		t.Fatalf("write error got %v, want sink error", err)
	}
	if !errors.Is(err, traceErr) {
		t.Fatalf("write error got %v, want trace error", err)
	}
	if got, want := sink.buf.String(), "he"; got != want {
		t.Fatalf("sink output got %q want %q", got, want)
	}
}

func TestWriteTraceWriterHandlesNilTraceAsPassThrough(t *testing.T) {
	var sink bytes.Buffer
	writer := NewWriteTraceWriter(&sink, nil)

	n, err := writer.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 5 {
		t.Fatalf("write count got %d want 5", n)
	}
	if got, want := sink.String(), "hello"; got != want {
		t.Fatalf("sink output got %q want %q", got, want)
	}
}

func TestWriteTraceWriterRejectsNilSink(t *testing.T) {
	trace := &captureTraceEncoder{}
	n, err := NewWriteTraceWriter(nil, trace).Write([]byte("hello"))
	if err == nil {
		t.Fatalf("expected nil sink error")
	}
	if n != 0 {
		t.Fatalf("write count got %d want 0", n)
	}
	if len(trace.events) != 0 {
		t.Fatalf("nil sink must not emit trace events")
	}
}

func TestNDJSONWriteTraceEncoder(t *testing.T) {
	var out bytes.Buffer
	encoder := NewNDJSONWriteTraceEncoder(&out)
	event := WriteTraceEvent{
		Seq:     7,
		Op:      "write",
		Bytes:   3,
		DataB64: base64.StdEncoding.EncodeToString([]byte{0, '\n', 255}),
	}
	if err := encoder.EncodeWriteTraceEvent(event); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.HasSuffix(out.String(), "\n") {
		t.Fatalf("NDJSON event missing trailing newline: %q", out.String())
	}
	var got WriteTraceEvent
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &got); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if got != event {
		t.Fatalf("event got %+v want %+v", got, event)
	}
}

func TestNDJSONWriteTraceEncoderRejectsNilWriter(t *testing.T) {
	encoder := NewNDJSONWriteTraceEncoder(nil)
	err := encoder.EncodeWriteTraceEvent(WriteTraceEvent{Seq: 1, Op: "write"})
	if err == nil {
		t.Fatalf("expected nil writer error")
	}
	if !strings.Contains(err.Error(), "encoder is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderWriteTraceReconstructsExactANSIOutput(t *testing.T) {
	src := "# Title\n\nParagraph with *emphasis* and `code`.\n"
	var normal bytes.Buffer
	if err := Render(RenderRequest{
		Reader: strings.NewReader(src),
		Writer: &normal,
		Width:  40,
		Theme:  DefaultTheme(),
	}); err != nil {
		t.Fatalf("render normal: %v", err)
	}

	var tracedOut bytes.Buffer
	var traceData bytes.Buffer
	tracedWriter := NewWriteTraceWriter(&tracedOut, NewNDJSONWriteTraceEncoder(&traceData))
	if err := Render(RenderRequest{
		Reader: strings.NewReader(src),
		Writer: tracedWriter,
		Width:  40,
		Theme:  DefaultTheme(),
	}); err != nil {
		t.Fatalf("render traced: %v", err)
	}

	if tracedOut.String() != normal.String() {
		t.Fatalf("traced render changed output\nnormal: %q\ntraced: %q", normal.String(), tracedOut.String())
	}
	events := decodeTraceEvents(t, traceData.Bytes())
	if len(events) < 2 {
		t.Fatalf("expected multiple write trace events, got %d", len(events))
	}
	if got := reconstructTracePayload(t, events); got != normal.String() {
		t.Fatalf("reconstructed trace got %q want %q", got, normal.String())
	}
	for i, event := range events {
		if event.Seq != uint64(i+1) {
			t.Fatalf("event %d sequence got %d want %d", i, event.Seq, i+1)
		}
		if event.Op != "write" {
			t.Fatalf("event %d op got %q want write", i, event.Op)
		}
	}
}

func TestRenderWriteTraceEmitsBeforeInputEOF(t *testing.T) {
	reader, writer := io.Pipe()
	events := make(chan WriteTraceEvent, 16)
	errs := make(chan error, 1)

	go func() {
		errs <- Render(RenderRequest{
			Reader: reader,
			Writer: NewWriteTraceWriter(io.Discard, channelTraceEncoder{events: events}),
			Width:  80,
			Theme:  DefaultTheme(),
		})
	}()

	if _, err := writer.Write([]byte("hello w")); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
	deadline := time.After(time.Second)
	var traced bytes.Buffer
	for !strings.Contains(traced.String(), "hello") {
		select {
		case event := <-events:
			data, err := base64.StdEncoding.DecodeString(event.DataB64)
			if err != nil {
				t.Fatalf("decode event %d: %v", event.Seq, err)
			}
			traced.Write(data)
		case err := <-errs:
			t.Fatalf("render returned before EOF: %v", err)
		case <-deadline:
			t.Fatalf("timed out waiting for trace payload before EOF; got %q", traced.String())
		}
	}

	if _, err := writer.Write([]byte("orld\n")); err != nil {
		t.Fatalf("write rest: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("render after EOF: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for render completion")
	}
}

type channelTraceEncoder struct {
	events chan<- WriteTraceEvent
}

func (e channelTraceEncoder) EncodeWriteTraceEvent(event WriteTraceEvent) error {
	e.events <- event
	return nil
}

func assertTraceEvent(t *testing.T, event WriteTraceEvent, seq uint64, payload string) {
	t.Helper()
	if event.Seq != seq {
		t.Fatalf("sequence got %d want %d", event.Seq, seq)
	}
	if event.Op != "write" {
		t.Fatalf("op got %q want write", event.Op)
	}
	if event.Bytes != len(payload) {
		t.Fatalf("bytes got %d want %d", event.Bytes, len(payload))
	}
	data, err := base64.StdEncoding.DecodeString(event.DataB64)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if string(data) != payload {
		t.Fatalf("payload got %q want %q", string(data), payload)
	}
}

func decodeTraceEvents(t *testing.T, data []byte) []WriteTraceEvent {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	var events []WriteTraceEvent
	for {
		var event WriteTraceEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode trace event %d: %v", len(events)+1, err)
		}
		events = append(events, event)
	}
	return events
}

func reconstructTracePayload(t *testing.T, events []WriteTraceEvent) string {
	t.Helper()
	var out bytes.Buffer
	for _, event := range events {
		data, err := base64.StdEncoding.DecodeString(event.DataB64)
		if err != nil {
			t.Fatalf("decode event %d: %v", event.Seq, err)
		}
		if len(data) != event.Bytes {
			t.Fatalf("event %d decoded bytes got %d want %d", event.Seq, len(data), event.Bytes)
		}
		if _, err := out.Write(data); err != nil {
			t.Fatalf("write reconstructed event %d: %v", event.Seq, err)
		}
	}
	return out.String()
}

func ExampleNewWriteTraceWriter() {
	var rendered bytes.Buffer
	var trace bytes.Buffer
	writer := NewWriteTraceWriter(&rendered, NewNDJSONWriteTraceEncoder(&trace))

	_, _ = writer.Write([]byte("hi"))

	fmt.Print(rendered.String())
	// Output:
	// hi
}
