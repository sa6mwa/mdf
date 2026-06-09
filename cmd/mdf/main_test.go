package main

import (
	"bytes"
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
	err := renderHTML(strings.NewReader("# Title\n\nBody\n"), &out, nil, false, pdfConfig{
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
	err := renderHTML(strings.NewReader("Body\n"), &out, nil, false, pdfConfig{
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
	err := renderHTML(strings.NewReader("Body\n"), &out, nil, false, pdfConfig{
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
