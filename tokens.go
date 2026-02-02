package mdf

// Token is a text segment with a style applied.
type Token struct {
	Text      string
	Style     Style
	Kind      tokenKind
	LinkURL   string
	CodeBlock bool
}

type tokenKind uint8

// TokenKind is the exported alias of tokenKind for tooling and reference renderers.
type TokenKind = tokenKind

const (
	tokenText tokenKind = iota
	tokenLinkStart
	tokenLinkEnd
	tokenURL
	tokenCode
	tokenThematicBreak
)

const (
	// TokenText represents plain text segments.
	TokenText tokenKind = tokenText
	// TokenLinkStart marks the beginning of a link span.
	TokenLinkStart tokenKind = tokenLinkStart
	// TokenLinkEnd marks the end of a link span.
	TokenLinkEnd tokenKind = tokenLinkEnd
	// TokenURL represents a URL within a link.
	TokenURL tokenKind = tokenURL
	// TokenCode represents inline or block code.
	TokenCode tokenKind = tokenCode
	// TokenThematicBreak represents a thematic break token.
	TokenThematicBreak tokenKind = tokenThematicBreak
)
