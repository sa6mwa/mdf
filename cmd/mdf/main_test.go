package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkt.systems/mdf"
)

func TestOpenInputFileAndURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.md")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	reader, closer, err := openInputs([]string{path})
	if err != nil {
		t.Fatalf("openInputs file: %v", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	buf, _ := io.ReadAll(reader)
	if string(buf) != "hello" {
		t.Fatalf("unexpected file content: %q", string(buf))
	}

	fileURL := "file://" + path
	reader, closer, err = openInputs([]string{fileURL})
	if err != nil {
		t.Fatalf("openInputs file URL: %v", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	buf, _ = io.ReadAll(reader)
	if string(buf) != "hello" {
		t.Fatalf("unexpected file URL content: %q", string(buf))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("stream"))
	}))
	defer srv.Close()
	reader, closer, err = openInputs([]string{srv.URL})
	if err != nil {
		t.Fatalf("openInputs http: %v", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	buf, _ = io.ReadAll(reader)
	if string(buf) != "stream" {
		t.Fatalf("unexpected http content: %q", string(buf))
	}
}

func TestRenderHTML(t *testing.T) {
	var out bytes.Buffer
	err := renderHTML(strings.NewReader("# Title\n\nBody\n"), &out, nil, false, nil, pdfConfig{
		margin:           24,
		htmlContentWidth: 72,
		fontSize:         10,
		lineHeight:       1.2,
		h1Scale:          2,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	wants := []string{
		"<!doctype html>",
		"--mdf-page-padding-block:24pt;",
		"--mdf-page-padding-inline:24pt;",
		"--mdf-content-max-width:72ch;",
		"@font-face{font-family:\"JetBrains Mono\"",
		"src:url(data:font/woff2;base64,",
		"font-size:10pt;",
		"line-height:1.2;",
		"Body",
	}
	for _, want := range wants {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in rendered HTML", want)
		}
	}
}

func TestRenderHTMLCanOptIntoEmbeddedHackFont(t *testing.T) {
	var out bytes.Buffer
	err := renderHTML(strings.NewReader("Body\n"), &out, nil, false, nil, pdfConfig{
		htmlEmbeddedFont: "hack",
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, `@font-face{font-family:"HackNerdFontMono"`) {
		t.Fatalf("missing embedded hack font: %q", rendered)
	}
	if !strings.Contains(rendered, "src:url(data:font/ttf;base64,") {
		t.Fatalf("expected embedded hack TTF font: %q", rendered)
	}
}

func TestRenderHTMLRejectsUnknownEmbeddedFont(t *testing.T) {
	var out bytes.Buffer
	err := renderHTML(strings.NewReader("Body\n"), &out, nil, false, nil, pdfConfig{
		htmlEmbeddedFont: "unknown",
	})
	if err == nil {
		t.Fatalf("expected unknown embedded html font to be rejected")
	}
	if !strings.Contains(err.Error(), `html embedded font "unknown" is unsupported`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenInputsConcatenates(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.md")
	second := filepath.Join(dir, "b.md")
	if err := os.WriteFile(first, []byte("one "), 0o644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte("two"), 0o644); err != nil {
		t.Fatalf("write second: %v", err)
	}
	reader, closer, err := openInputs([]string{first, second})
	if err != nil {
		t.Fatalf("openInputs concat: %v", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	buf, _ := io.ReadAll(reader)
	if string(buf) != "one two" {
		t.Fatalf("unexpected concatenated content: %q", string(buf))
	}
}

func TestResolveOSC8(t *testing.T) {
	cases := map[string]bool{
		"on":  true,
		"off": false,
		"1":   true,
		"0":   false,
	}
	for input, want := range cases {
		got, err := resolveOSC8(input)
		if err != nil {
			t.Fatalf("resolveOSC8(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("resolveOSC8(%q)=%v want %v", input, got, want)
		}
	}
	if _, err := resolveOSC8("nope"); err == nil {
		t.Fatalf("expected error for invalid osc8 value")
	}
}

func TestResolveTableBufferMode(t *testing.T) {
	cases := map[string]mdf.TableBufferMode{
		"":     mdf.TableBufferFull,
		"full": mdf.TableBufferFull,
		"row":  mdf.TableBufferRow,
	}
	for input, want := range cases {
		got, err := resolveTableBufferMode(input)
		if err != nil {
			t.Fatalf("resolveTableBufferMode(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("resolveTableBufferMode(%q)=%v want %v", input, got, want)
		}
	}
	if _, err := resolveTableBufferMode("stream"); err == nil {
		t.Fatalf("expected error for invalid table buffer mode")
	}
}

func TestResolveTableWireMode(t *testing.T) {
	cases := map[string]mdf.TableWireMode{
		"":      mdf.TableWireLine,
		"line":  mdf.TableWireLine,
		"ascii": mdf.TableWireASCII,
		"space": mdf.TableWireSpace,
	}
	for input, want := range cases {
		got, err := resolveTableWireMode(input)
		if err != nil {
			t.Fatalf("resolveTableWireMode(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("resolveTableWireMode(%q)=%v want %v", input, got, want)
		}
	}
	if _, err := resolveTableWireMode("unicode"); err == nil {
		t.Fatalf("expected error for invalid table wire mode")
	}
}

func TestValidateTableWireForModeRejectsHTMLASCII(t *testing.T) {
	err := validateTableWireForMode(true, mdf.TableWireASCII)
	if err == nil {
		t.Fatalf("expected html ascii table wire mode to be rejected")
	}
	if !strings.Contains(err.Error(), "--table-wire ascii is not supported with --html") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTableWireForModeAllowsHTMLLineAndSpace(t *testing.T) {
	for _, mode := range []mdf.TableWireMode{mdf.TableWireLine, mdf.TableWireSpace} {
		if err := validateTableWireForMode(true, mode); err != nil {
			t.Fatalf("expected html table wire mode %v to be allowed: %v", mode, err)
		}
	}
	if err := validateTableWireForMode(false, mdf.TableWireASCII); err != nil {
		t.Fatalf("expected ansi ascii table wire mode to be allowed: %v", err)
	}
}

func TestValidateTraceWritesForModeRejectsPDF(t *testing.T) {
	err := validateTraceWritesForMode("trace.ndjson", true, false)
	if err == nil {
		t.Fatalf("expected trace mode rejection")
	}
	if !strings.Contains(err.Error(), "--trace-writes is only supported for ANSI and HTML output") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateTraceWritesForMode("trace.ndjson", false, false); err != nil {
		t.Fatalf("expected ANSI trace mode to be allowed: %v", err)
	}
	if err := validateTraceWritesForMode("trace.ndjson", false, true); err != nil {
		t.Fatalf("expected HTML trace mode to be allowed: %v", err)
	}
	if err := validateTraceWritesForMode("", true, true); err != nil {
		t.Fatalf("expected disabled trace mode to be allowed: %v", err)
	}
}

func TestOpenWriteTraceDashWritesEventsToStderr(t *testing.T) {
	var stderr bytes.Buffer
	encoder, closer, err := openWriteTrace("-", &stderr)
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	if closer != nil {
		t.Fatalf("stderr trace should not return closer")
	}
	if err := mdf.NewWriteTraceEmitter(mdf.WriteTraceFormatANSI, encoder).EmitString("hello"); err != nil {
		t.Fatalf("emit trace: %v", err)
	}
	event := decodeCLITraceEvent(t, stderr.Bytes())
	assertCLITraceEvent(t, event, 1, mdf.WriteTraceFormatANSI, "hello")
}

func TestOpenWriteTracePathWritesEventsToFile(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "trace", "writes.ndjson")
	encoder, closer, err := openWriteTrace(tracePath, io.Discard)
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	if closer == nil {
		t.Fatalf("file trace should return closer")
	}
	if err := mdf.NewWriteTraceEmitter(mdf.WriteTraceFormatHTML, encoder).EmitString("hello"); err != nil {
		t.Fatalf("emit trace: %v", err)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("close trace: %v", err)
	}
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	event := decodeCLITraceEvent(t, data)
	assertCLITraceEvent(t, event, 1, mdf.WriteTraceFormatHTML, "hello")
}

func TestConfigureWriteTraceRejectsOutputPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	_, _, err := configureWriteTrace(io.Discard, path, path, io.Discard)
	if err == nil {
		t.Fatalf("expected same trace and output path rejection")
	}
	if !strings.Contains(err.Error(), "trace path must be different from output path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigureOutputRejectsTraceOutputAbsoluteAliasBeforeTruncatingOutput(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.ansi")
	original := []byte("existing output must survive absolute alias")
	if err := os.WriteFile(outPath, original, 0o644); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		t.Fatalf("abs output path: %v", err)
	}

	_, _, _, _, err = configureOutput(outPath, absOut, io.Discard)
	if err == nil {
		t.Fatalf("expected absolute alias rejection")
	}
	assertOutputUnchanged(t, outPath, original)
}

func TestConfigureOutputRejectsTraceOutputSymlinkAliasBeforeTruncatingOutput(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.ansi")
	original := []byte("existing output must survive symlink alias")
	if err := os.WriteFile(outPath, original, 0o644); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	tracePath := filepath.Join(dir, "trace-link")
	if err := os.Symlink(outPath, tracePath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, _, _, _, err := configureOutput(outPath, tracePath, io.Discard)
	if err == nil {
		t.Fatalf("expected symlink alias rejection")
	}
	assertOutputUnchanged(t, outPath, original)
}

func TestConfigureOutputRejectsTraceOutputHardLinkAliasBeforeTruncatingOutput(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.ansi")
	original := []byte("existing output must survive hard link alias")
	if err := os.WriteFile(outPath, original, 0o644); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	tracePath := filepath.Join(dir, "trace-hardlink")
	if err := os.Link(outPath, tracePath); err != nil {
		t.Skipf("hard link unavailable: %v", err)
	}

	_, _, _, _, err := configureOutput(outPath, tracePath, io.Discard)
	if err == nil {
		t.Fatalf("expected hard link alias rejection")
	}
	assertOutputUnchanged(t, outPath, original)
}

func TestConfigureOutputRejectsTraceOutputPathBeforeTruncatingOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.ansi")
	original := []byte("existing output must survive")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	_, _, _, _, err := configureOutput(path, path, io.Discard)
	if err == nil {
		t.Fatalf("expected same trace and output path rejection")
	}
	if !strings.Contains(err.Error(), "trace path must be different from output path") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertOutputUnchanged(t, path, original)
}

func TestConfigureOutputRejectsInvalidTracePathBeforeTruncatingOutput(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.ansi")
	original := []byte("existing output must survive trace setup failure")
	if err := os.WriteFile(outPath, original, 0o644); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	traceParent := filepath.Join(dir, "trace-parent-is-file")
	if err := os.WriteFile(traceParent, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write trace parent file: %v", err)
	}
	tracePath := filepath.Join(traceParent, "writes.ndjson")

	_, _, _, _, err := configureOutput(outPath, tracePath, io.Discard)
	if err == nil {
		t.Fatalf("expected invalid trace path rejection")
	}
	assertOutputUnchanged(t, outPath, original)
}

func TestConfigureOutputExitCodeClassifiesUsageAndOperationalErrors(t *testing.T) {
	if got := configureOutputExitCode(usageError("bad flag")); got != 2 {
		t.Fatalf("usage exit code got %d want 2", got)
	}
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.ansi")
	traceParent := filepath.Join(dir, "trace-parent-is-file")
	if err := os.WriteFile(traceParent, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write trace parent file: %v", err)
	}
	_, _, _, _, err := configureOutput(outPath, filepath.Join(traceParent, "writes.ndjson"), io.Discard)
	if err == nil {
		t.Fatalf("expected operational trace setup error")
	}
	if got := configureOutputExitCode(err); got != 1 {
		t.Fatalf("operational exit code got %d want 1 for %v", got, err)
	}
}

func TestBoringThemeHasNoPrefixes(t *testing.T) {
	theme := boringTheme()
	styles := theme.Styles()
	if styles.Text.Prefix != "" {
		t.Fatalf("expected empty text prefix")
	}
	for i, h := range styles.Heading {
		if h.Prefix != "" {
			t.Fatalf("expected empty heading %d prefix", i+1)
		}
	}
	others := []string{
		styles.Emphasis.Prefix,
		styles.Strong.Prefix,
		styles.EmphasisStrong.Prefix,
		styles.CodeInline.Prefix,
		styles.CodeBlock.Prefix,
		styles.Quote.Prefix,
		styles.ListMarker.Prefix,
		styles.LinkText.Prefix,
		styles.LinkURL.Prefix,
		styles.ThematicBreak.Prefix,
	}
	for _, prefix := range others {
		if strings.TrimSpace(prefix) != "" {
			t.Fatalf("expected empty prefix, got %q", prefix)
		}
	}
}

func decodeCLITraceEvent(t *testing.T, data []byte) mdf.WriteTraceEvent {
	t.Helper()
	var event mdf.WriteTraceEvent
	if err := json.Unmarshal(bytes.TrimSpace(data), &event); err != nil {
		t.Fatalf("decode trace event: %v", err)
	}
	return event
}

func assertCLITraceEvent(t *testing.T, event mdf.WriteTraceEvent, seq uint64, format string, payload string) {
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
	data, err := base64.StdEncoding.DecodeString(event.DataB64)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if string(data) != payload {
		t.Fatalf("payload got %q want %q", string(data), payload)
	}
}

func assertOutputUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read existing output: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("output was truncated or changed: got %q want %q", got, want)
	}
}
