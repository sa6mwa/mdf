package pdf

import (
	"math"
	"testing"

	"pkt.systems/mdf"
	"pkt.systems/mdf/pdf/gofpdf"
)

func TestLineLimitWithCornerImage(t *testing.T) {
	cfg := DefaultConfig()
	s := &pdfStream{
		width:             80,
		y:                 10,
		pageW:             200,
		cfg:               cfg,
		cornerImage:       &cornerImage{width: 30},
		cornerImageBottom: 20,
		pageNum:           1,
	}
	got := s.lineLimit()
	want := s.pageW - 2*s.cfg.Margin - s.cornerImage.width - s.cfg.CornerImagePadding
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("unexpected line limit: got %v want %v", got, want)
	}
	s.y = 25
	got = s.lineLimit()
	want = s.pageW - 2*s.cfg.Margin
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("unexpected line limit after image: got %v want %v", got, want)
	}
	s.y = 5
	s.cornerImage.width = 500
	if got := s.lineLimit(); got != 1 {
		t.Fatalf("unexpected minimum line limit: got %v want %v", got, 1)
	}
	s.pageNum = 2
	s.cornerImage.width = 30
	s.y = 10
	got = s.lineLimit()
	want = s.pageW - 2*s.cfg.Margin
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("unexpected line limit on later page: got %v want %v", got, want)
	}
}

func TestPageBreakResetsState(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	if stream.pageNum != 1 {
		t.Fatalf("expected initial page num 1, got %d", stream.pageNum)
	}
	if err := stream.WriteToken(mdf.StreamToken{Token: mdf.Token{Kind: tokenThematicBreak}}); err != nil {
		t.Fatalf("page break: %v", err)
	}
	if stream.pageNum != 2 {
		t.Fatalf("expected page num 2 after break, got %d", stream.pageNum)
	}
	if stream.lineWidth != 0 || !stream.atLineStart {
		t.Fatalf("expected reset line state after break")
	}
}

func TestHeadingLevelFromPrefixBuf(t *testing.T) {
	styles := mdf.DefaultTheme().Styles()
	style := styles.Heading[0]
	buf := []byte("# ")
	if got := headingLevelFromPrefixBuf(buf, style, styles); got != 1 {
		t.Fatalf("unexpected heading level: got %d want %d", got, 1)
	}
	buf = []byte("### ")
	if got := headingLevelFromPrefixBuf(buf, styles.Heading[2], styles); got != 3 {
		t.Fatalf("unexpected heading level: got %d want %d", got, 3)
	}
	if got := headingLevelFromPrefixBuf([]byte("# "), styles.Text, styles); got != 0 {
		t.Fatalf("expected no heading for text style")
	}
}

func TestEmitTextAdvancesByFontWidth(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	cfg.HeadingScale[0] = 2.0
	pdf := gofpdf.New("P", "pt", "A4", "")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, 7, nil, pdfLayers{})
	stream.headingLevel = 1
	style := theme.Styles().Heading[0]
	startX := stream.x
	stream.emitText("Test", style)
	expected := pdf.GetStringWidth("Test")
	got := stream.x - startX
	if expected == 0 {
		t.Fatalf("expected non-zero width")
	}
	if math.Abs(got-expected) > 0.0001 {
		t.Fatalf("unexpected advance: got %v want %v", got, expected)
	}
}

func TestHeadingPendingCollectsMarkerAndText(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	pdf := gofpdf.New("P", "pt", "A4", "")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, 7, nil, pdfLayers{})
	style := theme.Styles().Heading[2]

	stream.emitText("#", style)
	stream.emitText("#", style)
	stream.emitText("#", style)
	stream.emitText(" ", style)
	stream.emitText("Title", style)

	if !stream.headingPending {
		t.Fatalf("expected heading to be pending")
	}
	if stream.headingMarker != "### " {
		t.Fatalf("unexpected heading marker: %q", stream.headingMarker)
	}
	if stream.headingBuf.String() != "Title" {
		t.Fatalf("unexpected heading buffer: %q", stream.headingBuf.String())
	}
}

func TestHeadingInQuoteListDoesNotTriggerHeadingMode(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	pdf := gofpdf.New("P", "pt", "A4", "")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, 7, nil, pdfLayers{})
	stream.atLineStart = true
	stream.wrapIndent = "  "
	stream.listIndentActive = true
	stream.inQuoteLine = true
	stream.prefixBuf = append(stream.prefixBuf, []byte("> ")...)

	style := theme.Styles().Heading[1]
	stream.emitText("#", style)
	stream.emitText("#", style)
	stream.emitText(" ", style)
	stream.emitText("Title", style)

	if stream.headingLevel != 0 || stream.headingPending {
		t.Fatalf("expected heading mode suppressed inside quote/list")
	}
}

func TestHeadingWrapIndentPinsToMarker(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetFont(cfg.FontFamily, "", cfg.FontSize)
	charWidth := pdf.GetStringWidth("M")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, charWidth, nil, pdfLayers{})
	style := theme.Styles().Heading[2]

	stream.emitText("#", style)
	stream.emitText("#", style)
	stream.emitText("#", style)
	stream.emitText(" ", style)

	if stream.wrapIndent != "    " {
		t.Fatalf("expected heading wrap indent to match marker width, got %q", stream.wrapIndent)
	}
}

func TestHeadingBlankLineCollapsed(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	startY := stream.y
	stream.headingLevel = 2
	stream.emitBoundary(boundaryNewline)
	firstY := stream.y
	if firstY == startY {
		t.Fatalf("expected newline to advance after heading")
	}
	stream.emitBoundary(boundaryNewline)
	if stream.y != firstY {
		t.Fatalf("expected second newline to be suppressed after heading")
	}
}

func TestRenderHeadingBlockResetsState(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	pdf := gofpdf.New("P", "pt", "A4", "")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, 7, nil, pdfLayers{})
	stream.headingPending = true
	stream.headingLevel = 2
	stream.headingStyle = theme.Styles().Heading[1]
	stream.headingMarker = "## "
	stream.headingBuf.Reset()
	stream.headingBuf.WriteString("Heading")

	stream.renderHeadingBlock()

	if stream.headingPending {
		t.Fatalf("expected heading pending to be cleared")
	}
	if stream.headingLevel != 0 {
		t.Fatalf("expected heading level reset")
	}
	if stream.headingMarker != "" {
		t.Fatalf("expected heading marker reset")
	}
	if stream.headingBuf.Len() != 0 {
		t.Fatalf("expected heading buffer reset")
	}
}

func TestHeadingWrapIndentUsesMarkerWidth(t *testing.T) {
	lines := wrapHeadingByCols("2. Clarify the Purpose of the Outcome", 20, 4, 4)
	if len(lines) < 2 {
		t.Fatalf("expected heading to wrap")
	}
	if textColumns(lines[1]) > 16 {
		t.Fatalf("expected wrapped line within indent-adjusted limit")
	}
}

func TestClassifyBoundaryKeepsPeriodCommaTogether(t *testing.T) {
	if got := classifyBoundary('.', ','); got != boundaryNone {
		t.Fatalf("expected period before comma to be non-boundary")
	}
	if got := classifyBoundary('.', ' '); got != boundaryNone {
		t.Fatalf("expected period before space to be non-boundary")
	}
	if got := classifyBoundary('.', 'G'); got != boundaryPunct {
		t.Fatalf("expected period before uppercase to be punctuation boundary")
	}
}

func TestWrapHeadingByColsRespectsIndent(t *testing.T) {
	lines := wrapHeadingByCols("2. Clarify the Purpose of the Outcome", 20, 4, 4)
	if len(lines) < 2 {
		t.Fatalf("expected heading to wrap")
	}
	if textColumns(lines[0]) > 20 {
		t.Fatalf("expected first line within limit")
	}
	if textColumns(lines[1]) > 16 {
		t.Fatalf("expected wrapped line within indent-adjusted limit")
	}
}

func TestHeadingPageBreakKeepsRoomForBody(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetFont(cfg.FontFamily, "", cfg.FontSize)
	charWidth := pdf.GetStringWidth("M")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, charWidth, nil, pdfLayers{})
	if stream.pageNum != 1 {
		t.Fatalf("expected initial page num 1")
	}

	stream.headingPending = true
	stream.headingLevel = 2
	stream.headingStyle = theme.Styles().Heading[1]
	stream.headingMarker = "## "
	stream.headingBuf.Reset()
	stream.headingBuf.WriteString("Heading that would fit but leave no body line")
	stream.wrapIndent = "   "
	stream.wrapIndentUseWidth = true
	stream.wrapIndentWidth = charWidth * 3

	stream.y = stream.pageH - cfg.Margin - cfg.FontSize
	stream.renderHeadingBlock()
	if stream.pageNum != 2 {
		t.Fatalf("expected page break before heading to keep room for body")
	}
}

func TestListWrapIndentUsesMeasuredPrefixWidth(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetFont(cfg.FontFamily, "", cfg.FontSize)
	charWidth := pdf.GetStringWidth("M")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, charWidth, nil, pdfLayers{})

	stream.emitText("  ", theme.Styles().Text)
	stream.emitText("-", theme.Styles().ListMarker)
	stream.emitText(" ", theme.Styles().Text)
	stream.emitText("If", theme.Styles().Text)

	if stream.wrapIndent == "" {
		t.Fatalf("expected wrap indent to be set for list prefix")
	}
	if !stream.wrapIndentUseWidth {
		t.Fatalf("expected wrap indent to use measured width")
	}
	if stream.wrapIndentWidth <= 0 {
		t.Fatalf("expected wrap indent width to be > 0")
	}
	want := charWidth * float64(textColumns(stream.wrapIndent))
	if math.Abs(stream.wrapIndentWidth-want) > 0.0001 {
		t.Fatalf("expected wrap indent width to match column width")
	}
}

func TestTaskListWrapIndentUsesMeasuredPrefixWidth(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetFont(cfg.FontFamily, "", cfg.FontSize)
	charWidth := pdf.GetStringWidth("M")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, charWidth, nil, pdfLayers{})

	stream.emitText("  ", theme.Styles().Text)
	stream.emitText("-", theme.Styles().ListMarker)
	stream.emitText(" ", theme.Styles().Text)
	stream.emitText("[", theme.Styles().Text)
	stream.emitText(" ", theme.Styles().Text)
	stream.emitText("]", theme.Styles().Text)
	stream.emitText(" ", theme.Styles().Text)
	stream.emitText("Item", theme.Styles().Text)

	if stream.wrapIndent == "" {
		t.Fatalf("expected wrap indent to be set for task list prefix")
	}
	if !stream.wrapIndentUseWidth {
		t.Fatalf("expected wrap indent to use measured width")
	}
	if stream.wrapIndentWidth <= 0 {
		t.Fatalf("expected wrap indent width to be > 0")
	}
	want := charWidth * float64(textColumns(stream.wrapIndent))
	if math.Abs(stream.wrapIndentWidth-want) > 0.0001 {
		t.Fatalf("expected wrap indent width to match column width")
	}
}

func TestQuoteWrapIndentKeepsPrefixBytes(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetFont(cfg.FontFamily, "", cfg.FontSize)
	charWidth := pdf.GetStringWidth("M")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, charWidth, nil, pdfLayers{})

	stream.emitText(">", theme.Styles().Quote)
	stream.emitText(" ", theme.Styles().Text)
	stream.emitText("Quote", theme.Styles().Text)

	if stream.wrapIndent != "> " {
		t.Fatalf("expected wrap indent to preserve quote prefix, got %q", stream.wrapIndent)
	}
}

func TestSetWrapIndentStripsANSIPrefix(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetFont(cfg.FontFamily, "", cfg.FontSize)
	charWidth := pdf.GetStringWidth("M")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, charWidth, nil, pdfLayers{})

	indent := theme.Styles().Quote.Prefix + ">" + "\x1b[0m" + " "
	stream.SetWrapIndent(indent)

	if stream.wrapIndentPrefix != "> " {
		t.Fatalf("expected stripped wrap indent prefix, got %q", stream.wrapIndentPrefix)
	}
}

func TestHeadingInQuoteUsesBodyFontSize(t *testing.T) {
	theme := mdf.DefaultTheme()
	cfg := DefaultConfig()
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	cfg.HeadingFont = "SomeHeadingFont"
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetFont(cfg.FontFamily, "", cfg.FontSize)
	charWidth := pdf.GetStringWidth("M")
	stream := newPDFStream(pdf, cfg, theme.Styles(), 80, charWidth, nil, pdfLayers{})
	stream.inQuoteLine = true

	style := stream.styleForPrefix(theme.Styles().Heading[1].Prefix, 2)
	if style.fontFamily != cfg.FontFamily {
		t.Fatalf("expected heading in quote to use body font family, got %q", style.fontFamily)
	}
	if style.size != cfg.FontSize {
		t.Fatalf("expected heading in quote to use body font size, got %v", style.size)
	}
}
