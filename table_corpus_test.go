package mdf

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tableCorpusDir = "testdata/table-corpus"

var tableCorpusExpectedTables = map[string]int{
	"alignment.md":                        7,
	"block-contexts.md":                   4,
	"container-contexts.md":               11,
	"headerless-pipe.md":                  3,
	"inline-code-links.md":                3,
	"inline-emphasis.md":                  3,
	"inline-escapes-entities.md":          3,
	"near-misses.md":                      0,
	"position-end.md":                     1,
	"position-middle.md":                  1,
	"position-multiple.md":                4,
	"position-top.md":                     1,
	"regression-declared-columns.md":      2,
	"regression-headerless-row-buffer.md": 2,
	"regression-non-table-pipes.md":       1,
	"regression-non-table-pipes-start.md": 1,
	"regression-pdf-page-fragment.md":     1,
	"regression-spacing-contexts.md":      3,
	"regression-streaming-ambiguity.md":   0,
	"row-shapes.md":                       4,
	"syntax-edge-pipes.md":                4,
	"syntax-spacing.md":                   4,
	"unicode.md":                          3,
	"wrapping.md":                         4,
}

var tableCorpusExpectedHeaderRows = map[string]int{
	"container-contexts.md":               10,
	"headerless-pipe.md":                  0,
	"regression-headerless-row-buffer.md": 0,
	"regression-streaming-ambiguity.md":   0,
}

func TestMarkdownTableCorpusParserEvents(t *testing.T) {
	for _, path := range tableCorpusMarkdownFiles(t) {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			src := readTableCorpusFile(t, path)
			stream := &captureStream{}
			err := Parse(ParseRequest{
				Reader: bytes.NewReader(src),
				Stream: stream,
				Theme:  DefaultTheme(),
			})
			if err != nil {
				t.Fatalf("parse corpus: %v", err)
			}
			want := tableCorpusExpectedTables[filepath.Base(path)]
			if got := len(stream.tableStarts); got != want {
				t.Fatalf("table starts: got %d want %d", got, want)
			}
			if got := stream.tableEnds; got != len(stream.tableStarts) {
				t.Fatalf("table ends: got %d want %d", got, len(stream.tableStarts))
			}
			wantHeaders := len(stream.tableStarts)
			if expected, ok := tableCorpusExpectedHeaderRows[filepath.Base(path)]; ok {
				wantHeaders = expected
			}
			if got := countHeaderRows(stream.tableRows); got != wantHeaders {
				t.Fatalf("header rows: got %d want %d", got, wantHeaders)
			}
			if want > 0 {
				assertCorpusValidTablesHaveMultipleRows(t, stream.tableRows)
			}
		})
	}
}

func TestMarkdownTableCorpusInlineCells(t *testing.T) {
	var rows []TableRow
	for _, name := range []string{"inline-emphasis.md", "inline-code-links.md", "inline-escapes-entities.md", "container-contexts.md"} {
		rows = append(rows, parseTableCorpusRows(t, filepath.Join(tableCorpusDir, name))...)
	}
	assertCorpusTableWithCellText(t, rows, "italic text")
	assertCorpusTableWithCellText(t, rows, "bold text")
	assertCorpusTableWithCellText(t, rows, "bold italic text")
	assertCorpusTableWithCellText(t, rows, "value")
	assertCorpusTableWithCellText(t, rows, "x|y")
	assertCorpusTableWithCellText(t, rows, "left|right (https://example.com)")
	assertCorpusTableWithCellText(t, rows, "pipe url (https://example.com/a|b)")
	assertCorpusTableWithCellText(t, rows, "[left|right]")
	assertCorpusTableWithCellText(t, rows, "site (https://example.com)")
	assertCorpusTableWithCellText(t, rows, "https://example.com")
	assertCorpusTableWithCellText(t, rows, "https://example.com/a|b")
	assertCorpusTableWithCellText(t, rows, "value | with pipe")
	assertCorpusTableWithCellText(t, rows, "AT&T")
	assertCorpusTableWithCellText(t, rows, "10\u00A0000")
	assertCorpusTableWithCellText(t, rows, "<tag>")
	assertCorpusTableWithStyledToken(t, rows, "italic text", DefaultTheme().Styles().Emphasis)
	assertCorpusTableWithStyledToken(t, rows, "bold text", DefaultTheme().Styles().Strong)
	assertCorpusTableWithStyledToken(t, rows, "bold italic text", DefaultTheme().Styles().EmphasisStrong)
	assertCorpusTableWithStyledToken(t, rows, "value", DefaultTheme().Styles().CodeInline)
	assertCorpusTableWithStyledToken(t, rows, "x|y", DefaultTheme().Styles().CodeInline)
	assertCorpusTableWithStyledToken(t, rows, "site", DefaultTheme().Styles().LinkText)
	assertCorpusTableWithStyledToken(t, rows, "left|right", DefaultTheme().Styles().LinkText)
	assertCorpusTableWithStyledToken(t, rows, "https://example.com", DefaultTheme().Styles().LinkURL)
	assertCorpusTableWithStyledToken(t, rows, "https://example.com/a|b", DefaultTheme().Styles().LinkURL)
}

func TestMarkdownTableCorpusRenderModes(t *testing.T) {
	cases := []struct {
		name   string
		buffer TableBufferMode
		wire   TableWireMode
		width  int
	}{
		{name: "full-line-narrow", buffer: TableBufferFull, wire: TableWireLine, width: 38},
		{name: "full-ascii", buffer: TableBufferFull, wire: TableWireASCII, width: 60},
		{name: "full-space", buffer: TableBufferFull, wire: TableWireSpace, width: 60},
		{name: "row-line", buffer: TableBufferRow, wire: TableWireLine, width: 60},
		{name: "row-ascii-narrow", buffer: TableBufferRow, wire: TableWireASCII, width: 38},
		{name: "row-space", buffer: TableBufferRow, wire: TableWireSpace, width: 60},
	}
	for _, path := range tableCorpusMarkdownFiles(t) {
		src := readTableCorpusFile(t, path)
		for _, tc := range cases {
			t.Run(filepath.Base(path)+"/"+tc.name, func(t *testing.T) {
				out := stripANSI(renderStreamWithOptions(
					t,
					src,
					tc.width,
					WithOSC8(false),
					WithTableBufferMode(tc.buffer),
					WithTableWireMode(tc.wire),
				))
				assertRenderedCorpusMode(t, out, tc.wire, tableCorpusExpectedTables[filepath.Base(path)])
			})
		}
	}
}

func TestMarkdownTableCorpusNonTablePipeRegression(t *testing.T) {
	out := renderTableCorpusFixture(t, "regression-non-table-pipes.md", 80, TableBufferFull, TableWireLine)
	plain := stripANSI(out)
	for _, raw := range []string{"*em*", "[site](https://example.com)", "**strong**", "`code`"} {
		if strings.Contains(plain, raw) {
			t.Fatalf("non-table pipe corpus rendered raw inline markdown %q:\n%s", raw, plain)
		}
	}
	for _, want := range []string{
		"A | em site (https://example.com)",
		"literal | strong code text",
		"│ Real",
		"│ value",
	} {
		assertContains(t, plain, want)
	}
	styles := DefaultTheme().Styles()
	for _, want := range []string{
		styles.Emphasis.Prefix + "em",
		styles.Strong.Prefix + "strong",
		styles.CodeInline.Prefix + "code",
		styles.LinkText.Prefix + "site",
		styles.LinkURL.Prefix + "https://example.com",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("non-table pipe corpus missing styled segment %q:\n%q", want, out)
		}
	}
}

func TestMarkdownTableCorpusNonTablePipeAtDocumentStartRegression(t *testing.T) {
	out := renderTableCorpusFixture(t, "regression-non-table-pipes-start.md", 80, TableBufferFull, TableWireLine)
	plain := stripANSI(out)
	if strings.HasPrefix(plain, " ") {
		t.Fatalf("document-leading non-table pipe corpus rendered with leading space:\n%q", plain)
	}
	assertContains(t, plain, "A | em site (https://example.com)")
	assertContains(t, plain, "│ Real")
}

func TestMarkdownTableCorpusDeclaredColumnRegression(t *testing.T) {
	for _, mode := range []TableBufferMode{TableBufferFull, TableBufferRow} {
		out := stripANSI(renderTableCorpusFixture(t, "regression-declared-columns.md", 80, mode, TableWireASCII))
		if strings.Contains(out, "EXTRA") || strings.Contains(out, "EXTRA-LONG-CELL") {
			t.Fatalf("declared-column corpus rendered truncated body cell in mode %v:\n%s", mode, out)
		}
		for _, want := range []string{
			"| A",
			"| one",
			"| six",
		} {
			assertContains(t, out, want)
		}
	}
}

func TestMarkdownTableCorpusHeaderlessRowBufferRegression(t *testing.T) {
	row := stripANSI(renderTableCorpusFixture(t, "regression-headerless-row-buffer.md", 80, TableBufferRow, TableWireASCII))
	if countExactRenderedLine(row, "+---+---+") != 4 {
		t.Fatalf("headerless edge-pipe table has unexpected separator count:\n%s", row)
	}
	if !strings.Contains(row, "5") {
		t.Fatalf("headerless row-buffer corpus dropped late extra cell:\n%s", row)
	}
}

func TestMarkdownTableCorpusSpacingRegression(t *testing.T) {
	out := stripANSI(renderTableCorpusFixture(t, "regression-spacing-contexts.md", 80, TableBufferFull, TableWireLine))
	for _, bad := range []string{
		"┘\n\n\nParagraph directly after the table.",
		"┘\n\n\n## Heading After Table",
		"┘\n\n\n## Heading After List Table",
	} {
		if strings.Contains(out, bad) {
			t.Fatalf("spacing corpus contains extra blank separator %q:\n%s", bad, out)
		}
	}
	for _, want := range []string{
		"┘\n\nParagraph directly after the table.",
		"┘\n\n## Heading After Table",
		"┘\n\n## Heading After List Table",
	} {
		assertContains(t, out, want)
	}
}

func TestMarkdownTableCorpusStreamingRegression(t *testing.T) {
	src := readTableCorpusFile(t, filepath.Join(tableCorpusDir, "regression-streaming-ambiguity.md"))
	if !bytes.Contains(src, []byte("hello world streams before newline")) {
		t.Fatalf("streaming regression corpus lost the plain paragraph probe")
	}
	parser := newLiveParser(DefaultTheme(), false)
	stream := &captureStream{}
	if err := parser.feedRune(stream, 'h'); err != nil {
		t.Fatalf("feed corpus probe rune: %v", err)
	}
	if len(stream.tokens) == 0 {
		t.Fatalf("expected corpus plain paragraph probe to stream at first paragraph decision")
	}
}

func assertRenderedCorpusMode(t *testing.T, out string, wire TableWireMode, tables int) {
	t.Helper()
	if strings.TrimSpace(out) == "" {
		t.Fatalf("rendered corpus is empty")
	}
	if tables == 0 {
		return
	}
	switch wire {
	case TableWireLine:
		if !strings.Contains(out, "┌") || !strings.Contains(out, "└") {
			t.Fatalf("line-wire corpus render missing table borders:\n%s", out)
		}
	case TableWireASCII:
		if !strings.Contains(out, "+") || !strings.Contains(out, "|") {
			t.Fatalf("ascii-wire corpus render missing table borders:\n%s", out)
		}
	case TableWireSpace:
		if strings.ContainsAny(out, "┌┬┐├┼┤└┴┘│") {
			t.Fatalf("space-wire corpus render contains unicode borders:\n%s", out)
		}
	}
}

func renderTableCorpusFixture(t *testing.T, name string, width int, buffer TableBufferMode, wire TableWireMode) string {
	t.Helper()
	return renderStreamWithOptions(
		t,
		readTableCorpusFile(t, filepath.Join(tableCorpusDir, name)),
		width,
		WithOSC8(false),
		WithTableBufferMode(buffer),
		WithTableWireMode(wire),
	)
}

func countExactRenderedLine(out string, want string) int {
	count := 0
	for _, line := range strings.Split(out, "\n") {
		if line == want {
			count++
		}
	}
	return count
}

func tableCorpusMarkdownFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(tableCorpusDir, "*.md"))
	if err != nil {
		t.Fatalf("glob table corpus: %v", err)
	}
	if len(matches) != len(tableCorpusExpectedTables) {
		t.Fatalf("corpus file count: got %d want %d", len(matches), len(tableCorpusExpectedTables))
	}
	for _, path := range matches {
		if _, ok := tableCorpusExpectedTables[filepath.Base(path)]; !ok {
			t.Fatalf("unexpected table corpus file %s", path)
		}
	}
	return matches
}

func readTableCorpusFile(t *testing.T, path string) []byte {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return src
}

func parseTableCorpusRows(t *testing.T, path string) []TableRow {
	t.Helper()
	stream := &captureStream{}
	err := Parse(ParseRequest{
		Reader: bytes.NewReader(readTableCorpusFile(t, path)),
		Stream: stream,
		Theme:  DefaultTheme(),
	})
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return stream.tableRows
}

func countHeaderRows(rows []TableRow) int {
	count := 0
	for _, row := range rows {
		if row.Header {
			count++
		}
	}
	return count
}

func assertCorpusValidTablesHaveMultipleRows(t *testing.T, rows []TableRow) {
	t.Helper()
	rowCount := 0
	for _, row := range rows {
		if row.Header {
			if rowCount > 0 && rowCount < 3 {
				t.Fatalf("valid table has only %d rows; expected header plus multiple body rows", rowCount)
			}
			rowCount = 1
			continue
		}
		rowCount++
	}
	if rowCount > 0 && rowCount < 3 {
		t.Fatalf("valid table has only %d rows; expected header plus multiple body rows", rowCount)
	}
}

func assertCorpusTableWithCellText(t *testing.T, rows []TableRow, want string) {
	t.Helper()
	for _, row := range rows {
		for _, cell := range row.Cells {
			if cell.Text == want {
				return
			}
		}
	}
	t.Fatalf("corpus table cell %q not found", want)
}

func assertCorpusTableWithStyledToken(t *testing.T, rows []TableRow, text string, style Style) {
	t.Helper()
	for _, row := range rows {
		for _, cell := range row.Cells {
			var b strings.Builder
			for _, tok := range cell.Tokens {
				if tok.Style.Prefix == style.Prefix {
					b.WriteString(tok.Text)
					if strings.Contains(b.String(), text) {
						return
					}
					continue
				}
				b.Reset()
			}
		}
	}
	t.Fatalf("corpus table token %q with style %q not found", text, style.Prefix)
}
