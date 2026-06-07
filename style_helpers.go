package mdf

import (
	"strconv"
	"strings"
)

func combineTableHeaderStyle(header Style, inline Style) Style {
	if inline.Prefix == "" {
		return header
	}
	return combineStyles(header, Style{Prefix: stripANSIForeground(inline.Prefix)})
}

func stripANSIForeground(prefix string) string {
	var b strings.Builder
	for i := 0; i < len(prefix); {
		if prefix[i] != '\x1b' || i+1 >= len(prefix) || prefix[i+1] != '[' {
			b.WriteByte(prefix[i])
			i++
			continue
		}
		end := strings.IndexByte(prefix[i+2:], 'm')
		if end < 0 {
			b.WriteString(prefix[i:])
			break
		}
		end += i + 2
		params := strings.Split(prefix[i+2:end], ";")
		kept := stripANSIForegroundParams(params)
		if len(kept) > 0 {
			b.WriteString("\x1b[")
			b.WriteString(strings.Join(kept, ";"))
			b.WriteByte('m')
		}
		i = end + 1
	}
	return b.String()
}

func stripANSIForegroundParams(params []string) []string {
	if len(params) == 0 {
		return nil
	}
	kept := make([]string, 0, len(params))
	for i := 0; i < len(params); i++ {
		n, err := strconv.Atoi(params[i])
		if err != nil {
			kept = append(kept, params[i])
			continue
		}
		switch {
		case n == 38:
			if i+1 < len(params) {
				mode, _ := strconv.Atoi(params[i+1])
				if mode == 5 {
					i += 2
					continue
				}
				if mode == 2 {
					i += 4
					continue
				}
			}
			continue
		case n == 39:
			continue
		case n >= 30 && n <= 37:
			continue
		case n >= 90 && n <= 97:
			continue
		default:
			kept = append(kept, params[i])
		}
	}
	return kept
}
