package mdf

import (
	"strings"
	"testing"

	"pkt.systems/mdf/internal/palette"
)

func TestIntegrationRenderPlainAndANSI(t *testing.T) {
	src := strings.Join([]string{
		"# Title",
		"",
		"Paragraph with *emphasis*, **strong**, and ***strong+em*** plus `code`.",
		"",
		"> Quote line one",
		"> Quote line two",
		"",
		"- item one",
		"- item two",
		"  - nested one",
		"  - nested two",
		"",
		"1. ordered one",
		"2. ordered two",
		"",
		"| Col A | Col B |",
		"| --- | --- |",
		"| A1 | B1 |",
		"| A2 | B2 |",
		"",
		"[site](https://example.com)",
		"",
		"---",
		"",
		"```go",
		"fmt.Println(\"hello\")",
		"```",
	}, "\n")

	out := renderStream(t, []byte(src), 0)
	plain := stripANSI(out)
	wantPlain := strings.Join([]string{
		"# Title",
		"",
		"Paragraph with emphasis, strong, and strong+em plus code.",
		"",
		"> Quote line one Quote line two",
		"",
		"- item one",
		"- item two",
		"  - nested one",
		"  - nested two",
		"",
		"1. ordered one",
		"2. ordered two",
		"",
		"| Col A | Col B | | --- | --- | | A1 | B1 | | A2 | B2 |",
		"",
		"site (https://example.com)",
		"",
		"fmt.Println(\"hello\")",
	}, "\n") + "\n"

	if plain != wantPlain {
		t.Fatalf("plain output mismatch\n---want---\n%s\n---got---\n%s", wantPlain, plain)
	}

	if !strings.Contains(out, palette.PaletteDefault.H1) {
		t.Fatalf("missing H1 ANSI prefix")
	}
	if !strings.Contains(out, palette.PaletteDefault.Emphasis) {
		t.Fatalf("missing emphasis ANSI prefix")
	}
	if !strings.Contains(out, palette.PaletteDefault.Strong) {
		t.Fatalf("missing strong ANSI prefix")
	}
	if !strings.Contains(out, palette.PaletteDefault.CodeInline) {
		t.Fatalf("missing code inline ANSI prefix")
	}
	if !strings.Contains(out, palette.PaletteDefault.Quote) {
		t.Fatalf("missing quote ANSI prefix")
	}
	if !strings.Contains(out, palette.PaletteDefault.ListMarker) {
		t.Fatalf("missing list marker ANSI prefix")
	}
	if !strings.Contains(out, palette.PaletteDefault.LinkText) {
		t.Fatalf("missing link text ANSI prefix")
	}
}

func TestIntegrationRenderWrapped(t *testing.T) {
	src := strings.Join([]string{
		"Paragraph one with words that should wrap cleanly at a smaller width.",
		"",
		"- list item with a long line that wraps properly",
		"  - nested item with more words and wrapping",
	}, "\n")

	out := renderStream(t, []byte(src), 40)
	plain := stripANSI(out)
	if strings.Contains(plain, "  - -") {
		t.Fatalf("nested list marker collapsed: %q", plain)
	}
	if !strings.Contains(plain, "- list item") {
		t.Fatalf("missing list item text")
	}
}

func TestOSC8WrappedPreservesSpaces(t *testing.T) {
	src := []byte("A paragraph with a link to [site](https://example.com) and more text.")
	out := renderStreamWithOptions(t, src, 30, WithOSC8(true))
	plain := stripANSI(out)
	if strings.Contains(plain, "paragraphwith") {
		t.Fatalf("spaces collapsed in OSC8 wrapped output: %q", plain)
	}
	if !strings.Contains(out, "\x1b]8;;https://example.com\x1b\\") {
		t.Fatalf("missing OSC8 link start in wrapped output")
	}
}
