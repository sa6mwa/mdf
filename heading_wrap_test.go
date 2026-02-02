package mdf

import (
	"strings"
	"testing"
)

func TestHeadingWrapIndentation(t *testing.T) {
	src := strings.Join([]string{
		"# This is a long header",
		"",
		"## This is an even longer header",
		"",
		"### This is a super-long header",
	}, "\n")

	out := stripANSI(renderStream(t, []byte(src), 12))
	lines := strings.Split(out, "\n")

	want := []string{
		"# This is a",
		"  long",
		"  header",
		"",
		"## This is",
		"   an even",
		"   longer",
		"   header",
		"",
		"### This is",
		"    a",
		"    super-long",
		"    header",
	}

	if len(lines) < len(want) {
		t.Fatalf("too few lines: got %d want %d", len(lines), len(want))
	}
	for i, line := range want {
		if lines[i] != line {
			t.Fatalf("line %d mismatch\nwant: %q\n got: %q", i+1, line, lines[i])
		}
	}
}
