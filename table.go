package mdf

// TableAlignment describes horizontal alignment for a markdown table column.
type TableAlignment uint8

const (
	// TableAlignLeft aligns cell content to the left.
	TableAlignLeft TableAlignment = iota
	// TableAlignRight aligns cell content to the right.
	TableAlignRight
	// TableAlignCenter centers cell content.
	TableAlignCenter
)

// TableBufferMode controls how much table data renderers buffer before
// emitting output.
type TableBufferMode uint8

const (
	// TableBufferDefault uses the renderer's default table buffering mode.
	TableBufferDefault TableBufferMode = iota
	// TableBufferFull buffers the full table before rendering. This gives the
	// renderer all rows for column sizing and page/fragment planning.
	TableBufferFull
	// TableBufferRow buffers only enough rows to establish the table layout,
	// then emits rows as they arrive. Later rows are fit to the established
	// column count and width.
	TableBufferRow
)

// TableWireMode controls the visible separators used for text-like table
// renderers.
type TableWireMode uint8

const (
	// TableWireLine uses Unicode box drawing separators.
	TableWireLine TableWireMode = iota
	// TableWireASCII uses ASCII separators.
	TableWireASCII
	// TableWireSpace uses spacing without visible separator lines.
	TableWireSpace
)

// TableStart describes a table before its rows are streamed.
type TableStart struct {
	// Alignments contains one alignment per declared table column.
	Alignments []TableAlignment
	// HeaderStyle is the semantic style for header text.
	HeaderStyle Style
	// WireStyle is the semantic style for borders, separators, and padding.
	WireStyle Style
	// Prefix is emitted before every rendered table line for contained tables
	// such as blockquote or list-item tables.
	Prefix []TablePrefixSegment
}

// TablePrefixSegment is fixed text emitted before every rendered table line.
type TablePrefixSegment struct {
	// Text is the prefix text.
	Text string
	// Style is the style applied to Text.
	Style Style
}

// TableCell contains the parsed content for one table cell.
type TableCell struct {
	// Text is the plain cell text with surrounding table-cell padding removed.
	Text string
	// Tokens contains inline Markdown tokens parsed from Text.
	Tokens []StreamToken
}

// TableRow contains one parsed markdown table row.
type TableRow struct {
	// Cells contains the parsed cells for this row.
	Cells []TableCell
	// Header is true for the table header row.
	Header bool
}
