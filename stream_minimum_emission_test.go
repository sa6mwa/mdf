package mdf

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLiveParserMinimumParagraphEmissionMatrix(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		prefix string
		want   string
	}{
		{name: "lowercase", src: "hello world", prefix: "h", want: "h"},
		{name: "uppercase", src: "Hello world", prefix: "H", want: "H"},
		{name: "header word", src: "Name | Value", prefix: "N", want: "N"},
		{name: "multi word header phrase", src: "current status | value", prefix: "c", want: "c"},
		{name: "numeric paragraph", src: "1abc", prefix: "1a", want: "1a"},
		{name: "ordered-list-looking paragraph", src: "1.x", prefix: "1.x", want: "1.x"},
		{name: "hash paragraph", src: "#not heading", prefix: "#n", want: "#n"},
		{name: "backtick paragraph", src: "`not fence` text", prefix: "`not fence` ", want: "not fence"},
		{name: "tilde paragraph", src: "~not fence", prefix: "~n", want: "~n"},
		{name: "inline emphasis text", src: "normal *emphasis* text", prefix: "n", want: "n"},
		{name: "inline code text", src: "normal `code` text", prefix: "n", want: "n"},
		{name: "inline link text", src: "normal [link](https://example.com) text", prefix: "n", want: "n"},
		{name: "inline pipe code", src: "`x|y` ok", prefix: "`x|y` ", want: "x|y"},
		{name: "inline pipe link", src: "[x|y](https://example.com) ok", prefix: "[x|y](https://example.com) ", want: "x|y (https://example.com)"},
		{name: "no edge pipe row", src: "A | B", prefix: "A", want: "A"},
		{name: "no edge delimiter row", src: "--- | ---", prefix: "--- |", want: "--- |"},
		{name: "trailing edge pipe row", src: "A | B |", prefix: "A", want: "A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := feedParserPrefix(t, tc.src, tc.prefix)
			if got := tokenTexts(stream.tokens); !strings.Contains(got, tc.want) {
				t.Fatalf("after prefix %q tokens = %q, want to contain %q", tc.prefix, got, tc.want)
			}
			if len(stream.tableStarts) != 0 || len(stream.tableRows) != 0 || stream.tableEnds != 0 {
				t.Fatalf("paragraph emitted table events: starts=%d rows=%d ends=%d", len(stream.tableStarts), len(stream.tableRows), stream.tableEnds)
			}
		})
	}
}

func TestLiveParserMinimumBlockEmissionMatrix(t *testing.T) {
	t.Run("heading emits once content starts", func(t *testing.T) {
		stream := feedParserPrefix(t, "# Heading", "# H")
		if got := tokenTexts(stream.tokens); !strings.Contains(got, "# H") {
			t.Fatalf("heading tokens = %q, want marker and first content", got)
		}
	})

	t.Run("list waits for content", func(t *testing.T) {
		parser := newLiveParser(DefaultTheme(), false)
		stream := &captureStream{}
		for _, r := range "- " {
			if err := parser.feedRune(stream, r); err != nil {
				t.Fatalf("feed prefix: %v", err)
			}
		}
		if got := tokenTexts(stream.tokens); got != "" {
			t.Fatalf("list marker without content emitted %q", got)
		}
		if err := parser.feedRune(stream, 'i'); err != nil {
			t.Fatalf("feed content: %v", err)
		}
		if got := tokenTexts(stream.tokens); !strings.Contains(got, "- i") {
			t.Fatalf("list tokens = %q, want marker and first content", got)
		}
	})

	t.Run("blockquote emits once content starts", func(t *testing.T) {
		stream := feedParserPrefix(t, "> quote", "> q")
		if got := tokenTexts(stream.tokens); !strings.Contains(got, "> q") {
			t.Fatalf("blockquote tokens = %q, want prefix and first content", got)
		}
	})

	t.Run("thematic break waits until newline", func(t *testing.T) {
		parser := newLiveParser(DefaultTheme(), false)
		stream := &captureStream{}
		for _, r := range "---" {
			if err := parser.feedRune(stream, r); err != nil {
				t.Fatalf("feed thematic prefix: %v", err)
			}
		}
		if got := tokenTexts(stream.tokens); got != "" {
			t.Fatalf("thematic candidate emitted text before newline: %q", got)
		}
		if err := parser.feedRune(stream, '\n'); err != nil {
			t.Fatalf("feed newline: %v", err)
		}
		found := false
		for _, tok := range stream.tokens {
			if tok.Kind == tokenThematicBreak {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected thematic break token after newline, got %q", tokenTexts(stream.tokens))
		}
	})

	t.Run("edge table header waits for delimiter", func(t *testing.T) {
		parser := newLiveParser(DefaultTheme(), false)
		stream := &captureStream{}
		if err := parser.feedBytes(stream, []byte("| A | B |\n")); err != nil {
			t.Fatalf("feed header: %v", err)
		}
		if got := tokenTexts(stream.tokens); got != "" {
			t.Fatalf("edge table header emitted text before delimiter: %q", got)
		}
		if parser.tablePendingHeader == "" {
			t.Fatalf("expected pending edge table header")
		}
		if err := parser.feedBytes(stream, []byte("| --- | --- |\n")); err != nil {
			t.Fatalf("feed delimiter: %v", err)
		}
		if len(stream.tableStarts) != 1 {
			t.Fatalf("table starts = %d, want 1", len(stream.tableStarts))
		}
	})
}

func TestRenderMinimumNonTableOutputBeforeLineEnd(t *testing.T) {
	cases := []struct {
		name       string
		src        string
		checkpoint string
		want       string
		fullLine   string
	}{
		{name: "plain paragraph", src: "hello world\n", checkpoint: "hello w", want: "hello", fullLine: "hello world"},
		{name: "capitalized paragraph", src: "Hello world\n", checkpoint: "Hello w", want: "Hello", fullLine: "Hello world"},
		{name: "no edge table-like line", src: "Name | Value\n", checkpoint: "Name |", want: "Name", fullLine: "Name | Value"},
		{name: "no edge delimiter-like line", src: "--- | ---\n", checkpoint: "--- |", want: "---", fullLine: "--- | ---"},
		{name: "heading", src: "# Heading Text\n", checkpoint: "# Heading T", want: "# Heading", fullLine: "# Heading Text"},
		{name: "list", src: "- item text\n", checkpoint: "- item t", want: "- item", fullLine: "- item text"},
		{name: "blockquote", src: "> quote text\n", checkpoint: "> quote t", want: "> quote", fullLine: "> quote text"},
		{name: "inline emphasis", src: "hello *world* today\n", checkpoint: "hello *", want: "hello", fullLine: "hello world today"},
		{name: "inline code", src: "hello `code` today\n", checkpoint: "hello `", want: "hello", fullLine: "hello code today"},
		{name: "inline link", src: "see [site](https://example.com) now\n", checkpoint: "see [", want: "see", fullLine: "see site (https://example.com) now"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			var out checkpointWriter
			done := make(chan error, 1)
			go func() {
				done <- Render(RenderRequest{
					Reader: reader,
					Writer: &out,
					Width:  80,
					Theme:  DefaultTheme(),
					Options: []RenderOption{
						WithOSC8(false),
						WithTableBufferMode(TableBufferFull),
						WithTableWireMode(TableWireLine),
					},
				})
			}()
			if _, err := writer.Write([]byte(tc.checkpoint)); err != nil {
				t.Fatalf("write checkpoint: %v", err)
			}
			waitForOutput(t, &out, tc.checkpoint, tc.want, tc.fullLine)
			rest := strings.TrimPrefix(tc.src, tc.checkpoint)
			if _, err := writer.Write([]byte(rest)); err != nil {
				t.Fatalf("write rest: %v", err)
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("close input: %v", err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("render: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatalf("render did not finish")
			}
		})
	}
}

func waitForOutput(t *testing.T, out *checkpointWriter, checkpoint string, want string, fullLine string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got := stripANSI(out.String())
		if strings.Contains(got, fullLine) {
			t.Fatalf("output after writing %q already contains full line %q: %q", checkpoint, fullLine, got)
		}
		if strings.Contains(got, want) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("output after writing %q = %q, want partial output containing %q", checkpoint, stripANSI(out.String()), want)
}

func feedParserPrefix(t *testing.T, src string, prefix string) *captureStream {
	t.Helper()
	if !strings.HasPrefix(src, prefix) {
		t.Fatalf("test setup: %q does not have prefix %q", src, prefix)
	}
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	for _, r := range prefix {
		if err := parser.feedRune(stream, r); err != nil {
			t.Fatalf("feed prefix %q: %v", prefix, err)
		}
	}
	return stream
}

type checkpointWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *checkpointWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *checkpointWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}
