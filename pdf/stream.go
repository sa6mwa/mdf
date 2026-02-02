package pdf

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/muesli/reflow/ansi"
	"pkt.systems/mdf"
	"pkt.systems/mdf/pdf/gofpdf"
)

const (
	tokenText                    = 0
	tokenLinkStart               = 1
	tokenLinkEnd                 = 2
	tokenURL                     = 3
	tokenCode                    = 4
	tokenThematicBreak           = 5
	headingSpaceBeforeMultiplier = 0.35
	headingSpaceAfterMultiplier  = 1.3
)

type pdfStream struct {
	pdf                 *gofpdf.Fpdf
	cfg                 Config
	styles              mdf.Styles
	styleCache          map[string]pdfStyle
	printStyleCache     map[string]pdfStyle
	width               int
	lineWidth           float64
	pending             wordBuffer
	pendingSpaces       []mdf.StreamToken
	atLineStart         bool
	wrapIndent          string
	wrapIndentWidth     float64
	wrapIndentUseWidth  bool
	wrapIndentPrefix    string
	wrapIndentPrefixSt  mdf.Style
	lastStylePrefix     string
	lastPDFStyle        pdfStyle
	lastStyleSet        bool
	lastListIndentWidth float64
	lastListIndentCols  int
	listIndentActive    bool
	prefixBuf           []byte
	prefixWidth         float64
	inQuoteLine         bool
	pendingIndent       bool

	charWidth             float64
	x                     float64
	y                     float64
	lineHeight            float64
	baseLineHeight        float64
	pageW                 float64
	pageH                 float64
	currentLink           string
	headingLevel          int
	cornerImage           *cornerImage
	cornerImageBottom     float64
	pageNum               int
	skipLeadingNewline    bool
	pendingHeadingBuf     []byte
	headingJustEnded      bool
	headingBlankConsumed  bool
	headingSpacingApplied bool
	lastHeadingSize       float64
	lastHeadingLineHeight float64
	lineHadHeading        bool
	headingPending        bool
	headingBuf            strings.Builder
	headingStyle          mdf.Style
	headingMarker         string
	layers                pdfLayers
	nbspBuf               []atom
	punctQuotePending     bool
}

type wordBuffer struct {
	atoms []mdf.StreamToken
	kind  uint8
	text  strings.Builder
	delay time.Duration
	style mdf.Style
}

type cornerImage struct {
	path   string
	opts   gofpdf.ImageOptions
	width  float64
	height float64
}

type pdfLayers struct {
	enabled   bool
	viewBg    int
	viewText  int
	printText int
	image     int
}

func newPDFStream(pdf *gofpdf.Fpdf, cfg Config, styles mdf.Styles, width int, charWidth float64, corner *cornerImage, layers pdfLayers) *pdfStream {
	s := &pdfStream{
		pdf:             pdf,
		cfg:             cfg,
		styles:          styles,
		styleCache:      make(map[string]pdfStyle),
		printStyleCache: make(map[string]pdfStyle),
		width:           width,
		pending:         wordBuffer{atoms: make([]mdf.StreamToken, 0, 64)},
		pendingSpaces:   make([]mdf.StreamToken, 0, 16),
		atLineStart:     true,
		prefixBuf:       make([]byte, 0, 64),
		charWidth:       charWidth,
		cornerImage:     corner,
		layers:          layers,
	}
	s.nbspBuf = make([]atom, 0, 6)
	s.punctQuotePending = false
	s.pending.text.Grow(64)
	s.pageW, s.pageH = pdf.GetPageSize()
	s.baseLineHeight = cfg.FontSize * cfg.LineHeight
	s.lineHeight = s.baseLineHeight
	if s.cornerImage != nil {
		pad := cfg.CornerImagePadding
		s.cornerImageBottom = cfg.Margin + s.cornerImage.height + pad
	}
	s.addPage()
	return s
}

func (s *pdfStream) addPage() {
	s.pdf.AddPage()
	s.pageNum++
	s.pdf.SetFillColor(s.cfg.BackgroundRGB[0], s.cfg.BackgroundRGB[1], s.cfg.BackgroundRGB[2])
	if s.cfg.BackgroundEnabled {
		if s.layers.enabled {
			s.pdf.BeginLayer(s.layers.viewBg)
			s.pdf.Rect(0, 0, s.pageW, s.pageH, "F")
			s.pdf.EndLayer()
		} else {
			s.pdf.Rect(0, 0, s.pageW, s.pageH, "F")
		}
	}
	if s.cornerImage != nil && s.pageNum == 1 {
		x := s.pageW - s.cfg.Margin - s.cornerImage.width
		y := s.cfg.Margin
		if s.layers.enabled {
			s.pdf.BeginLayer(s.layers.image)
		}
		s.pdf.ImageOptions(s.cornerImage.path, x, y, s.cornerImage.width, s.cornerImage.height, false, s.cornerImage.opts, 0, "")
		if s.layers.enabled {
			s.pdf.EndLayer()
		}
	}
	s.x = s.cfg.Margin
	s.y = s.cfg.Margin + s.cfg.FontSize
	s.lineHeight = s.baseLineHeight
	s.atLineStart = true
	s.wrapIndent = ""
	s.wrapIndentWidth = 0
	s.wrapIndentUseWidth = false
	s.wrapIndentPrefix = ""
	s.wrapIndentPrefixSt = mdf.Style{}
	s.lastListIndentWidth = 0
	s.lastListIndentCols = 0
	s.listIndentActive = false
	s.prefixBuf = s.prefixBuf[:0]
	s.prefixWidth = 0
	s.inQuoteLine = false
	s.pendingIndent = false
	s.lineWidth = 0
	s.pendingHeadingBuf = s.pendingHeadingBuf[:0]
	s.headingJustEnded = false
	s.headingBlankConsumed = false
	s.headingSpacingApplied = false
	s.lastHeadingSize = 0
	s.lastHeadingLineHeight = 0
	s.lineHadHeading = false
	s.headingPending = false
	s.headingBuf.Reset()
	s.headingMarker = ""
	s.lastStylePrefix = ""
	s.lastStyleSet = false
}

func (s *pdfStream) Width() int {
	return s.width
}

func (s *pdfStream) SetWidth(width int) {
	s.width = width
}

func (s *pdfStream) SetWrapIndent(indent string) {
	if indent == "" {
		s.lastListIndentWidth = 0
		s.lastListIndentCols = 0
		s.listIndentActive = false
		return
	}
	plain := stripANSICodes(indent)
	s.wrapIndent = plain
	s.wrapIndentUseWidth = false
	s.wrapIndentWidth = 0
	if strings.Contains(plain, ">") {
		s.wrapIndentPrefix = plain
		s.wrapIndentPrefixSt = s.styles.Quote
	} else {
		s.wrapIndentPrefix = ""
		s.wrapIndentPrefixSt = mdf.Style{}
		s.wrapIndentUseWidth = true
		s.wrapIndentWidth = s.charWidth * float64(textColumns(plain))
	}
}

func (s *pdfStream) WriteToken(tok mdf.StreamToken) error {
	if tok.Kind == tokenThematicBreak {
		if len(s.pending.atoms) > 0 {
			s.flushWord(boundaryNone)
		} else if len(s.pendingSpaces) > 0 {
			s.emitAtoms(s.pendingSpaces)
			s.pendingSpaces = s.pendingSpaces[:0]
		}
		s.pageBreak()
		return nil
	}
	if tok.Kind == tokenCode && tok.CodeBlock {
		if len(s.pending.atoms) > 0 {
			s.flushWord(boundaryNone)
		} else if len(s.pendingSpaces) > 0 {
			s.emitAtoms(s.pendingSpaces)
			s.pendingSpaces = s.pendingSpaces[:0]
		}
		s.emitCodeBlockText(tok.Text, tok.Style)
		return nil
	}
	if tok.Kind == tokenLinkStart {
		if len(s.pending.atoms) > 0 {
			s.flushWord(boundaryNone)
		} else if len(s.pendingSpaces) > 0 {
			s.emitAtoms(s.pendingSpaces)
			s.pendingSpaces = s.pendingSpaces[:0]
		}
		s.currentLink = tok.LinkURL
		return nil
	}
	if tok.Kind == tokenLinkEnd {
		if s.currentLink != "" {
			if len(s.pending.atoms) > 0 {
				s.flushWord(boundaryNone)
			} else if len(s.pendingSpaces) > 0 {
				s.emitAtoms(s.pendingSpaces)
				s.pendingSpaces = s.pendingSpaces[:0]
			}
		}
		s.currentLink = ""
		return nil
	}
	if s.skipLeadingNewline {
		if tok.Kind == tokenText && isOnlyNewline(tok.Text) {
			return nil
		}
		s.skipLeadingNewline = false
	}
	if tok.Text == "" {
		return nil
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
		if r > 0xFFFF {
			i += size
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
			StreamToken: mdf.StreamToken{
				Token: mdf.Token{Text: part, Style: tok.Style, Kind: tok.Kind},
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
	return nil
}

func (s *pdfStream) Flush() error {
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
		s.emitAtoms(s.pendingSpaces)
		s.pendingSpaces = s.pendingSpaces[:0]
	}
	return nil
}

func isControlRune(r rune) bool {
	if r == '\n' || r == '\r' || r == '\t' {
		return false
	}
	if r < 0x20 || r == 0x7F {
		return true
	}
	return false
}

type atom struct {
	mdf.StreamToken
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

func (w *wordBuffer) reset() {
	w.atoms = w.atoms[:0]
	w.kind = 0
	w.text.Reset()
	w.delay = 0
	w.style = mdf.Style{}
}

func (w *wordBuffer) appendAtom(a mdf.StreamToken) {
	if len(w.atoms) == 0 {
		w.kind = uint8(a.Kind)
		w.style = a.Style
	}
	if uint8(a.Kind) == tokenURL {
		w.kind = tokenURL
	}
	if uint8(a.Kind) == tokenCode {
		w.kind = tokenCode
	}
	w.atoms = append(w.atoms, a)
	w.text.WriteString(a.Text)
	w.delay += a.Delay
}

func (s *pdfStream) processAtom(a atom) error {
	if s.handleNBSPAtom(a) {
		return nil
	}
	return s.processAtomRaw(a)
}

func (s *pdfStream) processAtomRaw(a atom) error {
	if s.punctQuotePending {
		if isQuoteAtom(a) {
			s.punctQuotePending = false
			a.boundary = boundaryNone
		} else {
			s.flushWord(boundaryNone)
			s.punctQuotePending = false
		}
	}
	if uint8(a.Kind) == tokenCode && a.boundary == boundarySpace {
		s.pending.appendAtom(a.StreamToken)
		return nil
	}
	if a.boundary == boundaryNone {
		s.pending.appendAtom(a.StreamToken)
		return nil
	}
	if a.boundary == boundaryPunct || a.boundary == boundaryPunctEnd {
		if uint8(a.Kind) == tokenURL {
			s.pending.appendAtom(a.StreamToken)
			return nil
		}
		s.pending.appendAtom(a.StreamToken)
		if a.boundary == boundaryPunctEnd {
			s.punctQuotePending = true
			return nil
		}
		s.flushWord(boundaryNone)
		return nil
	}
	if a.boundary == boundarySpace {
		s.flushWord(boundaryNone)
		if s.atLineStart {
			if !isLinePrefixBytes(s.prefixBuf) {
				s.atLineStart = false
				s.pendingSpaces = append(s.pendingSpaces, a.StreamToken)
				return nil
			}
			s.emitAtoms([]mdf.StreamToken{a.StreamToken})
			s.prefixBuf = append(s.prefixBuf, a.Text...)
			s.maybeSetWrapIndent()
			if bytes.Contains(s.prefixBuf, []byte(" ")) && !isLinePrefixBytes(s.prefixBuf) {
				s.atLineStart = false
			}
			return nil
		}
		s.pendingSpaces = append(s.pendingSpaces, a.StreamToken)
		return nil
	}
	s.flushWord(boundaryNone)
	s.emitBoundary(boundaryNewline)
	return nil
}

func (s *pdfStream) handleNBSPAtom(a atom) bool {
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
	if !isNBSPCompatiblePDF(a, s.nbspBuf[0]) || !isNBSPChar(a.Text) {
		s.flushNBSPBuf()
		return false
	}
	s.nbspBuf = append(s.nbspBuf, a)
	if len(s.nbspBuf) < 6 {
		return true
	}
	if len(s.nbspBuf) == 6 && isNBSPEntityPDF(s.nbspBuf) {
		first := s.nbspBuf[0]
		delay := time.Duration(0)
		for _, b := range s.nbspBuf {
			delay += b.Delay
		}
		nb := atom{
			StreamToken: mdf.StreamToken{
				Token: mdf.Token{Text: "\u00A0", Style: first.Style, Kind: first.Kind},
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

func (s *pdfStream) flushNBSPBuf() {
	for _, b := range s.nbspBuf {
		_ = s.processAtomRaw(b)
	}
	s.nbspBuf = s.nbspBuf[:0]
}

func isNBSPCompatiblePDF(a atom, first atom) bool {
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

func (s *pdfStream) flushWord(boundary boundaryKind) {
	if len(s.pending.atoms) == 0 {
		s.emitBoundary(boundary)
		return
	}
	wordText := s.pending.text.String()
	wordWidth := s.measureText(wordText, s.pending.style)
	tempLink := ""
	if s.currentLink == "" && s.pending.kind == tokenURL {
		tempLink = linkTargetForURL(wordText)
	}
	spacesWidth := 0.0
	for _, sp := range s.pendingSpaces {
		spacesWidth += s.measureText(sp.Text, sp.Style)
	}
	lineLimit := s.lineLimit()
	if lineLimit > 0 && s.lineWidth > 0 {
		limit := lineLimit
		if s.pending.kind == tokenCode {
			limit += s.charWidth * 1.1
		}
		if s.lineWidth+spacesWidth+wordWidth > limit {
			s.wrapNewline()
			s.pendingSpaces = s.pendingSpaces[:0]
		}
	}
	if len(s.pendingSpaces) > 0 {
		s.emitAtoms(s.pendingSpaces)
		s.pendingSpaces = s.pendingSpaces[:0]
	}
	if tempLink != "" {
		prev := s.currentLink
		s.currentLink = tempLink
		if lineLimit > 0 && wordWidth > lineLimit {
			s.emitOverlongWord(wordText, s.pending, lineLimit)
		} else {
			s.emitAtoms(s.pending.atoms)
		}
		s.currentLink = prev
	} else {
		if lineLimit > 0 && wordWidth > lineLimit {
			s.emitOverlongWord(wordText, s.pending, lineLimit)
		} else {
			s.emitAtoms(s.pending.atoms)
		}
	}
	s.pending.reset()
	s.emitBoundary(boundary)
}

func (s *pdfStream) emitBoundary(boundary boundaryKind) {
	switch boundary {
	case boundaryNewline:
		if s.headingPending {
			s.renderHeadingBlock()
			return
		}
		if len(s.pendingSpaces) > 0 {
			s.emitAtoms(s.pendingSpaces)
			s.pendingSpaces = s.pendingSpaces[:0]
		}
		if s.headingLevel > 0 {
			s.headingJustEnded = true
			s.headingBlankConsumed = false
		}
		if s.headingJustEnded {
			if s.headingBlankConsumed {
				s.headingJustEnded = false
				return
			}
			s.headingBlankConsumed = true
			if s.lastHeadingSize > 0 {
				after := s.lastHeadingSize * headingSpaceAfterMultiplier
				if after > s.baseLineHeight {
					s.lineHeight = after
				} else {
					s.lineHeight = s.baseLineHeight
				}
			} else {
				s.lineHeight = s.baseLineHeight
			}
			s.lastHeadingLineHeight = 0
			s.lineHadHeading = false
			s.newline(true)
			return
		}
		s.newline(true)
	case boundaryPunct:
	}
}

func (s *pdfStream) emitOverlongWord(wordText string, pending wordBuffer, lineLimit float64) {
	availableCols := s.availableCols(lineLimit, pending.style)
	if pending.kind == tokenURL {
		if prefix, url, suffix, ok := splitURLWrapper(wordText); ok {
			available := availableCols - textColumns(prefix) - textColumns(suffix)
			if available > 0 {
				fitted := fitURL(url, available)
				text := prefix + fitted + suffix
				s.emitText(text, pending.style)
				return
			}
		}
		fitted := fitURL(wordText, availableCols)
		s.emitText(fitted, pending.style)
		return
	}
	if pending.kind == tokenCode {
		s.emitCodeSegments(wordText, pending.style, lineLimit)
		return
	}
	parts := splitWordToWidth(wordText, availableCols)
	if len(parts) == 1 {
		s.emitText(parts[0], pending.style)
		return
	}
	s.emitParts(parts, pending.style)
}

func (s *pdfStream) emitCodeSegments(text string, style mdf.Style, lineLimit float64) {
	if text == "" || lineLimit <= 0 {
		return
	}
	segments := splitByDelimiters(text, "(){}[]<>.,;:/\\")
	if len(segments) == 0 {
		cols := s.availableCols(lineLimit, style)
		s.emitText(truncateWithEllipsis(text, cols), style)
		return
	}
	for _, seg := range segments {
		segWidth := s.measureText(seg, style)
		if segWidth > lineLimit {
			cols := s.availableCols(lineLimit, style)
			seg = truncateWithEllipsis(seg, cols)
			segWidth = s.measureText(seg, style)
		}
		if s.lineWidth > 0 && s.lineWidth+segWidth > lineLimit {
			if !s.lineHasOnlyPrefix() {
				s.wrapNewline()
			}
		}
		s.emitText(seg, style)
	}
}

func (s *pdfStream) emitCodeBlockText(text string, style mdf.Style) {
	if text == "" {
		return
	}
	lineLimit := s.lineLimit()
	emitChunk := func(chunk string) {
		if chunk == "" {
			return
		}
		if s.pendingIndent {
			s.pendingIndent = false
			s.emitIndent()
		}
		s.emitTextDirect(chunk, style)
	}
	emitWrapped := func(line string) {
		if line == "" {
			return
		}
		if s.atLineStart {
			s.maybeSetWrapIndent()
			s.inQuoteLine = lineHasQuotePrefix(s.prefixBuf)
			if s.wrapIndent != "" {
				s.wrapIndentWidth = s.prefixWidth
				s.wrapIndentUseWidth = true
			}
			s.atLineStart = false
		}
		if lineLimit <= 0 {
			emitChunk(line)
			return
		}
		baseLimit := lineLimit
		if s.lineWidth == 0 && s.wrapIndentUseWidth && s.wrapIndentWidth > 0 {
			baseLimit -= s.wrapIndentWidth
		}
		if s.lineWidth+s.measureText(line, style) <= baseLimit {
			emitChunk(line)
			return
		}
		splitByWidth := func(text string, maxWidth float64) (string, string) {
			if text == "" {
				return "", ""
			}
			if maxWidth <= 0 {
				r, size := utf8.DecodeRuneInString(text)
				if r == utf8.RuneError && size == 1 {
					return text[:1], text[1:]
				}
				return text[:size], text[size:]
			}
			width := 0.0
			i := 0
			for i < len(text) {
				r, size := utf8.DecodeRuneInString(text[i:])
				if r == utf8.RuneError && size == 1 {
					break
				}
				w := s.pdf.GetStringWidth(text[i : i+size])
				if w <= 0 {
					w = s.charWidth
					if w <= 0 {
						w = 1
					}
				}
				if width+w > maxWidth {
					break
				}
				width += w
				i += size
			}
			if i == 0 {
				r, size := utf8.DecodeRuneInString(text)
				if r == utf8.RuneError && size == 1 {
					return text[:1], text[1:]
				}
				return text[:size], text[size:]
			}
			return text[:i], text[i:]
		}
		for len(line) > 0 {
			if baseLimit-s.lineWidth <= 0 {
				s.wrapNewline()
			}
			chunk, rest := splitByWidth(line, baseLimit-s.lineWidth)
			emitChunk(chunk)
			line = rest
			if line != "" {
				s.wrapNewline()
			}
		}
	}
	start := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			s.flushWord(boundaryNone)
			return
		}
		if r == '\n' {
			line := stripCarriageReturn(text[start:i])
			emitWrapped(line)
			s.newline(true)
			start = i + size
			i += size
			continue
		}
		i += size
	}
	if start < len(text) {
		emitWrapped(stripCarriageReturn(text[start:]))
	}
}

func linkTargetForURL(text string) string {
	if text == "" {
		return ""
	}
	if strings.Contains(text, "@") && !strings.Contains(text, "://") && !strings.Contains(text, ":") {
		return "mailto:" + text
	}
	return text
}

func (s *pdfStream) emitParts(parts []string, style mdf.Style) {
	if len(parts) == 0 {
		return
	}
	for i, part := range parts {
		if i > 0 {
			s.wrapNewline()
		}
		s.emitText(part, style)
	}
}

func (s *pdfStream) wrapNewline() {
	if s.headingLevel > 0 {
		idx := s.headingLevel - 1
		if idx >= 0 && idx < len(s.cfg.HeadingScale) {
			s.lastHeadingLineHeight = s.cfg.FontSize * s.cfg.HeadingScale[idx] * s.cfg.LineHeight
		}
		s.lineHadHeading = true
	}
	s.newline(false)
	if s.pendingIndent {
		return
	}
	s.emitIndent()
}

func (s *pdfStream) renderHeadingBlock() {
	text := s.headingBuf.String()
	marker := s.headingMarker
	style := s.headingStyle
	s.headingPending = false
	s.headingBuf.Reset()
	s.headingMarker = ""
	pstyle := s.styleForPrefix(style.Prefix, s.headingLevel)
	lineHeight := pstyle.size * s.cfg.LineHeight
	s.lastHeadingLineHeight = lineHeight

	before := 0.0
	if s.y > s.cfg.Margin+s.cfg.FontSize {
		before = pstyle.size * headingSpaceBeforeMultiplier
	}
	after := s.baseLineHeight
	afterCandidate := pstyle.size * headingSpaceAfterMultiplier
	if afterCandidate > after {
		after = afterCandidate
	}

	lineLimit := s.lineLimit()
	maxCols := s.availableCols(lineLimit, style)
	markerCols := textColumns(marker)
	indentCols := markerCols
	if indentCols < 0 {
		indentCols = 0
	}
	lines := wrapHeadingByCols(text, maxCols, indentCols, markerCols)
	if len(lines) == 0 {
		lines = []string{text}
	}

	total := before + float64(len(lines))*lineHeight + after + 3*s.baseLineHeight
	if s.y+total > s.pageH-s.cfg.Margin {
		s.pageBreak()
	}

	if before > 0 {
		s.y += before
	}
	for i, line := range lines {
		s.x = s.cfg.Margin
		if i == 0 {
			line = marker + line
		} else if indentCols > 0 {
			line = indentSpaces(indentCols) + line
		}
		width := 0.0
		if s.layers.enabled {
			width = s.drawTextLayer(s.layers.viewText, pstyle, line, false)
			printStyle := s.styleForPrefixPrint(style.Prefix, s.headingLevel)
			_ = s.drawTextLayer(s.layers.printText, printStyle, line, false)
		} else {
			s.applyStyle(pstyle)
			s.pdf.Text(s.x, s.y, line)
			width = s.pdf.GetStringWidth(line)
		}
		s.x += width
		s.lineWidth = width
		s.lineHeight = lineHeight
		if i < len(lines)-1 {
			s.y += lineHeight
		}
	}

	s.headingLevel = 0
	s.headingSpacingApplied = false
	s.lineHadHeading = false
	s.headingJustEnded = true
	s.headingBlankConsumed = true
	s.lineHeight = after
	s.newline(true)
}

func (s *pdfStream) emitAtoms(atoms []mdf.StreamToken) {
	for _, a := range atoms {
		s.emitText(a.Text, a.Style)
	}
}

func (s *pdfStream) emitText(text string, style mdf.Style) {
	if text == "" {
		return
	}
	if strings.ContainsRune(text, '\u00A0') {
		text = strings.ReplaceAll(text, "\u00A0", " ")
	}
	if s.pendingIndent {
		s.pendingIndent = false
		s.emitIndent()
	}
	if s.atLineStart {
		s.prefixBuf = append(s.prefixBuf, text...)
		s.maybeSetWrapIndent()
		s.inQuoteLine = lineHasQuotePrefix(s.prefixBuf)
		if bytes.Contains(s.prefixBuf, []byte(" ")) && isLinePrefixBytes(s.prefixBuf) {
			s.noteListIndentPrefix()
		}
		if s.headingLevel == 0 && isHeadingStyle(style, s.styles) {
			linePrefix := isLinePrefixBytes(s.prefixBuf)
			lineHasQuote := linePrefix && lineHasQuotePrefix(s.prefixBuf)
			lineHasList := linePrefix && listPrefixDetected(s.prefixBuf)
			if lineHasQuote || lineHasList {
				s.emitTextDirect(text, style)
				if bytes.Contains(s.prefixBuf, []byte(" ")) && !isLinePrefixBytes(s.prefixBuf) {
					s.atLineStart = false
				}
				return
			}
		}
		if s.headingLevel == 0 && isHeadingStyle(style, s.styles) {
			if len(s.pendingHeadingBuf) > 0 && text != "#" && text != " " {
				buf := string(s.pendingHeadingBuf)
				s.pendingHeadingBuf = s.pendingHeadingBuf[:0]
				s.emitTextDirect(buf, style)
			}
			if text == "#" || text == " " || len(s.pendingHeadingBuf) > 0 {
				s.pendingHeadingBuf = append(s.pendingHeadingBuf, text...)
				if text == " " {
					if level := headingLevelFromPrefixBuf(s.pendingHeadingBuf, style, s.styles); level > 0 {
						s.headingLevel = level
						s.wrapIndent = indentSpaces(len(s.pendingHeadingBuf))
						s.wrapIndentUseWidth = true
						s.wrapIndentWidth = s.charWidth * float64(textColumns(s.wrapIndent))
						s.wrapIndentPrefix = ""
						s.wrapIndentPrefixSt = mdf.Style{}
						pstyle := s.styleForPrefix(style.Prefix, s.headingLevel)
						s.lastHeadingLineHeight = pstyle.size * s.cfg.LineHeight
						s.headingPending = true
						s.headingStyle = style
						s.headingBuf.Reset()
						s.headingMarker = ""
					}
					buf := string(s.pendingHeadingBuf)
					s.pendingHeadingBuf = s.pendingHeadingBuf[:0]
					if s.headingPending {
						s.headingMarker = buf
					} else {
						s.emitTextDirect(buf, style)
					}
					if bytes.Contains(s.prefixBuf, []byte(" ")) && !isLinePrefixBytes(s.prefixBuf) {
						s.atLineStart = false
					}
					return
				}
				return
			}
		}
		if bytes.Contains(s.prefixBuf, []byte(" ")) && !isLinePrefixBytes(s.prefixBuf) {
			s.atLineStart = false
		}
	}
	if s.headingPending && s.headingLevel > 0 {
		s.headingBuf.WriteString(text)
		return
	}
	s.emitTextDirect(text, style)
}

func (s *pdfStream) applyStyle(style pdfStyle) {
	family := s.cfg.FontFamily
	if style.fontFamily != "" {
		family = style.fontFamily
	}
	s.pdf.SetFont(family, style.fontStyle, style.size)
	s.pdf.SetTextColor(style.r, style.g, style.b)
}

func (s *pdfStream) lineLimit() float64 {
	limit := s.pageW - 2*s.cfg.Margin
	if s.cornerImage != nil && s.pageNum == 1 && s.y < s.cornerImageBottom {
		limit -= s.cornerImage.width + s.cfg.CornerImagePadding
	}
	if limit < 1 {
		return 1
	}
	return limit
}

func (s *pdfStream) pageBreak() {
	s.pending.reset()
	s.pendingSpaces = s.pendingSpaces[:0]
	s.atLineStart = true
	s.wrapIndent = ""
	s.wrapIndentWidth = 0
	s.wrapIndentUseWidth = false
	s.wrapIndentPrefix = ""
	s.wrapIndentPrefixSt = mdf.Style{}
	s.lastListIndentWidth = 0
	s.lastListIndentCols = 0
	s.listIndentActive = false
	s.prefixBuf = s.prefixBuf[:0]
	s.prefixWidth = 0
	s.inQuoteLine = false
	s.pendingIndent = false
	s.lineWidth = 0
	s.lineHeight = s.baseLineHeight
	s.skipLeadingNewline = true
	s.pendingHeadingBuf = s.pendingHeadingBuf[:0]
	s.headingJustEnded = false
	s.headingBlankConsumed = false
	s.headingSpacingApplied = false
	s.lastHeadingSize = 0
	s.lastHeadingLineHeight = 0
	s.lineHadHeading = false
	s.headingPending = false
	s.headingBuf.Reset()
	s.headingMarker = ""
	s.lastStylePrefix = ""
	s.lastStyleSet = false
	s.addPage()
}

func isOnlyNewline(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] != '\n' && text[i] != '\r' {
			return false
		}
	}
	return len(text) > 0
}

func (s *pdfStream) measureText(text string, style mdf.Style) float64 {
	pstyle := s.styleForPrefix(style.Prefix, s.headingLevel)
	s.applyStyle(pstyle)
	return s.pdf.GetStringWidth(text)
}

func (s *pdfStream) drawTextLayer(layerID int, style pdfStyle, text string, link bool) float64 {
	if s.layers.enabled {
		s.pdf.BeginLayer(layerID)
	}
	s.applyStyle(style)
	s.pdf.Text(s.x, s.y, text)
	width := s.pdf.GetStringWidth(text)
	if link && s.currentLink != "" && width > 0 {
		s.pdf.LinkString(s.x, s.y-style.size, width, style.size*1.1, s.currentLink)
	}
	if s.layers.enabled {
		s.pdf.EndLayer()
	}
	return width
}

func (s *pdfStream) availableCols(limit float64, style mdf.Style) int {
	if limit <= 0 {
		return 0
	}
	pstyle := s.styleForPrefix(style.Prefix, s.headingLevel)
	s.applyStyle(pstyle)
	charWidth := s.pdf.GetStringWidth("M")
	if charWidth <= 0 {
		return 0
	}
	return int(math.Floor(limit / charWidth))
}

func (s *pdfStream) emitTextDirect(text string, style mdf.Style) {
	if strings.ContainsRune(text, '\u00A0') {
		text = strings.ReplaceAll(text, "\u00A0", " ")
	}
	pstyle := s.styleForPrefix(style.Prefix, s.headingLevel)
	if s.headingLevel > 0 {
		s.lastHeadingSize = pstyle.size
		s.lastHeadingLineHeight = pstyle.size * s.cfg.LineHeight
		// heading spacing is applied when the marker is detected
	}
	if s.layers.enabled {
		width := s.drawTextLayer(s.layers.viewText, pstyle, text, s.currentLink != "")
		printStyle := s.styleForPrefixPrint(style.Prefix, s.headingLevel)
		_ = s.drawTextLayer(s.layers.printText, printStyle, text, false)
		if s.atLineStart && isLinePrefixBytes(s.prefixBuf) {
			s.prefixWidth += width
		}
		s.x += width
		s.lineWidth += width
		multiplier := s.cfg.LineHeight
		if pstyle.size*multiplier > s.lineHeight {
			s.lineHeight = pstyle.size * multiplier
		}
		if s.headingLevel > 0 && s.lastHeadingLineHeight > 0 {
			s.lineHeight = s.lastHeadingLineHeight
		}
		if isHeadingStyle(style, s.styles) {
			s.lineHadHeading = true
			s.lastHeadingLineHeight = pstyle.size * s.cfg.LineHeight
		}
		s.lastStylePrefix = style.Prefix
		s.lastPDFStyle = pstyle
		s.lastStyleSet = true
		return
	}
	s.applyStyle(pstyle)
	s.pdf.Text(s.x, s.y, text)
	width := s.pdf.GetStringWidth(text)
	if s.atLineStart && isLinePrefixBytes(s.prefixBuf) {
		s.prefixWidth += width
	}
	if s.currentLink != "" && width > 0 {
		s.pdf.LinkString(s.x, s.y-pstyle.size, width, pstyle.size*1.1, s.currentLink)
	}
	s.x += width
	s.lineWidth += width
	multiplier := s.cfg.LineHeight
	if pstyle.size*multiplier > s.lineHeight {
		s.lineHeight = pstyle.size * multiplier
	}
	if s.headingLevel > 0 && s.lastHeadingLineHeight > 0 {
		s.lineHeight = s.lastHeadingLineHeight
	}
	if isHeadingStyle(style, s.styles) {
		s.lineHadHeading = true
		s.lastHeadingLineHeight = pstyle.size * s.cfg.LineHeight
	}
	s.lastStylePrefix = style.Prefix
	s.lastPDFStyle = pstyle
	s.lastStyleSet = true
}

func (s *pdfStream) lineHasOnlyPrefix() bool {
	if s.lineWidth == 0 {
		return true
	}
	if s.prefixWidth > 0 {
		return s.lineWidth == s.prefixWidth
	}
	return false
}

func isHeadingStyle(style mdf.Style, styles mdf.Styles) bool {
	for i := 0; i < len(styles.Heading); i++ {
		if style.Prefix == styles.Heading[i].Prefix {
			return true
		}
	}
	return false
}

func lineHasQuotePrefix(buf []byte) bool {
	if !isLinePrefixBytes(buf) {
		return false
	}
	for _, b := range buf {
		if b == '>' {
			return true
		}
	}
	return false
}

func (s *pdfStream) noteListIndentPrefix() {
	if s.wrapIndent == "" {
		return
	}
	if lineHasQuotePrefix(s.prefixBuf) {
		s.lastListIndentWidth = 0
		s.lastListIndentCols = 0
		s.listIndentActive = false
		return
	}
	if !listPrefixDetected(s.prefixBuf) {
		s.lastListIndentWidth = 0
		s.lastListIndentCols = 0
		s.listIndentActive = false
		return
	}
	s.lastListIndentWidth = s.prefixWidth
	s.lastListIndentCols = textColumns(s.wrapIndent)
	s.listIndentActive = true
}

func (s *pdfStream) styleForPrefix(prefix string, headingLevel int) pdfStyle {
	key := prefix
	if headingLevel > 0 {
		key = prefix + "#h" + strconv.Itoa(headingLevel)
		if s.inQuoteLine {
			key += "#q"
		}
	}
	if cached, ok := s.styleCache[key]; ok {
		return cached
	}
	attrs := parseANSIPrefix(prefix, s.cfg.TextRGB)
	if s.cfg.IgnoreColors {
		attrs.color = s.cfg.TextRGB
	}
	forceBold := headingLevel > 0
	allowBoldItalic := s.cfg.HeadingFont == "" && (s.cfg.BoldItalicFont != "" || len(s.cfg.BoldItalicFontBytes) > 0)
	fontStyle := styleToFontStyle(attrs, forceBold, allowBoldItalic)
	fontFamily := s.cfg.FontFamily
	if headingLevel > 0 && s.cfg.HeadingFont != "" {
		fontFamily = headingFontFamily
	}
	size := s.cfg.FontSize
	if headingLevel > 0 {
		size = s.cfg.FontSize * s.cfg.HeadingScale[headingLevel-1]
		if s.inQuoteLine {
			size = s.cfg.FontSize
			fontFamily = s.cfg.FontFamily
		}
	}
	color := attrs.color
	style := pdfStyle{
		fontFamily: fontFamily,
		fontStyle:  fontStyle,
		size:       size,
		r:          color[0],
		g:          color[1],
		b:          color[2],
	}
	s.styleCache[key] = style
	return style
}

func (s *pdfStream) styleForPrefixPrint(prefix string, headingLevel int) pdfStyle {
	key := prefix
	if headingLevel > 0 {
		key = prefix + "#p" + strconv.Itoa(headingLevel)
		if s.inQuoteLine {
			key += "#q"
		}
	}
	if cached, ok := s.printStyleCache[key]; ok {
		return cached
	}
	attrs := parseANSIPrefix(prefix, [3]int{0, 0, 0})
	attrs.color = [3]int{0, 0, 0}
	forceBold := headingLevel > 0
	allowBoldItalic := s.cfg.HeadingFont == "" && (s.cfg.BoldItalicFont != "" || len(s.cfg.BoldItalicFontBytes) > 0)
	fontStyle := styleToFontStyle(attrs, forceBold, allowBoldItalic)
	fontFamily := s.cfg.FontFamily
	if headingLevel > 0 && s.cfg.HeadingFont != "" {
		fontFamily = headingFontFamily
	}
	size := s.cfg.FontSize
	if headingLevel > 0 {
		size = s.cfg.FontSize * s.cfg.HeadingScale[headingLevel-1]
		if s.inQuoteLine {
			size = s.cfg.FontSize
			fontFamily = s.cfg.FontFamily
		}
	}
	style := pdfStyle{
		fontFamily: fontFamily,
		fontStyle:  fontStyle,
		size:       size,
		r:          0,
		g:          0,
		b:          0,
	}
	s.printStyleCache[key] = style
	return style
}

func (s *pdfStream) newline(resetStyle bool) {
	s.x = s.cfg.Margin
	if (s.headingLevel > 0 || s.lineHadHeading) && !resetStyle {
		advance := s.lastHeadingLineHeight
		if advance <= 0 && s.headingLevel > 0 {
			idx := s.headingLevel - 1
			if idx >= 0 && idx < len(s.cfg.HeadingScale) {
				advance = s.cfg.FontSize * s.cfg.HeadingScale[idx] * s.cfg.LineHeight
			}
		}
		if advance > 0 {
			s.y += advance
		} else {
			s.y += s.lineHeight
		}
	} else {
		s.y += s.lineHeight
	}
	s.lineWidth = 0
	s.lineHeight = s.baseLineHeight
	if s.y+s.lineHeight > s.pageH-s.cfg.Margin {
		if resetStyle {
			s.addPage()
			return
		}
		wrapIndent := s.wrapIndent
		wrapIndentWidth := s.wrapIndentWidth
		wrapIndentUseWidth := s.wrapIndentUseWidth
		wrapIndentPrefix := s.wrapIndentPrefix
		wrapIndentPrefixSt := s.wrapIndentPrefixSt
		s.addPage()
		s.wrapIndent = wrapIndent
		s.wrapIndentWidth = wrapIndentWidth
		s.wrapIndentUseWidth = wrapIndentUseWidth
		s.wrapIndentPrefix = wrapIndentPrefix
		s.wrapIndentPrefixSt = wrapIndentPrefixSt
		s.pendingIndent = true
		s.atLineStart = true
		return
	}
	if resetStyle {
		s.atLineStart = true
		s.wrapIndent = ""
		s.wrapIndentWidth = 0
		s.wrapIndentUseWidth = false
		s.wrapIndentPrefix = ""
		s.wrapIndentPrefixSt = mdf.Style{}
		s.prefixBuf = s.prefixBuf[:0]
		s.prefixWidth = 0
		s.inQuoteLine = false
		s.pendingIndent = false
		s.headingLevel = 0
		s.pendingHeadingBuf = s.pendingHeadingBuf[:0]
		s.headingSpacingApplied = false
		s.lastHeadingLineHeight = 0
		s.lineHadHeading = false
		s.lastStylePrefix = ""
		s.lastStyleSet = false
		return
	}
	s.atLineStart = false
}

func (s *pdfStream) emitIndent() {
	if s.wrapIndent == "" {
		return
	}
	if s.wrapIndentPrefix != "" {
		pstyle := s.styleForPrefix(s.wrapIndentPrefixSt.Prefix, 0)
		width := s.drawTextLayer(s.layers.viewText, pstyle, s.wrapIndentPrefix, false)
		if s.layers.enabled {
			printStyle := s.styleForPrefixPrint(s.wrapIndentPrefixSt.Prefix, 0)
			_ = s.drawTextLayer(s.layers.printText, printStyle, s.wrapIndentPrefix, false)
		}
		s.x += width
		s.lineWidth += width
		s.atLineStart = false
		return
	}
	if s.lastStyleSet {
		width := s.drawTextLayer(s.layers.viewText, s.lastPDFStyle, s.wrapIndent, false)
		if s.layers.enabled {
			printStyle := s.styleForPrefixPrint(s.lastStylePrefix, s.headingLevel)
			_ = s.drawTextLayer(s.layers.printText, printStyle, s.wrapIndent, false)
		}
		s.x += width
		s.lineWidth += width
		multiplier := s.cfg.LineHeight
		if s.lastPDFStyle.size*multiplier > s.lineHeight {
			s.lineHeight = s.lastPDFStyle.size * multiplier
		}
		s.atLineStart = false
		return
	}
	cols := textColumns(s.wrapIndent)
	width := s.charWidth * float64(cols)
	if s.wrapIndentUseWidth {
		width = s.wrapIndentWidth
		if width <= 0 {
			width = s.charWidth * float64(cols)
		}
	}
	s.x += width
	s.lineWidth += width
	s.atLineStart = false
}

func (s *pdfStream) maybeSetWrapIndent() {
	if indent, ok := taskListWrapIndent(s.prefixBuf); ok {
		s.wrapIndent = indentSpaces(indent)
		s.wrapIndentUseWidth = true
		s.wrapIndentWidth = s.charWidth * float64(textColumns(s.wrapIndent))
		s.wrapIndentPrefix = ""
		s.wrapIndentPrefixSt = mdf.Style{}
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
		level := 0
		allHashes := true
		for _, b := range trim {
			if b != '#' {
				allHashes = false
				break
			}
			level++
		}
		if allHashes && level > 0 && level <= 6 {
			s.wrapIndent = indentSpaces(len(prefix))
			s.wrapIndentPrefix = ""
			s.wrapIndentPrefixSt = mdf.Style{}
			s.wrapIndentUseWidth = true
			s.wrapIndentWidth = s.charWidth * float64(textColumns(s.wrapIndent))
			return
		}
	}
	if len(trim) == 1 && (trim[0] == '>' || trim[0] == '-' || trim[0] == '*' || trim[0] == '+') {
		if trim[0] == '>' {
			s.wrapIndent = indentFromBytes(prefix)
			s.wrapIndentPrefix = s.wrapIndent
			s.wrapIndentPrefixSt = s.styles.Quote
			s.wrapIndentUseWidth = false
			s.wrapIndentWidth = 0
		} else {
			s.wrapIndent = indentSpaces(len(prefix))
			s.wrapIndentPrefix = ""
			s.wrapIndentPrefixSt = mdf.Style{}
			s.wrapIndentUseWidth = true
			s.wrapIndentWidth = s.charWidth * float64(textColumns(s.wrapIndent))
		}
		return
	}
	if len(trim) >= 2 && trim[0] >= '0' && trim[0] <= '9' {
		i := 0
		for i < len(trim) && trim[i] >= '0' && trim[i] <= '9' {
			i++
		}
		if i < len(trim) && (trim[i] == '.' || trim[i] == ')') {
			s.wrapIndent = indentSpaces(len(prefix))
			s.wrapIndentPrefix = ""
			s.wrapIndentPrefixSt = mdf.Style{}
			s.wrapIndentUseWidth = true
			s.wrapIndentWidth = s.charWidth * float64(textColumns(s.wrapIndent))
		}
	}
}

func headingLevelFromPrefixBuf(buf []byte, style mdf.Style, styles mdf.Styles) int {
	space := bytes.IndexByte(buf, ' ')
	if space == -1 || space == 0 {
		return 0
	}
	i := 0
	for i < space && (buf[i] == ' ' || buf[i] == '\t') {
		i++
	}
	if i >= space {
		return 0
	}
	level := 0
	for i < space && buf[i] == '#' {
		level++
		i++
	}
	if level < 1 || level > 6 || i != space {
		return 0
	}
	if style.Prefix != styles.Heading[level-1].Prefix {
		return 0
	}
	return level
}

func isLinePrefixBytes(buf []byte) bool {
	trim := bytes.TrimLeft(buf, " \t")
	trim = bytes.TrimRight(trim, " ")
	if len(trim) == 0 {
		return true
	}
	fields := bytes.Fields(trim)
	if len(fields) == 0 {
		return true
	}
	for _, field := range fields {
		if !isPrefixToken(field) {
			return false
		}
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
		case '>', '-', '*', '+', '[', ']':
			return true
		}
		return false
	}
	if len(tok) == 2 && tok[0] == '[' && tok[1] == ']' {
		return true
	}
	if len(tok) == 3 && tok[0] == '[' && tok[2] == ']' {
		switch tok[1] {
		case ' ', 'x', 'X':
			return true
		}
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

func listPrefixDetected(prefix []byte) bool {
	trim := bytes.TrimSpace(prefix)
	if len(trim) == 0 {
		return false
	}
	fields := bytes.Fields(trim)
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		if len(field) == 1 {
			switch field[0] {
			case '-', '+', '*':
				return true
			}
		}
		if field[0] >= '0' && field[0] <= '9' {
			i := 0
			for i < len(field) && field[i] >= '0' && field[i] <= '9' {
				i++
			}
			if i < len(field) && (field[i] == '.' || field[i] == ')') && i+1 == len(field) {
				return true
			}
		}
	}
	return false
}

func indentSpaces(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.Repeat(" ", count)
}

func isNBSPEntityPDF(buf []atom) bool {
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

func indentFromBytes(prefix []byte) string {
	if len(prefix) == 0 {
		return ""
	}
	return string(prefix)
}

func wrapHeadingByCols(text string, maxCols int, indentCols int, markerCols int) []string {
	if maxCols <= 0 {
		return []string{text}
	}
	if indentCols < 0 {
		indentCols = 0
	}
	firstLimit := maxCols - markerCols
	if firstLimit <= 0 {
		firstLimit = maxCols
	}
	nextLimit := maxCols - indentCols
	if nextLimit <= 0 {
		nextLimit = maxCols
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	lines := make([]string, 0, 4)
	limit := firstLimit
	var current strings.Builder
	cols := 0
	flush := func() {
		if current.Len() == 0 {
			return
		}
		lines = append(lines, current.String())
		current.Reset()
		cols = 0
		limit = nextLimit
	}
	for _, w := range words {
		wcols := textColumns(w)
		if cols == 0 {
			if wcols > limit {
				parts := splitWordToWidth(w, limit)
				for i, part := range parts {
					if i == 0 {
						current.WriteString(part)
						cols = textColumns(part)
						flush()
						continue
					}
					current.WriteString(part)
					cols = textColumns(part)
					flush()
				}
				continue
			}
			current.WriteString(w)
			cols = wcols
			continue
		}
		if cols+1+wcols <= limit {
			current.WriteByte(' ')
			current.WriteString(w)
			cols += 1 + wcols
			continue
		}
		flush()
		if wcols > limit {
			parts := splitWordToWidth(w, limit)
			for _, part := range parts {
				current.WriteString(part)
				cols = textColumns(part)
				flush()
			}
			continue
		}
		current.WriteString(w)
		cols = wcols
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func stripANSICodes(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) || s[i+1] != '[' {
			continue
		}
		i += 2
		for i < len(s) {
			c := s[i]
			if c >= 0x40 && c <= 0x7e {
				break
			}
			i++
		}
	}
	return b.String()
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

func textColumns(text string) int {
	return ansi.PrintableRuneWidth(text)
}

func stripCarriageReturn(text string) string {
	if text == "" || strings.IndexByte(text, '\r') == -1 {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == '\r' {
			i += size
			continue
		}
		b.WriteString(text[i : i+size])
		i += size
	}
	return b.String()
}

func fitURL(url string, limit int) string {
	if ansi.PrintableRuneWidth(url) <= limit {
		return url
	}
	if idx := strings.Index(url, "://"); idx != -1 {
		trimmed := url[idx+3:]
		if ansi.PrintableRuneWidth(trimmed) <= limit {
			return trimmed
		}
	}
	return truncateWithEllipsis(url, limit)
}

func splitURLWrapper(text string) (prefix, url, suffix string, ok bool) {
	runes := []rune(text)
	if len(runes) < 2 {
		return "", "", "", false
	}
	open := runes[0]
	close := runes[len(runes)-1]
	var want rune
	switch open {
	case '(':
		want = ')'
	case '[':
		want = ']'
	case '{':
		want = '}'
	case '<':
		want = '>'
	default:
		return "", "", "", false
	}
	if close != want {
		return "", "", "", false
	}
	return string(open), string(runes[1 : len(runes)-1]), string(close), true
}

func splitWordToWidth(word string, limit int) []string {
	if ansi.PrintableRuneWidth(word) <= limit {
		return []string{word}
	}
	if strings.Contains(word, "-") {
		segments := splitHyphenated(word)
		return splitSegmentsToWidth(segments, limit)
	}
	segments := splitSyllables(word)
	if len(segments) <= 1 {
		return splitRunesToWidth(word, limit)
	}
	return splitSegmentsToWidth(segments, limit)
}

func splitRunesToWidth(text string, limit int) []string {
	if limit < 1 {
		return []string{text}
	}
	var parts []string
	var current strings.Builder
	width := 0
	for _, r := range text {
		rw := ansi.PrintableRuneWidth(string(r))
		if width+rw > limit && width > 0 {
			parts = append(parts, current.String())
			current.Reset()
			width = 0
		}
		current.WriteRune(r)
		width += rw
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return adjustQuoteTail(parts)
}

func splitSyllables(word string) []string {
	runes := []rune(word)
	if len(runes) < 4 {
		return []string{word}
	}
	breaks := make([]int, 0, len(runes))
	for i := 1; i < len(runes)-1; i++ {
		if isVowel(runes[i-1]) && !isVowel(runes[i]) && i >= 2 && len(runes)-i >= 2 {
			breaks = append(breaks, i)
		}
	}
	if len(breaks) == 0 {
		return []string{word}
	}
	var parts []string
	start := 0
	for _, br := range breaks {
		parts = append(parts, string(runes[start:br]))
		start = br
	}
	parts = append(parts, string(runes[start:]))
	return parts
}

func splitHyphenated(word string) []string {
	var parts []string
	var current strings.Builder
	for _, r := range word {
		current.WriteRune(r)
		if r == '-' {
			parts = append(parts, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func splitSegmentsToWidth(segments []string, limit int) []string {
	var parts []string
	var current strings.Builder
	currentWidth := 0
	for _, seg := range segments {
		segWidth := ansi.PrintableRuneWidth(seg)
		if currentWidth+segWidth <= limit {
			current.WriteString(seg)
			currentWidth += segWidth
			continue
		}
		if currentWidth > 0 {
			parts = append(parts, current.String())
			current.Reset()
			currentWidth = 0
		}
		if segWidth > limit {
			parts = append(parts, splitRunesToWidth(seg, limit)...)
			continue
		}
		current.WriteString(seg)
		currentWidth = segWidth
	}
	if currentWidth > 0 {
		parts = append(parts, current.String())
	}
	if len(parts) == 0 {
		parts = append(parts, strings.Join(segments, ""))
	}
	return adjustQuoteTail(parts)
}

func adjustQuoteTail(parts []string) []string {
	if len(parts) >= 2 {
		last := []rune(parts[len(parts)-1])
		if len(last) == 1 && isQuote(last[0]) {
			prev := []rune(parts[len(parts)-2])
			if len(prev) > 1 {
				parts[len(parts)-2] = string(prev[:len(prev)-1])
				parts[len(parts)-1] = string(append(prev[len(prev)-1:], last...))
			}
		}
	}
	return parts
}

func isVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u', 'y', 'A', 'E', 'I', 'O', 'U', 'Y':
		return true
	default:
		return false
	}
}

func splitByDelimiters(text, delims string) []string {
	var out []string
	var current strings.Builder
	for _, r := range text {
		current.WriteRune(r)
		if strings.ContainsRune(delims, r) {
			out = append(out, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

func truncateWithEllipsis(text string, limit int) string {
	if ansi.PrintableRuneWidth(text) <= limit {
		return text
	}
	if limit <= 1 {
		return "..."
	}
	var b strings.Builder
	width := 0
	for _, r := range text {
		rw := ansi.PrintableRuneWidth(string(r))
		if width+rw+1 > limit {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	b.WriteString("...")
	return b.String()
}
