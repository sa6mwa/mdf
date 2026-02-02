package mdf

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/muesli/reflow/ansi"
)

type captureStream struct {
	tokens []StreamToken
}

func (c *captureStream) WriteToken(tok StreamToken) error {
	c.tokens = append(c.tokens, tok)
	return nil
}

func (c *captureStream) Flush() error {
	return nil
}

func (c *captureStream) Width() int {
	return 80
}

func (c *captureStream) SetWidth(int) {}

func (c *captureStream) SetWrapIndent(string) {}

func TestLiveParserEmitsThematicBreakToken(t *testing.T) {
	src := "one\n---\ntwo\n"
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream parse live: %v", err)
	}
	found := false
	for _, tok := range stream.tokens {
		if tok.Kind == tokenThematicBreak {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected thematic break token")
	}
}

func TestRenderWrapIndentation(t *testing.T) {
	src := strings.Join([]string{
		"- Parent item with enough text to wrap cleanly",
		"  - If cycles occur, extract core functionality into a core package",
		"12. Ordered item with enough text to wrap across lines properly",
		"> quote line one with more words to wrap",
		"> quote line two with additional words",
	}, "\n")
	width := 40

	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader(src),
		Writer: &out,
		Width:  width,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	plain := stripANSI(out.String())
	if !strings.Contains(plain, "\n    functionality into a core package") {
		t.Fatalf("missing nested list wrap indentation: %q", plain)
	}
	if !strings.Contains(plain, "\n    wrap across lines properly") {
		t.Fatalf("missing ordered list wrap indentation: %q", plain)
	}
	if !strings.Contains(plain, "quote line one") || !strings.Contains(plain, "quote line two") {
		t.Fatalf("missing blockquote content: %q", plain)
	}
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "quote line") && !strings.HasPrefix(strings.TrimLeft(line, " "), "> ") {
			t.Fatalf("missing blockquote prefix on line: %q", line)
		}
	}
}

func TestRenderDoesNotSplitCommaAfterPeriod(t *testing.T) {
	src := "Cadence Design: Establish short cycles of delivery and feedback (e.g., 1-2 weeks)"
	width := 40

	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader(src),
		Writer: &out,
		Width:  width,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	plain := stripANSI(out.String())
	if strings.Contains(plain, "e.\n") || strings.Contains(plain, "\n g.") {
		t.Fatalf("unexpected split inside abbreviation: %q", plain)
	}
}

func TestLiveParserAutoLinkTokens(t *testing.T) {
	src := "<https://pkt.systems> and <sa6mwa@gmail.com>"
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader:  strings.NewReader(src),
		Stream:  stream,
		Theme:   DefaultTheme(),
		Options: []RenderOption{WithOSC8(true)},
	})
	if err != nil {
		t.Fatalf("stream parse live: %v", err)
	}
	var links []string
	var inside bool
	var sawLinkText bool
	linkTextStyle := DefaultTheme().Styles().LinkText.Prefix
	for _, tok := range stream.tokens {
		switch tok.Kind {
		case tokenLinkStart:
			links = append(links, tok.LinkURL)
			inside = true
			sawLinkText = false
		case tokenLinkEnd:
			if !inside {
				t.Fatalf("unexpected link end")
			}
			if !sawLinkText {
				t.Fatalf("expected link text tokens for %q", links[len(links)-1])
			}
			inside = false
		default:
			if inside && tok.Kind == tokenURL && tok.Style.Prefix == linkTextStyle {
				sawLinkText = true
			}
		}
	}
	if inside {
		t.Fatalf("unterminated link token")
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0] != "https://pkt.systems" {
		t.Fatalf("unexpected first link url: %q", links[0])
	}
	if links[1] != "mailto:sa6mwa@gmail.com" {
		t.Fatalf("unexpected second link url: %q", links[1])
	}
}

func TestLiveParserBracketedAutoLink(t *testing.T) {
	src := "[<http://example.com>]"
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader:  strings.NewReader(src),
		Stream:  stream,
		Theme:   DefaultTheme(),
		Options: []RenderOption{WithOSC8(true)},
	})
	if err != nil {
		t.Fatalf("stream parse live: %v", err)
	}
	openIdx, linkStartIdx, linkEndIdx, closeIdx := -1, -1, -1, -1
	linkURL := ""
	for i, tok := range stream.tokens {
		switch tok.Kind {
		case tokenText:
			if tok.Text == "[" && openIdx == -1 {
				openIdx = i
			}
			if tok.Text == "]" && closeIdx == -1 {
				closeIdx = i
			}
		case tokenLinkStart:
			if linkStartIdx == -1 {
				linkStartIdx = i
				linkURL = tok.LinkURL
			}
		case tokenLinkEnd:
			if linkEndIdx == -1 {
				linkEndIdx = i
			}
		}
	}
	if openIdx == -1 || closeIdx == -1 {
		t.Fatalf("expected literal bracket tokens around autolink")
	}
	if linkStartIdx == -1 || linkEndIdx == -1 {
		t.Fatalf("expected autolink tokens inside brackets")
	}
	if linkURL != "http://example.com" {
		t.Fatalf("unexpected autolink url: %q", linkURL)
	}
	if !(openIdx < linkStartIdx && linkStartIdx < linkEndIdx && linkEndIdx < closeIdx) {
		t.Fatalf("expected bracketed autolink ordering, got open=%d start=%d end=%d close=%d", openIdx, linkStartIdx, linkEndIdx, closeIdx)
	}
}

func TestLiveParserAutoLinkStylingWithoutOSC8(t *testing.T) {
	src := "<https://pkt.systems>"
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream parse live: %v", err)
	}
	linkTextStyle := DefaultTheme().Styles().LinkText.Prefix
	for _, tok := range stream.tokens {
		if tok.Kind == tokenLinkStart || tok.Kind == tokenLinkEnd {
			t.Fatalf("unexpected link token when osc8 disabled")
		}
		if tok.Kind == tokenURL {
			if tok.Style.Prefix != linkTextStyle {
				t.Fatalf("expected link style for autolink token")
			}
			return
		}
	}
	t.Fatalf("expected url token for autolink")
}

func TestRenderOSC8LinkSpan(t *testing.T) {
	src := "This is [an example](http://example.com/) inline link."
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader:  strings.NewReader(src),
		Writer:  &out,
		Width:   80,
		Theme:   DefaultTheme(),
		Options: []RenderOption{WithOSC8(true)},
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	rendered := out.String()
	start := strings.Index(rendered, osc8Start)
	if start == -1 {
		t.Fatalf("missing osc8 start: %q", rendered)
	}
	urlEnd := strings.Index(rendered[start+len(osc8Start):], "\x1b\\")
	if urlEnd == -1 {
		t.Fatalf("missing osc8 url terminator: %q", rendered)
	}
	urlEnd += start + len(osc8Start)
	linkEnd := strings.Index(rendered[urlEnd+2:], osc8End)
	if linkEnd == -1 {
		t.Fatalf("missing osc8 end: %q", rendered)
	}
	linkEnd += urlEnd + 2
	linkText := rendered[urlEnd+2 : linkEnd]
	ansiRe := regexp.MustCompile("\x1b\\[[0-9;]*m")
	linkText = ansiRe.ReplaceAllString(linkText, "")
	if linkText != "an example" {
		t.Fatalf("unexpected osc8 link text: %q", linkText)
	}
}

func TestRenderListItemBoundary(t *testing.T) {
	src := strings.Join([]string{
		"- Outputs:",
		"  - A user-facing function or interface method must return no more than two values:",
		"    (T, error) or (Response, error).",
		"  - If multiple outputs are required, return a response/result struct as the first value.",
	}, "\n")
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader(src),
		Writer: &out,
		Width:  80,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	plain := stripANSI(out.String())
	if strings.Contains(plain, "error).  - If multiple outputs") {
		t.Fatalf("list items merged onto same line: %q", plain)
	}
	if !strings.Contains(plain, "\n  - If multiple outputs are required") {
		t.Fatalf("missing list item boundary: %q", plain)
	}
}

func TestRenderKeepsPunctWithCodeSpan(t *testing.T) {
	src := strings.Join([]string{
		"- Outputs:",
		"  - A user-facing function or interface method must return no more than two values:",
		"    `(T, error)` or `(Response, error)`.",
		"  - If multiple outputs are required, return a response/result struct as the first value.",
	}, "\n")
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader(src),
		Writer: &out,
		Width:  60,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	plain := stripANSI(out.String())
	if !strings.Contains(plain, "error)\n    .") {
		t.Fatalf("expected punctuation to be allowed to wrap after code span: %q", plain)
	}
}

func TestRenderDecodesNBSP(t *testing.T) {
	src, err := os.ReadFile("testdata/parity__nbsp.md")
	if err != nil {
		t.Fatalf("read parity__nbsp.md: %v", err)
	}
	var out bytes.Buffer
	err = Render(RenderRequest{
		Reader: bytes.NewReader(src),
		Writer: &out,
		Width:  60,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	plain := stripANSI(out.String())
	if strings.Contains(plain, "&nbsp;") {
		t.Fatalf("expected &nbsp; to be decoded, got: %q", plain)
	}
	if strings.Contains(plain, "\u00A0") {
		t.Fatalf("expected NBSP entities to render as regular spaces")
	}
}

func TestDecodeEntityNBSP(t *testing.T) {
	r, ok := decodeEntity([]byte("&nbsp;"))
	if !ok || r != '\u00A0' {
		t.Fatalf("expected decodeEntity to return NBSP, got %q ok=%v", r, ok)
	}
}

func TestEmitInlineEntityNBSP(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	for _, r := range "&nbsp;" {
		if err := parser.emitInline(stream, r); err != nil {
			t.Fatalf("emit inline: %v", err)
		}
	}
	parser.flushPendingDelims()
	var b strings.Builder
	for _, tok := range stream.tokens {
		b.WriteString(tok.Text)
	}
	got := b.String()
	if strings.Contains(got, "&nbsp;") {
		t.Fatalf("expected emitInline to decode &nbsp;, got: %q", got)
	}
	if !strings.Contains(got, "\u00A0") {
		t.Fatalf("expected emitInline output to contain NBSP")
	}
}

func TestEmitInlineRunesNBSP(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	if err := parser.emitInlineRunes(stream, []rune("350&nbsp;000")); err != nil {
		t.Fatalf("emit inline runes: %v", err)
	}
	var b strings.Builder
	for _, tok := range stream.tokens {
		b.WriteString(tok.Text)
	}
	got := b.String()
	if strings.Contains(got, "&nbsp;") {
		t.Fatalf("expected emitInlineRunes to decode &nbsp;, got: %q", got)
	}
	if !strings.Contains(got, "\u00A0") {
		t.Fatalf("expected emitInlineRunes output to contain NBSP")
	}
}

func TestNumericUnderscoreRendersAsSpace(t *testing.T) {
	src := "10_000_000\n"
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader(src),
		Writer: &out,
		Width:  80,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	raw := out.String()
	if strings.Contains(raw, "\x1b[3") {
		t.Fatalf("expected no italic styling for numeric underscores, got: %q", raw)
	}
	plain := stripANSI(raw)
	if !strings.Contains(plain, "10 000 000") {
		t.Fatalf("expected numeric underscores to render as spaces, got: %q", plain)
	}
}

func TestNumericUnderscoreUnitsNoWrap(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		width int
		want  string
	}{
		{
			name:  "gib",
			src:   "X X 4_2GiB Y\n",
			width: 9,
			want:  "4 2GiB",
		},
		{
			name:  "ms",
			src:   "Time 1_000_000ms end\n",
			width: 14,
			want:  "1 000 000ms",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := Render(RenderRequest{
				Reader: strings.NewReader(tc.src),
				Writer: &out,
				Width:  tc.width,
				Theme:  DefaultTheme(),
			})
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			raw := out.String()
			if strings.Contains(raw, "\x1b[3") {
				t.Fatalf("expected no italic styling for numeric underscores, got: %q", raw)
			}
			plain := stripANSI(raw)
			lines := strings.Split(plain, "\n")
			found := false
			for _, line := range lines {
				if strings.Contains(line, tc.want) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected %q to appear on a single line, got: %q", tc.want, plain)
			}
		})
	}
}

func TestQuoteWrappingScenarios(t *testing.T) {
	puncts := []rune{'.', ',', ';', ':', '!', '?'}
	quotes := []string{"\"", "”", "’"}
	for _, p := range puncts {
		for _, q := range quotes {
			name := fmt.Sprintf("punct_%c_quote_%s", p, q)
			t.Run(name, func(t *testing.T) {
				src := fmt.Sprintf("X Y Z%c*%sword*\n", p, q)
				var out bytes.Buffer
				err := Render(RenderRequest{
					Reader: strings.NewReader(src),
					Writer: &out,
					Width:  6,
					Theme:  DefaultTheme(),
				})
				if err != nil {
					t.Fatalf("render: %v", err)
				}
				plain := stripANSI(out.String())
				if strings.Contains(plain, fmt.Sprintf("%c\n%s", p, q)) {
					t.Fatalf("expected quote to stay attached to punctuation, got: %q", plain)
				}
			})
		}
	}

	t.Run("no_lone_quote_line", func(t *testing.T) {
		src := "A *\"hello\"* world\n"
		var out bytes.Buffer
		err := Render(RenderRequest{
			Reader: strings.NewReader(src),
			Writer: &out,
			Width:  6,
			Theme:  DefaultTheme(),
		})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		plain := stripANSI(out.String())
		for _, line := range strings.Split(plain, "\n") {
			for _, q := range quotes {
				if line == q {
					t.Fatalf("expected quote to stay with words, got: %q", plain)
				}
			}
		}
	})
}

func TestRenderWidths(t *testing.T) {
	src := strings.Join([]string{
		"# Title",
		"",
		"- item one with a long line that should wrap cleanly",
		"  - nested item with more words and wrapping",
		"",
		"> Quote line one with more words to wrap",
		"> Quote line two with additional words to wrap",
		"",
		"Paragraph with **strong** and _em_ text plus a link [site](https://example.com).",
	}, "\n")
	for width := 20; width <= 100; width += 5 {
		var out bytes.Buffer
		err := Render(RenderRequest{
			Reader: strings.NewReader(src),
			Writer: &out,
			Width:  width,
			Theme:  DefaultTheme(),
		})
		if err != nil {
			t.Fatalf("stream live width %d: %v", width, err)
		}
		plain := stripANSI(out.String())
		lines := strings.Split(plain, "\n")
		for i, line := range lines {
			if line == "" {
				continue
			}
			if ansi.PrintableRuneWidth(line) > width {
				t.Fatalf("width %d: line %d exceeds width: %q", width, i+1, line)
			}
		}
	}
}

func TestRenderAgentsTextPresent(t *testing.T) {
	src := readAgents(t)
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: bytes.NewReader(src),
		Writer: &out,
		Width:  80,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	plain := normalizeWhitespace(stripANSI(out.String()))
	lines := strings.Split(string(src), "\n")
	for _, line := range lines {
		if strings.Contains(line, "<") {
			continue
		}
		line = strings.TrimLeft(line, " \t")
		want := normalizeMarkdownLine(line)
		if want == "" {
			continue
		}
		if !strings.Contains(plain, normalizeWhitespace(want)) {
			t.Fatalf("missing text %q in live output", want)
		}
	}
}

func TestRenderTaskListIndent(t *testing.T) {
	src := "- [ ] Task item with enough words to wrap"
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader(src),
		Writer: &out,
		Width:  20,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	plain := stripANSI(out.String())
	if !strings.Contains(plain, "\n      enough") {
		t.Fatalf("expected task list wrap indent, got: %q", plain)
	}
}

func TestRenderReflow(t *testing.T) {
	src := "This is a wrapped line that should\nflow into the next line without\nblank lines."
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader(src),
		Writer: &out,
		Width:  80,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	plain := stripANSI(out.String())
	if strings.Contains(plain, "\n\n") {
		t.Fatalf("expected reflowed output, got: %q", plain)
	}
	if !strings.Contains(plain, "line that should flow into the next line without blank lines.") {
		t.Fatalf("expected reflowed paragraph, got: %q", plain)
	}
}

func TestRenderHardBreak(t *testing.T) {
	src := "Line one with break  \nLine two after break."
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader(src),
		Writer: &out,
		Width:  80,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	plain := stripANSI(out.String())
	if !strings.Contains(plain, "Line one with break\nLine two") {
		t.Fatalf("expected hard line break, got: %q", plain)
	}
}

type oneByteReader struct {
	data []byte
	pos  int
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}

func TestRenderOneByteChunks(t *testing.T) {
	src := "UTF-8 test — ok"
	r := &oneByteReader{data: []byte(src)}
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: r,
		Writer: &out,
		Width:  80,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
}
