package mdf

import (
	"strings"
	"testing"
)

func TestRenderOmitsFrontMatterAtStreamStart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		src      string
		contains []string
		omits    []string
	}{
		{
			name: "yaml",
			src:  "---\ntitle: Post\ndate: 2026-02-09\n---\n\n# Hello\n\nBody.\n",
			contains: []string{
				"# Hello",
				"Body.",
			},
			omits: []string{
				"title: Post",
				"date: 2026-02-09",
			},
		},
		{
			name: "toml",
			src:  "+++\ntitle = \"Post\"\n+++\n\n# Hello\n",
			contains: []string{
				"# Hello",
			},
			omits: []string{
				"title = \"Post\"",
			},
		},
		{
			name: "json",
			src:  ";;;\n{\"title\": \"Post\"}\n;;;\n\n# Hello\n",
			contains: []string{
				"# Hello",
			},
			omits: []string{
				"\"title\": \"Post\"",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := stripANSI(renderStream(t, []byte(tc.src), 0))
			for _, want := range tc.contains {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q in output: %q", want, out)
				}
			}
			for _, bad := range tc.omits {
				if strings.Contains(out, bad) {
					t.Fatalf("unexpected %q in output: %q", bad, out)
				}
			}
		})
	}
}

func TestRenderFrontMatterIsOnlyCheckedAtStart(t *testing.T) {
	t.Parallel()
	src := "# Intro\n\n+++\ntitle = \"Keep me\"\n+++\n\nTail\n"
	out := stripANSI(renderStream(t, []byte(src), 0))
	for _, want := range []string{"# Intro", "title = \"Keep me\"", "Tail"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
	}
}

func TestRenderUnclosedFrontMatterIsNotStripped(t *testing.T) {
	t.Parallel()
	src := "---\ntitle: Post\n\n# Hello\n"
	out := stripANSI(renderStream(t, []byte(src), 0))
	for _, want := range []string{"title: Post", "# Hello"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
	}
}

func TestRenderStartDelimiterWithoutMetadataIsNotStripped(t *testing.T) {
	t.Parallel()
	src := "---\n# Keep\n---\n\nTail\n"
	out := stripANSI(renderStream(t, []byte(src), 0))
	for _, want := range []string{"# Keep", "Tail"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
	}
}

func TestRenderAfterInitialFrontMatterStopsCheckingForMore(t *testing.T) {
	t.Parallel()
	src := "---\ntitle: Skip\n---\n\nBody\n\n---\nkeep: yes\n---\n"
	out := stripANSI(renderStream(t, []byte(src), 0))
	if strings.Contains(out, "title: Skip") {
		t.Fatalf("unexpected front-matter content in output: %q", out)
	}
	for _, want := range []string{"Body", "keep: yes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
	}
}

func TestFrontMatterFilterPassesNonDelimiterStartImmediately(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"# A Header\n",
		"hello world\n",
		"Name | Value\n",
		"> quote\n",
	} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			var filter frontMatterFilter
			filter.reset()
			out := filter.process([]byte(src[:1]))
			if string(out) != src[:1] {
				t.Fatalf("first byte output = %q, want %q", string(out), src[:1])
			}
			if !filter.passthrough {
				t.Fatalf("filter did not enter passthrough after non-delimiter start")
			}
		})
	}
}

func TestFrontMatterFilterKeepsDelimiterCandidatesBuffered(t *testing.T) {
	t.Parallel()
	for _, src := range []string{"-", "--", "---", "--- ", "+", "+++", ";;"} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			var filter frontMatterFilter
			filter.reset()
			if out := filter.process([]byte(src)); len(out) != 0 {
				t.Fatalf("candidate %q output too early: %q", src, string(out))
			}
			if filter.passthrough {
				t.Fatalf("candidate %q entered passthrough too early", src)
			}
		})
	}
}

func TestFrontMatterFilterPassesInvalidDelimiterPrefixImmediately(t *testing.T) {
	t.Parallel()
	for _, src := range []string{"- ", "----", "---x", "++++", ";;;;"} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			var filter frontMatterFilter
			filter.reset()
			if out := filter.process([]byte(src)); string(out) != src {
				t.Fatalf("invalid delimiter output = %q, want %q", string(out), src)
			}
			if !filter.passthrough {
				t.Fatalf("invalid delimiter %q did not enter passthrough", src)
			}
		})
	}
}
