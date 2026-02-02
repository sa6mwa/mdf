package mdf

import (
	"os"
	"testing"
)

func renderStream(t *testing.T, src []byte, width int) string {
	t.Helper()
	return renderStreamWithOptions(t, src, width, WithOSC8(false))
}

func readAgents(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/agents.md")
	if err != nil {
		t.Fatalf("read agents.md: %v", err)
	}
	return data
}

func readMdtest(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/mdtest/TEST.md")
	if err != nil {
		t.Fatalf("read mdtest/TEST.md: %v", err)
	}
	return data
}
