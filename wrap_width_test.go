package mdf

import (
	"strings"
	"testing"

	"github.com/muesli/reflow/ansi"
)

func TestWrapWidthBounds(t *testing.T) {
	src := strings.Join([]string{
		"# Heading One",
		"",
		"Paragraph with a [link](https://example.com) and some emphasized *text* plus **bold** words.",
		"",
		"> Quote line one with more words to wrap",
		"> Quote line two with additional words to wrap",
		"",
		"- item one with a long line that should wrap cleanly at small widths",
		"  - nested item with more words and wrapping",
		"",
		"```go",
		"fmt.Println(\"hello there from a longer code line\")",
		"```",
	}, "\n")

	assertWidths := func(name string, render func(width int) string, minWidth int, allowCodeOverflow bool) {
		for width := minWidth; width <= 100; width += 5 {
			out := render(width)
			lines := strings.Split(out, "\n")
			for i, line := range lines {
				plain := stripANSI(line)
				if allowCodeOverflow && strings.HasPrefix(strings.TrimLeft(plain, " \t"), "fmt.Println(") {
					continue
				}
				if ansi.PrintableRuneWidth(plain) > width {
					t.Fatalf("%s: line %d exceeds width %d: %q", name, i+1, width, plain)
				}
			}
		}
	}

	linkMinWidth := len("(https://example.com)")
	assertWidths("wrap", func(width int) string {
		return renderStream(t, []byte(src), width)
	}, linkMinWidth, true)

	assertWidths("wrap-osc8", func(width int) string {
		return renderStreamWithOptions(t, []byte(src), width, WithOSC8(true))
	}, 20, true)
}
