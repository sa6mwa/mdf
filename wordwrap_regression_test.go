package mdf

import (
	"strings"
	"testing"
)

func TestWrappedBulletIndentation(t *testing.T) {
	src := strings.Join([]string{
		"- Inputs:",
		"",
		"  - If a user-facing function or interface method takes more than 4",
		"    parameters total (including context.Context), move non-ctx inputs into",
		"    a request struct (e.g. FooRequest).",
	}, "\n")

	out := stripANSI(renderStream(t, []byte(src), 60))
	lines := strings.Split(out, "\n")

	var got []string
	for _, line := range lines {
		line = strings.TrimRight(line, " ")
		if line == "" {
			continue
		}
		got = append(got, line)
	}

	want := []string{
		"- Inputs:",
		"  - If a user-facing function or interface method takes more",
		"    than 4 parameters total (including context.Context),",
		"    move non-ctx inputs into a request struct (e.g.",
		"    FooRequest).",
	}

	if len(got) < len(want) {
		t.Fatalf("too few lines: got %d want %d\n%q", len(got), len(want), got)
	}
	for i, line := range want {
		if got[i] != line {
			t.Fatalf("line %d mismatch\nwant: %q\n got: %q", i+1, line, got[i])
		}
	}
}
