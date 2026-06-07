package pdf

import (
	"bytes"
	"regexp"
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

func TestApplyConfigKeepsDefaultTableBufferForPartialConfig(t *testing.T) {
	cfg := DefaultConfig()
	applyConfig(&cfg, Config{Margin: 24})
	if cfg.Margin != 24 {
		t.Fatalf("margin = %v, want 24", cfg.Margin)
	}
	if cfg.TableBufferMode != mdf.TableBufferFull {
		t.Fatalf("table buffer mode = %v, want %v", cfg.TableBufferMode, mdf.TableBufferFull)
	}
}

func TestApplyConfigAllowsExplicitFullTableBuffer(t *testing.T) {
	cfg := DefaultConfig()
	applyConfig(&cfg, Config{TableBufferMode: mdf.TableBufferFull})
	if cfg.TableBufferMode != mdf.TableBufferFull {
		t.Fatalf("table buffer mode = %v, want %v", cfg.TableBufferMode, mdf.TableBufferFull)
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

func TestRenderPDFWithPrintViewAnnotations(t *testing.T) {
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
	if !bytes.Contains(data, []byte("/Subtype /Square")) {
		t.Fatalf("expected print/view square annotation in output")
	}
	if !bytes.Contains(data, []byte("/Subtype /Form")) {
		t.Fatalf("expected annotation appearance form in output")
	}
	if !bytes.Contains(data, []byte("/Rect [0.00 841.89 595.28 0.00]")) {
		t.Fatalf("expected page-sized print/view annotation rect in output")
	}
	if !bytes.Contains(data, []byte("/BBox [0 0 595.28 841.89]")) {
		t.Fatalf("expected page-sized print/view appearance bbox in output")
	}
	if bytes.Contains(data, []byte("/OCProperties")) {
		t.Fatalf("unexpected OCG properties in annotation-based print/view output")
	}
}

func TestRenderPDFWithPrintViewWithoutBackgroundUsesPrintOnlyAnnotation(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("# Title\n\nBody."),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: Config{
			PageSize:          "A4",
			Margin:            36,
			FontFamily:        "Courier",
			FontSize:          12,
			LineHeight:        1.4,
			UseOCGPrintView:   true,
			BackgroundEnabled: false,
			BackgroundRGB:     [3]int{255, 0, 0},
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	data := out.Bytes()
	if !bytes.Contains(data, []byte("/F 36")) {
		t.Fatalf("expected print-only annotation flags in output")
	}
	if bytes.Contains(data, []byte("1.000 0.000 0.000 rg 0 0 595.28 841.89 re f")) {
		t.Fatalf("did not expect no-background split view overlay to inject a backdrop fill")
	}
}

func TestRenderPDFWithPrintViewKeepsLinksAboveOverlay(t *testing.T) {
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader: strings.NewReader("[an example](http://example.com)\n"),
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
	annots := extractFirstPageAnnots(t, out.Bytes())
	if !bytes.Contains(annots, []byte("/Subtype /Link")) {
		t.Fatalf("expected link annotation in first-page /Annots array")
	}
	trimmed := bytes.TrimSpace(annots)
	if len(trimmed) == 0 {
		t.Fatalf("expected non-empty /Annots array")
	}
	if trimmed[0] == '<' {
		t.Fatalf("expected appearance annotation ref before inline link annotation, got %q", trimmed)
	}
	refRE := regexp.MustCompile(`^\d+\s+0\s+R\b`)
	if !refRE.Match(trimmed) {
		t.Fatalf("expected /Annots to start with an indirect appearance annotation ref, got %q", trimmed)
	}
}

func TestWrapAppearanceContent(t *testing.T) {
	got := string(wrapAppearanceContent(612, 792, [3]int{0, 0, 0}, []byte("BT\n/F1 12 Tf\nET\n")))
	if !strings.Contains(got, "q 0.000 0.000 0.000 rg 0 0 612.00 792.00 re f Q\n") {
		t.Fatalf("expected page underpaint, got %q", got)
	}
	if !strings.Contains(got, "q\nBT\n/F1 12 Tf\nET\nQ\n") {
		t.Fatalf("expected wrapped page content, got %q", got)
	}
}

func TestRenderPDFWithPrintViewIgnoresOpenLayerPane(t *testing.T) {
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
	if bytes.Contains(out.Bytes(), []byte("/PageMode /UseOC")) {
		t.Fatalf("did not expect OCG page mode in annotation-based print/view output")
	}
}

func TestRenderBoringAndPrintViewConflict(t *testing.T) {
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

func extractFirstPageAnnots(t *testing.T, data []byte) []byte {
	t.Helper()
	pageIdx := bytes.Index(data, []byte("/Type /Page"))
	if pageIdx == -1 {
		t.Fatalf("missing first page object")
	}
	annotsIdx := bytes.Index(data[pageIdx:], []byte("/Annots ["))
	if annotsIdx == -1 {
		t.Fatalf("missing /Annots array on first page")
	}
	annotsIdx += pageIdx
	start := annotsIdx + len("/Annots ")
	if start >= len(data) || data[start] != '[' {
		t.Fatalf("malformed /Annots array")
	}
	depth := 0
	for i := start; i < len(data); i++ {
		switch data[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return data[start+1 : i]
			}
		}
	}
	t.Fatalf("unterminated /Annots array")
	return nil
}
