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

func TestWriteTraceEmitterRecordsEmissions(t *testing.T) {
	trace := &captureTraceEncoder{}
	emitter := NewWriteTraceEmitter(WriteTraceFormatANSI, trace)

	if err := emitter.EmitString("\x1b[1mhello"); err != nil {
		t.Fatalf("emit first: %v", err)
	}
	if err := emitter.EmitString("\x1b[0m\n"); err != nil {
		t.Fatalf("emit second: %v", err)
	}

	if got, want := len(trace.events), 2; got != want {
		t.Fatalf("trace event count got %d want %d", got, want)
	}
	assertTraceEvent(t, trace.events[0], 1, WriteTraceFormatANSI, "\x1b[1mhello")
	assertTraceEvent(t, trace.events[1], 2, WriteTraceFormatANSI, "\x1b[0m\n")
}

func TestWriteTraceEmitterReturnsTraceError(t *testing.T) {
	traceErr := errors.New("trace failed")
	emitter := NewWriteTraceEmitter(WriteTraceFormatANSI, &captureTraceEncoder{err: traceErr})

	err := emitter.EmitString("hello")
	if !errors.Is(err, traceErr) {
		t.Fatalf("emit error got %v want trace error", err)
	}
}

func TestNDJSONWriteTraceEncoder(t *testing.T) {
	var out bytes.Buffer
	encoder := NewNDJSONWriteTraceEncoder(&out)
	event := WriteTraceEvent{
		Seq:     7,
		Format:  WriteTraceFormatANSI,
		Op:      "emit",
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
	err := encoder.EncodeWriteTraceEvent(WriteTraceEvent{Seq: 1, Op: "emit"})
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
	if err := Render(RenderRequest{
		Reader:  strings.NewReader(src),
		Writer:  &tracedOut,
		Width:   40,
		Theme:   DefaultTheme(),
		Options: []RenderOption{WithWriteTrace(NewNDJSONWriteTraceEncoder(&traceData))},
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
		if event.Format != WriteTraceFormatANSI {
			t.Fatalf("event %d format got %q want ansi", i, event.Format)
		}
		if event.Op != "emit" {
			t.Fatalf("event %d op got %q want emit", i, event.Op)
		}
	}
}

func TestRenderWriteTraceRecordsWordEmissionsNotSinkWrites(t *testing.T) {
	src := `> **"Governance exists to support autonomy"** does **not** imply` + "\n"
	trace := &captureTraceEncoder{}
	var out bytes.Buffer
	if err := Render(RenderRequest{
		Reader:  strings.NewReader(src),
		Writer:  &out,
		Width:   80,
		Theme:   DefaultTheme(),
		Options: []RenderOption{WithWriteTrace(trace)},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}

	var visible []string
	for _, event := range trace.events {
		text := stripANSITracePayload(tracePayload(t, event))
		text = strings.TrimSpace(text)
		if text != "" {
			visible = append(visible, text)
		}
	}
	joined := "\n" + strings.Join(visible, "\n") + "\n"
	for _, want := range []string{`"Governance`, "exists", "to", "support", `autonomy"`} {
		if !strings.Contains(joined, "\n"+want+"\n") {
			t.Fatalf("missing word emission %q in visible trace chunks:\n%s", want, strings.Join(visible, "\n"))
		}
	}
	for _, got := range visible {
		if len([]rune(got)) == 1 && strings.Contains(`Governanceexiststosupportautonomy`, got) {
			t.Fatalf("trace contains character-sized content emission %q in chunks:\n%s", got, strings.Join(visible, "\n"))
		}
	}
}

func TestRenderWriteTraceEmitsBeforeInputEOF(t *testing.T) {
	reader, writer := io.Pipe()
	events := make(chan WriteTraceEvent, 16)
	errs := make(chan error, 1)

	go func() {
		errs <- Render(RenderRequest{
			Reader:  reader,
			Writer:  io.Discard,
			Width:   80,
			Theme:   DefaultTheme(),
			Options: []RenderOption{WithWriteTrace(channelTraceEncoder{events: events})},
		})
	}()

	if _, err := writer.Write([]byte("hello w")); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
	deadline := time.After(time.Second)
	var traced bytes.Buffer
	for !strings.Contains(stripANSITracePayload(traced.String()), "hello") {
		select {
		case event := <-events:
			traced.WriteString(tracePayload(t, event))
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

func assertTraceEvent(t *testing.T, event WriteTraceEvent, seq uint64, format string, payload string) {
	t.Helper()
	if event.Seq != seq {
		t.Fatalf("sequence got %d want %d", event.Seq, seq)
	}
	if event.Format != format {
		t.Fatalf("format got %q want %q", event.Format, format)
	}
	if event.Op != "emit" {
		t.Fatalf("op got %q want emit", event.Op)
	}
	if event.Bytes != len(payload) {
		t.Fatalf("bytes got %d want %d", event.Bytes, len(payload))
	}
	if got := tracePayload(t, event); got != payload {
		t.Fatalf("payload got %q want %q", got, payload)
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
		data := tracePayload(t, event)
		if len(data) != event.Bytes {
			t.Fatalf("event %d decoded bytes got %d want %d", event.Seq, len(data), event.Bytes)
		}
		if _, err := out.WriteString(data); err != nil {
			t.Fatalf("write reconstructed event %d: %v", event.Seq, err)
		}
	}
	return out.String()
}

func tracePayload(t *testing.T, event WriteTraceEvent) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(event.DataB64)
	if err != nil {
		t.Fatalf("decode event %d: %v", event.Seq, err)
	}
	return string(data)
}

func stripANSITracePayload(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != 0x1b {
			b.WriteByte(text[i])
			continue
		}
		i++
		if i >= len(text) {
			break
		}
		if text[i] == '[' {
			for i++; i < len(text) && (text[i] < '@' || text[i] > '~'); i++ {
			}
			continue
		}
		for i < len(text) && text[i] != '\\' && text[i] != 0x07 {
			i++
		}
	}
	return b.String()
}

func ExampleWriteTraceEmitter() {
	var trace bytes.Buffer
	emitter := NewWriteTraceEmitter(WriteTraceFormatANSI, NewNDJSONWriteTraceEncoder(&trace))
	_ = emitter.EmitString("hi")

	fmt.Print(strings.Contains(trace.String(), `"op":"emit"`))
	// Output:
	// true
}
