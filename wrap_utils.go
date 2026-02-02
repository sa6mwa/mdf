package mdf

import (
	"strings"

	"github.com/muesli/reflow/ansi"
)

func truncateWithEllipsis(text string, limit int) string {
	if ansi.PrintableRuneWidth(text) <= limit {
		return text
	}
	if limit <= 0 {
		return ""
	}
	if limit == 1 {
		return "…"
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
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
