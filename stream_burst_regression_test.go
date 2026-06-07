package mdf

import (
	"io"
	"strings"
	"testing"
)

func TestParseDoesNotBurstMultipleWordsFromSingleRead(t *testing.T) {
	cases := []string{
		"# The Outcome-Based Agile Framework\n",
		"# AGENTS.md\n",
		"hello world streams before newline in live parsing\n",
		"Name | Value\n",
		"--- | ---\n",
		"- item text\n",
		"> quote text\n",
		"Much of the current discourse on AI productivity is framed around tools.\n",
	}
	for _, src := range cases {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			reader := &oneByteStepReader{src: []byte(src)}
			stream := &readStepBurstStream{reader: reader}
			if err := Parse(ParseRequest{
				Reader: reader,
				Stream: stream,
				Theme:  DefaultTheme(),
			}); err != nil {
				t.Fatalf("parse: %v", err)
			}
			for step, text := range stream.textByStep {
				if countWords(text) >= 2 {
					t.Fatalf("read step %d emitted multiple words %q for input %q", step, text, src)
				}
			}
		})
	}
}

type oneByteStepReader struct {
	src  []byte
	pos  int
	step int
}

func (r *oneByteStepReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.src) {
		return 0, io.EOF
	}
	r.step++
	p[0] = r.src[r.pos]
	r.pos++
	return 1, nil
}

type readStepBurstStream struct {
	reader     *oneByteStepReader
	textByStep map[int]string
}

func (s *readStepBurstStream) WriteToken(tok StreamToken) error {
	if tok.Text == "" || tok.Kind == tokenThematicBreak {
		return nil
	}
	if s.textByStep == nil {
		s.textByStep = make(map[int]string)
	}
	s.textByStep[s.reader.step] += tok.Text
	return nil
}

func (s *readStepBurstStream) Flush() error         { return nil }
func (s *readStepBurstStream) Width() int           { return 80 }
func (s *readStepBurstStream) SetWidth(int)         {}
func (s *readStepBurstStream) SetWrapIndent(string) {}

func countWords(text string) int {
	count := 0
	for _, field := range strings.Fields(text) {
		field = strings.Trim(field, "#>|*-_`[]()")
		if field != "" {
			count++
		}
	}
	return count
}
