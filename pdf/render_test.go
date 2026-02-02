package pdf

import (
	"bytes"
	"strings"
	"testing"

	"pkt.systems/mdf"
)

func TestRenderPDFWithCoreFonts(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("# Title\n\nThis is [a link](http://example.com/)."),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: Config{
			PageSize:   "A4",
			Margin:     36,
			FontFamily: "Courier",
			FontSize:   12,
			LineHeight: 1.4,
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(out.Bytes(), []byte("%PDF")) {
		t.Fatalf("unexpected pdf header: %q", out.Bytes()[:8])
	}
}

func TestRenderPDFSkipsUnsupportedRunes(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("# Title\n\nEmoji 😀 should be ignored.\n"),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: Config{
			PageSize:   "A4",
			Margin:     36,
			FontFamily: "Courier",
			FontSize:   12,
			LineHeight: 1.4,
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(out.Bytes(), []byte("%PDF")) {
		t.Fatalf("unexpected pdf header: %q", out.Bytes()[:8])
	}
}

func TestImageTypeForPath(t *testing.T) {
	cases := map[string]string{
		"/tmp/foo.png":  "PNG",
		"/tmp/foo.jpg":  "JPG",
		"/tmp/foo.jpeg": "JPG",
		"/tmp/foo.gif":  "",
	}
	for path, want := range cases {
		if got := imageTypeForPath(path); got != want {
			t.Fatalf("imageTypeForPath(%q) = %q, want %q", path, got, want)
		}
	}
	if err := validateImagePath("/tmp/foo.gif"); err == nil {
		t.Fatalf("expected validation error for unsupported image type")
	}
}

func TestRenderPDFWithOCGPrintView(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("# Title\n\nBody."),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: Config{
			PageSize:        "A4",
			Margin:          36,
			FontFamily:      "Courier",
			FontSize:        12,
			LineHeight:      1.4,
			UseOCGPrintView: true,
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	data := out.Bytes()
	if !bytes.Contains(data, []byte("/OCProperties")) {
		t.Fatalf("expected OCG properties in output")
	}
	if !bytes.Contains(data, []byte("/ViewState")) {
		t.Fatalf("expected view usage state in output")
	}
	if !bytes.Contains(data, []byte("/PrintState")) {
		t.Fatalf("expected print usage state in output")
	}
}

func TestRenderPDFWithOCGOpenPane(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("# Title\n\nBody."),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: Config{
			PageSize:        "A4",
			Margin:          36,
			FontFamily:      "Courier",
			FontSize:        12,
			LineHeight:      1.4,
			UseOCGPrintView: true,
			OpenLayerPane:   true,
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("/PageMode /UseOC")) {
		t.Fatalf("expected PageMode UseOC in output")
	}
}

func TestRenderBoringAndOCGConflict(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("hello"),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: Config{
			PageSize:        "A4",
			Margin:          36,
			FontFamily:      "Courier",
			FontSize:        12,
			LineHeight:      1.4,
			UseOCGPrintView: true,
			Boring:          true,
		},
	})
	if err == nil {
		t.Fatalf("expected error for boring+ocg")
	}
	if !strings.Contains(err.Error(), "life is too short for doubling down on boring") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderPDFAutoLinks(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("<https://pkt.systems> <sa6mwa@gmail.com> [<http://example.com>]"),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: Config{
			PageSize:   "A4",
			Margin:     36,
			FontFamily: "Courier",
			FontSize:   12,
			LineHeight: 1.4,
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	data := out.Bytes()
	if !bytes.Contains(data, []byte("https://pkt.systems")) {
		t.Fatalf("expected https autolink in pdf output")
	}
	if !bytes.Contains(data, []byte("mailto:sa6mwa@gmail.com")) {
		t.Fatalf("expected mailto autolink in pdf output")
	}
	if !bytes.Contains(data, []byte("http://example.com")) {
		t.Fatalf("expected bracketed autolink in pdf output")
	}
}

func TestRenderPDFLinkTextWithSpaces(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("[an example](http://example.com)"),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: Config{
			PageSize:   "A4",
			Margin:     36,
			FontFamily: "Courier",
			FontSize:   12,
			LineHeight: 1.4,
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("http://example.com")) {
		t.Fatalf("expected link target in pdf output")
	}
}
