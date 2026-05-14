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
		margin:     24,
		fontSize:   10,
		lineHeight: 1.2,
		h1Scale:    2,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	wants := []string{
		"<!doctype html>",
		"padding:24pt;",
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
