package mdf

import (
	"strings"
	"unicode"

	"github.com/muesli/reflow/ansi"
)

type tableSegmentKind uint8

const (
	tableSegmentText tableSegmentKind = iota
	tableSegmentHeader
	tableSegmentWire
)

type tableSegment struct {
	Text      string
	Kind      tableSegmentKind
	Style     Style
	TokenKind TokenKind
	LinkURL   string
}

type tableLine []tableSegment

type tableCellWord struct {
	separator         tableLine
	segments          tableLine
	preserveSeparator bool
}

// TableTextSegmentKind identifies the role of a fixed-width table segment.
type TableTextSegmentKind uint8

const (
	// TableTextSegmentText is ordinary body-cell text.
	TableTextSegmentText TableTextSegmentKind = iota
	// TableTextSegmentHeader is header-cell text.
	TableTextSegmentHeader
	// TableTextSegmentWire is table border, separator, or padding wire.
	TableTextSegmentWire
)

// TableTextSegment is a styled segment of a fixed-width rendered table line.
type TableTextSegment struct {
	// Text is the segment text.
	Text string
	// Kind identifies the table role of Text.
	Kind TableTextSegmentKind
	// Style is the semantic style for Text.
	Style Style
	// TokenKind preserves inline token metadata for text segments.
	TokenKind TokenKind
	// LinkURL is set when the segment belongs to a link.
	LinkURL string
}

// TableTextLine is one fixed-width rendered table line.
type TableTextLine []TableTextSegment

// RenderTableTextLines renders Markdown table rows into fixed-width text lines.
// The width argument is measured in terminal cells; values <= 0 use the
// natural table width.
func RenderTableTextLines(rows []TableRow, alignments []TableAlignment, width int, wire TableWireMode) []string {
	return tableLinesToStrings(layoutTable(rows, alignments, width, wire))
}

// RenderTableStyledLines renders Markdown table rows into styled fixed-width text lines.
// The returned segments preserve table and inline style metadata.
func RenderTableStyledLines(rows []TableRow, alignments []TableAlignment, width int, wire TableWireMode) []TableTextLine {
	return exportTableLines(layoutTable(rows, alignments, width, wire))
}

type tableLayout struct {
	alignments      []TableAlignment
	widths          []int
	wire            TableWireMode
	declaredColumns bool
}

// TableTextPlan stores fixed-width text table layout decisions.
type TableTextPlan struct {
	layout tableLayout
}

// NewTableTextPlan creates a fixed-width text table plan from representative rows.
// Row-buffered renderers can build a plan from the header and first body row,
// then render later rows with stable widths.
func NewTableTextPlan(rows []TableRow, alignments []TableAlignment, width int, wire TableWireMode) TableTextPlan {
	return TableTextPlan{layout: buildTableLayout(rows, alignments, width, wire)}
}

// TopBorder returns the table's top border line, if the wire mode uses one.
func (p TableTextPlan) TopBorder() []string {
	if p.layout.wire == TableWireSpace {
		return nil
	}
	return tableLinesToStrings([]tableLine{p.layout.borderLine(tableBorderTop)})
}

// StyledTopBorder returns the table's styled top border line, if the wire mode uses one.
func (p TableTextPlan) StyledTopBorder() []TableTextLine {
	if p.layout.wire == TableWireSpace {
		return nil
	}
	return exportTableLines([]tableLine{p.layout.borderLine(tableBorderTop)})
}

// HeaderBorder returns the table's header separator line, if the wire mode uses one.
func (p TableTextPlan) HeaderBorder() []string {
	if p.layout.wire == TableWireSpace {
		return nil
	}
	return tableLinesToStrings([]tableLine{p.layout.borderLine(tableBorderMiddle)})
}

// StyledHeaderBorder returns the table's styled header separator line, if the wire mode uses one.
func (p TableTextPlan) StyledHeaderBorder() []TableTextLine {
	if p.layout.wire == TableWireSpace {
		return nil
	}
	return exportTableLines([]tableLine{p.layout.borderLine(tableBorderMiddle)})
}

// BottomBorder returns the table's bottom border line, if the wire mode uses one.
func (p TableTextPlan) BottomBorder() []string {
	if p.layout.wire == TableWireSpace {
		return nil
	}
	return tableLinesToStrings([]tableLine{p.layout.borderLine(tableBorderBottom)})
}

// StyledBottomBorder returns the table's styled bottom border line, if the wire mode uses one.
func (p TableTextPlan) StyledBottomBorder() []TableTextLine {
	if p.layout.wire == TableWireSpace {
		return nil
	}
	return exportTableLines([]tableLine{p.layout.borderLine(tableBorderBottom)})
}

// RowLines renders one table row using the plan's fixed column widths.
func (p TableTextPlan) RowLines(row TableRow) []string {
	return tableLinesToStrings(p.layout.rowLines(row))
}

// StyledRowLines renders one table row using the plan's fixed column widths.
func (p TableTextPlan) StyledRowLines(row TableRow) []TableTextLine {
	return exportTableLines(p.layout.rowLines(row))
}

func tableLinesToStrings(lines []tableLine) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		var b strings.Builder
		for _, segment := range line {
			b.WriteString(segment.Text)
		}
		out = append(out, b.String())
	}
	return out
}

func exportTableLines(lines []tableLine) []TableTextLine {
	out := make([]TableTextLine, 0, len(lines))
	for _, line := range lines {
		exported := make(TableTextLine, 0, len(line))
		for _, segment := range line {
			exported = append(exported, TableTextSegment{
				Text:      segment.Text,
				Kind:      exportTableSegmentKind(segment.Kind),
				Style:     segment.Style,
				TokenKind: segment.TokenKind,
				LinkURL:   segment.LinkURL,
			})
		}
		out = append(out, exported)
	}
	return out
}

func exportTableSegmentKind(kind tableSegmentKind) TableTextSegmentKind {
	switch kind {
	case tableSegmentHeader:
		return TableTextSegmentHeader
	case tableSegmentWire:
		return TableTextSegmentWire
	default:
		return TableTextSegmentText
	}
}

func layoutTable(rows []TableRow, alignments []TableAlignment, width int, wire TableWireMode) []tableLine {
	if len(rows) == 0 {
		return nil
	}
	layout := buildTableLayout(rows, alignments, width, wire)
	var out []tableLine
	if wire != TableWireSpace {
		out = append(out, layout.borderLine(tableBorderTop))
	}
	for _, row := range rows {
		out = append(out, layout.rowLines(row)...)
		if row.Header && wire != TableWireSpace {
			out = append(out, layout.borderLine(tableBorderMiddle))
		}
	}
	if wire != TableWireSpace {
		out = append(out, layout.borderLine(tableBorderBottom))
	}
	return out
}

func buildTableLayout(rows []TableRow, alignments []TableAlignment, width int, wire TableWireMode) tableLayout {
	cols := len(alignments)
	declaredColumns := cols > 0
	if cols == 0 {
		for _, row := range rows {
			if len(row.Cells) > cols {
				cols = len(row.Cells)
			}
		}
	}
	if cols == 0 {
		cols = 1
	}
	widths := make([]int, cols)
	for i := range widths {
		widths[i] = 1
	}
	minWidths := make([]int, cols)
	for i := range minWidths {
		minWidths[i] = 1
	}
	breakMinWidths := make([]int, cols)
	for i := range breakMinWidths {
		breakMinWidths[i] = 1
	}
	for _, row := range rows {
		for i := 0; i < cols && i < len(row.Cells); i++ {
			for _, line := range wrapStyledCell(row.Cells[i], 0) {
				if w := tableLineWidth(line); w > widths[i] {
					widths[i] = w
				}
			}
			if w := tableCellPreferredMinWidth(row.Cells[i]); w > minWidths[i] {
				minWidths[i] = w
			}
			if w := tableCellBreakMinWidth(row.Cells[i]); w > breakMinWidths[i] {
				breakMinWidths[i] = w
			}
		}
	}
	splitCosts := tableColumnSplitCosts(rows, cols, widths)
	maxWidth := width
	if maxWidth <= 0 {
		maxWidth = tableTotalWidth(widths, wire)
	}
	for tableTotalWidth(widths, wire) > maxWidth {
		idx := widestColumnAboveMin(widths, minWidths)
		if idx < 0 {
			break
		}
		widths[idx]--
	}
	for tableTotalWidth(widths, wire) > maxWidth {
		widths = allocateConstrainedTableWidths(widths, breakMinWidths, maxWidth, wire, splitCosts)
		break
	}
	align := make([]TableAlignment, cols)
	for i := range align {
		align[i] = TableAlignLeft
		if i < len(alignments) {
			align[i] = alignments[i]
		}
	}
	return tableLayout{alignments: align, widths: widths, wire: wire, declaredColumns: declaredColumns}
}

func allocateConstrainedTableWidths(naturalWidths []int, breakMinWidths []int, maxWidth int, wire TableWireMode, splitCosts [][]int) []int {
	widths := make([]int, len(naturalWidths))
	for i := range widths {
		widths[i] = widthAt(breakMinWidths, i, 1)
	}
	budget := maxWidth - tableWireOverhead(len(widths), wire)
	remaining := budget - sumInts(widths)
	if remaining <= 0 {
		return widths
	}
	for remaining > 0 {
		idx := bestColumnForSplitCostReduction(widths, naturalWidths, splitCosts)
		if idx < 0 {
			idx = widestColumnBelowTarget(widths, naturalWidths)
		}
		if idx < 0 {
			break
		}
		widths[idx]++
		remaining--
	}
	return widths
}

func bestColumnForSplitCostReduction(widths []int, targets []int, splitCosts [][]int) int {
	idx := -1
	bestReduction := 0
	bestCurrentCost := 0
	for i, width := range widths {
		if width >= widthAt(targets, i, width) {
			continue
		}
		currentCost := tableColumnSplitCostAt(splitCosts, i, width)
		nextCost := tableColumnSplitCostAt(splitCosts, i, width+1)
		reduction := currentCost - nextCost
		if reduction <= 0 {
			continue
		}
		if idx < 0 || reduction > bestReduction || (reduction == bestReduction && currentCost > bestCurrentCost) {
			idx = i
			bestReduction = reduction
			bestCurrentCost = currentCost
		}
	}
	return idx
}

func tableColumnSplitCostAt(costs [][]int, col int, width int) int {
	if col >= len(costs) || width >= len(costs[col]) {
		return 0
	}
	return costs[col][width]
}

func widestColumnBelowTarget(widths []int, targets []int) int {
	idx := -1
	bestTarget := 0
	for i, width := range widths {
		target := widthAt(targets, i, width)
		if width >= target {
			continue
		}
		if idx < 0 || target > bestTarget {
			idx = i
			bestTarget = target
		}
	}
	return idx
}

func tableWireOverhead(cols int, wire TableWireMode) int {
	if cols <= 0 {
		return 0
	}
	switch wire {
	case TableWireSpace:
		return (cols - 1) * 2
	default:
		return 3*cols + 1
	}
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func widthAt(widths []int, idx int, fallback int) int {
	if idx < len(widths) && widths[idx] > fallback {
		return widths[idx]
	}
	return fallback
}

func tableTotalWidth(widths []int, wire TableWireMode) int {
	if len(widths) == 0 {
		return 0
	}
	total := 0
	for _, width := range widths {
		switch wire {
		case TableWireSpace:
			total += width
		default:
			total += width + 2
		}
	}
	switch wire {
	case TableWireSpace:
		return total + (len(widths)-1)*2
	default:
		return total + len(widths) + 1
	}
}

func widestColumnAboveMin(widths []int, minWidths []int) int {
	idx := -1
	maxWidth := 0
	for i, width := range widths {
		minWidth := 1
		if i < len(minWidths) && minWidths[i] > minWidth {
			minWidth = minWidths[i]
		}
		if width > minWidth && width > maxWidth {
			idx = i
			maxWidth = width
		}
	}
	return idx
}

func (l tableLayout) rowLines(row TableRow) []tableLine {
	if !l.declaredColumns {
		row = mergeExtraTableCells(row, len(l.widths))
	}
	wrapped := make([][]tableLine, len(l.widths))
	height := 1
	for i := range l.widths {
		cell := TableCell{}
		if i < len(row.Cells) {
			cell = row.Cells[i]
		}
		wrapped[i] = wrapStyledCell(cell, l.widths[i])
		if len(wrapped[i]) > height {
			height = len(wrapped[i])
		}
	}
	lines := make([]tableLine, 0, height)
	for lineIdx := 0; lineIdx < height; lineIdx++ {
		var line tableLine
		if l.wire != TableWireSpace {
			line = append(line, tableSegment{Text: tableChars(l.wire).vertical, Kind: tableSegmentWire})
		}
		for col := range l.widths {
			cellLine := tableLine{}
			if lineIdx < len(wrapped[col]) {
				cellLine = wrapped[col][lineIdx]
			}
			if l.wire == TableWireSpace {
				if col > 0 {
					line = append(line, tableSegment{Text: "  ", Kind: tableSegmentWire})
				}
				line = append(line, alignStyledCell(cellLine, l.widths[col], l.alignments[col], tableTextSegmentKind(row))...)
				continue
			}
			line = append(line, tableSegment{Text: " ", Kind: tableSegmentWire})
			line = append(line, alignStyledCell(cellLine, l.widths[col], l.alignments[col], tableTextSegmentKind(row))...)
			line = append(line, tableSegment{Text: " ", Kind: tableSegmentWire})
			line = append(line, tableSegment{Text: tableChars(l.wire).vertical, Kind: tableSegmentWire})
		}
		lines = append(lines, line)
	}
	return lines
}

func mergeExtraTableCells(row TableRow, cols int) TableRow {
	if cols <= 0 || len(row.Cells) <= cols {
		return row
	}
	merged := TableRow{Header: row.Header, Cells: make([]TableCell, cols)}
	copy(merged.Cells, row.Cells[:cols])
	last := merged.Cells[cols-1]
	for _, extra := range row.Cells[cols:] {
		last = appendTableCell(last, TableCell{
			Text:   " | ",
			Tokens: []StreamToken{{Token: Token{Text: " | "}}},
		})
		last = appendTableCell(last, extra)
	}
	merged.Cells[cols-1] = last
	return merged
}

func appendTableCell(dst TableCell, src TableCell) TableCell {
	srcTokens := src.Tokens
	if len(srcTokens) == 0 && src.Text != "" {
		srcTokens = []StreamToken{{Token: Token{Text: src.Text}}}
	}
	dst.Text += src.Text
	if len(srcTokens) == 0 {
		return dst
	}
	if len(dst.Tokens) == 0 && dst.Text != src.Text {
		dst.Tokens = append(dst.Tokens, StreamToken{Token: Token{Text: strings.TrimSuffix(dst.Text, src.Text)}})
	}
	dst.Tokens = append(dst.Tokens, srcTokens...)
	return dst
}

func alignStyledCell(line tableLine, width int, align TableAlignment, kind tableSegmentKind) tableLine {
	lineWidth := tableLineWidth(line)
	pad := width - lineWidth
	if pad < 0 {
		pad = 0
	}
	left := 0
	right := pad
	switch align {
	case TableAlignRight:
		left, right = pad, 0
	case TableAlignCenter:
		left = pad / 2
		right = pad - left
	}
	var out tableLine
	if left > 0 {
		out = append(out, tableSegment{Text: strings.Repeat(" ", left), Kind: kind})
	}
	for _, segment := range line {
		segment.Kind = kind
		out = append(out, segment)
	}
	if right > 0 {
		out = append(out, tableSegment{Text: strings.Repeat(" ", right), Kind: kind})
	}
	return out
}

func tableLineWidth(line tableLine) int {
	width := 0
	for _, segment := range line {
		width += ansi.PrintableRuneWidth(segment.Text)
	}
	return width
}

func tableTextSegmentKind(row TableRow) tableSegmentKind {
	if row.Header {
		return tableSegmentHeader
	}
	return tableSegmentText
}

type tableBorderKind uint8

const (
	tableBorderTop tableBorderKind = iota
	tableBorderMiddle
	tableBorderBottom
)

func (l tableLayout) borderLine(kind tableBorderKind) tableLine {
	chars := tableChars(l.wire)
	left, mid, right := chars.topLeft, chars.topMid, chars.topRight
	switch kind {
	case tableBorderMiddle:
		left, mid, right = chars.midLeft, chars.midMid, chars.midRight
	case tableBorderBottom:
		left, mid, right = chars.bottomLeft, chars.bottomMid, chars.bottomRight
	}
	var b strings.Builder
	b.WriteString(left)
	for i, width := range l.widths {
		b.WriteString(strings.Repeat(chars.horizontal, width+2))
		if i == len(l.widths)-1 {
			b.WriteString(right)
		} else {
			b.WriteString(mid)
		}
	}
	return tableLine{{Text: b.String(), Kind: tableSegmentWire}}
}

type tableCharSet struct {
	topLeft     string
	topMid      string
	topRight    string
	midLeft     string
	midMid      string
	midRight    string
	bottomLeft  string
	bottomMid   string
	bottomRight string
	horizontal  string
	vertical    string
}

func tableChars(wire TableWireMode) tableCharSet {
	if wire == TableWireASCII {
		return tableCharSet{
			topLeft: "+", topMid: "+", topRight: "+",
			midLeft: "+", midMid: "+", midRight: "+",
			bottomLeft: "+", bottomMid: "+", bottomRight: "+",
			horizontal: "-", vertical: "|",
		}
	}
	return tableCharSet{
		topLeft: "┌", topMid: "┬", topRight: "┐",
		midLeft: "├", midMid: "┼", midRight: "┤",
		bottomLeft: "└", bottomMid: "┴", bottomRight: "┘",
		horizontal: "─", vertical: "│",
	}
}

func wrapStyledCell(cell TableCell, width int) []tableLine {
	words := tableCellWords(cell)
	if len(words) == 0 {
		return []tableLine{{}}
	}
	if width <= 0 {
		return []tableLine{joinStyledWords(words)}
	}
	var lines []tableLine
	var current tableLine
	currentWidth := 0
	for _, word := range words {
		wordWidth := tableLineWidth(word.segments)
		separatorWidth := tableLineWidth(word.separator)
		if currentWidth > 0 && currentWidth+separatorWidth+wordWidth <= width {
			current = append(current, word.separator...)
			current = append(current, word.segments...)
			currentWidth += separatorWidth + wordWidth
			continue
		}
		if currentWidth > 0 {
			lines = append(lines, current)
			current = nil
			currentWidth = 0
		}
		if word.preserveSeparator && separatorWidth > 0 {
			current = append(current, word.separator...)
			currentWidth += separatorWidth
		}
		if wordWidth <= width {
			current = append(current, word.segments...)
			currentWidth = wordWidth
			if word.preserveSeparator {
				currentWidth += separatorWidth
			}
			continue
		}
		chunks := splitStyledWord(word.segments, width)
		for i, chunk := range chunks {
			if i == len(chunks)-1 {
				current = append(current, chunk...)
				currentWidth = tableLineWidth(chunk)
			} else {
				lines = append(lines, chunk)
			}
		}
	}
	if currentWidth > 0 || len(lines) == 0 {
		lines = append(lines, current)
	}
	return lines
}

func joinStyledWords(words []tableCellWord) tableLine {
	var out tableLine
	for _, word := range words {
		if len(out) > 0 || word.preserveSeparator {
			out = append(out, word.separator...)
		}
		out = append(out, word.segments...)
	}
	return out
}

func tableCellWords(cell TableCell) []tableCellWord {
	tokens := cell.Tokens
	if len(tokens) == 0 && cell.Text != "" {
		tokens = []StreamToken{{Token: Token{Text: cell.Text}}}
	}
	var words []tableCellWord
	var word tableLine
	var separator tableLine
	var linkURL string
	sawWord := false
	for _, tok := range tokens {
		if tok.Kind == tokenLinkStart {
			linkURL = tok.LinkURL
			continue
		}
		if tok.Kind == tokenLinkEnd {
			linkURL = ""
			continue
		}
		if tok.Text == "" {
			continue
		}
		segmentLinkURL := linkURL
		if tok.LinkURL != "" {
			segmentLinkURL = tok.LinkURL
		}
		for _, part := range splitStyledToken(tok) {
			if part.space {
				if len(word) > 0 {
					words = append(words, tableCellWord{separator: separator, segments: word})
					sawWord = true
					word = nil
					separator = nil
				}
				separator = append(separator, tableSegment{Text: part.text, Style: tok.Style, LinkURL: segmentLinkURL})
				continue
			}
			word = append(word, tableSegment{Text: part.text, Style: tok.Style, LinkURL: segmentLinkURL})
		}
	}
	if len(word) > 0 {
		words = append(words, tableCellWord{separator: separator, segments: word, preserveSeparator: !sawWord})
		separator = nil
		sawWord = true
	}
	if len(separator) > 0 && sawWord {
		words = append(words, tableCellWord{segments: separator})
	}
	return words
}

func tableCellPreferredMinWidth(cell TableCell) int {
	width := tableCellBreakMinWidth(cell)
	for _, word := range tableCellWords(cell) {
		if wordWidth := tableLineWidth(word.segments); wordWidth > width {
			width = wordWidth
		}
	}
	return width
}

func tableCellBreakMinWidth(cell TableCell) int {
	width := 0
	for _, word := range tableCellWords(cell) {
		if wordWidth := tableLineNBSPWordWidth(word.segments); wordWidth > width {
			width = wordWidth
		}
		if runeWidth := tableLineMaxRuneWidth(word.segments); runeWidth > width {
			width = runeWidth
		}
	}
	return width
}

func tableColumnSplitCosts(rows []TableRow, cols int, naturalWidths []int) [][]int {
	costs := make([][]int, cols)
	for col := 0; col < cols; col++ {
		maxWidth := widthAt(naturalWidths, col, 1)
		costs[col] = make([]int, maxWidth+1)
		var wordWidths []int
		for _, row := range rows {
			if col >= len(row.Cells) {
				continue
			}
			for _, word := range tableCellWords(row.Cells[col]) {
				if wordWidth := tableLineWidth(word.segments); wordWidth > 0 {
					wordWidths = append(wordWidths, wordWidth)
				}
			}
		}
		for width := range costs[col] {
			for _, wordWidth := range wordWidths {
				if wordWidth <= width {
					continue
				}
				overflow := wordWidth - width
				costs[col][width] += 1000 + overflow*overflow
			}
		}
	}
	return costs
}

func tableLineNBSPWordWidth(line tableLine) int {
	for _, segment := range line {
		if strings.ContainsRune(segment.Text, '\u00A0') {
			return tableLineWidth(line)
		}
	}
	return 0
}

func tableLineMaxRuneWidth(line tableLine) int {
	width := 0
	for _, segment := range line {
		for _, r := range segment.Text {
			if w := ansi.PrintableRuneWidth(string(r)); w > width {
				width = w
			}
		}
	}
	return width
}

type styledTokenPart struct {
	text  string
	space bool
}

func splitStyledToken(tok StreamToken) []styledTokenPart {
	var parts []styledTokenPart
	var b strings.Builder
	inSpace := false
	flush := func() {
		if b.Len() == 0 {
			return
		}
		parts = append(parts, styledTokenPart{text: b.String(), space: inSpace})
		b.Reset()
	}
	for _, r := range tok.Text {
		space := r != '\u00A0' && unicode.IsSpace(r)
		if b.Len() > 0 && space != inSpace {
			flush()
		}
		inSpace = space
		b.WriteRune(r)
	}
	flush()
	return parts
}

func splitStyledWord(word tableLine, width int) []tableLine {
	if width <= 0 {
		return []tableLine{word}
	}
	var chunks []tableLine
	var current tableLine
	currentWidth := 0
	for _, segment := range word {
		for _, r := range segment.Text {
			rw := ansi.PrintableRuneWidth(string(r))
			if currentWidth > 0 && currentWidth+rw > width {
				chunks = append(chunks, current)
				current = nil
				currentWidth = 0
			}
			current = append(current, tableSegment{
				Text:    string(r),
				Style:   segment.Style,
				Kind:    segment.Kind,
				LinkURL: segment.LinkURL,
			})
			currentWidth += rw
		}
	}
	if len(current) > 0 || len(chunks) == 0 {
		chunks = append(chunks, current)
	}
	return chunks
}
