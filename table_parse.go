package mdf

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func parseTableRow(line string) ([]string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || !hasUnescapedPipe(line) {
		return nil, false
	}
	hasEdgePipe := strings.HasPrefix(line, "|") || (strings.HasSuffix(line, "|") && !hasEscapedTrailingPipe(line))
	if !hasEdgePipe {
		return nil, false
	}
	cells := splitTableCells(line)
	if len(cells) < 2 && !(hasEdgePipe && len(cells) == 1) {
		return nil, false
	}
	return cells, true
}

func parseTableDelimiter(line string, columns int) ([]TableAlignment, bool) {
	cells, ok := parseTableRow(line)
	if !ok || len(cells) != columns {
		return nil, false
	}
	alignments := make([]TableAlignment, 0, len(cells))
	for _, cell := range cells {
		align, ok := parseTableDelimiterCell(cell)
		if !ok {
			return nil, false
		}
		alignments = append(alignments, align)
	}
	return alignments, true
}

func parseTableDelimiterCell(cell string) (TableAlignment, bool) {
	cell = strings.TrimSpace(cell)
	if len(cell) < 1 {
		return TableAlignLeft, false
	}
	left := strings.HasPrefix(cell, ":")
	right := strings.HasSuffix(cell, ":")
	if left {
		cell = cell[1:]
	}
	if right {
		cell = cell[:len(cell)-1]
	}
	if len(cell) < 1 {
		return TableAlignLeft, false
	}
	for _, r := range cell {
		if r != '-' {
			return TableAlignLeft, false
		}
	}
	switch {
	case left && right:
		return TableAlignCenter, true
	case right:
		return TableAlignRight, true
	default:
		return TableAlignLeft, true
	}
}

func splitTableCells(line string) []string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "|") {
		line = line[1:]
	}
	if strings.HasSuffix(line, "|") && !hasEscapedTrailingPipe(line) {
		line = line[:len(line)-1]
	}
	var cells []string
	var b strings.Builder
	scanTableSeparators(line, func(text string) {
		_, _ = b.WriteString(text)
	}, func() bool {
		cells = append(cells, strings.TrimSpace(b.String()))
		b.Reset()
		return false
	})
	cells = append(cells, strings.TrimSpace(b.String()))
	return cells
}

func hasUnescapedPipe(line string) bool {
	return scanTableSeparators(line, nil, func() bool {
		return true
	})
}

func scanTableSeparators(line string, write func(string), separator func() bool) bool {
	escaped := false
	codeTicks := 0
	bracketDepth := 0
	linkDestDepth := 0
	linkDestQuote := rune(0)
	linkDestEscaped := false
	bracketEscaped := false
	inAngle := false
	angleEnd := -1
	afterBracket := false
	emit := func(text string) {
		if write != nil {
			write(text)
		}
	}
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if codeTicks > 0 {
			if r == '`' {
				ticks := countBacktickRun(line[i:])
				if ticks == codeTicks {
					emit(line[i : i+ticks])
					i += ticks
					codeTicks = 0
					continue
				}
			}
			emit(line[i : i+size])
			i += size
			continue
		}
		if escaped {
			if r != '|' && r != '\\' {
				emit("\\")
			}
			emit(line[i : i+size])
			escaped = false
			i += size
			continue
		}
		if r == '\\' {
			escaped = true
			i += size
			continue
		}
		if r == '`' {
			ticks := countBacktickRun(line[i:])
			if !hasClosingBacktickRun(line[i+ticks:], ticks) {
				emit(line[i : i+ticks])
				i += ticks
				afterBracket = false
				continue
			}
			emit(line[i : i+ticks])
			i += ticks
			codeTicks = ticks
			afterBracket = false
			continue
		}
		if inAngle {
			if i == angleEnd && r == '>' {
				inAngle = false
				angleEnd = -1
			}
			emit(line[i : i+size])
			i += size
			continue
		}
		if linkDestDepth > 0 {
			if linkDestEscaped {
				linkDestEscaped = false
			} else if r == '\\' {
				linkDestEscaped = true
			} else if linkDestQuote != 0 {
				if r == linkDestQuote {
					linkDestQuote = 0
				}
			} else {
				switch r {
				case '\'', '"':
					linkDestQuote = r
				case '(':
					linkDestDepth++
				case ')':
					linkDestDepth--
				}
			}
			emit(line[i : i+size])
			i += size
			continue
		}
		if bracketDepth > 0 {
			if bracketEscaped {
				bracketEscaped = false
			} else if r == '\\' {
				bracketEscaped = true
			} else {
				switch r {
				case '[':
					bracketDepth++
				case ']':
					bracketDepth--
					afterBracket = bracketDepth == 0
				}
			}
			emit(line[i : i+size])
			i += size
			continue
		}
		if afterBracket {
			afterBracket = false
			if r == '(' {
				if !hasClosingLinkDestination(line[i+size:]) {
					emit(line[i : i+size])
					i += size
					continue
				}
				linkDestDepth = 1
				emit(line[i : i+size])
				i += size
				continue
			}
		}
		if r == '<' {
			end := angleSpanEnd(line[i:])
			if end > 0 && isAngleSpanBody(line[i+1:i+end]) {
				angleEnd = i + end
				inAngle = true
				emit(line[i : i+size])
				i += size
				afterBracket = false
				continue
			}
		}
		if r == '[' {
			if !hasClosingBracket(line[i+size:]) {
				emit(line[i : i+size])
				i += size
				continue
			}
			bracketDepth = 1
			emit(line[i : i+size])
			i += size
			continue
		}
		if r == '|' {
			if separator == nil || separator() {
				return true
			}
			i += size
			continue
		}
		emit(line[i : i+size])
		i += size
	}
	if escaped {
		emit("\\")
	}
	return false
}

func countBacktickRun(s string) int {
	count := 0
	for count < len(s) && s[count] == '`' {
		count++
	}
	return count
}

func hasClosingBacktickRun(s string, ticks int) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if r != '`' {
			i += size
			continue
		}
		run := countBacktickRun(s[i:])
		if run == ticks {
			return true
		}
		i += run
	}
	return false
}

func hasClosingBracket(s string) bool {
	depth := 1
	escaped := false
	for _, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		switch r {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func hasClosingLinkDestination(s string) bool {
	depth := 1
	quote := rune(0)
	escaped := false
	for _, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func angleSpanEnd(s string) int {
	if len(s) < 3 || s[0] != '<' {
		return -1
	}
	quote := byte(0)
	for i := 1; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == '>' {
			return i
		}
	}
	return -1
}

func isAngleSpanBody(body string) bool {
	return isAutolinkAngleBody(body) || isHTMLTagAngleBody(body)
}

func isAutolinkAngleBody(body string) bool {
	if body == "" || strings.ContainsFunc(body, unicode.IsSpace) {
		return false
	}
	if at := strings.IndexByte(body, '@'); at > 0 && at < len(body)-1 {
		return !strings.ContainsAny(body, "<>")
	}
	colon := strings.IndexByte(body, ':')
	if colon <= 0 {
		return false
	}
	for i := 0; i < colon; i++ {
		c := body[i]
		if i == 0 {
			if !isASCIILetter(c) {
				return false
			}
			continue
		}
		if !isASCIILetter(c) && !isASCIIDigit(c) && c != '+' && c != '.' && c != '-' {
			return false
		}
	}
	return !strings.ContainsAny(body[colon+1:], "<>")
}

func isHTMLTagAngleBody(body string) bool {
	if body == "" {
		return false
	}
	if strings.HasPrefix(body, "!--") {
		return strings.Contains(body[3:], "--")
	}
	if body[0] == '/' {
		body = body[1:]
	}
	if body == "" || !isASCIILetter(body[0]) {
		return false
	}
	i := 1
	for i < len(body) && (isASCIILetter(body[i]) || isASCIIDigit(body[i]) || body[i] == '-') {
		i++
	}
	if i == len(body) {
		return true
	}
	if body[i] != '/' && !isASCIIWhitespace(body[i]) {
		return false
	}
	quote := byte(0)
	for ; i < len(body); i++ {
		c := body[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
	}
	return quote == 0
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isASCIIWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func hasEscapedTrailingPipe(line string) bool {
	if line == "" {
		return false
	}
	i := len(line)
	r, size := utf8.DecodeLastRuneInString(line)
	if r != '|' {
		return false
	}
	i -= size
	slashes := 0
	for i > 0 {
		r, size = utf8.DecodeLastRuneInString(line[:i])
		if r != '\\' {
			break
		}
		slashes++
		i -= size
	}
	return slashes%2 == 1
}
