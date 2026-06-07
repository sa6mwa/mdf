package pdf

import (
	"math"
	"strings"
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

func TestPDFStreamRendersMarkdownTable(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	err := stream.StartTable(mdf.TableStart{
		Alignments: []mdf.TableAlignment{mdf.TableAlignLeft, mdf.TableAlignRight},
		WireStyle:  mdf.DefaultTheme().Styles().TableWire,
	})
	if err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Header: true, Cells: []mdf.TableCell{{Text: "A"}, {Text: "B"}}}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "1"}, {Text: "2"}}}); err != nil {
		t.Fatalf("write row: %v", err)
	}
	if err := stream.EndTable(); err != nil {
		t.Fatalf("end table: %v", err)
	}
	if stream.tableActive {
		t.Fatalf("expected table to be inactive")
	}
	if stream.y <= cfg.Margin+cfg.FontSize {
		t.Fatalf("expected table to advance y position")
	}
}

func TestPDFStreamTableLinesCarryHeaderStyle(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferFull
	styles := mdf.DefaultTheme().Styles()
	stream := newPDFStream(pdf, cfg, styles, 80, 7, nil, pdfLayers{})
	if err := stream.StartTable(mdf.TableStart{
		Alignments:  []mdf.TableAlignment{mdf.TableAlignLeft},
		HeaderStyle: styles.TableHeader,
		WireStyle:   styles.TableWire,
	}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Header: true, Cells: []mdf.TableCell{{Text: "A"}}}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{
		Text: "bold",
		Tokens: []mdf.StreamToken{
			{Token: mdf.Token{Text: "bold", Style: styles.Strong, LinkURL: "https://example.com"}},
		},
	}}}); err != nil {
		t.Fatalf("write row: %v", err)
	}
	lines := stream.fullTableLines()
	var sawHeader bool
	var sawBody bool
	for _, line := range lines {
		text := pdfTableLineString(line)
		switch {
		case strings.Contains(text, "A"):
			sawHeader = true
			if !pdfTableLineHasSegment(line, "A", mdf.TableTextSegmentHeader, "") {
				t.Fatalf("header line missing header segment: %#v", line)
			}
		case strings.Contains(text, "bold"):
			sawBody = true
			if !pdfTableLineHasSegment(line, "bold", mdf.TableTextSegmentText, styles.Strong.Prefix) {
				t.Fatalf("body line missing strong segment: %#v", line)
			}
			if !pdfTableLineHasLinkedSegment(line, "bold", "https://example.com") {
				t.Fatalf("body line missing linked segment: %#v", line)
			}
		}
	}
	if !sawHeader || !sawBody {
		t.Fatalf("expected header and body lines, got %#v", lines)
	}
}

func TestPDFStreamTablePrefixReducesLayoutWidth(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 20, 7, nil, pdfLayers{})
	if err := stream.StartTable(mdf.TableStart{
		Alignments: []mdf.TableAlignment{mdf.TableAlignLeft},
		Prefix: []mdf.TablePrefixSegment{
			{Text: "> ", Style: mdf.DefaultTheme().Styles().Quote},
		},
	}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if got, want := stream.tableLayoutWidth(), 18; got != want {
		t.Fatalf("table layout width: got %d want %d", got, want)
	}
}

func TestPDFStreamTableIgnoresExtraBodyCellsPastDeclaredColumns(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	if err := stream.StartTable(mdf.TableStart{Alignments: []mdf.TableAlignment{mdf.TableAlignLeft, mdf.TableAlignLeft}}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Header: true, Cells: []mdf.TableCell{{Text: "A"}, {Text: "B"}}}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "1"}, {Text: "2"}, {Text: "EXTRA"}}}); err != nil {
		t.Fatalf("write row: %v", err)
	}
	lines := stream.fullTableLines()
	for _, line := range lines {
		text := pdfTableLineString(line)
		if strings.Contains(text, "EXTRA") {
			t.Fatalf("PDF table line rendered extra body cell: %#v", lines)
		}
	}
}

func TestPDFStreamHeaderOnlyTableKeepsDivider(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferFull
	cfg.TableWireMode = mdf.TableWireASCII
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	if err := stream.StartTable(mdf.TableStart{
		Alignments: []mdf.TableAlignment{mdf.TableAlignLeft, mdf.TableAlignLeft},
	}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Header: true, Cells: []mdf.TableCell{{Text: "A"}, {Text: "B"}}}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	out := strings.Join(pdfTableLineStrings(stream.fullTableLines()), "\n")
	if strings.Count(out, "+---+---+") != 3 {
		t.Fatalf("expected top/header/bottom borders for header-only table:\n%s", out)
	}
	if !strings.Contains(out, "| A | B |") {
		t.Fatalf("missing header row:\n%s", out)
	}
}

func TestPDFTableHeaderStyleKeepsHeaderPalette(t *testing.T) {
	styles := mdf.DefaultTheme().Styles()
	for name, inline := range map[string]mdf.Style{
		"emphasis": styles.Emphasis,
		"link":     styles.LinkText,
		"code":     styles.CodeInline,
	} {
		t.Run(name, func(t *testing.T) {
			style := pdfTableHeaderStyle(styles.TableHeader, inline)
			if !strings.HasPrefix(style.Prefix, styles.TableHeader.Prefix) {
				t.Fatalf("header inline style does not start with table header style: got %q want prefix %q", style.Prefix, styles.TableHeader.Prefix)
			}
			if strings.Contains(style.Prefix, "\x1b[34m") || strings.Contains(style.Prefix, "\x1b[35m") || strings.Contains(style.Prefix, "\x1b[1;34m") {
				t.Fatalf("header inline style kept body/link foreground color: %q", style.Prefix)
			}
		})
	}
}

func TestPDFStreamHeadingAfterTableDoesNotStackBeforeSpacing(t *testing.T) {
	cfg := DefaultConfig()
	styles := mdf.DefaultTheme().Styles()
	normal := headingSpacingProbeStream(cfg, styles)
	afterTable := headingSpacingProbeStream(cfg, styles)
	startY := afterTable.y
	normal.renderHeadingBlock()
	if err := afterTable.StartTable(mdf.TableStart{Alignments: []mdf.TableAlignment{mdf.TableAlignLeft}}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := afterTable.WriteTableRow(mdf.TableRow{Header: true, Cells: []mdf.TableCell{{Text: "A"}}}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := afterTable.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "1"}}}); err != nil {
		t.Fatalf("write row: %v", err)
	}
	if err := afterTable.EndTable(); err != nil {
		t.Fatalf("end table: %v", err)
	}
	afterTable.y = startY
	afterTable.headingPending = true
	afterTable.headingLevel = 2
	afterTable.headingStyle = styles.Heading[1]
	afterTable.headingMarker = "## "
	afterTable.headingBuf.WriteString("Next Section")
	afterTable.renderHeadingBlock()
	wantDelta := stylesToHeadingSize(cfg, 2) * headingSpaceBeforeMultiplier
	gotDelta := normal.y - afterTable.y
	if math.Abs(gotDelta-wantDelta) > 0.0001 {
		t.Fatalf("expected heading after table to suppress before spacing, delta=%v want %v normal y=%v after-table y=%v", gotDelta, wantDelta, normal.y, afterTable.y)
	}
}

func stylesToHeadingSize(cfg Config, level int) float64 {
	if level <= 0 || level > len(cfg.HeadingScale) {
		return cfg.FontSize
	}
	return cfg.FontSize * cfg.HeadingScale[level-1]
}

func headingSpacingProbeStream(cfg Config, styles mdf.Styles) *pdfStream {
	pdf := gofpdf.New("P", "pt", "A4", "")
	stream := newPDFStream(pdf, cfg, styles, 80, 7, nil, pdfLayers{})
	stream.y = cfg.Margin + 5*stream.baseLineHeight
	stream.headingPending = true
	stream.headingLevel = 2
	stream.headingStyle = styles.Heading[1]
	stream.headingMarker = "## "
	stream.headingBuf.WriteString("Next Section")
	return stream
}

func pdfTableLineHasSegment(line pdfTableLine, text string, kind mdf.TableTextSegmentKind, prefix string) bool {
	for _, segment := range line {
		if segment.Text == text && segment.Kind == kind && segment.Style.Prefix == prefix {
			return true
		}
	}
	return false
}

func pdfTableLineHasLinkedSegment(line pdfTableLine, text string, url string) bool {
	for _, segment := range line {
		if segment.Text == text && segment.LinkURL == url {
			return true
		}
	}
	return false
}

func indexPDFTableLine(lines []string, want string) int {
	for i, line := range lines {
		if line == want {
			return i
		}
	}
	return -1
}

func TestPDFStreamTableStartsFreshPageWhenItFits(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight
	if err := stream.StartTable(mdf.TableStart{Alignments: []mdf.TableAlignment{mdf.TableAlignLeft}}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Header: true, Cells: []mdf.TableCell{{Text: "A"}}}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "1"}}}); err != nil {
		t.Fatalf("write row: %v", err)
	}
	if err := stream.EndTable(); err != nil {
		t.Fatalf("end table: %v", err)
	}
	if stream.pageNum != 2 {
		t.Fatalf("expected table to start fresh page, got page %d", stream.pageNum)
	}
}

func TestPDFStreamFullBufferedTableStartsFreshPageWithoutDuplicateHeader(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferFull
	cfg.TableWireMode = mdf.TableWireASCII
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight
	if err := stream.StartTable(mdf.TableStart{Alignments: []mdf.TableAlignment{mdf.TableAlignLeft, mdf.TableAlignLeft}}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Header: true, Cells: []mdf.TableCell{{Text: "Outside Quote"}, {Text: "Value"}}}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "must not inherit quote prefix"}, {Text: "ok"}}}); err != nil {
		t.Fatalf("write row: %v", err)
	}
	lines := stream.fullTableLines()
	if err := stream.EndTable(); err != nil {
		t.Fatalf("end table: %v", err)
	}
	if stream.pageNum != 2 {
		t.Fatalf("expected table to start fresh page, got page %d", stream.pageNum)
	}
	if stream.tableContinuePending {
		t.Fatalf("fresh full-buffered table should not leave continuation pending")
	}
	wantY := cfg.Margin + cfg.FontSize + stream.baseLineHeight + float64(len(lines))*stream.baseLineHeight
	if math.Abs(stream.y-wantY) > 0.0001 {
		t.Fatalf("fresh full-buffered table lost top spacing or emitted duplicate continuation/header lines: got y=%v want %v line count=%d", stream.y, wantY, len(lines))
	}
}

func TestPDFStreamNestedListTableKeepsSpacingAfterFreshPageBreak(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferFull
	cfg.TableWireMode = mdf.TableWireASCII
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight
	if err := stream.StartTable(mdf.TableStart{
		Alignments: []mdf.TableAlignment{mdf.TableAlignLeft, mdf.TableAlignLeft},
		Prefix: []mdf.TablePrefixSegment{
			{Text: "    ", Style: mdf.DefaultTheme().Styles().Text},
		},
	}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Header: true, Cells: []mdf.TableCell{{Text: "Nested"}, {Text: "Value"}}}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "short"}, {Text: "value"}}}); err != nil {
		t.Fatalf("write row: %v", err)
	}
	lines := stream.fullTableLines()
	if err := stream.EndTable(); err != nil {
		t.Fatalf("end table: %v", err)
	}
	wantY := cfg.Margin + cfg.FontSize + stream.baseLineHeight + float64(len(lines))*stream.baseLineHeight
	if math.Abs(stream.y-wantY) > 0.0001 {
		t.Fatalf("nested list table moved to fresh page lost spacing before table: got y=%v want %v", stream.y, wantY)
	}
}

func TestPDFStreamHeadingAfterFreshPageTableKeepsBeforeSpacing(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferFull
	cfg.TableWireMode = mdf.TableWireASCII
	styles := mdf.DefaultTheme().Styles()
	stream := newPDFStream(pdf, cfg, styles, 80, 7, nil, pdfLayers{})
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight
	if err := stream.StartTable(mdf.TableStart{
		Alignments: []mdf.TableAlignment{mdf.TableAlignLeft, mdf.TableAlignLeft},
		Prefix: []mdf.TablePrefixSegment{
			{Text: "    ", Style: styles.Text},
		},
	}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Header: true, Cells: []mdf.TableCell{{Text: "Nested"}, {Text: "Value"}}}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "short"}, {Text: "value"}}}); err != nil {
		t.Fatalf("write row: %v", err)
	}
	if err := stream.EndTable(); err != nil {
		t.Fatalf("end table: %v", err)
	}
	afterTableY := stream.y
	stream.headingPending = true
	stream.headingLevel = 2
	stream.headingStyle = styles.Heading[1]
	stream.headingMarker = "## "
	stream.headingBuf.WriteString("Block Quote Containing List Table")
	stream.renderHeadingBlock()
	after := stylesToHeadingSize(cfg, 2) * headingSpaceAfterMultiplier
	wantY := afterTableY + stream.baseLineHeight + after
	if math.Abs(stream.y-wantY) > 0.0001 {
		t.Fatalf("heading after table moved to fresh page lost full blank-line spacing: got y=%v want %v afterTableY=%v", stream.y, wantY, afterTableY)
	}
}

func TestPDFStreamTableReemitsFrameAfterPageBreak(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	styles := mdf.DefaultTheme().Styles()
	stream := newPDFStream(pdf, cfg, styles, 80, 7, nil, pdfLayers{})
	rows := []mdf.TableRow{
		{Header: true, Cells: []mdf.TableCell{{Text: "A"}}},
		{Cells: []mdf.TableCell{{Text: "1"}}},
		{Cells: []mdf.TableCell{{Text: "2"}}},
	}
	plan := mdf.NewTableTextPlan(rows, []mdf.TableAlignment{mdf.TableAlignLeft}, stream.tableLayoutWidth(), cfg.TableWireMode)
	stream.tableActive = true
	stream.tableHeaderStyle = styles.TableHeader
	stream.tableWireStyle = styles.TableWire
	stream.setTableContinuation(plan, rows)
	stream.pageH = cfg.Margin*2 + stream.baseLineHeight*10
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight
	first := plan.StyledRowLines(rows[1])
	if len(first) != 1 {
		t.Fatalf("expected one first body line, got %d", len(first))
	}
	stream.emitTableLine(first[0])
	if !stream.tableContinuePending {
		t.Fatalf("expected table continuation to be pending after page break")
	}
	pageAfterBreak := stream.pageNum
	second := plan.StyledRowLines(rows[2])
	if len(second) != 1 {
		t.Fatalf("expected one second body line, got %d", len(second))
	}
	stream.emitTableLine(second[0])
	if stream.tableContinuePending {
		t.Fatalf("expected table continuation to be consumed before next row")
	}
	if stream.pageNum != pageAfterBreak {
		t.Fatalf("continuation should fit on the fresh page, page got %d want %d", stream.pageNum, pageAfterBreak)
	}
	wantY := cfg.Margin + cfg.FontSize + float64(len(stream.tableContinuation)+1)*stream.baseLineHeight
	if math.Abs(stream.y-wantY) > 0.0001 {
		t.Fatalf("table continuation was not emitted before next row: got y=%v want %v", stream.y, wantY)
	}
}

func TestPDFStreamFullBufferedTableClosesAndReopensFragments(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferFull
	cfg.TableWireMode = mdf.TableWireASCII
	styles := mdf.DefaultTheme().Styles()
	stream := newPDFStream(pdf, cfg, styles, 80, 7, nil, pdfLayers{})
	rows := []mdf.TableRow{
		{Header: true, Cells: []mdf.TableCell{{Text: "Category"}, {Text: "What it sells"}}},
		{Cells: []mdf.TableCell{{Text: "Resource consultancy"}, {Text: "Human capacity"}}},
		{Cells: []mdf.TableCell{{Text: "Specialist consultancy"}, {Text: "Deep expertise"}}},
	}
	if err := stream.StartTable(mdf.TableStart{Alignments: []mdf.TableAlignment{mdf.TableAlignLeft, mdf.TableAlignLeft}}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	for _, row := range rows {
		if err := stream.WriteTableRow(row); err != nil {
			t.Fatalf("write row: %v", err)
		}
	}
	plan := mdf.NewTableTextPlan(rows, []mdf.TableAlignment{mdf.TableAlignLeft, mdf.TableAlignLeft}, stream.tableLayoutWidth(), cfg.TableWireMode)
	headerLines := append([]string{}, plan.TopBorder()...)
	headerLines = append(headerLines, plan.RowLines(rows[0])...)
	headerLines = append(headerLines, plan.HeaderBorder()...)
	firstRow := plan.RowLines(rows[1])
	bottom := plan.BottomBorder()
	if len(firstRow) != 1 || len(bottom) != 1 || len(headerLines) != 3 {
		t.Fatalf("unexpected regression setup: header=%d first=%d bottom=%d", len(headerLines), len(firstRow), len(bottom))
	}
	stream.y = cfg.Margin + cfg.FontSize
	stream.pageH = stream.y + float64(len(headerLines)+len(firstRow)+len(bottom))*stream.baseLineHeight + cfg.Margin
	var observed []string
	stream.tableLineObserver = func(line string) {
		observed = append(observed, line)
	}
	if err := stream.EndTable(); err != nil {
		t.Fatalf("end table: %v", err)
	}
	firstIdx := indexPDFTableLine(observed, firstRow[0])
	if firstIdx < 0 {
		t.Fatalf("first row was not emitted:\n%s", strings.Join(observed, "\n"))
	}
	want := []string{
		bottom[0],
		headerLines[0],
		headerLines[1],
		headerLines[2],
	}
	if firstIdx+len(want) >= len(observed) {
		t.Fatalf("missing framed continuation after first row:\n%s", strings.Join(observed, "\n"))
	}
	for i, line := range want {
		if got := observed[firstIdx+1+i]; got != line {
			t.Fatalf("fragment line after first row %d = %q, want %q\n%s", i, got, line, strings.Join(observed, "\n"))
		}
	}
}

func TestPDFStreamRowBufferedTableClosesAndReopensFragments(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferRow
	cfg.TableWireMode = mdf.TableWireASCII
	styles := mdf.DefaultTheme().Styles()
	stream := newPDFStream(pdf, cfg, styles, 80, 7, nil, pdfLayers{})
	rows := []mdf.TableRow{
		{Header: true, Cells: []mdf.TableCell{{Text: "Category"}, {Text: "What it sells"}}},
		{Cells: []mdf.TableCell{{Text: "Resource consultancy"}, {Text: "Human capacity"}}},
		{Cells: []mdf.TableCell{{Text: "Specialist consultancy"}, {Text: "Deep expertise"}}},
	}
	if err := stream.StartTable(mdf.TableStart{Alignments: []mdf.TableAlignment{mdf.TableAlignLeft, mdf.TableAlignLeft}}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(rows[0]); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := stream.WriteTableRow(rows[1]); err != nil {
		t.Fatalf("write first body row: %v", err)
	}
	plan := stream.tablePlan
	continuation := pdfTableLineStrings(stream.tableContinuation)
	bottom := plan.BottomBorder()
	secondRow := plan.RowLines(rows[2])
	if len(continuation) != 3 || len(bottom) != 1 || len(secondRow) == 0 {
		t.Fatalf("unexpected regression setup: continuation=%d bottom=%d second=%d", len(continuation), len(bottom), len(secondRow))
	}
	stream.pageH = cfg.Margin*2 + stream.baseLineHeight*10
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight/2
	var observed []string
	stream.tableLineObserver = func(line string) {
		observed = append(observed, line)
	}
	if err := stream.WriteTableRow(rows[2]); err != nil {
		t.Fatalf("write second body row: %v", err)
	}
	want := append([]string{bottom[0]}, continuation...)
	want = append(want, secondRow...)
	if len(observed) < len(want) {
		t.Fatalf("missing framed row-buffer continuation, got:\n%s", strings.Join(observed, "\n"))
	}
	for i, line := range want {
		if got := observed[i]; got != line {
			t.Fatalf("row-buffer fragment line %d = %q, want %q\n%s", i, got, line, strings.Join(observed, "\n"))
		}
	}
}

func TestPDFStreamTableContinuationPendingAfterProactivePageBreak(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	styles := mdf.DefaultTheme().Styles()
	stream := newPDFStream(pdf, cfg, styles, 80, 7, nil, pdfLayers{})
	rows := []mdf.TableRow{
		{Header: true, Cells: []mdf.TableCell{{Text: "A"}}},
		{Cells: []mdf.TableCell{{Text: "1"}}},
	}
	plan := mdf.NewTableTextPlan(rows, []mdf.TableAlignment{mdf.TableAlignLeft}, stream.tableLayoutWidth(), cfg.TableWireMode)
	stream.tableActive = true
	stream.tablePlanReady = true
	stream.setTableContinuation(plan, rows)
	stream.pageH = cfg.Margin*2 + stream.baseLineHeight*10
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight/2
	pageBefore := stream.pageNum
	stream.ensureTableFits([]string{"| 2 |"})
	if stream.pageNum == pageBefore {
		t.Fatalf("expected proactive table page break")
	}
	if !stream.tableContinuePending {
		t.Fatalf("expected proactive table page break to schedule continuation")
	}
}

func TestPDFStreamTableFitIncludesContinuationHeight(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	rows := []mdf.TableRow{
		{Header: true, Cells: []mdf.TableCell{{Text: "A"}}},
		{Cells: []mdf.TableCell{{Text: "1"}}},
	}
	plan := mdf.NewTableTextPlan(rows, []mdf.TableAlignment{mdf.TableAlignLeft}, stream.tableLayoutWidth(), cfg.TableWireMode)
	stream.tableActive = true
	stream.tablePlanReady = true
	stream.setTableContinuation(plan, rows)
	stream.pageH = cfg.Margin*2 + stream.baseLineHeight*2
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight/2
	pageBefore := stream.pageNum
	stream.ensureTableFits([]string{"| 2 |"})
	if stream.pageNum != pageBefore {
		t.Fatalf("table row page-fit ignored continuation height")
	}
	if stream.tableContinuePending {
		t.Fatalf("did not expect continuation to be scheduled without a page break")
	}
}

func TestPDFStreamTableFitDoesNotBreakWhenCurrentChunkFits(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	rows := []mdf.TableRow{
		{Header: true, Cells: []mdf.TableCell{{Text: "A"}}},
		{Cells: []mdf.TableCell{{Text: "1"}}},
	}
	plan := mdf.NewTableTextPlan(rows, []mdf.TableAlignment{mdf.TableAlignLeft}, stream.tableLayoutWidth(), cfg.TableWireMode)
	stream.tableActive = true
	stream.tablePlanReady = true
	stream.setTableContinuation(plan, rows)
	stream.pageH = cfg.Margin*2 + stream.baseLineHeight*5
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight
	pageBefore := stream.pageNum
	stream.ensureTableFits([]string{"| 2 |"})
	if stream.pageNum != pageBefore {
		t.Fatalf("table row that fits current page should not be moved for continuation")
	}
	if stream.tableContinuePending {
		t.Fatalf("did not expect continuation without a page break")
	}
}

func TestPDFStreamBottomBorderFitDoesNotScheduleContinuation(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	rows := []mdf.TableRow{
		{Header: true, Cells: []mdf.TableCell{{Text: "A"}}},
		{Cells: []mdf.TableCell{{Text: "1"}}},
	}
	plan := mdf.NewTableTextPlan(rows, []mdf.TableAlignment{mdf.TableAlignLeft}, stream.tableLayoutWidth(), cfg.TableWireMode)
	stream.tableActive = true
	stream.tablePlanReady = true
	stream.setTableContinuation(plan, rows)
	stream.pageH = cfg.Margin*2 + stream.baseLineHeight*10
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight/2
	pageBefore := stream.pageNum
	stream.ensureTableFitsWithoutContinuation(plan.BottomBorder())
	if stream.pageNum == pageBefore {
		t.Fatalf("expected bottom border to trigger proactive page break")
	}
	if stream.tableContinuePending {
		t.Fatalf("bottom border only should not schedule repeated table header")
	}
}

func TestPDFStreamRowBufferedBottomBorderBreakEmitsContinuation(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferRow
	cfg.TableWireMode = mdf.TableWireASCII
	styles := mdf.DefaultTheme().Styles()
	stream := newPDFStream(pdf, cfg, styles, 80, 7, nil, pdfLayers{})
	rows := []mdf.TableRow{
		{Header: true, Cells: []mdf.TableCell{{Text: "A"}}},
		{Cells: []mdf.TableCell{{Text: "1"}}},
	}
	plan := mdf.NewTableTextPlan(rows, []mdf.TableAlignment{mdf.TableAlignLeft}, stream.tableLayoutWidth(), cfg.TableWireMode)
	stream.tableActive = true
	stream.tablePlanReady = true
	stream.tablePlan = plan
	stream.tableHeaderStyle = styles.TableHeader
	stream.tableWireStyle = styles.TableWire
	stream.setTableContinuation(plan, rows)
	stream.pageH = cfg.Margin*2 + stream.baseLineHeight*10
	stream.y = stream.pageH - cfg.Margin - stream.baseLineHeight/2
	var observed []string
	stream.tableLineObserver = func(line string) {
		observed = append(observed, line)
	}
	if err := stream.flushRowBufferedTable(); err != nil {
		t.Fatalf("flush row-buffered table: %v", err)
	}
	continuation := pdfTableLineStrings(stream.tableContinuation)
	bottom := plan.BottomBorder()
	if len(continuation) == 0 || len(bottom) != 1 {
		t.Fatalf("unexpected regression setup: continuation=%d bottom=%d", len(continuation), len(bottom))
	}
	if len(observed) != len(continuation)+len(bottom) {
		t.Fatalf("expected continuation plus bottom border, got:\n%s", strings.Join(observed, "\n"))
	}
	for i, want := range continuation {
		if observed[i] != want {
			t.Fatalf("continuation line %d = %q, want %q\n%s", i, observed[i], want, strings.Join(observed, "\n"))
		}
	}
	if got := observed[len(observed)-1]; got != bottom[0] {
		t.Fatalf("last row-buffered table close line = %q, want bottom border %q\n%s", got, bottom[0], strings.Join(observed, "\n"))
	}
}

func TestPDFStreamRowBufferedTableFlushesAfterFirstBodyRow(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferRow
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	startY := stream.y
	if err := stream.StartTable(mdf.TableStart{Alignments: []mdf.TableAlignment{mdf.TableAlignLeft}}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Header: true, Cells: []mdf.TableCell{{Text: "A"}}}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if stream.y != startY {
		t.Fatalf("expected header alone to remain buffered")
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "1"}}}); err != nil {
		t.Fatalf("write row: %v", err)
	}
	if stream.y <= startY {
		t.Fatalf("expected first body row to flush buffered table")
	}
}

func TestPDFStreamRowBufferedHeaderlessTableOmitsHeaderSeparator(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferRow
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	startY := stream.y
	if err := stream.StartTable(mdf.TableStart{Alignments: []mdf.TableAlignment{mdf.TableAlignLeft, mdf.TableAlignLeft}}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "A"}, {Text: "B"}}}); err != nil {
		t.Fatalf("write first row: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "1"}, {Text: "2"}}}); err != nil {
		t.Fatalf("write second row: %v", err)
	}
	wantY := startY + 3*stream.baseLineHeight
	if math.Abs(stream.y-wantY) > 0.0001 {
		t.Fatalf("headerless row-buffered table emitted unexpected separator: got y=%v want %v", stream.y, wantY)
	}
}

func TestPDFStreamRowBufferedHeaderlessTableFlushesAfterSecondRow(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	cfg := DefaultConfig()
	cfg.TableBufferMode = mdf.TableBufferRow
	stream := newPDFStream(pdf, cfg, mdf.DefaultTheme().Styles(), 80, 7, nil, pdfLayers{})
	startY := stream.y
	if err := stream.StartTable(mdf.TableStart{}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "A"}, {Text: "B"}}}); err != nil {
		t.Fatalf("write first row: %v", err)
	}
	if stream.y != startY {
		t.Fatalf("expected first headerless row to remain buffered")
	}
	if err := stream.WriteTableRow(mdf.TableRow{Cells: []mdf.TableCell{{Text: "1"}, {Text: "2"}}}); err != nil {
		t.Fatalf("write second row: %v", err)
	}
	if stream.y <= startY {
		t.Fatalf("expected second headerless row to flush row-buffered PDF output")
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
