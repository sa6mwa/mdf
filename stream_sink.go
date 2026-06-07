package mdf

// Stream receives tokens from the streaming parser.
type Stream interface {
	// WriteToken receives one parsed token.
	WriteToken(StreamToken) error
	// Flush completes any buffered output.
	Flush() error
	// Width returns the current target width in terminal cells.
	Width() int
	// SetWidth updates the target width in terminal cells.
	SetWidth(int)
	// SetWrapIndent sets the indentation prefix used for wrapped continuation lines.
	SetWrapIndent(string)
}

// TableStream receives structured table events from the streaming parser.
// Streams that do not implement TableStream receive table source text as
// ordinary tokens instead.
type TableStream interface {
	// StartTable starts a parsed Markdown table.
	StartTable(TableStart) error
	// WriteTableRow writes one parsed Markdown table row.
	WriteTableRow(TableRow) error
	// EndTable ends the current parsed Markdown table.
	EndTable() error
}
