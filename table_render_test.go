package mdf

import (
	"bytes"
	"strings"
	"testing"

	"github.com/muesli/reflow/ansi"
)

func TestRenderTableLineWire(t *testing.T) {
	src := "| Left | Right | Center |\n| :--- | ---: | :---: |\n| a | b | c |\n"
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireLine)))
	assertContains(t, out, "┌")
	assertContains(t, out, "│ Left │ Right │ Center │")
	assertContains(t, out, "│ a    │     b │   c    │")
	assertContains(t, out, "└")
}

func TestNormalizeRenderConfigDefaultsTableBufferToFull(t *testing.T) {
	var cfg renderConfig
	normalizeRenderConfig(&cfg)
	if cfg.tableBufferMode != TableBufferFull {
		t.Fatalf("default table buffer mode = %v, want %v", cfg.tableBufferMode, TableBufferFull)
	}
}

func TestRenderTableASCIIWire(t *testing.T) {
	src := "| A | B |\n| --- | --- |\n| 1 | 2 |\n"
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireASCII)))
	assertContains(t, out, "+---+---+")
	assertContains(t, out, "| A | B |")
}

func TestRenderTableAtEOFDropsConsumedSyntaxLine(t *testing.T) {
	src := "| A | B |\n| --- | --- |\n| 1 | 2 |"
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireLine)))
	assertContains(t, out, "│ A │ B │")
	assertContains(t, out, "│ 1 │ 2 │")
	if strings.Contains(out, "| --- | --- |") || strings.Contains(out, "| 1 | 2 |") {
		t.Fatalf("table ending at EOF rendered raw syntax alongside table:\n%s", out)
	}
}

func TestRenderHeaderlessTableAtEOFDropsConsumedSyntaxLine(t *testing.T) {
	src := "| A | B |\n| 1 | 2 |\n| 3 | 4 |"
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireLine)))
	assertContains(t, out, "│ A │ B │")
	assertContains(t, out, "│ 3 │ 4 │")
	if strings.Contains(out, "| 3 | 4 |") {
		t.Fatalf("headerless table ending at EOF rendered raw final row:\n%s", out)
	}
}

func TestStreamRendererResetNormalizesTableDefaults(t *testing.T) {
	src := "| A | B |\n| --- | --- |\n| 1 | 2 |\n"
	var out bytes.Buffer
	var stream StreamRenderer
	stream.Reset(&out, 80)
	if err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: &stream,
		Theme:  DefaultTheme(),
	}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	plain := stripANSI(out.String())
	assertContains(t, plain, "┌")
	assertContains(t, plain, "│ A │ B │")
	assertContains(t, plain, "│ 1 │ 2 │")
}

func TestRenderNoEdgePipeTable(t *testing.T) {
	src := "A | B\n--- | ---\n1 | 2\n"
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableBufferMode(TableBufferFull), WithTableWireMode(TableWireASCII)))
	assertContains(t, out, "+---+---+")
	assertContains(t, out, "| A | B |")
	assertContains(t, out, "| 1 | 2 |")
	if strings.Contains(out, "--- | ---") {
		t.Fatalf("no-edge table delimiter rendered as plain text:\n%s", out)
	}
}

func TestRenderNoEdgePipeTableWithMultiCharacterHeader(t *testing.T) {
	for _, tc := range []struct {
		src      string
		header   string
		body     string
		rawProbe string
	}{
		{src: "Name | Value\n--- | ---\nalpha | beta\n", header: "| Name  | Value |", body: "| alpha | beta  |", rawProbe: "Name | Value"},
		{src: "ID | Name\n--- | ---\n1 | Alice\n", header: "| ID | Name  |", body: "| 1  | Alice |", rawProbe: "ID | Name"},
		{src: "Foo | Bar\n--- | ---\none | two\n", header: "| Foo | Bar |", body: "| one | two |", rawProbe: "Foo | Bar"},
		{src: "Fruit | Count\n--- | ---:\napple | 10\n", header: "| Fruit | Count |", body: "| apple |    10 |", rawProbe: "Fruit | Count"},
		{src: "status | value\n--- | ---\nok | yes\n", header: "| status | value |", body: "| ok     | yes   |", rawProbe: "status | value"},
		{src: "current status | value\n--- | ---\nok | yes\n", header: "| current status | value |", body: "| ok             | yes   |", rawProbe: "current status | value"},
		{src: "longer header cell | value\n--- | ---\nok | yes\n", header: "| longer header cell | value |", body: "| ok                 | yes   |", rawProbe: "longer header cell | value"},
	} {
		t.Run(tc.rawProbe, func(t *testing.T) {
			out := stripANSI(renderStreamWithOptions(t, []byte(tc.src), 80, WithOSC8(false), WithTableBufferMode(TableBufferFull), WithTableWireMode(TableWireASCII)))
			assertContains(t, out, tc.header)
			assertContains(t, out, tc.body)
			if strings.Contains(out, "--- | ---") {
				t.Fatalf("no-edge multi-character header table rendered as paragraph:\n%s", out)
			}
		})
	}
}

func TestRenderHeaderOnlyTableKeepsDivider(t *testing.T) {
	src := "| A | B |\n| --- | --- |\n"
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireASCII)))
	if strings.Count(out, "+---+---+") != 3 {
		t.Fatalf("expected top/header/bottom borders for header-only table:\n%s", out)
	}
	assertContains(t, out, "| A | B |")
}

func TestRenderTableSpaceWire(t *testing.T) {
	src := "| A | B |\n| --- | --- |\n| 1 | 2 |\n"
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireSpace)))
	if strings.ContainsAny(out, "+|┌│") {
		t.Fatalf("space wire output contains visible borders: %q", out)
	}
	assertContains(t, out, "A  B")
	assertContains(t, out, "1  2")
}

func TestRenderTableShrinksBelowThreeColumnsForNarrowWidth(t *testing.T) {
	rows := []TableRow{
		{Cells: []TableCell{{Text: "abcdef"}}},
	}
	lines := RenderTableTextLines(rows, nil, 5, TableWireASCII)
	for _, line := range lines {
		if got := ansi.PrintableRuneWidth(line); got > 5 {
			t.Fatalf("table line exceeded requested width: got %d line %q\n%s", got, line, strings.Join(lines, "\n"))
		}
	}
}

func TestRenderTableDoesNotShrinkBelowFullWidthGlyph(t *testing.T) {
	lines := RenderTableTextLines([]TableRow{
		{Cells: []TableCell{{Text: "東"}}},
	}, nil, 5, TableWireASCII)
	width := ansi.PrintableRuneWidth(lines[0])
	for _, line := range lines {
		if got := ansi.PrintableRuneWidth(line); got != width {
			t.Fatalf("table line width changed: got %d want %d line %q\n%s", got, width, line, strings.Join(lines, "\n"))
		}
	}
	if got, want := lines[0], "+----+"; got != want {
		t.Fatalf("full-width glyph table border: got %q want %q\n%s", got, want, strings.Join(lines, "\n"))
	}
}

func TestRenderTablePreservesCellTokenEdgeSpaces(t *testing.T) {
	lines := RenderTableTextLines([]TableRow{
		{Cells: []TableCell{{
			Text: " go",
			Tokens: []StreamToken{
				{Token: Token{Text: " go", Style: DefaultTheme().Styles().CodeInline, Kind: tokenCode}},
			},
		}}},
		{Cells: []TableCell{{
			Text: "go ",
			Tokens: []StreamToken{
				{Token: Token{Text: "go ", Style: DefaultTheme().Styles().CodeInline, Kind: tokenCode}},
			},
		}}},
	}, nil, 80, TableWireASCII)
	out := strings.Join(lines, "\n")
	assertContains(t, out, "|  go |")
	assertContains(t, out, "| go  |")
}

func TestRenderTableRowBufferMode(t *testing.T) {
	src := "| A | B |\n| --- | --- |\n| 1 | 2 |\n| longer value | 3 |\n"
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableBufferMode(TableBufferRow), WithTableWireMode(TableWireASCII)))
	assertContains(t, out, "+---+---+")
	assertContains(t, out, "| A | B |")
	assertContains(t, out, "| 1 | 2 |")
	assertContains(t, out, "| l | 3 |")
	if !strings.HasSuffix(strings.TrimSpace(out), "+---+---+") {
		t.Fatalf("expected row-buffered table to end with a bottom border, got:\n%s", out)
	}
}

func TestRenderHeaderlessRowBufferedTableFlushesAfterSecondRow(t *testing.T) {
	var out bytes.Buffer
	stream := &StreamRenderer{}
	stream.resetWithConfig(&out, 80, renderConfig{tableBufferMode: TableBufferRow, tableWireMode: TableWireASCII})
	if err := stream.StartTable(TableStart{}); err != nil {
		t.Fatalf("start table: %v", err)
	}
	if err := stream.WriteTableRow(TableRow{Cells: []TableCell{{Text: "A"}, {Text: "B"}}}); err != nil {
		t.Fatalf("write first row: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected first headerless row to remain buffered, got %q", out.String())
	}
	if err := stream.WriteTableRow(TableRow{Cells: []TableCell{{Text: "1"}, {Text: "2"}}}); err != nil {
		t.Fatalf("write second row: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected second headerless row to flush row-buffered output")
	}
}

func TestRenderHeaderlessRowBufferedTableOmitsHeaderSeparator(t *testing.T) {
	src := "| A | B |\n| 1 | 2 |\n| 3 | 4 |\n"
	full := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableBufferMode(TableBufferFull), WithTableWireMode(TableWireASCII)))
	row := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableBufferMode(TableBufferRow), WithTableWireMode(TableWireASCII)))
	if full != row {
		t.Fatalf("row-buffered headerless table differs from full-buffered output:\nfull:\n%s\nrow:\n%s", full, row)
	}
	if strings.Count(row, "+---+---+") != 2 {
		t.Fatalf("headerless row-buffered table has extra separator:\n%s", row)
	}
}

func TestRenderHeaderlessRowBufferedTableKeepsStableLayoutForLaterWiderRows(t *testing.T) {
	src := "| A | B |\n| 1 | 2 |\n| 3 | 4 | 5 |\n"
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableBufferMode(TableBufferRow), WithTableWireMode(TableWireASCII)))
	if !strings.Contains(out, "5") {
		t.Fatalf("row-buffered headerless table dropped a later extra cell:\n%s", out)
	}
	if strings.Contains(out, "| 3 | 4 | 5 |") {
		t.Fatalf("row-buffered headerless table expanded a later row beyond its frame:\n%s", out)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(line, "+") && line != "+---+---+" {
			t.Fatalf("row-buffered headerless border changed width: %q\n%s", line, out)
		}
		if strings.Contains(line, "|") && ansi.PrintableRuneWidth(line) != len("+---+---+") {
			t.Fatalf("row-buffered headerless row changed width: %q\n%s", line, out)
		}
	}
}

func TestRenderClearsWrapIndentAfterContainedTable(t *testing.T) {
	src := strings.Join([]string{
		"- item",
		"",
		"  | A | B |",
		"  | --- | --- |",
		"  | 1 | 2 |",
		"",
		"This paragraph should wrap after the table without inheriting the list table indentation.",
		"",
	}, "\n")
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 34, WithOSC8(false), WithTableBufferMode(TableBufferFull), WithTableWireMode(TableWireASCII)))
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "This paragraph") {
			if i+1 >= len(lines) {
				t.Fatalf("expected wrapped continuation line after paragraph:\n%s", out)
			}
			if strings.HasPrefix(lines[i+1], "  ") {
				t.Fatalf("paragraph after contained table inherited table/list wrap indent:\n%s", out)
			}
			return
		}
	}
	t.Fatalf("missing paragraph after contained table:\n%s", out)
}

func TestTableTextPlanMergesLateHeaderlessCellsIntoStableRowBufferedLayout(t *testing.T) {
	plan := NewTableTextPlan([]TableRow{
		{Cells: []TableCell{{Text: "A"}, {Text: "B"}}},
		{Cells: []TableCell{{Text: "1"}, {Text: "2"}}},
	}, nil, 80, TableWireASCII)
	lines := plan.RowLines(TableRow{Cells: []TableCell{{Text: "3"}, {Text: "4"}, {Text: "5"}}})
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "5") {
		t.Fatalf("fixed row-buffered table plan dropped late extra cell:\n%s", joined)
	}
	for _, line := range lines {
		if ansi.PrintableRuneWidth(line) != len("+---+---+") {
			t.Fatalf("fixed row-buffered table plan changed row width: %q\n%s", line, joined)
		}
	}
}

func TestRenderTableIgnoresExtraBodyCellsPastDeclaredColumns(t *testing.T) {
	src := "| A | B |\n| --- | --- |\n| 1 | 2 | EXTRA |\n"
	for _, mode := range []TableBufferMode{TableBufferFull, TableBufferRow} {
		out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableBufferMode(mode), WithTableWireMode(TableWireASCII)))
		if strings.Contains(out, "EXTRA") {
			t.Fatalf("table rendered extra body cell in mode %v:\n%s", mode, out)
		}
		if strings.Contains(out, "+---+---+-------+") {
			t.Fatalf("table layout grew extra column in mode %v:\n%s", mode, out)
		}
		assertContains(t, out, "| A | B |")
		assertContains(t, out, "| 1 | 2 |")
	}
}

func TestRenderTableHeaderUsesPaletteColorAndBodyUsesTextStyle(t *testing.T) {
	src := "| A | B |\n| --- | --- |\n| 1 | 2 |\n"
	out := renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireLine))
	headerStyle := DefaultTheme().Styles().TableHeader.Prefix
	if headerStyle == "" {
		t.Fatalf("expected default table header style")
	}
	if !strings.Contains(out, headerStyle+"A") {
		t.Fatalf("expected header cell to use table header style %q, got:\n%q", headerStyle, out)
	}
	if strings.Contains(out, headerStyle+"1") {
		t.Fatalf("body cell unexpectedly used table header style:\n%q", out)
	}
	if strings.Contains(out, DefaultTheme().Styles().Strong.Prefix+"A") {
		t.Fatalf("header cell used strong style instead of table header style:\n%q", out)
	}
}

func TestRenderInlineTableHeaderKeepsHeaderPalette(t *testing.T) {
	src := "| *Head* | [Docs](https://example.com) |\n| --- | --- |\n| value | link |\n"
	out := renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableBufferMode(TableBufferFull), WithTableWireMode(TableWireASCII))
	styles := DefaultTheme().Styles()
	for text, style := range map[string]Style{
		"Head": combineTableHeaderStyle(styles.TableHeader, styles.Emphasis),
		"Docs": combineTableHeaderStyle(styles.TableHeader, styles.LinkText),
	} {
		if !strings.Contains(out, style.Prefix+text) {
			t.Fatalf("header text %q did not use header-preserving inline style %q:\n%q", text, style.Prefix, out)
		}
	}
	if strings.Contains(out, styles.Emphasis.Prefix+"Head") {
		t.Fatalf("emphasized header used body emphasis color:\n%q", out)
	}
	if strings.Contains(out, styles.LinkText.Prefix+"Docs") {
		t.Fatalf("linked header used body link color:\n%q", out)
	}
}

func TestRenderTableFollowedByParagraphUsesSingleBlankSeparator(t *testing.T) {
	src := "| A | B |\n| --- | --- |\n| 1 | 2 |\n\nAfter table.\n"
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireLine)))
	if strings.Contains(out, "┘\n\n\nAfter table.") {
		t.Fatalf("table followed by paragraph has extra blank line:\n%q", out)
	}
	if !strings.Contains(out, "┘\n\nAfter table.") {
		t.Fatalf("expected single blank separator after table, got:\n%q", out)
	}
}

func TestRenderNonTablePipeLineParsesInlineMarkdown(t *testing.T) {
	src := "A | *em* [site](https://example.com)\nnot a table\n"
	out := renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireLine))
	plain := stripANSI(out)
	if strings.HasPrefix(plain, " ") {
		t.Fatalf("non-table pipe line rendered with leading space:\n%q", plain)
	}
	if strings.Contains(plain, "*em*") || strings.Contains(plain, "[site](https://example.com)") {
		t.Fatalf("non-table pipe line rendered raw inline markdown:\n%s", plain)
	}
	for _, want := range []string{"A | em site (https://example.com)", "not a table"} {
		assertContains(t, plain, want)
	}
	styles := DefaultTheme().Styles()
	for _, want := range []string{
		styles.Emphasis.Prefix + "em",
		styles.LinkText.Prefix + "site",
		styles.LinkURL.Prefix + "https://example.com",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("non-table pipe line missing styled segment %q:\n%q", want, out)
		}
	}
}

func TestRenderRejectedQuoteTableCandidateContinuesParagraph(t *testing.T) {
	src := "> first line\n> a | b\n> not a table\n"
	plain := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireLine)))
	assertContains(t, plain, "> first line a | b not a table")
	if strings.Contains(plain, "first line > a | b") {
		t.Fatalf("rejected quote table candidate injected quote prefix mid-paragraph:\n%s", plain)
	}
}

func TestRenderRejectedTableCandidatePreservesHardBreakSpaces(t *testing.T) {
	src := "a | b  \nnot a table\n"
	plain := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireLine)))
	assertContains(t, plain, "a | b  \nnot a table")
	if strings.Contains(plain, "a | b not a table") {
		t.Fatalf("rejected table candidate lost hard break spaces:\n%s", plain)
	}
}

func TestRenderTableCellsParseInlineMarkdown(t *testing.T) {
	src := "| Kind | Value |\n| --- | --- |\n| em | *italic* |\n| strong | **bold** |\n| code | `value` |\n| link | [site](https://example.com) |\n"
	out := renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableBufferMode(TableBufferFull), WithTableWireMode(TableWireLine))
	plain := stripANSI(out)
	for _, raw := range []string{"*italic*", "**bold**", "`value`", "[site](https://example.com)"} {
		if strings.Contains(plain, raw) {
			t.Fatalf("table cell rendered raw markdown %q:\n%s", raw, plain)
		}
	}
	for _, want := range []string{"italic", "bold", "value", "site (https://example.com)"} {
		assertContains(t, plain, want)
	}
	styles := DefaultTheme().Styles()
	for _, want := range []string{
		styles.Emphasis.Prefix + "italic",
		styles.Strong.Prefix + "bold",
		styles.CodeInline.Prefix + "value",
		styles.LinkText.Prefix + "site",
		styles.LinkURL.Prefix + "https://example.com",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("table cell missing styled inline segment %q:\n%q", want, out)
		}
	}
}

func TestRenderTableCellLinkUsesOSC8(t *testing.T) {
	src := "| Kind | Value |\n| --- | --- |\n| link | [site](https://example.com) |\n"
	out := renderStreamWithOptions(t, []byte(src), 80, WithOSC8(true), WithTableWireMode(TableWireLine))
	linkStart := osc8Start + "https://example.com" + "\x1b\\"
	start := strings.Index(out, linkStart)
	if start == -1 {
		t.Fatalf("table cell link missing osc8 start: %q", out)
	}
	end := strings.Index(out[start+len(linkStart):], osc8End)
	if end == -1 {
		t.Fatalf("table cell link missing osc8 end: %q", out)
	}
	linkSpan := out[start+len(linkStart) : start+len(linkStart)+end]
	if !strings.Contains(linkSpan, "site") {
		t.Fatalf("table cell osc8 span does not contain link text: %q", linkSpan)
	}
	plain := stripANSI(out)
	if strings.Contains(plain, "[site](https://example.com)") {
		t.Fatalf("table cell rendered raw markdown link:\n%s", plain)
	}
	if strings.Contains(plain, "https://example.com") {
		t.Fatalf("osc8 table cell link rendered fallback url text:\n%s", plain)
	}
}

func TestRenderTableStyledSpacesKeepSpanMetadata(t *testing.T) {
	styles := DefaultTheme().Styles()
	rows := []TableRow{
		{
			Cells: []TableCell{{
				Text: "italic phrase",
				Tokens: []StreamToken{
					{Token: Token{Text: "italic phrase", Style: styles.Emphasis}},
				},
			}},
		},
		{
			Cells: []TableCell{{
				Text: "long label",
				Tokens: []StreamToken{
					{Token: Token{Kind: tokenLinkStart, LinkURL: "https://example.com"}},
					{Token: Token{Text: "long label", Style: styles.LinkText, LinkURL: "https://example.com"}},
					{Token: Token{Kind: tokenLinkEnd}},
				},
			}},
		},
	}
	lines := RenderTableStyledLines(rows, []TableAlignment{TableAlignLeft}, 80, TableWireSpace)
	assertTableSegment(t, lines, " ", styles.Emphasis.Prefix, "")
	assertTableSegment(t, lines, " ", styles.LinkText.Prefix, "https://example.com")
}

func TestRenderWrappedTableCellLinksBalanceOSC8PerLine(t *testing.T) {
	src := strings.Join([]string{
		"| Variant | Example | Long Example |",
		"| --- | --- | --- |",
		"| inline link | [site](https://example.com) | [long descriptive link label that should wrap](https://example.com/long/path) |",
		"| link in sentence | open [docs](https://example.com/docs) now | long sentence before [documentation link](https://example.com/docs/reference/table-renderer) and after |",
		"",
	}, "\n")
	out := renderStreamWithOptions(t, []byte(src), 76, WithOSC8(true), WithTableWireMode(TableWireLine))
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, osc8Start) || strings.Contains(line, osc8End) {
			starts := strings.Count(line, osc8Start+"https://")
			ends := strings.Count(line, osc8End)
			if starts != ends {
				t.Fatalf("unbalanced osc8 table line: starts=%d ends=%d line=%q\nfull output:\n%q", starts, ends, line, out)
			}
		}
	}
	plain := stripANSI(out)
	if strings.Contains(plain, "https://example.com/long/path") {
		t.Fatalf("wrapped osc8 table link rendered fallback url text:\n%s", plain)
	}
	assertContains(t, plain, "long descriptive link label that")
	assertContains(t, plain, "should wrap")
}

func TestRenderTableCellsDecodeEntities(t *testing.T) {
	src := strings.Join([]string{
		"| Entity | Example |",
		"| --- | --- |",
		"| amp | AT&amp;T |",
		"| angle | &lt;tag&gt; |",
		"| quoted | &quot;value&quot; and O&apos;Neil |",
		"| nbsp | 10&nbsp;000 |",
		"",
	}, "\n")
	out := renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableBufferMode(TableBufferFull), WithTableWireMode(TableWireLine))
	plain := stripANSI(out)
	visible := strings.ReplaceAll(plain, "\u00A0", " ")
	for _, raw := range []string{"AT&amp;T", "&lt;tag&gt;", "&quot;value&quot;", "O&apos;Neil", "10&nbsp;000"} {
		if strings.Contains(plain, raw) {
			t.Fatalf("table cell rendered raw entity %q:\n%s", raw, plain)
		}
	}
	for _, want := range []string{"AT&T", "<tag>", "\"value\" and O'Neil", "10 000"} {
		assertContains(t, visible, want)
	}
}

func TestRenderTableKeepsNBSPCellUnbroken(t *testing.T) {
	rows := []TableRow{
		{Header: true, Cells: []TableCell{{Text: "Label"}, {Text: "Value"}}},
		{Cells: []TableCell{{Text: "nbsp"}, {Text: "10\u00A0000"}}},
	}
	lines := RenderTableTextLines(rows, []TableAlignment{TableAlignLeft, TableAlignLeft}, 16, TableWireASCII)
	var sawTogether bool
	for _, line := range lines {
		visible := strings.ReplaceAll(line, "\u00A0", " ")
		if strings.Contains(visible, "10 000") {
			sawTogether = true
		}
		if strings.Contains(visible, "10") && !strings.Contains(visible, "10 000") {
			t.Fatalf("NBSP value split before suffix:\n%s", strings.Join(lines, "\n"))
		}
		if strings.Contains(visible, "000") && !strings.Contains(visible, "10 000") {
			t.Fatalf("NBSP value split after prefix:\n%s", strings.Join(lines, "\n"))
		}
	}
	if !sawTogether {
		t.Fatalf("NBSP value was not rendered intact:\n%s", strings.Join(lines, "\n"))
	}
}

func TestRenderFullBufferedTableSizesColumnsFromLaterRows(t *testing.T) {
	wide := stripANSI(renderTableCorpusFixture(t, "regression-pdf-page-fragment.md", 80, TableBufferFull, TableWireASCII))
	if !strings.Contains(wide, "recommendations") ||
		strings.Contains(wide, "recommendati |\n") ||
		strings.Contains(wide, "\n|                 | ons") {
		t.Fatalf("full-buffered table split a later long word despite enough width:\n%s", wide)
	}
	assertContains(t, wide, "AI-augmented")

	narrow := stripANSI(renderTableCorpusFixture(t, "regression-pdf-page-fragment.md", 62, TableBufferFull, TableWireASCII))
	if strings.Contains(narrow, "recommendati") || strings.Contains(narrow, "\n| ons") {
		t.Fatalf("full-buffered table used a pathological narrow-width split:\n%s", narrow)
	}
}

func TestRenderBlockQuoteTablePrefixesEveryLine(t *testing.T) {
	src := strings.Join([]string{
		"> | Kind | Value |",
		"> | --- | --- |",
		"> | link | [long descriptive link label that should wrap](https://example.com/long/path) |",
		"> | strong | **bold phrase with enough words to wrap** |",
		"",
	}, "\n")
	for _, mode := range []TableBufferMode{TableBufferFull, TableBufferRow} {
		out := stripANSI(renderStreamWithOptions(t, []byte(src), 50, WithOSC8(false), WithTableBufferMode(mode), WithTableWireMode(TableWireLine)))
		var tableLines int
		for _, line := range strings.Split(out, "\n") {
			if strings.ContainsAny(line, "┌├└│") {
				tableLines++
				if !strings.HasPrefix(line, "> ") {
					t.Fatalf("blockquote table line missing quote prefix in mode %v: %q\nfull output:\n%s", mode, line, out)
				}
			}
		}
		if tableLines < 5 {
			t.Fatalf("expected wrapped blockquote table lines in mode %v, got %d:\n%s", mode, tableLines, out)
		}
	}
}

func TestRenderListTablePrefixesEveryLine(t *testing.T) {
	src := strings.Join([]string{
		"- item before table",
		"",
		"  | Kind | Value |",
		"  | --- | --- |",
		"  | code | `very-long-inline-code-token-that-should-force-wrapping` |",
		"  | mixed | before *italic words that wrap* and **bold words** after |",
		"",
		"After list.",
		"",
	}, "\n")
	for _, mode := range []TableBufferMode{TableBufferFull, TableBufferRow} {
		out := stripANSI(renderStreamWithOptions(t, []byte(src), 56, WithOSC8(false), WithTableBufferMode(mode), WithTableWireMode(TableWireLine)))
		var tableLines int
		for _, line := range strings.Split(out, "\n") {
			if strings.ContainsAny(line, "┌├└│") {
				tableLines++
				if !strings.HasPrefix(line, "  ") {
					t.Fatalf("list table line missing list continuation prefix in mode %v: %q\nfull output:\n%s", mode, line, out)
				}
			}
		}
		if tableLines < 5 {
			t.Fatalf("expected wrapped list table lines in mode %v, got %d:\n%s", mode, tableLines, out)
		}
		assertContains(t, out, "\nAfter list.")
	}
}

func TestRenderStandaloneTableAfterListDoesNotInheritPrefix(t *testing.T) {
	src := strings.Join([]string{
		"- item before table",
		"- another item before table",
		"",
		"| A | B |",
		"| --- | --- |",
		"| 1 | 2 |",
		"",
	}, "\n")
	out := stripANSI(renderStreamWithOptions(t, []byte(src), 80, WithOSC8(false), WithTableWireMode(TableWireLine)))
	for _, line := range strings.Split(out, "\n") {
		if strings.ContainsAny(line, "┌├└│") && strings.HasPrefix(line, "  ") {
			t.Fatalf("standalone table inherited list prefix: %q\nfull output:\n%s", line, out)
		}
	}
}

func assertContains(t *testing.T, got string, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, got)
	}
}

func assertTableSegment(t *testing.T, lines []TableTextLine, text string, stylePrefix string, linkURL string) {
	t.Helper()
	for _, line := range lines {
		for _, segment := range line {
			if segment.Text == text && segment.Style.Prefix == stylePrefix && segment.LinkURL == linkURL {
				return
			}
		}
	}
	t.Fatalf("expected table segment text=%q style=%q link=%q, got %#v", text, stylePrefix, linkURL, lines)
}
