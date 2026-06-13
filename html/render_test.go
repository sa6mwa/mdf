package html

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"pkt.systems/mdf"
)

type captureTraceEncoder struct {
	events []mdf.WriteTraceEvent
}

func (e *captureTraceEncoder) EncodeWriteTraceEvent(event mdf.WriteTraceEvent) error {
	e.events = append(e.events, event)
	return nil
}

func TestRenderWritesSelfContainedHTML(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("# Title\n\nHello **world** and [site](https://example.com).\n"),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	wants := []string{
		"<!doctype html>",
		"@font-face{font-family:\"JetBrains Mono\"",
		"src:url(data:font/woff2;base64,",
		"format('woff2')",
		"--mdf-content-max-width:96ch;",
		"--mdf-page-padding-block:36pt;",
		"--mdf-page-padding-inline:36pt;",
		"width:min(100%,var(--mdf-content-max-width));",
		"margin-inline:auto;",
		"padding-inline:clamp(1rem,4vw,var(--mdf-page-padding-inline));",
		"font-size:12pt;",
		"line-height:1.4;",
		"white-space:pre-wrap;",
		"overflow-wrap:anywhere;",
		`class="mdf-heading"`,
		"--mdf-heading-indent:2ch;",
		"Hello ",
		"world",
		`<a href="https://example.com">`,
		"</html>",
	}
	for _, want := range wants {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in rendered HTML", want)
		}
	}
	if strings.Contains(rendered, "<script") {
		t.Fatalf("unexpected script tag in rendered HTML")
	}
}

func TestRenderWriteTraceReconstructsExactHTML(t *testing.T) {
	var out bytes.Buffer
	trace := &captureTraceEncoder{}
	err := Render(RenderRequest{
		Reader: strings.NewReader("Hello **world**.\n"),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Trace:  trace,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	if len(trace.events) < 2 {
		t.Fatalf("expected multiple trace events, got %d", len(trace.events))
	}
	var reconstructed bytes.Buffer
	for i, event := range trace.events {
		if event.Seq != uint64(i+1) {
			t.Fatalf("event %d sequence got %d want %d", i, event.Seq, i+1)
		}
		if event.Format != mdf.WriteTraceFormatHTML {
			t.Fatalf("event %d format got %q want html", i, event.Format)
		}
		if event.Op != "emit" {
			t.Fatalf("event %d op got %q want emit", i, event.Op)
		}
		data, err := base64.StdEncoding.DecodeString(event.DataB64)
		if err != nil {
			t.Fatalf("decode event %d: %v", event.Seq, err)
		}
		if len(data) != event.Bytes {
			t.Fatalf("event %d bytes got %d want %d", event.Seq, len(data), event.Bytes)
		}
		reconstructed.Write(data)
	}
	if reconstructed.String() != out.String() {
		t.Fatalf("reconstructed trace does not match HTML output")
	}
}

func TestRenderBlockQuoteTextUsesThemeColorInNonDefaultThemes(t *testing.T) {
	cfg := DefaultConfig()
	for _, name := range mdf.AvailableThemes() {
		if name == "default" {
			continue
		}
		theme, ok := mdf.ThemeByName(name)
		if !ok {
			t.Fatalf("missing theme %q", name)
		}
		styles := theme.Styles()

		var out bytes.Buffer
		err := Render(RenderRequest{
			Reader: strings.NewReader("> plain **strong** [link](https://example.com)\n"),
			Writer: &out,
			Theme:  theme,
			Config: cfg,
		})
		if err != nil {
			t.Fatalf("render html theme %q: %v", name, err)
		}

		rendered := out.String()
		quoteTextColor := rgbCSS(parseANSIPrefix(styles.QuoteText.Prefix, cfg.TextRGB).color)
		quoteMarkerColor := rgbCSS(parseANSIPrefix(styles.Quote.Prefix, cfg.TextRGB).color)
		strongColor := rgbCSS(parseANSIPrefix(styles.Strong.Prefix, cfg.TextRGB).color)
		linkColor := rgbCSS(parseANSIPrefix(styles.LinkText.Prefix, cfg.TextRGB).color)

		for _, want := range []string{
			`<span style="color:` + quoteMarkerColor + `;">&gt;</span>`,
			`<span style="color:` + quoteTextColor + `;`,
			`plain `,
			`<span style="color:` + strongColor + `;font-weight:700;">strong</span>`,
			`<span style="color:` + linkColor + `;font-weight:700;text-decoration:underline;">link</span>`,
		} {
			if !strings.Contains(rendered, want) {
				t.Fatalf("theme %q missing %q in rendered HTML:\n%s", name, want, rendered)
			}
		}
	}
}

func TestRenderCanOptIntoEmbeddedHackFont(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	cfg.EmbeddedFont = EmbeddedFontHack
	err := Render(RenderRequest{
		Reader: strings.NewReader("body\n"),
		Writer: &out,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	for _, want := range []string{
		"@font-face{font-family:\"HackNerdFontMono\"",
		"src:url(data:font/ttf;base64,",
		"format('truetype')",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in rendered HTML", want)
		}
	}
}

func TestRenderRejectsUnknownEmbeddedFont(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	cfg.EmbeddedFont = EmbeddedFont("nope")
	err := Render(RenderRequest{
		Reader: strings.NewReader("body\n"),
		Writer: &out,
		Config: cfg,
	})
	if err == nil {
		t.Fatalf("expected unknown embedded font to be rejected")
	}
	if !strings.Contains(err.Error(), `unsupported embedded font "nope"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderSupportsEmbeddedWOFFFontBytes(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	cfg.FontFamily = "Custom"
	cfg.RegularFontBytes = []byte("regular")
	cfg.BoldFontBytes = []byte("bold")
	cfg.ItalicFontBytes = []byte("italic")
	cfg.RegularFontFormat = "woff"
	cfg.BoldFontFormat = "woff"
	cfg.ItalicFontFormat = "woff"
	err := Render(RenderRequest{
		Reader: strings.NewReader("body\n"),
		Writer: &out,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	for _, want := range []string{
		`@font-face{font-family:"Custom";src:url(data:font/woff;base64,`,
		`format('woff')`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in rendered HTML: %q", want, rendered)
		}
	}
}

func TestRenderRejectsUnsupportedEmbeddedFontFormat(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	cfg.FontFamily = "Custom"
	cfg.RegularFontBytes = []byte("regular")
	cfg.BoldFontBytes = []byte("bold")
	cfg.ItalicFontBytes = []byte("italic")
	cfg.RegularFontFormat = "svg"
	cfg.BoldFontFormat = "svg"
	cfg.ItalicFontFormat = "svg"
	err := Render(RenderRequest{
		Reader: strings.NewReader("body\n"),
		Writer: &out,
		Config: cfg,
	})
	if err == nil {
		t.Fatalf("expected unsupported embedded font format to be rejected")
	}
	if !strings.Contains(err.Error(), `unsupported regular font format "svg"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderEscapesMarkdownText(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("literal <tag> & value\n"),
		Writer: &out,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "literal &lt;tag&gt; &amp; value") {
		t.Fatalf("expected escaped text, got %q", rendered)
	}
}

func TestRenderKeepsAmpersandsInsideLinkText(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("them into production workflows because output verification mechanisms were absent ([Apostolou, Bosch & Holmström Olsson, 2026](https://arxiv.org/abs/2605.14675)). Also [R&D](https://example.com/rd) and [AT&T](https://example.com/att).\n"),
		Writer: &out,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	want := `<a href="https://arxiv.org/abs/2605.14675"><span style="color:rgb(59,156,255);font-weight:700;text-decoration:underline;">Apostolou, Bosch &amp; Holmström Olsson, 2026</span></a>`
	if !strings.Contains(rendered, want) {
		t.Fatalf("missing linked citation with ampersand inside anchor: %q", rendered)
	}
	for _, want := range []string{
		`<a href="https://example.com/rd"><span style="color:rgb(59,156,255);font-weight:700;text-decoration:underline;">R&amp;D</span></a>`,
		`<a href="https://example.com/att"><span style="color:rgb(59,156,255);font-weight:700;text-decoration:underline;">AT&amp;T</span></a>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing raw ampersand link label %q in rendered HTML: %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "absent (&amp;</span><a") || strings.Contains(rendered, "Bosch  Holmström") || strings.Contains(rendered, "[R&amp;D]") || strings.Contains(rendered, "[AT&amp;T]") {
		t.Fatalf("ampersand escaped link label in rendered HTML: %q", rendered)
	}
}

func TestRenderIntentionallySuppressesThematicBreaks(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("before\n\n---\n\nafter\n"),
		Writer: &out,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	if strings.Contains(rendered, "<hr") || strings.Contains(rendered, "mdf-thematic-break") {
		t.Fatalf("HTML thematic breaks are intentionally suppressed, but rendered a visible rule: %q", rendered)
	}
	if strings.Contains(rendered, "---") {
		t.Fatalf("HTML thematic breaks are intentionally suppressed, but rendered the raw marker: %q", rendered)
	}
	for _, want := range []string{"before", "after"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in rendered HTML: %q", want, rendered)
		}
	}
}

func TestApplyConfigKeepsDefaultTableBufferForPartialConfig(t *testing.T) {
	cfg := DefaultConfig()
	applyConfig(&cfg, Config{Margin: 24})
	if cfg.Margin != 24 {
		t.Fatalf("margin = %v, want 24", cfg.Margin)
	}
	if cfg.ContentMaxWidthCh != 96 {
		t.Fatalf("content max width = %v, want 96", cfg.ContentMaxWidthCh)
	}
	if cfg.TableBufferMode != mdf.TableBufferFull {
		t.Fatalf("table buffer mode = %v, want %v", cfg.TableBufferMode, mdf.TableBufferFull)
	}
}

func TestRenderUsesConfiguredContentWidth(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	cfg.ContentMaxWidthCh = 72
	err := Render(RenderRequest{
		Reader: strings.NewReader("body\n"),
		Writer: &out,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "--mdf-content-max-width:72ch;") {
		t.Fatalf("configured content max width missing from rendered HTML: %q", rendered)
	}
}

func TestApplyConfigAllowsExplicitFullTableBuffer(t *testing.T) {
	cfg := DefaultConfig()
	applyConfig(&cfg, Config{TableBufferMode: mdf.TableBufferFull})
	if cfg.TableBufferMode != mdf.TableBufferFull {
		t.Fatalf("table buffer mode = %v, want %v", cfg.TableBufferMode, mdf.TableBufferFull)
	}
}

func TestRenderEmbedsCornerImageBytes(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	cfg.CornerImageBytes = []byte("png-ish")
	cfg.CornerImageMIME = "image/png"
	err := Render(RenderRequest{
		Reader: strings.NewReader("body\n"),
		Writer: &out,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, `<img class="mdf-corner-image"`) {
		t.Fatalf("missing corner image: %q", rendered)
	}
	if !strings.Contains(rendered, `src="data:image/png;base64,cG5nLWlzaA=="`) {
		t.Fatalf("corner image was not embedded: %q", rendered)
	}
}

func TestRenderUsesHangingIndentForListPrefixes(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("- list item that wraps\n- [ ] task item that wraps\n> - quote list item that wraps\n"),
		Writer: &out,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	wants := []string{
		`<span class="mdf-line"><span class="mdf-prefix"><span style="color:rgb(0,205,205);font-size:12pt;font-weight:700;">-</span><span style="color:rgb(220,220,220);"> </span></span><span class="mdf-content">`,
		`<span class="mdf-line"><span class="mdf-prefix"><span style="color:rgb(0,205,205);font-size:12pt;font-weight:700;">-</span><span style="color:rgb(220,220,220);"> [ ] </span></span><span class="mdf-content">`,
		`<span class="mdf-line"><span class="mdf-prefix"><span style="color:rgb(127,127,127);">&gt;</span><span style="color:rgb(220,220,220);"> </span><span style="color:rgb(0,205,205);font-size:12pt;font-weight:700;">-</span><span style="color:rgb(220,220,220);"> </span></span><span class="mdf-content">`,
	}
	for _, want := range wants {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in rendered HTML: %q", want, rendered)
		}
	}
}

func TestRenderUsesHangingIndentForIndentedContinuationLines(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("1. **Outcome:** Users complete tasks faster  \n   **Signals of change:** Higher task completion rates and a reduction in user drop-offs\n"),
		Writer: &out,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	want := `<span class="mdf-line"><span class="mdf-prefix"><span style="color:rgb(220,220,220);">   </span></span><span class="mdf-content">`
	if !strings.Contains(rendered, want) {
		t.Fatalf("missing indented continuation wrapper %q in rendered HTML: %q", want, rendered)
	}
}

func TestRenderMarkdownTable(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("| Left | Right | Center |\n| :--- | ---: | :---: |\n| <a> | b | c |\n"),
		Writer: &out,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	wants := []string{
		`<table class="mdf-table mdf-table-bordered">`,
		"<thead><tr>",
		`<th style="text-align:left;">Left</th>`,
		`<th style="text-align:right;">Right</th>`,
		`<th style="text-align:center;">Center</th>`,
		"</thead><tbody>",
		`<td style="text-align:left;">&lt;a&gt;</td>`,
		".mdf-table th{color:rgb(0,205,205);font-weight:700;overflow-wrap:normal;word-break:normal;}",
		"--mdf-table-wire:rgb(127,127,127);",
		"border:1px solid var(--mdf-table-wire);padding:.2em 1ch;",
	}
	for _, want := range wants {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in rendered HTML: %q", want, rendered)
		}
	}
}

func TestRenderMarkdownTableInlineCellContent(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("| Kind | Value |\n| --- | --- |\n| em | *italic* |\n| strong | **bold** |\n| code | `value` |\n| link | [site](https://example.com) |\n"),
		Writer: &out,
		Config: DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	for _, raw := range []string{"*italic*", "**bold**", "`value`", "[site](https://example.com)"} {
		if strings.Contains(rendered, raw) {
			t.Fatalf("table cell rendered raw markdown %q: %q", raw, rendered)
		}
	}
	for _, want := range []string{
		`<span style="color:rgb(59,156,255);font-style:italic;">italic</span>`,
		`<span style="color:rgb(229,229,229);font-weight:700;">bold</span>`,
		`<span style="color:rgb(205,0,205);">value</span>`,
		`<a href="https://example.com"><span style="color:rgb(59,156,255);font-weight:700;text-decoration:underline;">site</span></a>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in rendered HTML: %q", want, rendered)
		}
	}
}

func TestRenderMarkdownTableInlineHeaderContentKeepsHeaderStyle(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("| *Head* | [Docs](https://example.com) |\n| --- | --- |\n| value | link |\n"),
		Writer: &out,
		Config: DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	for _, want := range []string{
		`<span style="color:rgb(0,205,205);font-weight:700;font-style:italic;">Head</span>`,
		`<a href="https://example.com"><span style="color:rgb(0,205,205);font-weight:700;text-decoration:underline;">Docs</span></a>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing header inline style %q in rendered HTML: %q", want, rendered)
		}
	}
	if strings.Contains(rendered, `color:rgb(59,156,255);font-style:italic;">Head`) {
		t.Fatalf("header emphasis used body emphasis color: %q", rendered)
	}
}

func TestRenderMarkdownTablePreservesSignificantCellSpaces(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("| Kind | Value |\n| --- | --- |\n| code | `go  test` |\n"),
		Writer: &out,
		Config: DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "go  test") {
		t.Fatalf("table cell lost significant spaces: %q", rendered)
	}
	if !strings.Contains(rendered, ".mdf-table th,.mdf-table td{vertical-align:top;overflow-wrap:anywhere;white-space:pre-wrap;}") {
		t.Fatalf("table cells do not preserve significant whitespace: %q", rendered)
	}
}

func TestRenderBlockQuoteMarkdownTableKeepsVisiblePrefix(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("> | A | B |\n> | --- | --- |\n> | 1 | 2 |\n"),
		Writer: &out,
		Config: DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	for _, want := range []string{
		`<div class="mdf-table-block"><span class="mdf-prefix">`,
		`&gt;`,
		`</span><table class="mdf-table mdf-table-bordered">`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing visible table prefix %q in rendered HTML: %q", want, rendered)
		}
	}
}

func TestRenderMarkdownTablePadsAndTruncatesBodyRows(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("| A | B | C |\n| --- | --- | --- |\n| 1 | 2 |\n| 3 | 4 | 5 | 6 |\n"),
		Writer: &out,
		Config: DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	for _, want := range []string{
		`<tr><td style="text-align:left;">1</td><td style="text-align:left;">2</td><td style="text-align:left;"></td></tr>`,
		`<tr><td style="text-align:left;">3</td><td style="text-align:left;">4</td><td style="text-align:left;">5</td></tr>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing normalized row %q in rendered HTML: %q", want, rendered)
		}
	}
	if strings.Contains(rendered, ">6</td>") {
		t.Fatalf("HTML table rendered extra body cell: %q", rendered)
	}
}

func TestRenderHeaderlessMarkdownTableUsesTableWideColumnCount(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("| A | B |\n| 1 | 2 | 3 |\n"),
		Writer: &out,
		Config: DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	for _, want := range []string{
		`<tr><td style="text-align:left;">A</td><td style="text-align:left;">B</td><td style="text-align:left;"></td></tr>`,
		`<tr><td style="text-align:left;">1</td><td style="text-align:left;">2</td><td style="text-align:left;">3</td></tr>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing normalized headerless row %q in rendered HTML: %q", want, rendered)
		}
	}
}

func TestRenderRowBufferedHeaderlessMarkdownTableKeepsLateExtraCells(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferRow
	err := Render(RenderRequest{
		Reader: strings.NewReader("| A | B |\n| 1 | 2 |\n| 3 | 4 | 5 |\n"),
		Writer: &out,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	want := `<tr><td style="text-align:left;">3</td><td style="text-align:left;">4 | 5</td></tr>`
	if !strings.Contains(rendered, want) {
		t.Fatalf("row-buffered headerless HTML table dropped or widened a late extra cell, missing %q: %q", want, rendered)
	}
}

func TestRenderMarkdownTableSpaceWire(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	cfg.TableWireMode = mdf.TableWireSpace
	err := Render(RenderRequest{
		Reader: strings.NewReader("| A | B |\n| --- | --- |\n| 1 | 2 |\n"),
		Writer: &out,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, `<table class="mdf-table mdf-table-space">`) {
		t.Fatalf("missing space table class: %q", rendered)
	}
	if !strings.Contains(rendered, ".mdf-table-space th,.mdf-table-space td{border:0;padding:0 1ch;}") {
		t.Fatalf("missing space table CSS: %q", rendered)
	}
	if !strings.Contains(rendered, ".mdf-table-space th:first-child,.mdf-table-space td:first-child{padding-left:0;}") {
		t.Fatalf("missing space table left edge CSS: %q", rendered)
	}
	if !strings.Contains(rendered, ".mdf-table-space th:last-child,.mdf-table-space td:last-child{padding-right:0;}") {
		t.Fatalf("missing space table right edge CSS: %q", rendered)
	}
}

func TestRenderRejectsASCIIWireMode(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	cfg.TableWireMode = mdf.TableWireASCII
	err := Render(RenderRequest{
		Reader: strings.NewReader("| A | B |\n| --- | --- |\n| 1 | 2 |\n"),
		Writer: &out,
		Config: cfg,
	})
	if err == nil {
		t.Fatalf("expected ascii table wire mode to be rejected for HTML")
	}
	if !strings.Contains(err.Error(), "table wire mode ascii is not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderMarkdownTableBoringUsesTextBorderColor(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	cfg.Boring = true
	err := Render(RenderRequest{
		Reader: strings.NewReader("| A | B |\n| --- | --- |\n| 1 | 2 |\n"),
		Writer: &out,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "--mdf-table-wire:rgb(0,0,0);") {
		t.Fatalf("boring table wire did not use boring text color: %q", rendered)
	}
	if strings.Contains(rendered, "--mdf-table-wire:rgb(64,64,64);") {
		t.Fatalf("boring table wire kept themed gray border: %q", rendered)
	}
}

func TestRenderMarkdownTableUsesThemeWireColor(t *testing.T) {
	var out bytes.Buffer
	styles := mdf.DefaultTheme().Styles()
	styles.TableWire = mdf.Style{Prefix: "\x1b[31m"}
	err := Render(RenderRequest{
		Reader: strings.NewReader("| A | B |\n| --- | --- |\n| 1 | 2 |\n"),
		Writer: &out,
		Theme:  mdf.NewTheme("custom-wire", styles),
		Config: DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "--mdf-table-wire:rgb(205,0,0);") {
		t.Fatalf("table wire did not use theme color: %q", rendered)
	}
}

func TestRenderMarkdownTableContainerMargin(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("> | A | B |\n> | --- | --- |\n> | 1 | 2 |\n"),
		Writer: &out,
		Config: DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, `<div class="mdf-table-block"><span class="mdf-prefix">`) {
		t.Fatalf("missing contained table prefix wrapper: %q", rendered)
	}
	if !strings.Contains(rendered, `<table class="mdf-table mdf-table-bordered">`) {
		t.Fatalf("missing contained table: %q", rendered)
	}
}
