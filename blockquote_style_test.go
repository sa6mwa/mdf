package mdf

import (
	"bytes"
	"strings"
	"testing"
)

func TestBlockQuoteColorsOnlyPlainText(t *testing.T) {
	styles := quoteTextTestTheme().Styles()
	src := []byte("> plain *em* **strong** ***both*** `code` [link](https://example.com)\n")

	out := renderStreamWithTheme(t, src, 0, quoteTextTestTheme())

	assertContains(t, out, styles.Quote.Prefix+">\x1b[0m ")
	assertContains(t, out, styles.QuoteText.Prefix+"plain")
	assertContains(t, out, styles.Emphasis.Prefix+"em\x1b[0m")
	assertContains(t, out, styles.Strong.Prefix+"strong\x1b[0m")
	assertContains(t, out, styles.EmphasisStrong.Prefix+"both\x1b[0m")
	assertContains(t, out, styles.CodeInline.Prefix+"code\x1b[0m")
	assertContains(t, out, styles.LinkText.Prefix+"link\x1b[0m")
	assertContains(t, out, styles.LinkURL.Prefix+"https://example.com\x1b[0m")

	if strings.Contains(out, styles.Quote.Prefix+"plain") {
		t.Fatalf("quote marker style leaked into quote body: %q", out)
	}
	for _, forbidden := range []string{
		combineStyles(styles.QuoteText, styles.Emphasis).Prefix + "em",
		combineStyles(styles.QuoteText, styles.Strong).Prefix + "strong",
		combineStyles(styles.QuoteText, styles.EmphasisStrong).Prefix + "both",
		combineStyles(styles.QuoteText, styles.CodeInline).Prefix + "code",
		combineStyles(styles.QuoteText, styles.LinkText).Prefix + "link",
		combineStyles(styles.QuoteText, styles.LinkURL).Prefix + "https://example.com",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("quote text style leaked into inline style %q in %q", forbidden, out)
		}
	}
}

func TestBlockQuoteTextColorAppliesToQuotedListContent(t *testing.T) {
	theme := quoteTextTestTheme()
	styles := theme.Styles()
	out := renderStreamWithTheme(t, []byte("> - item\n"), 0, theme)

	assertContains(t, out, styles.Quote.Prefix+">\x1b[0m ")
	assertContains(t, out, styles.ListMarker.Prefix+"-\x1b[0m ")
	assertContains(t, out, styles.QuoteText.Prefix+"item\x1b[0m")
}

func TestBlockQuoteTextStateDoesNotDependOnStylePrefix(t *testing.T) {
	theme := quoteTextHeadingCollisionTheme()
	styles := theme.Styles()

	heading := renderStreamWithTheme(t, []byte("# *title*\n"), 0, theme)
	assertContains(t, heading, combineStyles(styles.Heading[0], styles.Emphasis).Prefix+"title")

	quote := renderStreamWithTheme(t, []byte("> *title*\n"), 0, theme)
	assertContains(t, quote, styles.Emphasis.Prefix+"title")
	forbidden := combineStyles(styles.QuoteText, styles.Emphasis).Prefix + "title"
	if strings.Contains(quote, forbidden) {
		t.Fatalf("quote text style leaked into inline emphasis after prefix collision: %q", quote)
	}
}

func TestBlockQuoteTextColorAppliesToQuotedTableBodyCells(t *testing.T) {
	theme := quoteTextTestTheme()
	styles := theme.Styles()
	src := []byte("> | H | S |\n> | --- | --- |\n> | plain | **strong** [link](https://example.com) |\n")

	out := renderStreamWithTheme(t, src, 120, theme, WithTableBufferMode(TableBufferFull), WithTableWireMode(TableWireLine))

	assertContains(t, out, styles.TableHeader.Prefix+"H")
	assertContains(t, out, styles.QuoteText.Prefix+"plain")
	assertContains(t, out, styles.Strong.Prefix+"strong")
	assertContains(t, out, styles.LinkText.Prefix+"link")
	assertContains(t, out, styles.LinkURL.Prefix+"https://example.com")
	for _, forbidden := range []string{
		combineStyles(styles.QuoteText, styles.Strong).Prefix + "strong",
		combineStyles(styles.QuoteText, styles.LinkText).Prefix + "link",
		combineStyles(styles.QuoteText, styles.LinkURL).Prefix + "https://example.com",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("quote table text style leaked into inline style %q in %q", forbidden, out)
		}
	}
}

func TestDefaultThemeLeavesBlockQuoteTextUncolored(t *testing.T) {
	styles := DefaultTheme().Styles()
	if styles.QuoteText.Prefix != "" {
		t.Fatalf("default quote text style got %q, want normal body text", styles.QuoteText.Prefix)
	}

	out := renderStream(t, []byte("> plain *em*\n"), 0)
	assertContains(t, out, styles.Quote.Prefix+">\x1b[0m ")
	if strings.Contains(out, ">\x1b[0m "+styles.Quote.Prefix+"plain") {
		t.Fatalf("default quote marker style leaked into quote body: %q", out)
	}
	if strings.Contains(out, "\x1b[0m "+styles.Emphasis.Prefix+"plain") {
		t.Fatalf("default quote body unexpectedly gained emphasis style: %q", out)
	}
	assertContains(t, out, ">\x1b[0m plain ")
	assertContains(t, out, styles.Emphasis.Prefix+"em\x1b[0m")
}

func TestNonDefaultBuiltInThemesDefineDistinctBlockQuoteTextColor(t *testing.T) {
	for _, name := range AvailableThemes() {
		if name == "default" {
			continue
		}
		theme, ok := ThemeByName(name)
		if !ok {
			t.Fatalf("missing theme %q", name)
		}
		styles := theme.Styles()
		if styles.QuoteText.Prefix == "" {
			t.Errorf("theme %q has empty quote text style", name)
		}
		if styles.QuoteText.Prefix == styles.Text.Prefix {
			t.Errorf("theme %q quote text style matches body text style", name)
		}
		if styles.QuoteText.Prefix == styles.Quote.Prefix {
			t.Errorf("theme %q quote text style matches quote marker style", name)
		}
		for i, heading := range styles.Heading {
			if styles.QuoteText.Prefix == heading.Prefix {
				t.Errorf("theme %q quote text style matches h%d style", name, i+1)
			}
		}
	}
}

func TestBlockQuoteWriteTraceIncludesQuoteTextANSI(t *testing.T) {
	theme := quoteTextTestTheme()
	styles := theme.Styles()
	trace := &captureTraceEncoder{}
	var out bytes.Buffer
	if err := Render(RenderRequest{
		Reader:  strings.NewReader("> traced *em*\n"),
		Writer:  &out,
		Width:   80,
		Theme:   theme,
		Options: []RenderOption{WithWriteTrace(trace)},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}

	payload := reconstructTracePayload(t, trace.events)
	assertContains(t, payload, styles.Quote.Prefix+">\x1b[0m ")
	assertContains(t, payload, styles.QuoteText.Prefix+"traced")
	assertContains(t, payload, styles.Emphasis.Prefix+"em\x1b[0m")
}

func quoteTextTestTheme() Theme {
	styles := DefaultTheme().Styles()
	styles.QuoteText = Style{Prefix: "\x1b[38;5;244m"}
	return NewTheme("quote-text-test", styles)
}

func quoteTextHeadingCollisionTheme() Theme {
	styles := DefaultTheme().Styles()
	styles.QuoteText = styles.Heading[0]
	return NewTheme("quote-text-heading-collision-test", styles)
}

func renderStreamWithTheme(t *testing.T, src []byte, width int, theme Theme, opts ...RenderOption) string {
	t.Helper()
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader:  bytes.NewReader(src),
		Writer:  &out,
		Width:   width,
		Theme:   theme,
		Options: opts,
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	return out.String()
}
