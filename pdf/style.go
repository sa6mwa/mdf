package pdf

import (
	"strconv"
	"strings"
)

type pdfStyle struct {
	fontFamily string
	fontStyle  string
	size       float64
	r          int
	g          int
	b          int
}

type ansiAttrs struct {
	bold      bool
	italic    bool
	underline bool
	colorSet  bool
	color     [3]int
}

func parseANSIPrefix(prefix string, defaultColor [3]int) ansiAttrs {
	attrs := ansiAttrs{color: defaultColor}
	if prefix == "" {
		return attrs
	}
	parts := strings.Split(prefix, "\x1b[")
	for _, part := range parts {
		if part == "" {
			continue
		}
		end := strings.IndexByte(part, 'm')
		if end == -1 {
			continue
		}
		codes := strings.Split(part[:end], ";")
		for i := 0; i < len(codes); i++ {
			code := codes[i]
			if code == "" {
				continue
			}
			n, err := strconv.Atoi(code)
			if err != nil {
				continue
			}
			switch {
			case n == 0:
				attrs.bold = false
				attrs.italic = false
				attrs.underline = false
				attrs.colorSet = false
				attrs.color = defaultColor
			case n == 1:
				attrs.bold = true
			case n == 3:
				attrs.italic = true
			case n == 4:
				attrs.underline = true
			case n >= 30 && n <= 37:
				attrs.color = ansiColor(n - 30)
				attrs.colorSet = true
			case n >= 90 && n <= 97:
				attrs.color = ansiColor(n - 90 + 8)
				attrs.colorSet = true
			case n == 38:
				if i+2 < len(codes) && codes[i+1] == "5" {
					idx, err := strconv.Atoi(codes[i+2])
					if err == nil {
						attrs.color = xtermColor(idx)
						attrs.colorSet = true
					}
					i += 2
				}
			}
		}
	}
	return attrs
}

func ansiColor(idx int) [3]int {
	colors := [16][3]int{
		{0, 0, 0},
		{205, 0, 0},
		{0, 205, 0},
		{205, 205, 0},
		{59, 156, 255},
		{205, 0, 205},
		{0, 205, 205},
		{229, 229, 229},
		{127, 127, 127},
		{255, 0, 0},
		{0, 255, 0},
		{255, 255, 0},
		{92, 92, 255},
		{255, 0, 255},
		{0, 255, 255},
		{255, 255, 255},
	}
	if idx < 0 || idx >= len(colors) {
		return colors[7]
	}
	return colors[idx]
}

func xtermColor(idx int) [3]int {
	switch {
	case idx < 16:
		return ansiColor(idx)
	case idx >= 16 && idx <= 231:
		idx -= 16
		r := idx / 36
		g := (idx / 6) % 6
		b := idx % 6
		return [3]int{
			colorLevel(r),
			colorLevel(g),
			colorLevel(b),
		}
	case idx >= 232 && idx <= 255:
		v := 8 + (idx-232)*10
		return [3]int{v, v, v}
	default:
		return ansiColor(7)
	}
}

func colorLevel(v int) int {
	if v == 0 {
		return 0
	}
	return 55 + v*40
}

func styleToFontStyle(attrs ansiAttrs, forceBold bool, allowBoldItalic bool) string {
	bold := attrs.bold || forceBold
	italic := attrs.italic
	if bold && italic && !allowBoldItalic {
		italic = false
	}
	var b strings.Builder
	if bold {
		b.WriteByte('B')
	}
	if italic {
		b.WriteByte('I')
	}
	if attrs.underline {
		b.WriteByte('U')
	}
	return b.String()
}
