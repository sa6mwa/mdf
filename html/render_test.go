package html

import (
	"bytes"
	"strings"
	"testing"

	"pkt.systems/mdf"
)

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
		"@font-face{font-family:\"HackNerdFontMono\"",
		"src:url(data:font/ttf;base64,",
		"padding:36pt;",
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
