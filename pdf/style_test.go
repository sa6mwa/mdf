package pdf

import (
	"testing"

	"pkt.systems/mdf"
)

func TestParseANSIPrefix(t *testing.T) {
	attrs := parseANSIPrefix("\x1b[1;3;4;38;5;81m", [3]int{1, 2, 3})
	if !attrs.bold {
		t.Fatalf("expected bold")
	}
	if !attrs.italic {
		t.Fatalf("expected italic")
	}
	if !attrs.underline {
		t.Fatalf("expected underline")
	}
	want := [3]int{95, 215, 255}
	if attrs.color != want {
		t.Fatalf("unexpected color: %+v", attrs.color)
	}
}

func TestStyleToFontStyle(t *testing.T) {
	attrs := ansiAttrs{bold: true, italic: true}
	got := styleToFontStyle(attrs, false, false)
	if got != "B" {
		t.Fatalf("expected bold-only fallback, got %q", got)
	}
	got = styleToFontStyle(attrs, false, true)
	if got != "BI" {
		t.Fatalf("expected bold-italic when allowed, got %q", got)
	}
}

func TestHeadingScaleApplied(t *testing.T) {
	cfg := DefaultConfig()
	styles := mdf.DefaultTheme().Styles()
	s := &pdfStream{
		cfg:        cfg,
		styles:     styles,
		styleCache: make(map[string]pdfStyle),
	}
	h1 := s.styleForPrefix(styles.Heading[0].Prefix, 1)
	if h1.size != cfg.FontSize*cfg.HeadingScale[0] {
		t.Fatalf("unexpected h1 size: got %v want %v", h1.size, cfg.FontSize*cfg.HeadingScale[0])
	}
	if h1.fontFamily != cfg.FontFamily {
		t.Fatalf("unexpected h1 family: got %q want %q", h1.fontFamily, cfg.FontFamily)
	}
	h4 := s.styleForPrefix(styles.Heading[3].Prefix, 4)
	if h4.size != cfg.FontSize*cfg.HeadingScale[3] {
		t.Fatalf("unexpected h4 size: got %v want %v", h4.size, cfg.FontSize*cfg.HeadingScale[3])
	}
	cfg.HeadingFont = "/tmp/heading.ttf"
	s.cfg = cfg
	h2 := s.styleForPrefix(styles.Heading[1].Prefix, 2)
	if h2.fontFamily != headingFontFamily {
		t.Fatalf("unexpected heading font family: got %q want %q", h2.fontFamily, headingFontFamily)
	}
}

func TestQuoteTextStyleAppliedForNonDefaultThemes(t *testing.T) {
	cfg := DefaultConfig()
	for _, name := range mdf.AvailableThemes() {
		theme, ok := mdf.ThemeByName(name)
		if !ok {
			t.Fatalf("missing theme %q", name)
		}
		styles := theme.Styles()
		s := &pdfStream{
			cfg:        cfg,
			styles:     styles,
			styleCache: make(map[string]pdfStyle),
		}

		got := s.styleForPrefix(styles.QuoteText.Prefix, 0)
		wantColor := parseANSIPrefix(styles.QuoteText.Prefix, cfg.TextRGB).color
		if got.r != wantColor[0] || got.g != wantColor[1] || got.b != wantColor[2] {
			t.Fatalf("theme %q quote text color got (%d,%d,%d) want (%d,%d,%d)", name, got.r, got.g, got.b, wantColor[0], wantColor[1], wantColor[2])
		}

		if name == "default" {
			if styles.QuoteText.Prefix != "" {
				t.Fatalf("default quote text prefix got %q, want empty", styles.QuoteText.Prefix)
			}
			continue
		}
		if styles.QuoteText.Prefix == "" {
			t.Fatalf("theme %q quote text prefix is empty", name)
		}
		if styles.QuoteText.Prefix == styles.Quote.Prefix {
			t.Fatalf("theme %q quote text prefix matches quote marker prefix", name)
		}
	}
}
