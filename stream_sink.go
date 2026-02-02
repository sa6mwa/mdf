package mdf

// Stream receives tokens from the streaming parser.
type Stream interface {
	WriteToken(StreamToken) error
	Flush() error
	Width() int
	SetWidth(int)
	SetWrapIndent(string)
}
