package mdf

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"
)

var hashStringsWithSpace = [...]string{
	"",
	"# ",
	"## ",
	"### ",
	"#### ",
	"##### ",
	"###### ",
}

var spaceString = strings.Repeat(" ", 256)

const maxOrderedMarker = 1024
const maxEntityLen = 32

var orderedMarkerDot = func() [maxOrderedMarker + 1]string {
	var out [maxOrderedMarker + 1]string
	for i := 0; i <= maxOrderedMarker; i++ {
		out[i] = strconv.Itoa(i) + "."
	}
	return out
}()

var orderedMarkerParen = func() [maxOrderedMarker + 1]string {
	var out [maxOrderedMarker + 1]string
	for i := 0; i <= maxOrderedMarker; i++ {
		out[i] = strconv.Itoa(i) + ")"
	}
	return out
}()

type liveParser struct {
	styles Styles
	osc8   bool

	frontMatter frontMatterFilter

	lineBuf                   []rune
	lineBytes                 []byte
	textArena                 []byte
	lineDecided               bool
	lineEmitIdx               int
	lineIgnoreRest            bool
	lineSkipBreak             bool
	lineStyle                 Style
	lineStyled                bool
	pendingBreaks             int
	hardBreakPending          bool
	immediateSpaces           []rune
	inParagraph               bool
	quoteDepth                int
	quoteLazy                 bool
	lastQuoteExplicit         bool
	prevQuoteDepth            int
	quoteListPrefixFirst      bool
	pendingQuoteBlank         bool
	pendingQuoteExplicit      bool
	pendingQuoteTrailingSpace bool
	pendingQuotePrevDepth     int
	pendingQuoteDepth         int
	pendingQuoteListLen       int
	pendingQuoteListFirst     bool
	listStack                 []listState
	listPrefixLen             int
	listLazy                  bool
	listItemFirstLine         bool
	seenLine                  bool
	lineHasNonSpace           bool

	inCodeFence         bool
	fenceMarker         string
	pendingCodeNL       bool
	inIndentCode        bool
	indentCode          int
	codePrevWidth       int
	codeLineDecided     bool
	codeLineIsCode      bool
	postCodeBreakSingle bool

	inline inlineState

	lineBufArr         [1024]rune
	lineBytesArr       [4096]byte
	textArenaArr       [4096]byte
	readBufArr         [4096]byte
	listStackArr       [32]listState
	immediateSpacesArr [64]rune
	inlineCodeBufArr   [128]byte
	inlineLinkTextArr  [128]byte
	inlineLinkURLArr   [128]byte
	inlineAutoLinkArr  [128]byte
	inlineEntityArr    [32]byte
}

type inlineState struct {
	inCode           bool
	codeFence        int
	codeBuf          []byte
	pendingBackticks int
	inEm             bool
	inStrong         bool
	inLink           bool
	inLinkURL        bool
	inAutoLink       bool
	inEntity         bool
	pendingNumUS     bool
	lastWasDigit     bool

	pendingDelim rune
	pendingCount int
	pendingClose bool

	linkText []byte
	linkURL  []byte
	autoLink []byte
	entity   []byte
}

type listState struct {
	indent          int
	ordered         bool
	marker          rune
	next            int
	contentIndent   int
	prefixLen       int
	itemIndentExtra int
}

func newLiveParser(theme Theme, osc8 bool) *liveParser {
	p := &liveParser{
		styles: theme.Styles(),
		osc8:   osc8,
	}
	p.lineBuf = p.lineBufArr[:0]
	p.lineBytes = p.lineBytesArr[:0]
	p.textArena = p.textArenaArr[:0]
	p.listStack = p.listStackArr[:0]
	p.immediateSpaces = p.immediateSpacesArr[:0]
	p.inline.codeBuf = p.inlineCodeBufArr[:0]
	p.inline.linkText = p.inlineLinkTextArr[:0]
	p.inline.linkURL = p.inlineLinkURLArr[:0]
	p.inline.autoLink = p.inlineAutoLinkArr[:0]
	p.inline.entity = p.inlineEntityArr[:0]
	return p
}

func (p *liveParser) Reset(theme Theme, osc8 bool) {
	if theme == nil {
		theme = DefaultTheme()
	}
	p.styles = theme.Styles()
	p.osc8 = osc8
	p.frontMatter.reset()
	p.lineBuf = p.lineBufArr[:0]
	p.lineBytes = p.lineBytesArr[:0]
	p.textArena = p.textArenaArr[:0]
	p.lineDecided = false
	p.lineEmitIdx = 0
	p.lineIgnoreRest = false
	p.lineSkipBreak = false
	p.lineStyle = Style{}
	p.lineStyled = false
	p.pendingBreaks = 0
	p.hardBreakPending = false
	p.immediateSpaces = p.immediateSpacesArr[:0]
	p.inParagraph = false
	p.quoteDepth = 0
	p.quoteLazy = false
	p.lastQuoteExplicit = false
	p.pendingQuoteBlank = false
	p.pendingQuoteExplicit = false
	p.pendingQuoteTrailingSpace = false
	p.pendingQuotePrevDepth = 0
	p.prevQuoteDepth = 0
	p.quoteListPrefixFirst = false
	p.listStack = p.listStackArr[:0]
	p.listPrefixLen = 0
	p.listLazy = false
	p.listItemFirstLine = false
	p.seenLine = false
	p.lineHasNonSpace = false
	p.inCodeFence = false
	p.fenceMarker = ""
	p.pendingCodeNL = false
	p.inIndentCode = false
	p.indentCode = 0
	p.codePrevWidth = 0
	p.codeLineDecided = false
	p.codeLineIsCode = false
	p.postCodeBreakSingle = false
	p.inline.codeBuf = p.inlineCodeBufArr[:0]
	p.inline.linkText = p.inlineLinkTextArr[:0]
	p.inline.linkURL = p.inlineLinkURLArr[:0]
	p.inline.autoLink = p.inlineAutoLinkArr[:0]
	p.inline.entity = p.inlineEntityArr[:0]
	p.resetInline()
}

func (p *liveParser) feedRune(stream Stream, r rune) error {
	if p.pendingQuoteBlank && !p.lineHasNonSpace && !p.pendingQuoteExplicit {
		if r != ' ' && r != '\t' && r != '\r' {
			if r == '>' {
				if p.pendingBreaks == 0 {
					p.pendingBreaks = 1
				}
				if err := p.applyPendingBreak(stream, breakSingle); err != nil {
					return err
				}
				listIndent := p.pendingQuoteListLen
				if indent, _ := leadingIndentCount(bytesToString(p.lineBytes)); indent > 0 {
					listIndent = indent
				}
				if listIndent > 0 {
					if err := p.emitListPrefix(stream, listIndent); err != nil {
						return err
					}
				}
				if err := p.emitQuotePrefixBare(stream, p.pendingQuoteDepth); err != nil {
					return err
				}
				if err := stream.WriteToken(StreamToken{Token: Token{Text: "\n", Style: Style{}, Kind: tokenText}}); err != nil {
					return err
				}
				p.pendingBreaks = 0
				p.hardBreakPending = false
				p.inParagraph = false
			} else {
				if p.pendingBreaks == 0 {
					p.pendingBreaks = 1
				}
				p.quoteDepth = 0
				p.quoteLazy = false
				p.lastQuoteExplicit = false
			}
			p.pendingQuoteBlank = false
			p.pendingQuoteExplicit = false
			p.pendingQuoteTrailingSpace = false
		}
	}
	if p.inCodeFence {
		if r == '\n' {
			line := strings.TrimSuffix(bytesToString(p.lineBytes), "\r")
			p.lineBuf = p.lineBuf[:0]
			p.lineBytes = p.lineBytes[:0]
			p.seenLine = true
			return p.processCodeFenceLine(stream, line)
		}
		p.lineBuf = append(p.lineBuf, r)
		p.lineBytes = utf8.AppendRune(p.lineBytes, r)
		return nil
	}
	if p.inIndentCode {
		if r == '\n' {
			if !p.codeLineDecided {
				if err := p.maybeDecideIndentCodeLine(stream, true); err != nil {
					return err
				}
			}
			if p.inIndentCode {
				if p.codeLineDecided && p.codeLineIsCode {
					if err := stream.WriteToken(StreamToken{Token: Token{Text: "\n", Style: Style{}, Kind: tokenText}}); err != nil {
						return err
					}
					p.pendingCodeNL = false
				}
				p.seenLine = true
				p.resetLine()
			}
			return nil
		}
		if p.codeLineDecided && p.codeLineIsCode {
			return p.emitCodeRune(stream, r)
		}
		p.lineBuf = append(p.lineBuf, r)
		p.lineBytes = utf8.AppendRune(p.lineBytes, r)
		return p.maybeDecideIndentCodeLine(stream, false)
	}
	if r == '\n' {
		if strings.TrimSpace(bytesToString(p.lineBytes)) == "" {
			if p.quoteDepth > 0 && p.quoteLazy && p.lastQuoteExplicit {
				lineIndent, _ := leadingIndentCount(bytesToString(p.lineBytes))
				p.pendingQuoteBlank = true
				p.pendingQuoteExplicit = false
				p.pendingQuoteTrailingSpace = false
				p.pendingQuotePrevDepth = p.quoteDepth
				p.pendingQuoteDepth = p.quoteDepth
				p.pendingQuoteListFirst = lineIndent > 0
				if p.pendingQuoteListFirst {
					p.pendingQuoteListLen = lineIndent
				} else {
					p.pendingQuoteListLen = 0
				}
				p.hardBreakPending = false
				p.inParagraph = false
				p.seenLine = true
				p.resetLine()
				return nil
			}
			if p.seenLine {
				p.pendingBreaks++
			}
			p.hardBreakPending = false
			p.inParagraph = false
			p.quoteDepth = 0
			p.quoteLazy = false
			p.lastQuoteExplicit = false
			p.listLazy = false
			p.listItemFirstLine = false
			p.seenLine = true
			p.resetLine()
			return nil
		}
		if !p.lineDecided && len(p.lineBuf) > 0 {
			if err := p.maybeDecideLine(stream, true); err != nil {
				return err
			}
		}
		if p.lineDecided {
			p.hardBreakPending = hasHardLineBreak(bytesToString(p.lineBytes))
			p.immediateSpaces = p.immediateSpaces[:0]
			if err := p.flushPendingBackticks(stream); err != nil {
				return err
			}
			if err := p.flushPendingEntity(stream); err != nil {
				return err
			}
			if err := p.flushPendingNumUS(stream); err != nil {
				return err
			}
			p.flushPendingDelims()
			p.lineStyled = false
			if !p.lineSkipBreak {
				p.pendingBreaks++
			}
			p.seenLine = true
		} else {
			if p.seenLine {
				p.pendingBreaks++
			}
			p.hardBreakPending = false
			p.inParagraph = false
			p.quoteLazy = false
			p.listLazy = false
			p.listItemFirstLine = false
			p.seenLine = true
		}
		p.resetLine()
		return nil
	}
	if p.lineDecided {
		p.lineBuf = append(p.lineBuf, r)
		p.lineBytes = utf8.AppendRune(p.lineBytes, r)
		if !p.lineIgnoreRest {
			if err := p.emitInline(stream, r); err != nil {
				return err
			}
		}
		p.lineEmitIdx = len(p.lineBuf)
		return nil
	}
	p.lineBuf = append(p.lineBuf, r)
	p.lineBytes = utf8.AppendRune(p.lineBytes, r)
	if !p.lineHasNonSpace {
		if r == ' ' || r == '\t' {
			return nil
		}
		p.lineHasNonSpace = true
		if !isPotentialBlockStart(r) {
			return p.maybeDecideLine(stream, false)
		}
		return nil
	}
	if r == ' ' || r == '\t' {
		return p.maybeDecideLine(stream, false)
	}
	if shouldAttemptDecision(p.lineBuf) {
		return p.maybeDecideLine(stream, false)
	}
	return nil
}

func (p *liveParser) resetLine() {
	p.lineBuf = p.lineBuf[:0]
	p.lineBytes = p.lineBytes[:0]
	p.lineDecided = false
	p.lineEmitIdx = 0
	p.lineIgnoreRest = false
	p.lineSkipBreak = false
	p.immediateSpaces = p.immediateSpaces[:0]
	p.lineHasNonSpace = false
	p.codeLineDecided = false
	p.codeLineIsCode = false
	p.quoteListPrefixFirst = false
}

func (p *liveParser) replayLine(stream Stream, line string) error {
	p.resetLine()
	for _, r := range line {
		if err := p.feedRune(stream, r); err != nil {
			return err
		}
	}
	return nil
}

func isPotentialBlockStart(r rune) bool {
	switch r {
	case '#', '-', '+', '*', '`', '~', '>':
		return true
	default:
		return r >= '0' && r <= '9'
	}
}

func shouldAttemptDecision(line []rune) bool {
	if len(line) < 2 {
		return false
	}
	last := line[len(line)-1]
	if last == ' ' || last == '\t' {
		return true
	}
	return !isPotentialBlockStart(last)
}

func (p *liveParser) maybeDecideLine(stream Stream, force bool) error {
	if p.lineDecided {
		return nil
	}
	p.prevQuoteDepth = p.quoteDepth
	line := bytesToString(p.lineBytes)
	if p.pendingQuoteBlank && !force && strings.TrimSpace(line) == ">" {
		return nil
	}
	if strings.TrimSpace(line) == "" {
		if force && p.quoteDepth > 0 && p.quoteLazy && p.lastQuoteExplicit {
			if p.pendingBreaks == 0 {
				p.pendingBreaks = 1
			}
			if err := p.applyPendingBreak(stream, breakSingle); err != nil {
				return err
			}
			if p.quoteListPrefixFirst && p.listPrefixLen > 0 {
				if err := p.emitListPrefix(stream, p.listPrefixLen); err != nil {
					return err
				}
			}
			if err := p.emitQuotePrefixBare(stream, p.quoteDepth); err != nil {
				return err
			}
			if err := stream.WriteToken(StreamToken{Token: Token{Text: "\n", Style: Style{}, Kind: tokenText}}); err != nil {
				return err
			}
			p.pendingBreaks = 0
			p.hardBreakPending = false
			p.inParagraph = false
			p.lineDecided = true
			p.lineIgnoreRest = true
			p.lineSkipBreak = true
		}
		return nil
	}
	lineIndent, _ := leadingIndentCount(line)
	depth, rest, explicit := parseQuotePrefix(line)
	if explicit {
		p.quoteDepth = depth
		p.quoteLazy = true
		p.lastQuoteExplicit = true
	} else if p.pendingQuoteBlank {
		p.quoteDepth = 0
		depth = 0
		p.quoteLazy = false
		p.lastQuoteExplicit = false
	} else if p.quoteDepth > 0 && p.quoteLazy {
		depth = p.quoteDepth
		p.lastQuoteExplicit = false
	} else {
		p.quoteDepth = 0
		depth = 0
		p.quoteLazy = false
		p.lastQuoteExplicit = false
	}
	if p.pendingQuoteBlank {
		if depth > 0 {
			if p.pendingBreaks == 0 {
				p.pendingBreaks = 1
			}
			if err := p.applyPendingBreak(stream, breakSingle); err != nil {
				return err
			}
			if p.pendingQuoteListFirst && p.pendingQuoteListLen > 0 {
				if err := p.emitListPrefix(stream, p.pendingQuoteListLen); err != nil {
					return err
				}
			}
			if err := p.emitQuotePrefixBare(stream, p.pendingQuoteDepth); err != nil {
				return err
			}
			if err := stream.WriteToken(StreamToken{Token: Token{Text: "\n", Style: Style{}, Kind: tokenText}}); err != nil {
				return err
			}
			p.pendingBreaks = 0
			p.hardBreakPending = false
			p.inParagraph = false
		} else if p.pendingBreaks == 0 {
			p.pendingBreaks = 1
		}
		p.pendingQuoteBlank = false
		p.pendingQuoteExplicit = false
		p.pendingQuoteTrailingSpace = false
		p.pendingQuotePrevDepth = 0
	}
	p.quoteListPrefixFirst = explicit && depth > 0 && p.listPrefixLen > 0 && lineIndent > 0
	if explicit && strings.TrimSpace(rest) == "" && force {
		p.pendingQuoteBlank = true
		p.pendingQuoteExplicit = true
		p.pendingQuoteTrailingSpace = quoteLineHasTrailingSpace(line)
		p.pendingQuotePrevDepth = p.prevQuoteDepth
		p.pendingQuoteDepth = depth
		p.pendingQuoteListFirst = p.quoteListPrefixFirst
		p.pendingQuoteListLen = p.listPrefixLen
		p.hardBreakPending = false
		p.inParagraph = false
		p.quoteDepth = depth
		p.quoteLazy = true
		p.listLazy = false
		p.listItemFirstLine = false
		p.lineDecided = true
		p.lineIgnoreRest = true
		p.lineSkipBreak = true
		return nil
	}
	trimmed := strings.TrimLeft(rest, " \t")
	if trimmed == "" {
		return nil
	}
	if trimmed[0] == '`' || trimmed[0] == '~' {
		if !force {
			return nil
		}
	}
	if isThematicBreak(rest) {
		if p.pendingBreaks == 0 {
			p.pendingBreaks = 1
		}
		p.lineDecided = true
		p.lineIgnoreRest = true
		p.lineSkipBreak = true
		p.inParagraph = false
		p.listLazy = false
		p.listItemFirstLine = false
		p.quoteLazy = false
		p.hardBreakPending = false
		return stream.WriteToken(StreamToken{Token: Token{Kind: tokenThematicBreak}})
	}
	if isMaybeThematicBreak(rest) {
		if !force {
			return nil
		}
	}
	if isMaybeFence(rest) {
		if !force {
			return nil
		}
	}
	if fence := fenceMarker(rest); fence != "" {
		if err := p.applyPendingBreak(stream, breakDouble); err != nil {
			return err
		}
		p.inCodeFence = true
		p.fenceMarker = fence
		p.pendingCodeNL = false
		p.enterCodeNoWrap(stream)
		p.inParagraph = false
		p.lineDecided = true
		p.lineIgnoreRest = true
		p.lineSkipBreak = true
		return nil
	}
	if strings.HasPrefix(trimmed, "#") {
		if level, content, ok := parseHeading(trimmed); ok {
			p.listLazy = false
			p.listItemFirstLine = false
			p.clearListIfOutdented(lineIndent)
			if err := p.applyPendingBreak(stream, breakDouble); err != nil {
				return err
			}
			if err := p.emitPrefix(stream, depth, p.listPrefixLen); err != nil {
				return err
			}
			style := p.styles.Heading[level-1]
			p.lineStyle = style
			p.lineStyled = true
			marker := hashStringsWithSpace[level]
			_ = stream.WriteToken(StreamToken{Token: Token{Text: marker, Style: style}})
			p.inParagraph = false
			p.resetInline()
			p.lineDecided = true
			p.lineSkipBreak = true
			p.lineEmitIdx = len(p.lineBuf) - utf8.RuneCountInString(content)
			p.pendingBreaks++
			return p.emitInlineRunes(stream, p.lineBuf[p.lineEmitIdx:])
		}
		hashes := 0
		for hashes < len(trimmed) && trimmed[hashes] == '#' {
			hashes++
		}
		if hashes > 0 && hashes < len(trimmed) && trimmed[hashes] != ' ' {
			quoteBlockStart := explicit && depth > 0 && p.inParagraph && (p.prevQuoteDepth != depth || p.listLazy)
			return p.decideParagraph(stream, depth, rest, quoteBlockStart)
		}
		return nil
	}
	ordered, markerRune, number, markerLen, padding, content, ok := parseListMarker(trimmed)
	if ok {
		if strings.TrimSpace(content) == "" {
			return nil
		}
		if strings.HasPrefix(content, "[") && len(content) < 4 {
			return nil
		}
		if depth > 0 && p.inParagraph && p.pendingBreaks == 1 && !p.listLazy {
			if err := p.applyPendingBreak(stream, breakSingle); err != nil {
				return err
			}
			if err := p.emitQuotePrefixBare(stream, depth); err != nil {
				return err
			}
			if err := stream.WriteToken(StreamToken{Token: Token{Text: "\n", Style: Style{}, Kind: tokenText}}); err != nil {
				return err
			}
			p.pendingBreaks = 0
			p.hardBreakPending = false
			p.inParagraph = false
		}
		if p.inParagraph && p.pendingBreaks == 0 {
			p.pendingBreaks = 1
		}
		if depth > 0 && p.listLazy && p.pendingBreaks > 1 {
			p.pendingBreaks = 1
		}
		prevDepth := len(p.listStack)
		prevIndent := 0
		if prevDepth > 0 {
			prevIndent = p.listStack[prevDepth-1].indent
		}
		parentOrdered := prevDepth > 0 && p.listStack[prevDepth-1].ordered
		parentPrefixLen, state := p.updateList(leadingIndentCountBytes(line), ordered, markerRune, number, markerLen, padding)
		nested := len(p.listStack) > prevDepth && leadingIndentCountBytes(line) > prevIndent
		mode := breakDouble
		if p.listLazy && p.pendingBreaks == 1 && (!nested || !parentOrdered) {
			mode = breakSingle
		}
		if depth > 0 && p.inParagraph && p.pendingBreaks == 1 && !p.listLazy {
			mode = breakDouble
		}
		if err := p.applyPendingBreak(stream, mode); err != nil {
			return err
		}
		if err := p.emitPrefix(stream, depth, parentPrefixLen); err != nil {
			return err
		}
		markerText := "-"
		if ordered {
			markerText = p.orderedMarkerText(state.next, state.marker)
			state.next++
			p.listStack[len(p.listStack)-1] = state
		}
		_ = stream.WriteToken(StreamToken{Token: Token{Text: markerText, Style: p.styles.ListMarker}})
		_ = stream.WriteToken(StreamToken{Token: Token{Text: " ", Style: p.styles.Text}})
		if len(p.listStack) > 0 {
			p.listStack[len(p.listStack)-1].itemIndentExtra = taskListExtraIndent(content)
		}
		extra := 0
		if len(p.listStack) > 0 {
			extra = p.listStack[len(p.listStack)-1].itemIndentExtra
		}
		if depth > 0 {
			stream.SetWrapIndent(p.quoteWrapIndent(depth, p.listPrefixLen+extra))
		} else {
			stream.SetWrapIndent(p.spaces(p.listPrefixLen + extra))
		}
		p.listLazy = true
		p.listItemFirstLine = true
		p.inParagraph = true
		p.resetInline()
		p.lineDecided = true
		p.lineEmitIdx = len(p.lineBuf) - utf8.RuneCountInString(content)
		return p.emitInlineRunes(stream, p.lineBuf[p.lineEmitIdx:])
	}
	indent, _ := leadingIndentCount(rest)
	codeIndent := 4
	if len(p.listStack) > 0 {
		state := p.listStack[len(p.listStack)-1]
		codeIndent = state.contentIndent + state.itemIndentExtra + 4
	}
	if indent >= codeIndent {
		p.inIndentCode = true
		p.indentCode = codeIndent
		p.pendingCodeNL = false
		p.enterCodeNoWrap(stream)
		p.inParagraph = false
		p.hardBreakPending = false
		return nil
	}
	if isMaybeListStart(trimmed) {
		if !force {
			return nil
		}
	}
	indentForList := indent
	if explicit && depth > 0 && lineIndent > 0 {
		indentForList = lineIndent
	}
	quoteLineWithIndent := lineIndent > 0 && strings.HasPrefix(strings.TrimLeft(line, " \t"), ">")
	if p.inListContinuation(indentForList, trimmed, explicit, lineIndent) {
		if explicit && depth > 0 && len(p.listStack) > 0 && quoteLineWithIndent {
			p.quoteListPrefixFirst = true
		}
		newParagraph := !p.inParagraph
		firstContinuation := p.listItemFirstLine
		p.listItemFirstLine = false
		state := listState{}
		listIndent := p.listPrefixLen
		if len(p.listStack) > 0 {
			state = p.listStack[len(p.listStack)-1]
			if state.itemIndentExtra > 0 {
				listIndent += state.itemIndentExtra
			}
		}
		wrapIndent := listIndent
		if explicit && depth > 0 && quoteLineWithIndent && indentForList > wrapIndent {
			wrapIndent = indentForList
		}
		forceQuoteOnly := false
		forceLineBreak := false
		if explicit && depth > 0 && len(p.listStack) > 0 && state.ordered && lineIndent == 0 {
			forceQuoteOnly = true
			forceLineBreak = firstContinuation
		}
		mode := breakDouble
		if p.inParagraph {
			if len(p.listStack) > 0 && indentForList >= p.listStack[len(p.listStack)-1].contentIndent+p.listStack[len(p.listStack)-1].itemIndentExtra && depth == 0 {
				mode = breakSpace
			} else {
				mode = p.softBreakMode(depth)
			}
		}
		if mode == breakSpace && p.hardBreakPending {
			mode = breakSingle
		}
		if quoteLineWithIndent && mode == breakSpace {
			mode = breakSingle
		}
		if forceLineBreak && mode == breakSpace {
			mode = breakSingle
		}
		if forceLineBreak && p.pendingBreaks == 0 {
			p.pendingBreaks = 1
		}
		if err := p.applyPendingBreak(stream, mode); err != nil {
			return err
		}
		if mode != breakSpace {
			if forceQuoteOnly {
				if err := p.emitQuotePrefix(stream, depth); err != nil {
					return err
				}
			} else if explicit && depth > 0 && len(p.listStack) > 0 && indentForList >= p.listStack[len(p.listStack)-1].contentIndent {
				if quoteLineWithIndent {
					if err := p.emitListPrefix(stream, wrapIndent); err != nil {
						return err
					}
					if err := p.emitQuotePrefix(stream, depth); err != nil {
						return err
					}
				} else if p.quoteListPrefixFirst {
					if err := p.emitListPrefix(stream, wrapIndent); err != nil {
						return err
					}
					if err := p.emitQuotePrefix(stream, depth); err != nil {
						return err
					}
				} else {
					if err := p.emitQuotePrefix(stream, depth); err != nil {
						return err
					}
					if err := p.emitListPrefix(stream, wrapIndent); err != nil {
						return err
					}
				}
			} else if err := p.emitPrefix(stream, depth, p.listPrefixLen); err != nil {
				return err
			}
			if wrapIndent > 0 {
				if depth > 0 {
					if forceQuoteOnly {
						stream.SetWrapIndent(p.quoteWrapIndent(depth, 0))
					} else if explicit && depth > 0 && quoteLineWithIndent {
						stream.SetWrapIndent(p.quoteWrapIndentListFirst(depth, wrapIndent))
					} else {
						stream.SetWrapIndent(p.quoteWrapIndent(depth, wrapIndent))
					}
				} else {
					stream.SetWrapIndent(p.spaces(depth*2 + wrapIndent))
				}
			}
		} else if explicit && depth > 0 && len(p.listStack) > 0 && indentForList >= p.listStack[len(p.listStack)-1].contentIndent {
			if quoteLineWithIndent {
				stream.SetWrapIndent(p.quoteWrapIndentListFirst(depth, wrapIndent))
			} else {
				stream.SetWrapIndent(p.quoteWrapIndent(depth, wrapIndent))
			}
		}
		content := p.trimListIndent(trimmed)
		p.inParagraph = true
		if newParagraph {
			p.resetInline()
		}
		p.lineDecided = true
		p.lineEmitIdx = len(p.lineBuf) - utf8.RuneCountInString(content)
		return p.emitInlineRunes(stream, p.lineBuf[p.lineEmitIdx:])
	}
	quoteBlockStart := explicit && depth > 0 && p.inParagraph && (p.prevQuoteDepth != depth || p.listLazy)
	p.listLazy = false
	p.listItemFirstLine = false
	outdentIndent := indentForList
	if explicit && depth > 0 {
		outdentIndent = lineIndent
	}
	p.clearListIfOutdented(outdentIndent)
	return p.decideParagraph(stream, depth, rest, quoteBlockStart)
}

func (p *liveParser) decideParagraph(stream Stream, depth int, rest string, blockStart bool) error {
	newParagraph := !p.inParagraph
	mode := breakDouble
	if p.inParagraph {
		mode = p.softBreakMode(depth)
		if blockStart {
			mode = breakSingle
		}
		if mode == breakSpace && p.hardBreakPending {
			mode = breakSingle
		}
	}
	suppressPrefix := p.inParagraph && p.hardBreakPending && depth == 0 && p.listPrefixLen == 0 && !blockStart
	if err := p.applyPendingBreak(stream, mode); err != nil {
		return err
	}
	if mode != breakSpace && !suppressPrefix {
		if err := p.emitPrefix(stream, depth, p.listPrefixLen); err != nil {
			return err
		}
	} else if suppressPrefix {
		stream.SetWrapIndent("")
	}
	p.inParagraph = true
	if newParagraph {
		p.resetInline()
	}
	content := strings.TrimLeft(rest, " \t")
	p.lineDecided = true
	p.lineEmitIdx = len(p.lineBuf) - utf8.RuneCountInString(content)
	return p.emitInlineRunes(stream, p.lineBuf[p.lineEmitIdx:])
}

func (p *liveParser) maybeDecideIndentCodeLine(stream Stream, force bool) error {
	if p.codeLineDecided {
		return nil
	}
	line := strings.TrimSuffix(bytesToString(p.lineBytes), "\r")
	if strings.TrimSpace(line) == "" && !force {
		return nil
	}
	depth, rest, explicit := parseQuotePrefix(line)
	if explicit {
		p.quoteDepth = depth
		p.quoteLazy = true
		p.lastQuoteExplicit = true
	} else if p.quoteDepth > 0 && p.quoteLazy {
		depth = p.quoteDepth
		p.lastQuoteExplicit = false
	}
	if p.pendingQuoteBlank {
		if explicit {
			p.quoteDepth = depth
			p.quoteLazy = true
			p.lastQuoteExplicit = true
		} else {
			p.quoteDepth = 0
			depth = 0
			p.quoteLazy = false
			p.lastQuoteExplicit = false
		}
		if depth > 0 {
			if p.pendingBreaks == 0 {
				p.pendingBreaks = 1
			}
			if err := p.applyPendingBreak(stream, breakSingle); err != nil {
				return err
			}
			if p.pendingQuoteListFirst && p.pendingQuoteListLen > 0 {
				if err := p.emitListPrefix(stream, p.pendingQuoteListLen); err != nil {
					return err
				}
			}
			if err := p.emitQuotePrefixBare(stream, p.pendingQuoteDepth); err != nil {
				return err
			}
			if err := stream.WriteToken(StreamToken{Token: Token{Text: "\n", Style: Style{}, Kind: tokenText}}); err != nil {
				return err
			}
			p.pendingBreaks = 0
			p.hardBreakPending = false
			p.inParagraph = false
		} else if p.pendingBreaks == 0 {
			p.pendingBreaks = 1
		}
		p.pendingQuoteBlank = false
		p.pendingQuoteExplicit = false
		p.pendingQuoteTrailingSpace = false
		p.pendingQuotePrevDepth = 0
	}
	if strings.TrimSpace(rest) == "" && !force {
		return nil
	}
	indent, _ := leadingIndentCount(rest)
	if indent >= p.indentCode {
		if !force {
			return nil
		}
		if p.pendingBreaks > 0 {
			if err := p.applyPendingBreak(stream, breakDouble); err != nil {
				return err
			}
		}
		if err := p.emitCodeLine(stream, trimIndent(rest, p.indentCode)); err != nil {
			return err
		}
		p.codeLineDecided = true
		p.codeLineIsCode = true
		p.lineBuf = p.lineBuf[:0]
		p.lineBytes = p.lineBytes[:0]
		p.lineHasNonSpace = false
		return nil
	}
	p.codeLineDecided = true
	p.codeLineIsCode = false
	p.inIndentCode = false
	p.pendingCodeNL = false
	p.exitCodeNoWrap(stream)
	if p.pendingBreaks == 0 {
		p.pendingBreaks = 1
	}
	p.postCodeBreakSingle = true
	if !explicit {
		p.quoteDepth = 0
		p.quoteLazy = false
		p.lastQuoteExplicit = false
	}
	p.codeLineDecided = false
	p.codeLineIsCode = false
	return p.replayLine(stream, line)
}

func (p *liveParser) emitInlineRunes(stream Stream, runes []rune) error {
	for i := 0; i < len(runes); {
		r := runes[i]
		if r == '_' && i > 0 && i+1 < len(runes) && !p.inline.inCode && !p.inline.inLink && !p.inline.inLinkURL && !p.inline.inAutoLink {
			if runes[i-1] >= '0' && runes[i-1] <= '9' && runes[i+1] >= '0' && runes[i+1] <= '9' {
				if err := p.emitInline(stream, '\u00A0'); err != nil {
					return err
				}
				i++
				continue
			}
		}
		if r == '&' && !p.inline.inCode && !p.inline.inAutoLink && !p.inline.inLinkURL {
			if i+5 < len(runes) && isNBSPRunes(runes[i:i+6]) {
				if err := p.emitInline(stream, '\u00A0'); err != nil {
					return err
				}
				i += 6
				continue
			}
		}
		if err := p.emitInline(stream, r); err != nil {
			return err
		}
		i++
	}
	if p.inline.inEntity && len(p.inline.entity) > 0 {
		text := bytesToString(p.inline.entity)
		p.inline.inEntity = false
		p.inline.entity = p.inline.entity[:0]
		if err := p.emitStyledText(stream, text); err != nil {
			return err
		}
	}
	p.lineEmitIdx = len(p.lineBuf)
	return nil
}

func isNBSPRunes(runes []rune) bool {
	if len(runes) != 6 {
		return false
	}
	if runes[0] != '&' || runes[5] != ';' {
		return false
	}
	return (runes[1] == 'n' || runes[1] == 'N') &&
		(runes[2] == 'b' || runes[2] == 'B') &&
		(runes[3] == 's' || runes[3] == 'S') &&
		(runes[4] == 'p' || runes[4] == 'P')
}

func decodeEntity(buf []byte) (rune, bool) {
	if len(buf) < 3 || buf[0] != '&' || buf[len(buf)-1] != ';' {
		return 0, false
	}
	body := buf[1 : len(buf)-1]
	if len(body) == 0 {
		return 0, false
	}
	if body[0] == '#' {
		if len(body) < 2 {
			return 0, false
		}
		start := 1
		base := 10
		if len(body) > 2 && (body[1] == 'x' || body[1] == 'X') {
			base = 16
			start = 2
		}
		if start >= len(body) {
			return 0, false
		}
		val, err := strconv.ParseInt(string(body[start:]), base, 32)
		if err != nil {
			return 0, false
		}
		if val < 0 || val > utf8.MaxRune {
			return 0, false
		}
		if val >= 0xD800 && val <= 0xDFFF {
			return 0, false
		}
		if val == 160 {
			return '\u00A0', true
		}
		return rune(val), true
	}
	if bytes.EqualFold(body, []byte("nbsp")) {
		return '\u00A0', true
	}
	return 0, false
}

func isMaybeListStart(text string) bool {
	if text == "" {
		return false
	}
	switch text[0] {
	case '-', '+', '*':
		if len(text) == 1 {
			return true
		}
		if text[1] == ' ' || text[1] == '\t' {
			return true
		}
		return false
	}
	if text[0] >= '0' && text[0] <= '9' {
		i := 0
		for i < len(text) && text[i] >= '0' && text[i] <= '9' {
			i++
		}
		if i == len(text) {
			return true
		}
		if i < len(text) && (text[i] == '.' || text[i] == ')') {
			if i+1 == len(text) {
				return true
			}
			if text[i+1] == ' ' || text[i+1] == '\t' {
				return true
			}
			return false
		}
		return false
	}
	return false
}

func leadingIndentCountBytes(s string) int {
	count, _ := leadingIndentCount(s)
	return count
}

func (p *liveParser) emitPrefix(stream Stream, quoteDepth int, listPrefixLen int) error {
	if p.quoteListPrefixFirst && quoteDepth > 0 && listPrefixLen > 0 {
		if err := p.emitListPrefix(stream, listPrefixLen); err != nil {
			return err
		}
		if err := p.emitQuotePrefix(stream, quoteDepth); err != nil {
			return err
		}
	} else {
		if err := p.emitQuotePrefix(stream, quoteDepth); err != nil {
			return err
		}
		if err := p.emitListPrefix(stream, listPrefixLen); err != nil {
			return err
		}
	}
	if quoteDepth > 0 {
		if p.quoteListPrefixFirst && listPrefixLen > 0 {
			stream.SetWrapIndent(p.quoteWrapIndentListFirst(quoteDepth, listPrefixLen))
		} else {
			stream.SetWrapIndent(p.quoteWrapIndent(quoteDepth, listPrefixLen))
		}
	}
	return nil
}

func (p *liveParser) emitQuotePrefix(stream Stream, quoteDepth int) error {
	for i := 0; i < quoteDepth; i++ {
		if err := stream.WriteToken(StreamToken{Token: Token{Text: ">", Style: p.styles.Quote}}); err != nil {
			return err
		}
		if err := stream.WriteToken(StreamToken{Token: Token{Text: " ", Style: p.styles.Text}}); err != nil {
			return err
		}
	}
	return nil
}

func (p *liveParser) emitQuotePrefixBare(stream Stream, quoteDepth int) error {
	for i := 0; i < quoteDepth; i++ {
		if err := stream.WriteToken(StreamToken{Token: Token{Text: ">", Style: p.styles.Quote}}); err != nil {
			return err
		}
	}
	return nil
}

func (p *liveParser) emitListPrefix(stream Stream, listPrefixLen int) error {
	if listPrefixLen == 0 {
		return nil
	}
	return stream.WriteToken(StreamToken{Token: Token{Text: p.spaces(listPrefixLen), Style: p.styles.Text}})
}

func (p *liveParser) processCodeFenceLine(stream Stream, line string) error {
	depth, rest, explicit := parseQuotePrefix(line)
	if explicit {
		p.quoteDepth = depth
		p.quoteLazy = true
		p.lastQuoteExplicit = true
	} else if p.quoteDepth > 0 && p.quoteLazy {
		p.lastQuoteExplicit = false
	}
	trim := strings.TrimSpace(rest)
	if strings.HasPrefix(trim, p.fenceMarker) && strings.TrimSpace(trim[len(p.fenceMarker):]) == "" {
		p.inCodeFence = false
		p.fenceMarker = ""
		p.pendingCodeNL = false
		p.pendingBreaks++
		p.exitCodeNoWrap(stream)
		return nil
	}
	return p.emitCodeLine(stream, rest)
}

func (p *liveParser) emitCodeLine(stream Stream, line string) error {
	if p.pendingCodeNL {
		if err := stream.WriteToken(StreamToken{Token: Token{Text: "\n", Style: Style{}, Kind: tokenText}}); err != nil {
			return err
		}
	}
	p.pendingCodeNL = true
	if err := p.emitPrefix(stream, p.quoteDepth, p.listPrefixLen); err != nil {
		return err
	}
	if line == "" {
		return nil
	}
	return stream.WriteToken(StreamToken{Token: Token{Text: line, Style: p.styles.CodeBlock, Kind: tokenCode, CodeBlock: true}})
}

func (p *liveParser) emitCodeRune(stream Stream, r rune) error {
	return stream.WriteToken(StreamToken{Token: Token{Text: p.runeTokenText(r), Style: p.styles.CodeBlock, Kind: tokenCode, CodeBlock: true}})
}

func (p *liveParser) enterCodeNoWrap(stream Stream) {
	_ = stream
}

func (p *liveParser) exitCodeNoWrap(stream Stream) {
	_ = stream
}

func (p *liveParser) updateList(indent int, ordered bool, marker rune, start int, markerLen int, padding int) (int, listState) {
	for len(p.listStack) > 0 && indent < p.listStack[len(p.listStack)-1].indent {
		p.popList()
	}
	if len(p.listStack) == 0 ||
		indent > p.listStack[len(p.listStack)-1].indent ||
		p.listStack[len(p.listStack)-1].ordered != ordered ||
		(ordered && p.listStack[len(p.listStack)-1].marker != marker) {
		if len(p.listStack) > 0 && indent == p.listStack[len(p.listStack)-1].indent {
			p.popList()
		}
		state := listState{
			indent:        indent,
			ordered:       ordered,
			marker:        marker,
			next:          start,
			contentIndent: markerLen + padding,
			prefixLen:     markerLen + 1,
		}
		p.listStack = append(p.listStack, state)
		p.listPrefixLen += state.prefixLen
	}
	parentPrefixLen := p.listParentPrefixLen()
	return parentPrefixLen, p.listStack[len(p.listStack)-1]
}

func (p *liveParser) listParentPrefixLen() int {
	if len(p.listStack) == 0 {
		return 0
	}
	last := p.listStack[len(p.listStack)-1]
	if p.listPrefixLen <= last.prefixLen {
		return 0
	}
	return p.listPrefixLen - last.prefixLen
}

func (p *liveParser) popList() {
	if len(p.listStack) == 0 {
		return
	}
	last := p.listStack[len(p.listStack)-1]
	p.listStack = p.listStack[:len(p.listStack)-1]
	if last.prefixLen <= p.listPrefixLen {
		p.listPrefixLen -= last.prefixLen
	} else {
		p.listPrefixLen = 0
	}
}

func (p *liveParser) inListContinuation(indent int, trimmed string, explicitQuote bool, lineIndent int) bool {
	if len(p.listStack) == 0 {
		return false
	}
	if explicitQuote && lineIndent > 0 {
		return false
	}
	if strings.HasPrefix(strings.TrimLeft(trimmed, " \t"), ">") {
		return false
	}
	state := p.listStack[len(p.listStack)-1]
	if indent >= state.contentIndent+state.itemIndentExtra {
		return true
	}
	if p.listLazy && strings.TrimSpace(trimmed) != "" {
		return true
	}
	return false
}

func (p *liveParser) trimListIndent(text string) string {
	if len(p.listStack) == 0 {
		return strings.TrimLeft(text, " \t")
	}
	state := p.listStack[len(p.listStack)-1]
	if indent, _ := leadingIndentCount(text); indent >= state.contentIndent+state.itemIndentExtra {
		return strings.TrimLeft(trimIndent(text, state.contentIndent+state.itemIndentExtra), " \t")
	}
	return strings.TrimLeft(text, " \t")
}

func (p *liveParser) clearListIfOutdented(indent int) {
	if len(p.listStack) == 0 {
		return
	}
	if !p.listLazy && indent <= p.listStack[len(p.listStack)-1].indent {
		p.listStack = p.listStack[:0]
		p.listPrefixLen = 0
	}
}

func (p *liveParser) resetInline() {
	p.inline.reset()
}

func (p *liveParser) softBreakMode(quoteDepth int) breakMode {
	_ = quoteDepth
	return breakSpace
}

func taskListExtraIndent(content string) int {
	if len(content) < 4 {
		return 0
	}
	if content[0] != '[' || content[2] != ']' || content[3] != ' ' {
		return 0
	}
	switch content[1] {
	case ' ', 'x', 'X':
		return 4
	default:
		return 0
	}
}

func (s *inlineState) reset() {
	s.inCode = false
	s.codeFence = 0
	s.codeBuf = s.codeBuf[:0]
	s.pendingBackticks = 0
	s.inEm = false
	s.inStrong = false
	s.inLink = false
	s.inLinkURL = false
	s.inAutoLink = false
	s.inEntity = false
	s.pendingNumUS = false
	s.lastWasDigit = false
	s.pendingDelim = 0
	s.pendingCount = 0
	s.pendingClose = false
	s.linkText = s.linkText[:0]
	s.linkURL = s.linkURL[:0]
	s.autoLink = s.autoLink[:0]
	s.entity = s.entity[:0]
}

func parseQuotePrefix(line string) (int, string, bool) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	j := i
	depth := 0
	for j < len(line) && line[j] == '>' {
		depth++
		j++
		if j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
	}
	if depth == 0 {
		return 0, line, false
	}
	return depth, line[j:], true
}

func quoteLineHasTrailingSpace(line string) bool {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	depth := 0
	lastHadSpace := false
	for i < len(line) && line[i] == '>' {
		depth++
		i++
		lastHadSpace = false
		if i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			lastHadSpace = true
			i++
		}
	}
	if depth == 0 {
		return false
	}
	return lastHadSpace
}

func parseListMarker(text string) (bool, rune, int, int, int, string, bool) {
	if text == "" {
		return false, 0, 0, 0, 0, "", false
	}
	switch text[0] {
	case '-', '+', '*':
		if len(text) < 2 || !isSpace(text[1]) {
			return false, 0, 0, 0, 0, "", false
		}
		padding, idx := countSpaces(text[1:])
		if padding == 0 {
			return false, 0, 0, 0, 0, "", false
		}
		return false, rune(text[0]), 0, 1, padding, text[1+idx:], true
	}
	i := 0
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(text) {
		return false, 0, 0, 0, 0, "", false
	}
	if text[i] != '.' && text[i] != ')' {
		return false, 0, 0, 0, 0, "", false
	}
	if i+1 >= len(text) || !isSpace(text[i+1]) {
		return false, 0, 0, 0, 0, "", false
	}
	num := 0
	for j := 0; j < i; j++ {
		num = num*10 + int(text[j]-'0')
	}
	padding, idx := countSpaces(text[i+1:])
	if padding == 0 {
		return false, 0, 0, 0, 0, "", false
	}
	return true, rune(text[i]), num, i + 1, padding, text[i+1+idx:], true
}

func parseHeading(text string) (int, string, bool) {
	if !strings.HasPrefix(text, "#") {
		return 0, "", false
	}
	level := 0
	for level < len(text) && text[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, "", false
	}
	if level >= len(text) || text[level] != ' ' {
		return 0, "", false
	}
	content := strings.TrimSpace(text[level+1:])
	return level, content, true
}

func fenceMarker(text string) string {
	trim := strings.TrimSpace(text)
	if strings.HasPrefix(trim, "```") {
		return "```"
	}
	if strings.HasPrefix(trim, "~~~") {
		return "~~~"
	}
	return ""
}

func isMaybeFence(text string) bool {
	trim := strings.TrimSpace(text)
	if len(trim) == 0 || len(trim) >= 3 {
		return false
	}
	ch := trim[0]
	if ch != '`' && ch != '~' {
		return false
	}
	for i := 0; i < len(trim); i++ {
		if trim[i] != ch {
			return false
		}
	}
	return true
}

func isThematicBreak(text string) bool {
	trim := strings.TrimSpace(text)
	if len(trim) < 3 {
		return false
	}
	ch := trim[0]
	if ch != '-' && ch != '*' && ch != '_' {
		return false
	}
	for i := 0; i < len(trim); i++ {
		if trim[i] != ch {
			return false
		}
	}
	return true
}

func isMaybeThematicBreak(text string) bool {
	trim := strings.TrimSpace(text)
	if len(trim) == 0 || len(trim) >= 3 {
		return false
	}
	ch := trim[0]
	if ch != '-' && ch != '*' && ch != '_' {
		return false
	}
	for i := 0; i < len(trim); i++ {
		if trim[i] != ch {
			return false
		}
	}
	return true
}

func leadingIndentCount(s string) (int, int) {
	count := 0
	i := 0
	for i < len(s) {
		if s[i] == ' ' {
			count++
			i++
			continue
		}
		if s[i] == '\t' {
			count += 4
			i++
			continue
		}
		break
	}
	return count, i
}

func trimIndent(s string, count int) string {
	i := 0
	for i < len(s) && count > 0 {
		if s[i] == ' ' {
			count--
			i++
			continue
		}
		if s[i] == '\t' {
			count -= 4
			i++
			continue
		}
		break
	}
	return s[i:]
}

func countSpaces(s string) (int, int) {
	count := 0
	i := 0
	for i < len(s) && isSpace(s[i]) {
		count++
		i++
	}
	return count, i
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

func bytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func (p *liveParser) bytesTokenText(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	start := len(p.textArena)
	p.textArena = append(p.textArena, b...)
	return bytesToString(p.textArena[start:len(p.textArena)])
}

func (p *liveParser) appendSpaces(count int) {
	if count <= 0 {
		return
	}
	for count > 0 {
		n := count
		if n > len(spaceString) {
			n = len(spaceString)
		}
		p.textArena = append(p.textArena, spaceString[:n]...)
		count -= n
	}
}

func (p *liveParser) spaces(count int) string {
	if count <= 0 {
		return ""
	}
	if count <= len(spaceString) {
		return spaceString[:count]
	}
	start := len(p.textArena)
	p.appendSpaces(count)
	return bytesToString(p.textArena[start:len(p.textArena)])
}

func (p *liveParser) runeTokenText(r rune) string {
	if r >= 0 && r < 128 {
		return asciiRuneStrings[r]
	}
	var buf [utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], r)
	start := len(p.textArena)
	p.textArena = append(p.textArena, buf[:n]...)
	return bytesToString(p.textArena[start : start+n])
}

func (p *liveParser) reserveTextArena(n int) {
	if cap(p.textArena) >= n {
		return
	}
	buf := make([]byte, 0, n)
	buf = append(buf, p.textArena...)
	p.textArena = buf
}

func (p *liveParser) reserveLineBuffers(maxBytes int, maxRunes int) {
	if maxBytes > 0 && cap(p.lineBytes) < maxBytes {
		p.lineBytes = make([]byte, 0, maxBytes)
	}
	if maxRunes > 0 && cap(p.lineBuf) < maxRunes {
		p.lineBuf = make([]rune, 0, maxRunes)
	}
}

func (p *liveParser) reserveInlineBuffers(maxBytes int) {
	if maxBytes <= 0 {
		return
	}
	if cap(p.inline.codeBuf) < maxBytes {
		p.inline.codeBuf = make([]byte, 0, maxBytes)
	}
	if cap(p.inline.linkText) < maxBytes {
		p.inline.linkText = make([]byte, 0, maxBytes)
	}
	if cap(p.inline.linkURL) < maxBytes {
		p.inline.linkURL = make([]byte, 0, maxBytes)
	}
	if cap(p.inline.autoLink) < maxBytes {
		p.inline.autoLink = make([]byte, 0, maxBytes)
	}
}

func (p *liveParser) orderedMarkerText(num int, marker rune) string {
	if num >= 0 && num <= maxOrderedMarker {
		if marker == '.' {
			return orderedMarkerDot[num]
		}
		if marker == ')' {
			return orderedMarkerParen[num]
		}
	}
	start := len(p.textArena)
	p.textArena = strconv.AppendInt(p.textArena, int64(num), 10)
	p.textArena = append(p.textArena, byte(marker))
	return bytesToString(p.textArena[start:len(p.textArena)])
}

func (p *liveParser) emitInline(stream Stream, r rune) error {
	if p.inline.pendingNumUS {
		p.inline.pendingNumUS = false
		if r >= '0' && r <= '9' && !p.inline.inCode && !p.inline.inLink && !p.inline.inLinkURL && !p.inline.inAutoLink {
			style, kind := p.inlineStyle()
			if err := stream.WriteToken(StreamToken{Token: Token{Text: "\u00A0", Style: style, Kind: kind}}); err != nil {
				return err
			}
		} else {
			style, kind := p.inlineStyle()
			if err := stream.WriteToken(StreamToken{Token: Token{Text: "_", Style: style, Kind: kind}}); err != nil {
				return err
			}
		}
	}
	if p.inline.inAutoLink {
		switch r {
		case '>':
			text := p.bytesTokenText(p.inline.autoLink)
			p.inline.autoLink = p.inline.autoLink[:0]
			p.inline.inAutoLink = false
			return p.emitAutoLink(stream, text)
		case '\n', ' ', '\t':
			start := len(p.textArena)
			p.textArena = append(p.textArena, '<')
			p.textArena = append(p.textArena, p.inline.autoLink...)
			text := bytesToString(p.textArena[start:len(p.textArena)])
			p.inline.autoLink = p.inline.autoLink[:0]
			p.inline.inAutoLink = false
			if err := p.emitStyledText(stream, text); err != nil {
				return err
			}
			// fall through to handle the current rune normally
		default:
			p.inline.lastWasDigit = false
			p.inline.autoLink = utf8.AppendRune(p.inline.autoLink, r)
			return nil
		}
	}
	if p.inline.inEntity {
		if r == ';' {
			p.inline.entity = append(p.inline.entity, ';')
			if ent, ok := decodeEntity(p.inline.entity); ok {
				p.inline.inEntity = false
				p.inline.entity = p.inline.entity[:0]
				return p.emitStyledText(stream, p.runeTokenText(ent))
			}
			text := bytesToString(p.inline.entity)
			p.inline.inEntity = false
			p.inline.entity = p.inline.entity[:0]
			return p.emitStyledText(stream, text)
		}
		if r == ' ' || r == '\t' || r == '\n' || len(p.inline.entity) >= maxEntityLen {
			text := bytesToString(p.inline.entity)
			p.inline.inEntity = false
			p.inline.entity = p.inline.entity[:0]
			if err := p.emitStyledText(stream, text); err != nil {
				return err
			}
			// fall through to handle the current rune normally
		} else {
			p.inline.entity = utf8.AppendRune(p.inline.entity, r)
			return nil
		}
	}
	if p.inline.pendingCount > 0 && r != p.inline.pendingDelim {
		p.flushPendingDelims()
	}
	if p.inline.inLink && p.inline.pendingClose && (r == ' ' || r == '\t') {
		p.inline.pendingClose = false
		_ = stream.WriteToken(StreamToken{Token: Token{Text: "[", Style: p.styles.Text}})
		_ = stream.WriteToken(StreamToken{Token: Token{Text: p.bytesTokenText(p.inline.linkText), Style: p.styles.Text}})
		_ = stream.WriteToken(StreamToken{Token: Token{Text: "]", Style: p.styles.Text}})
		p.inline.inLink = false
		p.inline.linkText = p.inline.linkText[:0]
		p.inline.linkURL = p.inline.linkURL[:0]
	}
	if p.lineDecided {
		if r == ' ' || r == '\t' {
			if p.inline.pendingBackticks == 0 && !p.inline.inCode && !p.inline.inLink && !p.inline.inLinkURL {
				p.immediateSpaces = append(p.immediateSpaces, r)
				return nil
			}
		} else if len(p.immediateSpaces) > 0 {
			style, kind := p.inlineStyle()
			for _, sp := range p.immediateSpaces {
				if err := stream.WriteToken(StreamToken{Token: Token{Text: p.runeTokenText(sp), Style: style, Kind: kind}}); err != nil {
					return err
				}
			}
			p.immediateSpaces = p.immediateSpaces[:0]
		}
	}
	if r == '`' {
		p.inline.pendingBackticks++
		return nil
	}
	if p.inline.pendingBackticks > 0 {
		if err := p.emitInlineBackticks(stream, p.inline.pendingBackticks); err != nil {
			return err
		}
		p.inline.pendingBackticks = 0
	}
	if r == '&' && !p.inline.inCode && !p.inline.inAutoLink && !p.inline.inLinkURL {
		p.inline.inEntity = true
		p.inline.entity = p.inline.entity[:0]
		p.inline.entity = append(p.inline.entity, '&')
		return nil
	}
	if r == '_' && p.inline.lastWasDigit && !p.inline.inCode && !p.inline.inLink && !p.inline.inLinkURL && !p.inline.inAutoLink {
		p.inline.pendingNumUS = true
		p.inline.lastWasDigit = false
		return nil
	}
	if p.inline.inCode {
		p.inline.codeBuf = utf8.AppendRune(p.inline.codeBuf, r)
		p.inline.lastWasDigit = false
		return nil
	}
	switch r {
	case '*', '_':
		if !p.inline.inCode && !p.inline.inLink {
			if p.inline.pendingDelim == r {
				p.inline.pendingCount++
			} else {
				p.inline.pendingDelim = r
				p.inline.pendingCount = 1
			}
			return nil
		}
	case '[':
		if !p.inline.inCode && !p.inline.inLink {
			p.inline.inLink = true
			p.inline.linkText = p.inline.linkText[:0]
			p.inline.linkURL = p.inline.linkURL[:0]
			return nil
		}
	case ']':
		if p.inline.inLink && !p.inline.inCode {
			p.inline.pendingClose = true
			return nil
		}
	case '(':
		if p.inline.inLink && p.inline.pendingClose {
			p.inline.pendingClose = false
			p.inline.inLinkURL = true
			return nil
		}
	case ')':
		if p.inline.inLink && p.inline.inLinkURL {
			return p.emitLink(stream)
		}
	case '<':
		if !p.inline.inCode {
			if p.inline.inLink && !p.inline.inLinkURL && !p.inline.pendingClose && len(p.inline.linkText) == 0 {
				_ = stream.WriteToken(StreamToken{Token: Token{Text: "[", Style: p.styles.Text}})
				p.inline.inLink = false
				p.inline.linkText = p.inline.linkText[:0]
				p.inline.linkURL = p.inline.linkURL[:0]
				p.inline.inAutoLink = true
				p.inline.autoLink = p.inline.autoLink[:0]
				return nil
			}
			if !p.inline.inLink {
				p.inline.inAutoLink = true
				p.inline.autoLink = p.inline.autoLink[:0]
				return nil
			}
		}
	}

	if p.inline.inLink {
		if p.inline.pendingClose {
			p.inline.pendingClose = false
			if r != '(' {
				_ = stream.WriteToken(StreamToken{Token: Token{Text: "[", Style: p.styles.Text}})
				_ = stream.WriteToken(StreamToken{Token: Token{Text: p.bytesTokenText(p.inline.linkText), Style: p.styles.Text}})
				_ = stream.WriteToken(StreamToken{Token: Token{Text: "]", Style: p.styles.Text}})
				p.inline.inLink = false
				p.inline.linkText = p.inline.linkText[:0]
				p.inline.linkURL = p.inline.linkURL[:0]
			}
		}
		if p.inline.inLink && p.inline.inLinkURL {
			p.inline.lastWasDigit = false
			p.inline.linkURL = utf8.AppendRune(p.inline.linkURL, r)
			return nil
		}
		if p.inline.inLink {
			p.inline.lastWasDigit = false
			p.inline.linkText = utf8.AppendRune(p.inline.linkText, r)
			return nil
		}
	}

	style, kind := p.inlineStyle()
	p.inline.lastWasDigit = r >= '0' && r <= '9'
	return stream.WriteToken(StreamToken{Token: Token{Text: p.runeTokenText(r), Style: style, Kind: kind}})
}

func (p *liveParser) inlineStyle() (Style, tokenKind) {
	style := p.styles.Text
	kind := tokenText
	if p.inline.inCode {
		style = p.styles.CodeInline
		kind = tokenCode
	} else if p.inline.inEm && p.inline.inStrong {
		style = p.styles.EmphasisStrong
	} else if p.inline.inStrong {
		style = p.styles.Strong
	} else if p.inline.inEm {
		style = p.styles.Emphasis
	} else if p.lineStyled {
		style = p.lineStyle
	}
	if p.lineStyled && style.Prefix != p.lineStyle.Prefix && style.Prefix != p.styles.Text.Prefix {
		style = combineStyles(p.lineStyle, style)
	}
	return style, kind
}

func (p *liveParser) emitStyledText(stream Stream, text string) error {
	if text == "" {
		return nil
	}
	style, kind := p.inlineStyle()
	if kind != tokenCode && strings.Contains(text, "&") {
		return p.emitStyledTextWithNBSP(stream, text, style, kind)
	}
	for _, r := range text {
		if err := stream.WriteToken(StreamToken{Token: Token{Text: p.runeTokenText(r), Style: style, Kind: kind}}); err != nil {
			return err
		}
	}
	return nil
}

func (p *liveParser) emitStyledTextWithNBSP(stream Stream, text string, style Style, kind tokenKind) error {
	for i := 0; i < len(text); {
		if text[i] == '&' && i+6 <= len(text) {
			seg := text[i : i+6]
			if strings.EqualFold(seg, "&nbsp;") {
				if err := stream.WriteToken(StreamToken{Token: Token{Text: p.runeTokenText('\u00A0'), Style: style, Kind: kind}}); err != nil {
					return err
				}
				i += 6
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			r = rune(text[i])
			size = 1
		}
		if err := stream.WriteToken(StreamToken{Token: Token{Text: p.runeTokenText(r), Style: style, Kind: kind}}); err != nil {
			return err
		}
		i += size
	}
	return nil
}

func (p *liveParser) emitAutoLink(stream Stream, text string) error {
	if text == "" {
		return p.emitStyledText(stream, "<>")
	}
	link := ""
	switch {
	case isEmailAutolink(text):
		link = "mailto:" + text
	case isSchemeAutolink(text):
		link = text
	default:
		return p.emitStyledText(stream, "<"+text+">")
	}
	if p.osc8 {
		if err := stream.WriteToken(StreamToken{Token: Token{Kind: tokenLinkStart, LinkURL: link}}); err != nil {
			return err
		}
	}
	for _, r := range text {
		if err := stream.WriteToken(StreamToken{Token: Token{Text: p.runeTokenText(r), Style: p.styles.LinkText, Kind: tokenURL}}); err != nil {
			return err
		}
	}
	if p.osc8 {
		return stream.WriteToken(StreamToken{Token: Token{Kind: tokenLinkEnd}})
	}
	return nil
}

func isEmailAutolink(text string) bool {
	if text == "" || strings.ContainsAny(text, " \t\r\n") {
		return false
	}
	if strings.Contains(text, "://") || strings.Contains(text, ":") {
		return false
	}
	at := strings.IndexByte(text, '@')
	if at <= 0 || at == len(text)-1 {
		return false
	}
	if strings.Count(text, "@") != 1 {
		return false
	}
	return true
}

func isSchemeAutolink(text string) bool {
	if text == "" || strings.ContainsAny(text, " \t\r\n") {
		return false
	}
	colon := strings.IndexByte(text, ':')
	if colon <= 0 {
		return false
	}
	if !isScheme(text[:colon]) {
		return false
	}
	return true
}

func isScheme(s string) bool {
	for i, r := range s {
		if i == 0 {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
				return false
			}
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '+' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return len(s) > 0
}

func (p *liveParser) emitInlineBackticks(stream Stream, count int) error {
	if p.inline.pendingCount > 0 && p.inline.pendingDelim != '`' {
		p.flushPendingDelims()
	}
	if !p.inline.inCode {
		p.inline.inCode = true
		p.inline.codeFence = count
		p.inline.codeBuf = p.inline.codeBuf[:0]
		return nil
	}
	if count == p.inline.codeFence {
		p.inline.inCode = false
		p.inline.codeFence = 0
		text := p.bytesTokenText(p.inline.codeBuf)
		p.inline.codeBuf = p.inline.codeBuf[:0]
		if len(text) >= 2 && text[0] == ' ' && text[len(text)-1] == ' ' {
			text = text[1 : len(text)-1]
		}
		if text != "" {
			return stream.WriteToken(StreamToken{Token: Token{Text: text, Style: p.styles.CodeInline, Kind: tokenCode}})
		}
		return nil
	}
	for i := 0; i < count; i++ {
		p.inline.codeBuf = append(p.inline.codeBuf, '`')
	}
	return nil
}

func (p *liveParser) finalize(stream Stream) {
	if len(p.lineBuf) > 0 {
		if p.lineDecided {
			_ = p.emitInlineRunes(stream, p.lineBuf[p.lineEmitIdx:])
			_ = p.flushPendingBackticks(stream)
			_ = p.flushPendingEntity(stream)
			_ = p.flushPendingNumUS(stream)
			p.flushPendingDelims()
			p.lineStyled = false
		} else {
			if p.inIndentCode {
				_ = p.maybeDecideIndentCodeLine(stream, true)
				if p.codeLineDecided && p.codeLineIsCode {
					p.resetLine()
				}
			} else {
				_ = p.maybeDecideLine(stream, true)
				if p.lineDecided && p.lineEmitIdx < len(p.lineBuf) {
					_ = p.emitInlineRunes(stream, p.lineBuf[p.lineEmitIdx:])
					_ = p.flushPendingBackticks(stream)
					_ = p.flushPendingEntity(stream)
					_ = p.flushPendingNumUS(stream)
					p.flushPendingDelims()
					p.lineStyled = false
				}
			}
		}
		p.resetLine()
	}
	if p.inCodeFence {
		p.inCodeFence = false
		p.fenceMarker = ""
		p.pendingCodeNL = false
		p.exitCodeNoWrap(stream)
	}
	if p.inIndentCode {
		p.inIndentCode = false
		p.exitCodeNoWrap(stream)
	}
	if p.inline.inLink {
		_ = stream.WriteToken(StreamToken{Token: Token{Text: "[", Style: p.styles.Text}})
		_ = stream.WriteToken(StreamToken{Token: Token{Text: p.bytesTokenText(p.inline.linkText), Style: p.styles.Text}})
		if p.inline.inLinkURL && len(p.inline.linkURL) > 0 {
			_ = stream.WriteToken(StreamToken{Token: Token{Text: "](", Style: p.styles.Text}})
			_ = stream.WriteToken(StreamToken{Token: Token{Text: p.bytesTokenText(p.inline.linkURL), Style: p.styles.Text}})
			_ = stream.WriteToken(StreamToken{Token: Token{Text: ")", Style: p.styles.Text}})
		} else {
			_ = stream.WriteToken(StreamToken{Token: Token{Text: "]", Style: p.styles.Text}})
		}
		p.inline.inLink = false
		p.inline.inLinkURL = false
		p.inline.linkText = p.inline.linkText[:0]
		p.inline.linkURL = p.inline.linkURL[:0]
	}
	if p.inline.inAutoLink {
		start := len(p.textArena)
		p.textArena = append(p.textArena, '<')
		p.textArena = append(p.textArena, p.inline.autoLink...)
		text := bytesToString(p.textArena[start:len(p.textArena)])
		p.inline.inAutoLink = false
		p.inline.autoLink = p.inline.autoLink[:0]
		_ = p.emitStyledText(stream, text)
	}
}

func (p *liveParser) flushPendingBackticks(stream Stream) error {
	if p.inline.pendingBackticks == 0 {
		return nil
	}
	if err := p.emitInlineBackticks(stream, p.inline.pendingBackticks); err != nil {
		return err
	}
	p.inline.pendingBackticks = 0
	return nil
}

func (p *liveParser) flushPendingDelims() {
	if p.inline.pendingCount == 0 {
		return
	}
	if p.inline.pendingCount >= 2 {
		p.inline.inStrong = !p.inline.inStrong
		p.inline.pendingCount -= 2
	}
	if p.inline.pendingCount >= 1 {
		p.inline.inEm = !p.inline.inEm
		p.inline.pendingCount = 0
	}
	p.inline.pendingDelim = 0
}

func (p *liveParser) flushPendingEntity(stream Stream) error {
	if !p.inline.inEntity || len(p.inline.entity) == 0 {
		return nil
	}
	text := bytesToString(p.inline.entity)
	p.inline.inEntity = false
	p.inline.entity = p.inline.entity[:0]
	return p.emitStyledText(stream, text)
}

func (p *liveParser) flushPendingNumUS(stream Stream) error {
	if !p.inline.pendingNumUS {
		return nil
	}
	p.inline.pendingNumUS = false
	style, kind := p.inlineStyle()
	return stream.WriteToken(StreamToken{Token: Token{Text: "_", Style: style, Kind: kind}})
}

func (p *liveParser) emitLink(stream Stream) error {
	text := p.bytesTokenText(p.inline.linkText)
	url := p.bytesTokenText(p.inline.linkURL)
	p.inline.inLink = false
	p.inline.inLinkURL = false
	p.inline.pendingClose = false
	p.inline.linkText = p.inline.linkText[:0]
	p.inline.linkURL = p.inline.linkURL[:0]
	if p.osc8 && url != "" {
		if err := stream.WriteToken(StreamToken{Token: Token{Kind: tokenLinkStart, LinkURL: url}}); err != nil {
			return err
		}
		if err := p.emitLinkText(stream, text); err != nil {
			return err
		}
		return stream.WriteToken(StreamToken{Token: Token{Kind: tokenLinkEnd}})
	}
	if err := p.emitLinkText(stream, text); err != nil {
		return err
	}
	if url != "" {
		if err := stream.WriteToken(StreamToken{Token: Token{Text: " (", Style: p.styles.Text}}); err != nil {
			return err
		}
		if err := stream.WriteToken(StreamToken{Token: Token{Text: url, Style: p.styles.LinkURL, Kind: tokenURL}}); err != nil {
			return err
		}
		if err := stream.WriteToken(StreamToken{Token: Token{Text: ")", Style: p.styles.Text}}); err != nil {
			return err
		}
	}
	return nil
}

func (p *liveParser) emitLinkText(stream Stream, text string) error {
	state := linkInlineState{}
	baseEm := p.inline.inEm
	baseStrong := p.inline.inStrong
	baseLine := Style{}
	if p.lineStyled {
		baseLine = p.lineStyle
	}
	outerEmph := p.emphasisStyle(baseEm, baseStrong)
	for _, r := range text {
		if r == '*' || r == '_' {
			if state.pendingDelim == r {
				state.pendingCount++
			} else {
				state.flushPending()
				state.pendingDelim = r
				state.pendingCount = 1
			}
			continue
		}
		if state.pendingCount > 0 && r != state.pendingDelim {
			state.flushPending()
		}
		innerEmph := p.emphasisStyle(state.inEm, state.inStrong)
		style := combineStyles(combineStyles(combineStyles(baseLine, outerEmph), p.styles.LinkText), innerEmph)
		if err := stream.WriteToken(StreamToken{Token: Token{Text: p.runeTokenText(r), Style: style, Kind: tokenText}}); err != nil {
			return err
		}
	}
	state.flushPending()
	return nil
}

func (p *liveParser) emphasisStyle(inEm bool, inStrong bool) Style {
	switch {
	case inEm && inStrong:
		return p.styles.EmphasisStrong
	case inStrong:
		return p.styles.Strong
	case inEm:
		return p.styles.Emphasis
	default:
		return Style{}
	}
}

type linkInlineState struct {
	inEm         bool
	inStrong     bool
	pendingDelim rune
	pendingCount int
}

func (s *linkInlineState) flushPending() {
	if s.pendingCount == 0 {
		return
	}
	if s.pendingCount >= 2 {
		s.inStrong = !s.inStrong
		s.pendingCount -= 2
	}
	if s.pendingCount >= 1 {
		s.inEm = !s.inEm
		s.pendingCount = 0
	}
	s.pendingDelim = 0
}

func combineStyles(base Style, extra Style) Style {
	if base.Prefix == "" {
		return extra
	}
	if extra.Prefix == "" {
		return base
	}
	return Style{Prefix: base.Prefix + extra.Prefix}
}

func (p *liveParser) quoteWrapIndent(depth int, listPrefixLen int) string {
	start := len(p.textArena)
	for i := 0; i < depth; i++ {
		if p.styles.Quote.Prefix != "" {
			p.textArena = append(p.textArena, p.styles.Quote.Prefix...)
		}
		p.textArena = append(p.textArena, '>')
		if p.styles.Quote.Prefix != "" {
			p.textArena = append(p.textArena, ansiReset...)
		}
		p.textArena = append(p.textArena, ' ')
	}
	if listPrefixLen > 0 {
		p.appendSpaces(listPrefixLen)
	}
	return bytesToString(p.textArena[start:len(p.textArena)])
}

func (p *liveParser) quoteWrapIndentListFirst(depth int, listPrefixLen int) string {
	start := len(p.textArena)
	if listPrefixLen > 0 {
		p.appendSpaces(listPrefixLen)
	}
	for i := 0; i < depth; i++ {
		if p.styles.Quote.Prefix != "" {
			p.textArena = append(p.textArena, p.styles.Quote.Prefix...)
		}
		p.textArena = append(p.textArena, '>')
		if p.styles.Quote.Prefix != "" {
			p.textArena = append(p.textArena, ansiReset...)
		}
		p.textArena = append(p.textArena, ' ')
	}
	return bytesToString(p.textArena[start:len(p.textArena)])
}

type breakMode uint8

const (
	breakSpace breakMode = iota
	breakSingle
	breakDouble
)

func (p *liveParser) applyPendingBreak(stream Stream, mode breakMode) error {
	if p.pendingBreaks == 0 {
		return nil
	}
	if p.pendingBreaks >= 2 {
		mode = breakDouble
	}
	if p.hardBreakPending && mode == breakSpace {
		mode = breakSingle
	}
	if p.postCodeBreakSingle && mode == breakDouble {
		mode = breakSingle
	}
	switch mode {
	case breakDouble:
		p.pendingBreaks = 0
		p.hardBreakPending = false
		p.postCodeBreakSingle = false
		return stream.WriteToken(StreamToken{Token: Token{Text: "\n\n", Style: Style{}, Kind: tokenText}})
	case breakSingle:
		p.pendingBreaks = 0
		p.hardBreakPending = false
		p.postCodeBreakSingle = false
		return stream.WriteToken(StreamToken{Token: Token{Text: "\n", Style: Style{}, Kind: tokenText}})
	default:
		p.pendingBreaks = 0
		p.hardBreakPending = false
		p.postCodeBreakSingle = false
		style, kind := p.inlineStyle()
		return stream.WriteToken(StreamToken{Token: Token{Text: " ", Style: style, Kind: kind}})
	}
}

func hasHardLineBreak(line string) bool {
	count := 0
	for i := len(line) - 1; i >= 0; i-- {
		if line[i] == ' ' {
			count++
			continue
		}
		break
	}
	return count >= 2
}
