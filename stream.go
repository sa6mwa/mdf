package mdf

import (
	"bytes"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/muesli/reflow/ansi"
)

// StreamToken represents a styled inference token with timing.
type StreamToken struct {
	Token
	Delay time.Duration
}

// StreamRenderer renders inference tokens to an io.Writer with hard wrapping.
type StreamRenderer struct {
	w                 io.Writer
	width             int
	osc8              bool
	softWrap          bool
	lineWidth         int
	style             string
	pending           wordBuffer
	pendingSpaces     []StreamToken
	lastWordCode      bool
	atLineStart       bool
	lastWasNewline    bool
	wrapIndent        string
	prefixBuf         []byte
	wordScratch       []byte
	indentArena       []byte
	runeScratch       [utf8.UTFMax]byte
	codeFlushPending  bool
	nbspBuf           []atom
	punctQuotePending bool

	pendingAtomsBuf  [512]StreamToken
	pendingSpacesBuf [128]StreamToken
	prefixBufArr     [256]byte
	wordScratchArr   [256]byte
	wordRunesArr     [256]rune
	carryRunesArr    [2]rune
	indentArenaArr   [256]byte
	nbspBufArr       [6]atom

	wordRunes  []rune
	carryRunes []rune
}

// NewStreamRenderer creates a streaming renderer.
func NewStreamRenderer(w io.Writer, width int, opts ...RenderOption) *StreamRenderer {
	cfg := renderConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	s := &StreamRenderer{}
	s.resetWithConfig(w, width, cfg)
	return s
}

// Reset clears stream state for reuse with a new writer or width.
func (s *StreamRenderer) Reset(w io.Writer, width int) {
	cfg := renderConfig{osc8: s.osc8, softWrap: s.softWrap}
	s.resetWithConfig(w, width, cfg)
}

func (s *StreamRenderer) resetWithConfig(w io.Writer, width int, cfg renderConfig) {
	s.initBuffers()
	s.w = w
	s.width = width
	s.osc8 = cfg.osc8
	s.softWrap = cfg.softWrap
	s.lineWidth = 0
	s.style = ""
	s.pending.atoms = s.pendingAtomsBuf[:0]
	s.pending.reset()
	s.pendingSpaces = s.pendingSpacesBuf[:0]
	s.lastWordCode = false
	s.atLineStart = true
	s.lastWasNewline = true
	s.wrapIndent = ""
	s.prefixBuf = s.prefixBufArr[:0]
	s.wordScratch = s.wordScratchArr[:0]
	s.wordRunes = s.wordRunesArr[:0]
	s.carryRunes = s.carryRunesArr[:0]
	s.indentArena = s.indentArenaArr[:0]
	s.codeFlushPending = false
	s.nbspBuf = s.nbspBufArr[:0]
	s.punctQuotePending = false
}

func (s *StreamRenderer) initBuffers() {
	if s.pending.atoms == nil {
		s.pending.atoms = s.pendingAtomsBuf[:0]
	}
	if s.pendingSpaces == nil {
		s.pendingSpaces = s.pendingSpacesBuf[:0]
	}
	if s.prefixBuf == nil {
		s.prefixBuf = s.prefixBufArr[:0]
	}
	if s.wordScratch == nil {
		s.wordScratch = s.wordScratchArr[:0]
	}
	if s.wordRunes == nil {
		s.wordRunes = s.wordRunesArr[:0]
	}
	if s.carryRunes == nil {
		s.carryRunes = s.carryRunesArr[:0]
	}
	if s.indentArena == nil {
		s.indentArena = s.indentArenaArr[:0]
	}
	if s.nbspBuf == nil {
		s.nbspBuf = s.nbspBufArr[:0]
	}
}

func (s *StreamRenderer) setWrapIndent(indent string) {
	if indent == "" {
		return
	}
	s.wrapIndent = indent
}

// Width returns the configured wrap width.
func (s *StreamRenderer) Width() int {
	return s.width
}

// SetWidth updates the wrap width.
func (s *StreamRenderer) SetWidth(width int) {
	s.width = width
}

// SetWrapIndent updates the wrap indentation for continued lines.
func (s *StreamRenderer) SetWrapIndent(indent string) {
	s.setWrapIndent(indent)
}

// WriteToken writes a single inference token, honoring its delay.
func (s *StreamRenderer) WriteToken(tok StreamToken) error {
	if tok.Kind == tokenLinkStart || tok.Kind == tokenLinkEnd {
		return s.writeLinkToken(tok)
	}
	if tok.Kind == tokenThematicBreak {
		if len(s.pending.atoms) > 0 {
			s.flushWord(boundaryNone)
		} else if len(s.pendingSpaces) > 0 {
			_ = s.emitAtoms(s.pendingSpaces)
			s.pendingSpaces = s.pendingSpaces[:0]
		}
		return nil
	}
	if tok.Text == "" {
		return nil
	}
	if tok.Kind == tokenCode && len(s.pending.atoms) > 0 && s.pending.kind != tokenCode {
		if !(s.pending.kind == tokenText && s.pending.endsWithOpenBracket()) {
			s.flushWord(boundaryNone)
		}
	}
	count := 0
	if tok.Delay > 0 {
		count = utf8.RuneCountInString(tok.Text)
	}
	if count == 0 {
		count = 1
	}
	per := tok.Delay / time.Duration(count)
	rem := tok.Delay - per*time.Duration(count)
	first := true
	for i := 0; i < len(tok.Text); {
		r, size := utf8.DecodeRuneInString(tok.Text[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if isControlRune(r) {
			i += size
			continue
		}
		next := rune(0)
		if i+size < len(tok.Text) {
			if nr, nsize := utf8.DecodeRuneInString(tok.Text[i+size:]); !(nr == utf8.RuneError && nsize == 1) {
				next = nr
			}
		}
		d := per
		if first {
			d += rem
			first = false
		}
		part := tok.Text[i : i+size]
		a := atom{
			StreamToken: StreamToken{
				Token: Token{Text: part, Style: tok.Style, Kind: tok.Kind},
				Delay: d,
			},
			boundary: classifyBoundary(r, next),
		}
		if a.boundary == boundaryPunct && next == 0 {
			a.boundary = boundaryPunctEnd
		}
		if err := s.processAtom(a); err != nil {
			return err
		}
		i += size
	}
	if tok.Kind == tokenCode && tok.CodeBlock {
		s.flushWord(boundaryNone)
		s.codeFlushPending = false
	} else if tok.Kind == tokenCode {
		s.codeFlushPending = true
	}
	return nil
}

// Flush resets the style at the end of a stream.
func (s *StreamRenderer) Flush() error {
	if len(s.nbspBuf) > 0 {
		s.flushNBSPBuf()
	}
	if s.punctQuotePending {
		s.flushWord(boundaryNone)
		s.punctQuotePending = false
	}
	if len(s.pending.atoms) > 0 {
		s.flushWord(boundaryNone)
	} else if len(s.pendingSpaces) > 0 {
		_ = s.emitAtoms(s.pendingSpaces)
		s.pendingSpaces = s.pendingSpaces[:0]
	}
	if s.style != "" {
		_, err := io.WriteString(s.w, ansiReset)
		s.style = ""
		if err != nil {
			return err
		}
	}
	if !s.lastWasNewline {
		_, _ = io.WriteString(s.w, "\n")
		s.lastWasNewline = true
	}
	return nil
}

type atom struct {
	StreamToken
	boundary boundaryKind
}

type boundaryKind uint8

const (
	boundaryNone boundaryKind = iota
	boundarySpace
	boundaryNewline
	boundaryPunct
	boundaryPunctEnd
)

func classifyBoundary(r rune, next rune) boundaryKind {
	if r == '\n' {
		return boundaryNewline
	}
	if r == '\u00A0' {
		return boundaryNone
	}
	if r == ' ' || r == '\t' {
		return boundarySpace
	}
	if isQuote(r) {
		return boundaryNone
	}
	if r == '.' {
		if unicode.IsUpper(next) {
			return boundaryPunct
		}
		return boundaryNone
	}
	if strings.ContainsRune(".,;:!?", r) {
		if isQuote(next) {
			return boundaryNone
		}
		return boundaryPunct
	}
	return boundaryNone
}

func isQuote(r rune) bool {
	switch r {
	case '"', '\'', '“', '”', '‘', '’':
		return true
	default:
		return false
	}
}

func isQuoteAtom(a atom) bool {
	if a.Text == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(a.Text)
	return isQuote(r)
}

func isLinePrefixBytes(buf []byte) bool {
	trim := bytes.TrimLeft(buf, " \t")
	trim = bytes.TrimRight(trim, " ")
	if len(trim) == 0 {
		return true
	}
	start := 0
	for start < len(trim) {
		for start < len(trim) && (trim[start] == ' ' || trim[start] == '\t') {
			start++
		}
		if start >= len(trim) {
			break
		}
		end := start
		for end < len(trim) && trim[end] != ' ' && trim[end] != '\t' {
			end++
		}
		if !isPrefixToken(trim[start:end]) {
			return false
		}
		start = end
	}
	return true
}

func isPrefixToken(tok []byte) bool {
	if len(tok) == 0 {
		return true
	}
	if tok[0] == '#' {
		for i := 0; i < len(tok); i++ {
			if tok[i] != '#' {
				return false
			}
		}
		return true
	}
	if len(tok) == 1 {
		switch tok[0] {
		case '>', '-', '*', '+':
			return true
		}
		return false
	}
	if tok[0] >= '0' && tok[0] <= '9' {
		i := 0
		for i < len(tok) && tok[i] >= '0' && tok[i] <= '9' {
			i++
		}
		if i == len(tok) {
			return true
		}
		if i < len(tok) && (tok[i] == '.' || tok[i] == ')') && i+1 == len(tok) {
			return true
		}
		return false
	}
	return false
}

func taskListWrapIndent(prefix []byte) (int, bool) {
	i := 0
	for i < len(prefix) && (prefix[i] == ' ' || prefix[i] == '\t') {
		i++
	}
	if i >= len(prefix) {
		return 0, false
	}
	j := i
	if prefix[j] == '-' || prefix[j] == '+' || prefix[j] == '*' {
		j++
	} else if prefix[j] >= '0' && prefix[j] <= '9' {
		for j < len(prefix) && prefix[j] >= '0' && prefix[j] <= '9' {
			j++
		}
		if j >= len(prefix) || (prefix[j] != '.' && prefix[j] != ')') {
			return 0, false
		}
		j++
	} else {
		return 0, false
	}
	if j >= len(prefix) || prefix[j] != ' ' {
		return 0, false
	}
	j++
	if j+3 >= len(prefix) {
		return 0, false
	}
	if prefix[j] != '[' {
		return 0, false
	}
	if prefix[j+2] != ']' {
		return 0, false
	}
	if prefix[j+1] != ' ' && prefix[j+1] != 'x' && prefix[j+1] != 'X' {
		return 0, false
	}
	if prefix[j+3] != ' ' {
		return 0, false
	}
	return j + 4, true
}

type wordBuffer struct {
	atoms   []StreamToken
	kind    tokenKind
	width   int
	delay   time.Duration
	style   Style
	hasURL  bool
	hasNon  bool
	hasCode bool
	last    rune
}

func (w *wordBuffer) reset() {
	w.atoms = w.atoms[:0]
	w.kind = tokenText
	w.width = 0
	w.delay = 0
	w.style = Style{}
	w.hasURL = false
	w.hasNon = false
	w.hasCode = false
	w.last = 0
}

func (w *wordBuffer) appendAtom(a StreamToken) {
	if len(w.atoms) == 0 {
		w.kind = a.Kind
		w.style = a.Style
	}
	if a.Kind == tokenURL {
		w.kind = tokenURL
		w.hasURL = true
	}
	if a.Kind == tokenCode {
		w.kind = tokenCode
		w.hasCode = true
		w.hasNon = true
	}
	if a.Kind != tokenURL && a.Kind != tokenCode {
		w.hasNon = true
	}
	if a.Text != "" {
		if r, _ := utf8.DecodeLastRuneInString(a.Text); r != utf8.RuneError {
			w.last = r
		}
	}
	w.atoms = append(w.atoms, a)
	w.width += ansi.PrintableRuneWidth(a.Text)
	w.delay += a.Delay
}

func (w *wordBuffer) endsWithOpenBracket() bool {
	return w.last == '(' || w.last == '[' || w.last == '{'
}

func (s *StreamRenderer) processAtom(a atom) error {
	if s.handleNBSPAtom(a) {
		return nil
	}
	return s.processAtomRaw(a)
}

func (s *StreamRenderer) processAtomRaw(a atom) error {
	if s.punctQuotePending {
		if isQuoteAtom(a) {
			s.punctQuotePending = false
			a.boundary = boundaryNone
		} else {
			s.flushWord(boundaryNone)
			s.punctQuotePending = false
		}
	}
	if s.codeFlushPending && a.boundary != boundaryPunct && a.boundary != boundaryPunctEnd {
		s.flushWord(boundaryNone)
		s.codeFlushPending = false
	}
	if a.Kind == tokenCode && a.boundary == boundarySpace {
		s.pending.appendAtom(a.StreamToken)
		return nil
	}
	if a.boundary == boundaryNone {
		s.pending.appendAtom(a.StreamToken)
		return nil
	}
	if a.boundary == boundaryPunct || a.boundary == boundaryPunctEnd {
		if a.Kind == tokenURL || a.Kind == tokenCode {
			s.pending.appendAtom(a.StreamToken)
			return nil
		}
		if a.Kind == tokenText {
			if len(s.pending.atoms) > 0 && s.pending.kind == tokenCode && s.width > 0 && s.lineWidth > 0 {
				spacesWidth := 0
				for _, sp := range s.pendingSpaces {
					spacesWidth += ansi.PrintableRuneWidth(sp.Text)
				}
				if s.lineWidth+spacesWidth+s.pending.width+ansi.PrintableRuneWidth(a.Text) > s.width {
					s.wrapNewline()
					s.pendingSpaces = s.pendingSpaces[:0]
				}
			}
			if s.lastWordCode && len(s.pending.atoms) == 0 && len(s.pendingSpaces) == 0 {
				s.lastWordCode = false
				return s.emitText(a.Text, a.Style)
			}
			if len(s.pending.atoms) == 0 && len(s.pendingSpaces) == 0 {
				return s.emitText(a.Text, a.Style)
			}
			if len(s.pending.atoms) > 0 {
				if s.pending.kind != tokenURL {
					s.pending.appendAtom(a.StreamToken)
					if a.boundary == boundaryPunctEnd {
						s.punctQuotePending = true
						s.codeFlushPending = false
						return nil
					}
					s.flushWord(boundaryNone)
					s.codeFlushPending = false
					return nil
				}
				s.flushWord(boundaryNone)
			}
			s.pending.appendAtom(a.StreamToken)
			if a.boundary == boundaryPunctEnd {
				s.punctQuotePending = true
				s.codeFlushPending = false
				return nil
			}
			s.flushWord(boundaryNone)
			s.codeFlushPending = false
			return nil
		}
		s.pending.appendAtom(a.StreamToken)
		s.codeFlushPending = false
		return nil
	}
	if a.boundary == boundarySpace {
		s.flushWord(boundaryNone)
		s.lastWordCode = false
		if s.atLineStart {
			if !isLinePrefixBytes(s.prefixBuf) {
				s.atLineStart = false
				s.pendingSpaces = append(s.pendingSpaces, a.StreamToken)
				return nil
			}
			if a.Delay > 0 {
				time.Sleep(a.Delay)
			}
			if a.Style.Prefix != s.style {
				if s.style != "" {
					_, _ = io.WriteString(s.w, ansiReset)
				}
				s.style = a.Style.Prefix
				if s.style != "" {
					_, _ = io.WriteString(s.w, s.style)
				}
			}
			_, _ = io.WriteString(s.w, a.Text)
			s.lineWidth += ansi.PrintableRuneWidth(a.Text)
			s.prefixBuf = append(s.prefixBuf, a.Text...)
			s.maybeSetWrapIndent()
			if s.style != "" {
				_, _ = io.WriteString(s.w, ansiReset)
				s.style = ""
			}
			if bytes.Contains(s.prefixBuf, []byte(" ")) && !isLinePrefixBytes(s.prefixBuf) {
				s.atLineStart = false
			}
			return nil
		}
		s.pendingSpaces = append(s.pendingSpaces, a.StreamToken)
		return nil
	}
	// newline
	s.flushWord(boundaryNone)
	s.lastWordCode = false
	if a.Delay > 0 {
		time.Sleep(a.Delay)
	}
	s.emitBoundary(boundaryNewline)
	return nil
}

func (s *StreamRenderer) handleNBSPAtom(a atom) bool {
	if a.Kind == tokenCode {
		if len(s.nbspBuf) > 0 {
			s.flushNBSPBuf()
		}
		return false
	}
	if len(s.nbspBuf) == 0 {
		if a.Text == "&" {
			s.nbspBuf = append(s.nbspBuf, a)
			return true
		}
		return false
	}
	if !isNBSPCompatible(a, s.nbspBuf[0]) || !isNBSPChar(a.Text) {
		s.flushNBSPBuf()
		return false
	}
	s.nbspBuf = append(s.nbspBuf, a)
	if len(s.nbspBuf) < 6 {
		return true
	}
	if len(s.nbspBuf) == 6 && isNBSPEntity(s.nbspBuf) {
		first := s.nbspBuf[0]
		delay := time.Duration(0)
		for _, b := range s.nbspBuf {
			delay += b.Delay
		}
		nb := atom{
			StreamToken: StreamToken{
				Token: Token{Text: "\u00A0", Style: first.Style, Kind: first.Kind},
				Delay: delay,
			},
			boundary: boundaryNone,
		}
		s.nbspBuf = s.nbspBuf[:0]
		_ = s.processAtomRaw(nb)
		return true
	}
	s.flushNBSPBuf()
	return true
}

func (s *StreamRenderer) flushNBSPBuf() {
	for _, b := range s.nbspBuf {
		_ = s.processAtomRaw(b)
	}
	s.nbspBuf = s.nbspBuf[:0]
}

func isNBSPCompatible(a atom, first atom) bool {
	return a.Kind == first.Kind && a.Style.Prefix == first.Style.Prefix
}

func isNBSPChar(text string) bool {
	if len(text) == 0 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text)
	if r == '&' || r == '#' || r == ';' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	switch r {
	case 'n', 'N', 'b', 'B', 's', 'S', 'p', 'P', 'x', 'X', 'a', 'A':
		return true
	}
	return false
}

func isNBSPEntity(buf []atom) bool {
	if len(buf) != 6 {
		return false
	}
	var b strings.Builder
	for _, a := range buf {
		b.WriteString(a.Text)
	}
	seq := strings.ToLower(b.String())
	return seq == "&nbsp;" || seq == "&#160;" || seq == "&#xa0;"
}

func (s *StreamRenderer) writeLinkToken(tok StreamToken) error {
	if len(s.pending.atoms) > 0 {
		s.flushWord(boundaryNone)
	} else if len(s.pendingSpaces) > 0 {
		_ = s.emitAtoms(s.pendingSpaces)
		s.pendingSpaces = s.pendingSpaces[:0]
	}
	if !s.osc8 {
		return nil
	}
	if tok.Delay > 0 {
		time.Sleep(tok.Delay)
	}
	if tok.Kind == tokenLinkStart {
		if tok.LinkURL != "" {
			_, err := io.WriteString(s.w, osc8Start+tok.LinkURL+"\x1b\\")
			return err
		}
		return nil
	}
	_, err := io.WriteString(s.w, osc8End)
	return err
}

func (s *StreamRenderer) flushWord(boundary boundaryKind) {
	if len(s.pending.atoms) == 0 {
		s.emitBoundary(boundary)
		return
	}
	wordWidth := s.pending.width
	spacesWidth := 0
	for _, sp := range s.pendingSpaces {
		spacesWidth += ansi.PrintableRuneWidth(sp.Text)
	}
	if s.width > 0 && s.lineWidth > 0 && s.lineWidth+spacesWidth+wordWidth > s.width {
		prefixWidth := ansi.PrintableRuneWidth(bytesToString(s.prefixBuf))
		if !(prefixWidth > 0 && s.lineWidth == prefixWidth && spacesWidth == 0) {
			s.wrapNewline()
			s.pendingSpaces = s.pendingSpaces[:0]
		}
	}
	if len(s.pendingSpaces) > 0 {
		_ = s.emitAtoms(s.pendingSpaces)
		s.pendingSpaces = s.pendingSpaces[:0]
	}
	if s.width > 0 && wordWidth > s.width {
		wordText := s.wordTextFromAtoms(s.pending.atoms)
		if s.pending.kind == tokenCode || s.pending.hasCode {
			s.emitTimedCodeSplit(wordText, s.pending.style, s.pending.delay, s.width)
		} else {
			s.emitOverlongWord(wordText, s.pending)
		}
	} else {
		_ = s.emitAtoms(s.pending.atoms)
	}
	s.lastWordCode = s.pending.kind == tokenCode || s.pending.hasCode
	s.pending.reset()
	s.emitBoundary(boundary)
}

func (s *StreamRenderer) emitBoundary(boundary boundaryKind) {
	switch boundary {
	case boundaryNewline:
		if len(s.pendingSpaces) > 0 {
			_ = s.emitAtoms(s.pendingSpaces)
			s.pendingSpaces = s.pendingSpaces[:0]
		}
		s.newline(true)
	case boundaryPunct:
	case boundaryPunctEnd:
		// punctuation emitted as part of word
	}
}

func (s *StreamRenderer) emitOverlongWord(wordText string, pending wordBuffer) {
	if pending.kind == tokenURL {
		if pending.hasNon {
			s.emitTimedWordSplit(wordText, pending.style, pending.delay, s.width)
			return
		}
		if prefix, url, suffix, ok := splitURLWrapper(wordText); ok {
			available := s.width - ansi.PrintableRuneWidth(prefix) - ansi.PrintableRuneWidth(suffix)
			if available > 0 {
				fitted := fitURL(url, available)
				text := prefix + fitted + suffix
				s.emitTimedText(text, pending.style, pending.delay)
				s.lineWidth += ansi.PrintableRuneWidth(text)
				return
			}
		}
		fitted := fitURL(wordText, s.width)
		s.emitTimedText(fitted, pending.style, pending.delay)
		s.lineWidth += ansi.PrintableRuneWidth(fitted)
		return
	}
	if pending.kind == tokenCode {
		s.emitTimedCodeSplit(wordText, pending.style, pending.delay, s.width)
		return
	}
	s.emitTimedWordSplit(wordText, pending.style, pending.delay, s.width)
}

func (s *StreamRenderer) emitTimedWordSplit(text string, style Style, delay time.Duration, limit int) {
	if text == "" || limit <= 0 {
		return
	}
	totalRunes := utf8.RuneCountInString(text)
	if totalRunes == 0 {
		return
	}
	per := delay / time.Duration(totalRunes)
	rem := delay - per*time.Duration(totalRunes)
	if cap(s.wordRunes) < limit+1 {
		s.wordRunes = make([]rune, 0, limit+1)
	}
	s.carryRunes = s.carryRunes[:0]
	consumed := 0
	chunkIndex := 0
	for i := 0; i < len(text) || len(s.carryRunes) > 0; {
		s.wordRunes = s.wordRunes[:0]
		if len(s.carryRunes) > 0 {
			s.wordRunes = append(s.wordRunes, s.carryRunes...)
			s.carryRunes = s.carryRunes[:0]
		}
		for len(s.wordRunes) < limit && i < len(text) {
			r, size := utf8.DecodeRuneInString(text[i:])
			if r == utf8.RuneError && size == 1 {
				r = rune(text[i])
				size = 1
			}
			s.wordRunes = append(s.wordRunes, r)
			i += size
			consumed++
		}
		if len(s.wordRunes) == 0 {
			break
		}
		remaining := totalRunes - consumed
		if remaining == 1 && i < len(text) && len(s.wordRunes) > 1 {
			next, _ := utf8.DecodeRuneInString(text[i:])
			if isQuote(next) {
				s.carryRunes = append(s.carryRunes, s.wordRunes[len(s.wordRunes)-1])
				s.wordRunes = s.wordRunes[:len(s.wordRunes)-1]
			}
		}
		if i < len(text) && len(s.wordRunes) > 1 {
			last := s.wordRunes[len(s.wordRunes)-1]
			if isQuote(last) {
				s.carryRunes = append(s.carryRunes, last)
				s.wordRunes = s.wordRunes[:len(s.wordRunes)-1]
			}
		}
		if chunkIndex > 0 {
			s.wrapNewline()
		}
		chunkIndex++
		for _, r := range s.wordRunes {
			d := per
			if rem > 0 {
				d += 1 * time.Nanosecond
				rem -= 1 * time.Nanosecond
			}
			if d > 0 {
				time.Sleep(d)
			}
			_ = s.emitRune(r, style)
		}
	}
}

func (s *StreamRenderer) emitTimedCodeSplit(text string, style Style, delay time.Duration, limit int) {
	if text == "" || limit <= 0 {
		return
	}
	totalRunes := 0
	segmentStart := 0
	for idx, r := range text {
		if strings.ContainsRune("(){}[]<>.,;:/\\", r) {
			end := idx + utf8.RuneLen(r)
			totalRunes += codeSegmentRunes(text[segmentStart:end], limit)
			segmentStart = end
		}
	}
	if segmentStart < len(text) {
		totalRunes += codeSegmentRunes(text[segmentStart:], limit)
	}
	if totalRunes == 0 {
		return
	}
	per := delay / time.Duration(totalRunes)
	rem := delay - per*time.Duration(totalRunes)
	segmentStart = 0
	for idx, r := range text {
		if strings.ContainsRune("(){}[]<>.,;:/\\", r) {
			end := idx + utf8.RuneLen(r)
			segment := text[segmentStart:end]
			if s.width > 0 && s.lineWidth > 0 && s.lineWidth+ansi.PrintableRuneWidth(segment) > limit {
				if !s.lineHasOnlyWhitespacePrefix() {
					s.wrapNewline()
				}
			}
			s.emitCodeSegment(segment, limit, style, per, &rem)
			segmentStart = end
		}
	}
	if segmentStart < len(text) {
		segment := text[segmentStart:]
		if s.width > 0 && s.lineWidth > 0 && s.lineWidth+ansi.PrintableRuneWidth(segment) > limit {
			if !s.lineHasOnlyWhitespacePrefix() {
				s.wrapNewline()
			}
		}
		s.emitCodeSegment(segment, limit, style, per, &rem)
	}
}

func codeSegmentRunes(segment string, limit int) int {
	if limit <= 0 {
		return 0
	}
	if ansi.PrintableRuneWidth(segment) <= limit {
		return utf8.RuneCountInString(segment)
	}
	if limit == 1 {
		return 1
	}
	return limit
}

func (s *StreamRenderer) emitCodeSegment(segment string, limit int, style Style, per time.Duration, rem *time.Duration) {
	if limit <= 0 {
		return
	}
	if ansi.PrintableRuneWidth(segment) <= limit {
		s.emitRunesWithDelay(segment, style, per, rem)
		return
	}
	if limit == 1 {
		s.emitRuneWithDelay('…', style, per, rem)
		return
	}
	count := 0
	for _, r := range segment {
		if count >= limit-1 {
			break
		}
		s.emitRuneWithDelay(r, style, per, rem)
		count++
	}
	s.emitRuneWithDelay('…', style, per, rem)
}

func (s *StreamRenderer) emitRunesWithDelay(text string, style Style, per time.Duration, rem *time.Duration) {
	for _, r := range text {
		s.emitRuneWithDelay(r, style, per, rem)
	}
}

func (s *StreamRenderer) emitRuneWithDelay(r rune, style Style, per time.Duration, rem *time.Duration) {
	d := per
	if *rem > 0 {
		d += 1 * time.Nanosecond
		*rem -= 1 * time.Nanosecond
	}
	if d > 0 {
		time.Sleep(d)
	}
	_ = s.emitRune(r, style)
}

func (s *StreamRenderer) wrapNewline() {
	if s.style != "" {
		_, _ = io.WriteString(s.w, ansiReset)
		s.style = ""
	}
	s.newline(false)
	s.emitIndent()
}

func (s *StreamRenderer) wordTextFromAtoms(atoms []StreamToken) string {
	if len(atoms) == 0 {
		return ""
	}
	total := 0
	for _, a := range atoms {
		total += len(a.Text)
	}
	if cap(s.wordScratch) < total {
		s.wordScratch = make([]byte, 0, total)
	}
	s.wordScratch = s.wordScratch[:0]
	for _, a := range atoms {
		s.wordScratch = append(s.wordScratch, a.Text...)
	}
	return bytesToString(s.wordScratch)
}

func (s *StreamRenderer) emitAtoms(atoms []StreamToken) error {
	for _, a := range atoms {
		if a.Delay > 0 {
			time.Sleep(a.Delay)
		}
		if err := s.emitText(a.Text, a.Style); err != nil {
			return err
		}
	}
	return nil
}

func (s *StreamRenderer) emitTimedText(text string, style Style, delay time.Duration) {
	totalRunes := utf8.RuneCountInString(text)
	if totalRunes == 0 {
		return
	}
	per := delay / time.Duration(totalRunes)
	rem := delay - per*time.Duration(totalRunes)
	for _, r := range text {
		d := per
		if rem > 0 {
			d += 1 * time.Nanosecond
			rem -= 1 * time.Nanosecond
		}
		if d > 0 {
			time.Sleep(d)
		}
		_ = s.emitRune(r, style)
	}
}

func (s *StreamRenderer) emitRune(r rune, style Style) error {
	if r >= 0 && r < 128 {
		return s.emitText(asciiRuneStrings[r], style)
	}
	buf := s.runeScratch[:0]
	buf = utf8.AppendRune(buf, r)
	return s.emitText(bytesToString(buf), style)
}

func (s *StreamRenderer) emitText(text string, style Style) error {
	if text == "" {
		return nil
	}
	if strings.ContainsRune(text, '\u00A0') {
		text = strings.ReplaceAll(text, "\u00A0", " ")
	}
	s.lastWasNewline = strings.HasSuffix(text, "\n")
	if s.atLineStart {
		s.prefixBuf = append(s.prefixBuf, text...)
		s.maybeSetWrapIndent()
		if s.wrapIndent != "" && len(s.prefixBuf) > 0 && s.prefixBuf[len(s.prefixBuf)-1] == ' ' {
			if s.style != "" {
				_, _ = io.WriteString(s.w, ansiReset)
				s.style = ""
			}
		}
		if bytes.IndexByte(s.prefixBuf, ' ') >= 0 && !isLinePrefixBytes(s.prefixBuf) {
			s.atLineStart = false
		}
	}
	if style.Prefix != s.style {
		if s.style != "" {
			_, _ = io.WriteString(s.w, ansiReset)
		}
		s.style = style.Prefix
		if s.style != "" {
			_, _ = io.WriteString(s.w, s.style)
		}
	}
	_, err := io.WriteString(s.w, text)
	s.lineWidth += ansi.PrintableRuneWidth(text)
	return err
}

func (s *StreamRenderer) newline(resetStyle bool) {
	if resetStyle && s.style != "" {
		_, _ = io.WriteString(s.w, ansiReset)
		s.style = ""
	}
	_, _ = io.WriteString(s.w, "\n")
	s.lineWidth = 0
	s.lastWasNewline = true
	if resetStyle {
		s.atLineStart = true
		s.wrapIndent = ""
		s.prefixBuf = s.prefixBuf[:0]
		return
	}
	s.atLineStart = false
}

func (s *StreamRenderer) emitIndent() {
	if s.wrapIndent == "" {
		return
	}
	_, _ = io.WriteString(s.w, s.wrapIndent)
	s.lineWidth += ansi.PrintableRuneWidth(s.wrapIndent)
	s.atLineStart = false
}

func (s *StreamRenderer) maybeSetWrapIndent() {
	if indent, ok := taskListWrapIndent(s.prefixBuf); ok {
		s.wrapIndent = s.indentSpaces(indent)
		return
	}
	if s.wrapIndent != "" {
		return
	}
	buf := s.prefixBuf
	nonSpace := -1
	for i, b := range buf {
		if b != ' ' && b != '\t' {
			nonSpace = i
			break
		}
	}
	if nonSpace == -1 {
		return
	}
	spaceIdx := -1
	for i := nonSpace; i < len(buf); i++ {
		if buf[i] == ' ' {
			spaceIdx = i
			break
		}
	}
	if spaceIdx == -1 {
		return
	}
	prefix := buf[:spaceIdx+1]
	trim := bytes.TrimLeft(prefix, " ")
	trim = bytes.TrimRight(trim, " ")
	if len(trim) > 0 && trim[0] == '#' {
		allHashes := true
		for _, b := range trim {
			if b != '#' {
				allHashes = false
				break
			}
		}
		if allHashes {
			s.wrapIndent = s.indentSpaces(len(prefix))
			return
		}
	}
	if len(trim) == 1 && (trim[0] == '>' || trim[0] == '-' || trim[0] == '*' || trim[0] == '+') {
		if trim[0] == '>' {
			s.wrapIndent = s.indentFromBytes(prefix)
		} else {
			s.wrapIndent = s.indentSpaces(len(prefix))
		}
		return
	}
	if len(trim) >= 2 && trim[0] >= '0' && trim[0] <= '9' {
		i := 0
		for i < len(trim) && trim[i] >= '0' && trim[i] <= '9' {
			i++
		}
		if i < len(trim) && (trim[i] == '.' || trim[i] == ')') {
			s.wrapIndent = s.indentSpaces(len(prefix))
		}
	}
}

func (s *StreamRenderer) indentSpaces(count int) string {
	if count <= 0 {
		return ""
	}
	if count <= len(spaceString) {
		return spaceString[:count]
	}
	start := len(s.indentArena)
	remaining := count
	for remaining > 0 {
		n := remaining
		if n > len(spaceString) {
			n = len(spaceString)
		}
		s.indentArena = append(s.indentArena, spaceString[:n]...)
		remaining -= n
	}
	return bytesToString(s.indentArena[start:len(s.indentArena)])
}

func (s *StreamRenderer) indentFromBytes(prefix []byte) string {
	if len(prefix) == 0 {
		return ""
	}
	start := len(s.indentArena)
	s.indentArena = append(s.indentArena, prefix...)
	return bytesToString(s.indentArena[start:len(s.indentArena)])
}

func (s *StreamRenderer) lineHasOnlyWhitespacePrefix() bool {
	if s.lineWidth == 0 || len(s.prefixBuf) == 0 {
		return false
	}
	for _, b := range s.prefixBuf {
		if b != ' ' && b != '\t' {
			return false
		}
	}
	return s.lineWidth == ansi.PrintableRuneWidth(bytesToString(s.prefixBuf))
}
