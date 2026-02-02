package mdf

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestHeadingsIncludeMarkers(t *testing.T) {
	src := []byte("# One\n## Two\n### Three\n")
	out := stripANSI(renderStream(t, src, 0))
	if !strings.Contains(out, "# One") {
		t.Fatalf("missing H1 marker: %q", out)
	}
	if !strings.Contains(out, "## Two") {
		t.Fatalf("missing H2 marker: %q", out)
	}
	if !strings.Contains(out, "### Three") {
		t.Fatalf("missing H3 marker: %q", out)
	}
}

func TestListItemsRenderText(t *testing.T) {
	src := []byte("- one\n- two\n\n- three\n  - nested\n")
	out := stripANSI(renderStream(t, src, 0))
	for _, item := range []string{"one", "two", "three", "nested"} {
		if !strings.Contains(out, item) {
			t.Fatalf("missing list item %q in %q", item, out)
		}
	}
	if strings.Contains(out, "- -") {
		t.Fatalf("nested list marker rendered inline: %q", out)
	}
}

func TestAllAgentsTextPresent(t *testing.T) {
	src := readAgents(t)
	out := normalizeWhitespace(stripANSI(renderStream(t, src, 0)))
	lines := strings.Split(string(src), "\n")
	for _, line := range lines {
		line = strings.TrimLeft(line, " \t")
		want := normalizeMarkdownLine(line)
		if want == "" {
			continue
		}
		if !strings.Contains(out, normalizeWhitespace(want)) {
			t.Fatalf("missing text %q in rendered output", want)
		}
	}
}

func TestOSC8Links(t *testing.T) {
	src := []byte("See [website](https://example.com) now.\n")
	no := stripANSI(renderStream(t, src, 0))
	if !strings.Contains(no, "website (https://example.com)") {
		t.Fatalf("expected fallback link rendering, got %q", no)
	}
	osc := renderStreamWithOptions(t, src, 0, WithOSC8(true))
	if !strings.Contains(osc, "\x1b]8;;https://example.com\x1b\\") {
		t.Fatalf("missing OSC 8 start sequence")
	}
	if !strings.Contains(osc, "\x1b]8;;\x1b\\") {
		t.Fatalf("missing OSC 8 end sequence")
	}
}

func TestAllMdtestTextPresent(t *testing.T) {
	src := readMdtest(t)
	out := normalizeWhitespace(stripANSI(renderStream(t, src, 0)))
	lines := strings.Split(string(src), "\n")
	for _, line := range lines {
		if strings.Contains(line, "<") {
			continue
		}
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if strings.Contains(line, "`") {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, ">    ") || strings.HasPrefix(trimmed, ">\t") {
			continue
		}
		want := normalizeMarkdownLine(line)
		if want == "" {
			continue
		}
		if !strings.Contains(out, normalizeWhitespace(want)) {
			t.Fatalf("missing mdtest text %q in rendered output", want)
		}
	}
}

func TestBlankLineBetweenListAndHeading(t *testing.T) {
	src := []byte("1. First\n2. Second\n\n## Header\n")
	out := stripANSI(renderStream(t, src, 0))
	if !strings.Contains(out, "Second\n\n## Header") {
		t.Fatalf("expected blank line before header, got %q", out)
	}
}

func TestAgentsNestedListIndentation(t *testing.T) {
	src := readAgents(t)
	out := stripANSI(renderStream(t, src, 60))
	if !strings.Contains(out, "\n    package or subpackage so both main/module code and") {
		t.Fatalf("missing nested list indentation for wrapped cycles line")
	}
	if !strings.Contains(out, "\n    than 4 parameters total (including context.Context),") {
		t.Fatalf("missing nested list indentation for wrapped context line")
	}
}

func TestAgentsNoBracketBoundaryWrap(t *testing.T) {
	src := readAgents(t)
	out := stripANSI(renderStream(t, src, 80))
	if strings.Contains(out, "Workflow (\n") {
		t.Fatalf("unexpected wrap after '(': %q", out)
	}
	if !strings.Contains(out, "Workflow (mandatory)") {
		t.Fatalf("missing workflow line: %q", out)
	}
	if strings.Contains(out, "(DX)\n.") {
		t.Fatalf("unexpected line break before '.': %q", out)
	}
	if !strings.Contains(out, "experience (DX). Speed") {
		t.Fatalf("missing DX sentence: %q", out)
	}
}

var ansiRegexp = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
var osc8Regexp = regexp.MustCompile("\x1b\\]8;;.*?\x1b\\\\")

func stripANSI(s string) string {
	s = ansiRegexp.ReplaceAllString(s, "")
	s = osc8Regexp.ReplaceAllString(s, "")
	return s
}

func renderStreamWithOptions(t *testing.T, src []byte, width int, opts ...RenderOption) string {
	t.Helper()
	var out bytes.Buffer
	err := Render(RenderRequest{
		Reader:  bytes.NewReader(src),
		Writer:  &out,
		Width:   width,
		Theme:   DefaultTheme(),
		Options: opts,
	})
	if err != nil {
		t.Fatalf("stream live: %v", err)
	}
	return out.String()
}

func normalizeMarkdownLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, "```") {
		return ""
	}
	if isRuleLine(line) {
		return ""
	}
	if strings.HasPrefix(line, "#") {
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
	}
	for strings.HasPrefix(line, ">") {
		line = strings.TrimPrefix(line, ">")
		line = strings.TrimSpace(line)
	}
	if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "+") {
		line = strings.TrimSpace(line[1:])
	}
	if len(line) > 2 {
		if line[1] == '.' && line[0] >= '0' && line[0] <= '9' {
			line = strings.TrimSpace(line[2:])
		}
	}
	line = replaceMarkdownLinks(line)
	line = strings.ReplaceAll(line, "`", "")
	line = strings.ReplaceAll(line, "*", "")
	line = strings.ReplaceAll(line, "_", "")
	line = strings.TrimSpace(line)
	return line
}

func isRuleLine(line string) bool {
	line = strings.ReplaceAll(line, " ", "")
	if len(line) < 3 {
		return false
	}
	ch := line[0]
	if ch != '-' && ch != '*' && ch != '_' {
		return false
	}
	for i := 1; i < len(line); i++ {
		if line[i] != ch {
			return false
		}
	}
	return true
}

func replaceMarkdownLinks(line string) string {
	for {
		start := strings.Index(line, "[")
		if start == -1 {
			return line
		}
		mid := strings.Index(line[start:], "](")
		if mid == -1 {
			return line
		}
		mid += start
		end := strings.Index(line[mid+2:], ")")
		if end == -1 {
			return line
		}
		end += mid + 2
		text := line[start+1 : mid]
		url := line[mid+2 : end]
		replacement := text + " (" + url + ")"
		line = line[:start] + replacement + line[end+1:]
	}
}

func normalizeWhitespace(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func TestMain(m *testing.M) {
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		os.Exit(m.Run())
	}
	os.Exit(m.Run())
}
