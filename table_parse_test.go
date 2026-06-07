package mdf

import (
	"slices"
	"strings"
	"testing"
)

func TestLiveParserEmitsTableEvents(t *testing.T) {
	src := strings.Join([]string{
		"| Left | Right | Center |",
		"| :--- | ---: | :---: |",
		"| a | b | c |",
		"| escaped \\| pipe | z | y |",
		"",
	}, "\n")
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stream.tableStarts) != 1 {
		t.Fatalf("expected one table start, got %d; tokens %q", len(stream.tableStarts), tokenTexts(stream.tokens))
	}
	align := stream.tableStarts[0].Alignments
	if got, want := align, []TableAlignment{TableAlignLeft, TableAlignRight, TableAlignCenter}; !sameTableAlignment(got, want) {
		t.Fatalf("unexpected alignments: got %v want %v", got, want)
	}
	if len(stream.tableRows) != 3 {
		t.Fatalf("expected header plus two body rows, got %d", len(stream.tableRows))
	}
	if !stream.tableRows[0].Header {
		t.Fatalf("expected first row to be header")
	}
	if got := stream.tableRows[1].Cells[1].Text; got != "b" {
		t.Fatalf("unexpected body cell: %q", got)
	}
	if got := stream.tableRows[2].Cells[0].Text; got != "escaped | pipe" {
		t.Fatalf("unexpected escaped pipe cell: %q", got)
	}
	if stream.tableEnds != 1 {
		t.Fatalf("expected one table end, got %d", stream.tableEnds)
	}
}

func TestParseTableDelimiter(t *testing.T) {
	header, ok := parseTableRow("| Left | Right | Center |")
	if !ok {
		t.Fatalf("expected header row")
	}
	if got, want := len(header), 3; got != want {
		t.Fatalf("header cells: got %d want %d: %v", got, want, header)
	}
	align, ok := parseTableDelimiter("| :--- | ---: | :---: |", len(header))
	if !ok {
		t.Fatalf("expected delimiter row")
	}
	if got, want := align, []TableAlignment{TableAlignLeft, TableAlignRight, TableAlignCenter}; !sameTableAlignment(got, want) {
		t.Fatalf("unexpected alignments: got %v want %v", got, want)
	}
}

func TestParseTableRowIgnoresPipesInsideInlineCodeAndLinks(t *testing.T) {
	cases := []struct {
		name string
		row  string
		want []string
	}{
		{
			name: "code",
			row:  "| `x|y` | z |",
			want: []string{"`x|y`", "z"},
		},
		{
			name: "unmatched code",
			row:  "| `oops | 1 |",
			want: []string{"`oops", "1"},
		},
		{
			name: "link label",
			row:  "| [x|y](https://example.com) | z |",
			want: []string{"[x|y](https://example.com)", "z"},
		},
		{
			name: "escaped bracket in link label",
			row:  "| [a\\]|b](https://example.com) | y |",
			want: []string{"[a\\]|b](https://example.com)", "y"},
		},
		{
			name: "unmatched link label",
			row:  "| [oops | 1 |",
			want: []string{"[oops", "1"},
		},
		{
			name: "link destination",
			row:  "| [x](https://example.com/a|b) | z |",
			want: []string{"[x](https://example.com/a|b)", "z"},
		},
		{
			name: "escaped paren in link destination",
			row:  "| [x](https://example.com/a\\)|b) | y |",
			want: []string{"[x](https://example.com/a\\)|b)", "y"},
		},
		{
			name: "reference-like label",
			row:  "| [x|y] | z |",
			want: []string{"[x|y]", "z"},
		},
		{
			name: "autolink",
			row:  "| <https://example.com/a|b> | z |",
			want: []string{"<https://example.com/a|b>", "z"},
		},
		{
			name: "html tag",
			row:  "| <span data-x=\"a|b\">value</span> | z |",
			want: []string{"<span data-x=\"a|b\">value</span>", "z"},
		},
		{
			name: "html tag quoted greater-than",
			row:  "| <span title=\"a>b|c\">value</span> | z |",
			want: []string{"<span title=\"a>b|c\">value</span>", "z"},
		},
		{
			name: "html tag unquoted pipe attribute",
			row:  "| <span data-x=a|b>value</span> | z |",
			want: []string{"<span data-x=a|b>value</span>", "z"},
		},
		{
			name: "literal angle text",
			row:  "| a <x|y> | z |",
			want: []string{"a <x", "y>", "z"},
		},
		{
			name: "link destination title",
			row:  "| [x](https://example.com \"a)|b\") | z |",
			want: []string{"[x](https://example.com \"a)|b\")", "z"},
		},
		{
			name: "unmatched link destination",
			row:  "| [x](https://example.com \"oops)|b | z |",
			want: []string{"[x](https://example.com \"oops)", "b", "z"},
		},
		{
			name: "literal comparison",
			row:  "| a < b | c > d | e |",
			want: []string{"a < b", "c > d", "e"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cells, ok := parseTableRow(tc.row)
			if !ok {
				t.Fatalf("expected table row")
			}
			if got, want := cells, tc.want; !slices.Equal(got, want) {
				t.Fatalf("cells: got %#v want %#v", got, want)
			}
		})
	}
}

func TestParseTableRowHonorsEscapedParenInUnterminatedLinkDestination(t *testing.T) {
	if hasClosingLinkDestination(`url\)|y | z |`) {
		t.Fatalf("escaped closing paren ended link destination probe")
	}
	cells, ok := parseTableRow(`| [x](url\)|y | z |`)
	if !ok {
		t.Fatalf("expected visible separators after unterminated link destination")
	}
	if len(cells) == 1 {
		t.Fatalf("unterminated link destination hid later separators: %v", cells)
	}
}

func TestParseTableRowDoesNotTreatInlineOnlyPipeAsTable(t *testing.T) {
	for _, line := range []string{
		"`x|y`",
		"[x|y](https://example.com)",
		"[x](https://example.com/a|b)",
		"<https://example.com/a|b>",
	} {
		if cells, ok := parseTableRow(line); ok {
			t.Fatalf("did not expect table row for %q, got %#v", line, cells)
		}
	}
}

func TestParseTableDelimiterWithoutEdgePipes(t *testing.T) {
	header, ok := parseTableRow("A | B")
	if !ok {
		t.Fatalf("expected header row")
	}
	align, ok := parseTableDelimiter("--- | ---", len(header))
	if !ok {
		t.Fatalf("expected delimiter row")
	}
	if got, want := align, []TableAlignment{TableAlignLeft, TableAlignLeft}; !sameTableAlignment(got, want) {
		t.Fatalf("unexpected alignments: got %v want %v", got, want)
	}
}

func TestParseTableDelimiterMinimumCells(t *testing.T) {
	header, ok := parseTableRow("|A|B|C|")
	if !ok {
		t.Fatalf("expected header row")
	}
	align, ok := parseTableDelimiter("|:-|-:|:-:|", len(header))
	if !ok {
		t.Fatalf("expected minimum delimiter row")
	}
	if got, want := align, []TableAlignment{TableAlignLeft, TableAlignRight, TableAlignCenter}; !sameTableAlignment(got, want) {
		t.Fatalf("unexpected alignments: got %v want %v", got, want)
	}
}

func TestParseOneColumnEdgePipeTable(t *testing.T) {
	header, ok := parseTableRow("| A |")
	if !ok {
		t.Fatalf("expected one-column edge-pipe header row")
	}
	if got, want := len(header), 1; got != want {
		t.Fatalf("header cells: got %d want %d", got, want)
	}
	if _, ok := parseTableDelimiter("| --- |", len(header)); !ok {
		t.Fatalf("expected one-column delimiter row")
	}
	if _, ok := parseTableRow("A"); ok {
		t.Fatalf("did not expect one-column row without an edge pipe")
	}
}

func TestLiveParserBuffersPotentialTableHeader(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	for _, r := range "| A | B |\n" {
		if err := parser.feedRune(stream, r); err != nil {
			t.Fatalf("feed rune: %v", err)
		}
	}
	if parser.tablePendingHeader == "" {
		t.Fatalf("expected pending table header")
	}
	if len(stream.tokens) != 0 {
		t.Fatalf("expected no text tokens before delimiter, got %d", len(stream.tokens))
	}
}

func TestLiveParserDirectFeedEmitsTableEvents(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	data := []byte("| A | B |\n| --- | --- |\n| 1 | 2 |\n")
	if err := parser.feedBytes(stream, data); err != nil {
		t.Fatalf("feed bytes: %v", err)
	}
	parser.finalize(stream)
	if len(stream.tableStarts) != 1 {
		t.Fatalf("expected one table start, got %d", len(stream.tableStarts))
	}
}

func TestLiveParserDirectFeedEmitsNoEdgeMultiCharacterHeaderTable(t *testing.T) {
	for _, src := range []string{
		"Name | Value\n--- | ---\nalpha | beta\n",
		"status | value\n--- | ---\nok | yes\n",
		"current status | value\n--- | ---\nok | yes\n",
		"longer header cell | value\n--- | ---\nok | yes\n",
	} {
		t.Run(strings.Split(src, "\n")[0], func(t *testing.T) {
			parser := newLiveParser(DefaultTheme(), false)
			stream := &captureStream{}
			if err := parser.feedBytes(stream, []byte(src)); err != nil {
				t.Fatalf("feed bytes: %v", err)
			}
			if err := parser.finalize(stream); err != nil {
				t.Fatalf("finalize: %v", err)
			}
			if len(stream.tableStarts) != 1 {
				t.Fatalf("expected one table start, got %d; tokens %q", len(stream.tableStarts), tokenTexts(stream.tokens))
			}
			if len(stream.tableRows) != 2 || !stream.tableRows[0].Header {
				t.Fatalf("expected header plus one body row, got rows=%d header=%v", len(stream.tableRows), len(stream.tableRows) > 0 && stream.tableRows[0].Header)
			}
		})
	}
}

func TestLiveParserBlankLineEndsTableBeforeFinalize(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	data := []byte("| A | B |\n| --- | --- |\n| 1 | 2 |\n\n")
	if err := parser.feedBytes(stream, data); err != nil {
		t.Fatalf("feed bytes: %v", err)
	}
	if stream.tableEnds != 1 {
		t.Fatalf("expected blank line to end table before finalize, got %d", stream.tableEnds)
	}
	if parser.tableActive {
		t.Fatalf("expected parser table state to be inactive after blank line")
	}
}

func TestLiveParserBlankLineFlushesPendingTableHeaderBeforeFinalize(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	data := []byte("A | B\n\n")
	if err := parser.feedBytes(stream, data); err != nil {
		t.Fatalf("feed bytes: %v", err)
	}
	if parser.tablePendingHeader != "" {
		t.Fatalf("expected pending table header to flush before finalize")
	}
	if got := tokenTexts(stream.tokens); !strings.Contains(got, "A | B") {
		t.Fatalf("expected pending table header to be emitted as paragraph before finalize, got %q", got)
	}
}

func TestLiveParserForcedBlankLineEndsTableBeforeFinalize(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	data := []byte("| A | B |\n| --- | --- |\n| 1 | 2 |\n")
	if err := parser.feedBytes(stream, data); err != nil {
		t.Fatalf("feed bytes: %v", err)
	}
	if !parser.tableActive {
		t.Fatalf("expected active table before forced blank line")
	}
	parser.lineBuf = []rune{' ', ' '}
	parser.lineBytes = []byte("  ")
	if err := parser.maybeDecideLine(stream, true); err != nil {
		t.Fatalf("decide blank line: %v", err)
	}
	if stream.tableEnds != 1 {
		t.Fatalf("expected forced blank line to end table before finalize, got %d", stream.tableEnds)
	}
	if parser.tableActive {
		t.Fatalf("expected parser table state to be inactive after forced blank line")
	}
}

func TestLiveParserForcedBlankLineFlushesPendingTableHeaderBeforeFinalize(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	if err := parser.feedBytes(stream, []byte("| A | B |\n")); err != nil {
		t.Fatalf("feed bytes: %v", err)
	}
	if parser.tablePendingHeader == "" {
		t.Fatalf("expected pending table header before forced blank line")
	}
	parser.lineBuf = []rune{' ', ' '}
	parser.lineBytes = []byte("  ")
	if err := parser.maybeDecideLine(stream, true); err != nil {
		t.Fatalf("decide blank line: %v", err)
	}
	if parser.tablePendingHeader != "" {
		t.Fatalf("expected pending table header to flush before finalize")
	}
	if got := tokenTexts(stream.tokens); !strings.Contains(got, "A | B") {
		t.Fatalf("expected pending table header to be emitted as paragraph before finalize, got %q", got)
	}
}

func TestLiveParserEmitsHeaderlessPipeTable(t *testing.T) {
	src := strings.Join([]string{
		"| A | B |",
		"| 1 | 2 |",
		"| 3 | 4 |",
		"",
	}, "\n")
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stream.tableStarts) != 1 {
		t.Fatalf("expected one table start, got %d; tokens %q", len(stream.tableStarts), tokenTexts(stream.tokens))
	}
	if len(stream.tableRows) != 3 {
		t.Fatalf("expected three body rows, got %d", len(stream.tableRows))
	}
	for i, row := range stream.tableRows {
		if row.Header {
			t.Fatalf("row %d unexpectedly marked as header", i)
		}
	}
	if got := stream.tableRows[0].Cells[0].Text; got != "A" {
		t.Fatalf("unexpected first cell: %q", got)
	}
	if stream.tableEnds != 1 {
		t.Fatalf("expected one table end, got %d", stream.tableEnds)
	}
}

func TestLiveParserSinglePipeRowRemainsParagraph(t *testing.T) {
	src := "| A | B |\n"
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stream.tableStarts) != 0 {
		t.Fatalf("expected no table starts, got %d", len(stream.tableStarts))
	}
	if got := tokenTexts(stream.tokens); !strings.Contains(got, "| A | B |") {
		t.Fatalf("expected paragraph text to contain source row, got %q", got)
	}
}

func TestLiveParserTableCellLinkPreservesOSC8Tokens(t *testing.T) {
	src := "| Kind | Value |\n| --- | --- |\n| link | [site](https://example.com) |\n"
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader:  strings.NewReader(src),
		Stream:  stream,
		Theme:   DefaultTheme(),
		Options: []RenderOption{WithOSC8(true)},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stream.tableRows) != 2 {
		t.Fatalf("expected header plus one body row, got %d", len(stream.tableRows))
	}
	tokens := stream.tableRows[1].Cells[1].Tokens
	var sawStart bool
	var sawEnd bool
	var text strings.Builder
	for _, tok := range tokens {
		switch tok.Kind {
		case tokenLinkStart:
			sawStart = tok.LinkURL == "https://example.com"
		case tokenLinkEnd:
			sawEnd = true
		default:
			text.WriteString(tok.Text)
		}
	}
	if !sawStart || text.String() != "site" || !sawEnd {
		t.Fatalf("table cell link tokens missing start/text/end: %#v", tokens)
	}
}

func TestLiveParserTableStartCarriesContainerPrefix(t *testing.T) {
	src := strings.Join([]string{
		"> | Quote | Value |",
		"> | --- | --- |",
		"> | a | b |",
		"",
		"- item",
		"",
		"  | List | Value |",
		"  | --- | --- |",
		"  | c | d |",
		"",
		"- [ ] task",
		"",
		"      | Task | Value |",
		"      | --- | --- |",
		"      | e | f |",
		"",
	}, "\n")
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stream.tableStarts) != 3 {
		t.Fatalf("expected three table starts, got %d", len(stream.tableStarts))
	}
	if got, want := tablePrefixText(stream.tableStarts[0].Prefix), "> "; got != want {
		t.Fatalf("quote table prefix: got %q want %q", got, want)
	}
	if got, want := tablePrefixText(stream.tableStarts[1].Prefix), "  "; got != want {
		t.Fatalf("list table prefix: got %q want %q", got, want)
	}
	if got, want := tablePrefixText(stream.tableStarts[2].Prefix), "      "; got != want {
		t.Fatalf("task list table prefix: got %q want %q", got, want)
	}
}

func TestLiveParserQuotedTableRequiresExplicitQuoteRows(t *testing.T) {
	src := strings.Join([]string{
		"> | A | B |",
		"> | --- | --- |",
		"| 1 | 2 |",
		"",
	}, "\n")
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, want := len(stream.tableStarts), 1; got != want {
		t.Fatalf("table starts: got %d want %d", got, want)
	}
	if got, want := tablePrefixText(stream.tableStarts[0].Prefix), "> "; got != want {
		t.Fatalf("quoted table prefix: got %q want %q", got, want)
	}
	if got, want := len(stream.tableRows), 1; got != want {
		t.Fatalf("quoted table rows: got %d want %d", got, want)
	}
	if got, want := stream.tableRows[0].Cells[0].Text, "A"; got != want {
		t.Fatalf("quoted table first cell: got %q want %q", got, want)
	}
}

func TestLiveParserQuotedTableStopsWhenQuoteDepthChanges(t *testing.T) {
	src := strings.Join([]string{
		"> | A | B |",
		"> | --- | --- |",
		">> | 1 | 2 |",
		"",
	}, "\n")
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, want := len(stream.tableStarts), 1; got != want {
		t.Fatalf("table starts: got %d want %d", got, want)
	}
	if got, want := len(stream.tableRows), 1; got != want {
		t.Fatalf("quoted table rows: got %d want %d", got, want)
	}
}

func TestLiveParserPendingQuotedTableRequiresSameQuoteDepth(t *testing.T) {
	src := strings.Join([]string{
		"> | A | B |",
		">> | --- | --- |",
		">> | 1 | 2 |",
		"",
	}, "\n")
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, want := len(stream.tableStarts), 1; got != want {
		t.Fatalf("table starts: got %d want %d", got, want)
	}
	if got, want := tablePrefixText(stream.tableStarts[0].Prefix), "> > "; got != want {
		t.Fatalf("nested table prefix: got %q want %q", got, want)
	}
}

func TestLiveParserPendingListTableRequiresSamePrefix(t *testing.T) {
	src := strings.Join([]string{
		"- item",
		"",
		"  | A | B |",
		"| 1 | 2 |",
		"",
	}, "\n")
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, want := len(stream.tableStarts), 0; got != want {
		t.Fatalf("table starts: got %d want %d", got, want)
	}
}

func TestLiveParserActiveListTableStopsWhenPrefixChanges(t *testing.T) {
	src := strings.Join([]string{
		"- item",
		"",
		"  | A | B |",
		"  | 1 | 2 |",
		"| 3 | 4 |",
		"",
	}, "\n")
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, want := len(stream.tableStarts), 1; got != want {
		t.Fatalf("table starts: got %d want %d", got, want)
	}
	if got, want := len(stream.tableRows), 2; got != want {
		t.Fatalf("list table rows: got %d want %d", got, want)
	}
	if got, want := tablePrefixText(stream.tableStarts[0].Prefix), "  "; got != want {
		t.Fatalf("list table prefix: got %q want %q", got, want)
	}
}

func TestLiveParserTableStartAlignmentsDoNotAliasParserScratch(t *testing.T) {
	src := strings.Join([]string{
		"| A | B |",
		"| :--- | ---: |",
		"| 1 | 2 |",
		"",
		"| C | D |",
		"| ---: | :--- |",
		"| 3 | 4 |",
		"",
	}, "\n")
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, want := len(stream.tableStarts), 2; got != want {
		t.Fatalf("table starts: got %d want %d", got, want)
	}
	first := stream.tableStarts[0].Alignments
	second := stream.tableStarts[1].Alignments
	if got, want := first, []TableAlignment{TableAlignLeft, TableAlignRight}; !sameTableAlignment(got, want) {
		t.Fatalf("first alignments changed: got %v want %v", got, want)
	}
	if got, want := second, []TableAlignment{TableAlignRight, TableAlignLeft}; !sameTableAlignment(got, want) {
		t.Fatalf("second alignments: got %v want %v", got, want)
	}
	second[0] = TableAlignCenter
	if got, want := first, []TableAlignment{TableAlignLeft, TableAlignRight}; !sameTableAlignment(got, want) {
		t.Fatalf("first alignments aliased second table: got %v want %v", got, want)
	}
}

func TestLiveParserDirectFeedEmitsEscapedPipeTableEvents(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	data := []byte("| Left | Right | Center |\n| :--- | ---: | :---: |\n| a | b | c |\n| escaped \\| pipe | z | y |\n")
	if err := parser.feedBytes(stream, data); err != nil {
		t.Fatalf("feed bytes: %v", err)
	}
	parser.finalize(stream)
	if len(stream.tableStarts) != 1 {
		t.Fatalf("expected one table start, got %d; tokens %q", len(stream.tableStarts), tokenTexts(stream.tokens))
	}
	if len(stream.tableRows) != 3 {
		t.Fatalf("expected header plus two body rows, got %d", len(stream.tableRows))
	}
}

func TestLiveParserDirectFeedEmitsAlignedTableEvents(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	data := []byte("| Left | Right | Center |\n| :--- | ---: | :---: |\n| a | b | c |\n")
	if err := parser.feedBytes(stream, data); err != nil {
		t.Fatalf("feed bytes: %v", err)
	}
	parser.finalize(stream)
	if len(stream.tableStarts) != 1 {
		t.Fatalf("expected one table start, got %d; tokens %q", len(stream.tableStarts), tokenTexts(stream.tokens))
	}
}

func TestLiveParserResetFeedEmitsTableEvents(t *testing.T) {
	parser := &liveParser{}
	parser.Reset(DefaultTheme(), false)
	stream := &captureStream{}
	data := []byte("| A | B |\n| --- | --- |\n| 1 | 2 |\n")
	if err := parser.feedBytes(stream, data); err != nil {
		t.Fatalf("feed bytes: %v", err)
	}
	parser.finalize(stream)
	if len(stream.tableStarts) != 1 {
		t.Fatalf("expected one table start, got %d", len(stream.tableStarts))
	}
}

func TestFrontMatterFilterPassesTableInput(t *testing.T) {
	var filter frontMatterFilter
	filter.reset()
	data := []byte("| A | B |\n| --- | --- |\n| 1 | 2 |\n")
	out := filter.process(data)
	if string(out) != string(data) {
		t.Fatalf("unexpected filter output: %q", string(out))
	}
}

func TestLiveParserStartsTableAfterDelimiter(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	for _, r := range "| A | B |\n" {
		if err := parser.feedRune(stream, r); err != nil {
			t.Fatalf("feed header: %v", err)
		}
	}
	for _, r := range "| --- | --- |\n" {
		if err := parser.feedRune(stream, r); err != nil {
			t.Fatalf("feed delimiter: %v", err)
		}
	}
	if len(stream.tableStarts) != 1 {
		t.Fatalf("expected one table start, got %d; pending header %q tokens %d", len(stream.tableStarts), parser.tablePendingHeader, len(stream.tokens))
	}
}

func TestLiveParserStartsTableAfterAlignedDelimiter(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	for _, r := range "| Left | Right | Center |\n" {
		if err := parser.feedRune(stream, r); err != nil {
			t.Fatalf("feed header: %v", err)
		}
	}
	if parser.tablePendingHeader != "| Left | Right | Center |" {
		t.Fatalf("unexpected pending header after header line: %q", parser.tablePendingHeader)
	}
	var delimPrefix strings.Builder
	for _, r := range "| :--- | ---: | :---: |\n" {
		delimPrefix.WriteRune(r)
		if err := parser.feedRune(stream, r); err != nil {
			t.Fatalf("feed delimiter: %v", err)
		}
		if r != '\n' && parser.tablePendingHeader != "| Left | Right | Center |" {
			t.Fatalf("pending header changed after delimiter prefix %q: %q tokens %q", delimPrefix.String(), parser.tablePendingHeader, tokenTexts(stream.tokens))
		}
	}
	if len(stream.tableStarts) != 1 {
		t.Fatalf("expected one table start, got %d; pending header %q tokens %q", len(stream.tableStarts), parser.tablePendingHeader, tokenTexts(stream.tokens))
	}
}

func TestLiveParserMaybeDecideAlignedDelimiter(t *testing.T) {
	parser := newLiveParser(DefaultTheme(), false)
	parser.tablePendingHeader = "| Left | Right | Center |"
	parser.lineBytes = append(parser.lineBytes, []byte("| :--- | ---: | :---: |")...)
	parser.lineBuf = append(parser.lineBuf, []rune("| :--- | ---: | :---: |")...)
	stream := &captureStream{}
	if err := parser.maybeDecideLine(stream, true); err != nil {
		t.Fatalf("decide line: %v", err)
	}
	if len(stream.tableStarts) != 1 {
		t.Fatalf("expected one table start, got %d; pending header %q tokens %q", len(stream.tableStarts), parser.tablePendingHeader, tokenTexts(stream.tokens))
	}
}

func TestLiveParserEmitsTableAtEOF(t *testing.T) {
	src := "| A | B |\n| --- | --- |\n| 1 | 2 |"
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: strings.NewReader(src),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stream.tableStarts) != 1 {
		t.Fatalf("expected one table start, got %d", len(stream.tableStarts))
	}
	if len(stream.tableRows) != 2 {
		t.Fatalf("expected header plus one body row, got %d", len(stream.tableRows))
	}
	if stream.tableEnds != 1 {
		t.Fatalf("expected one table end at EOF, got %d", stream.tableEnds)
	}
}

func sameTableAlignment(a, b []TableAlignment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func tokenTexts(tokens []StreamToken) string {
	var b strings.Builder
	for _, tok := range tokens {
		b.WriteString(tok.Text)
	}
	return b.String()
}

func tablePrefixText(prefix []TablePrefixSegment) string {
	var b strings.Builder
	for _, segment := range prefix {
		b.WriteString(segment.Text)
	}
	return b.String()
}
