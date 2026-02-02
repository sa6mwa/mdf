package mdf

import (
	"bytes"
	"os"
	"testing"
)

func TestRenderWrappedAllocations(t *testing.T) {
	src, err := os.ReadFile("testdata/agents.md")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	allocs := testing.AllocsPerRun(100, func() {
		var out bytes.Buffer
		_ = Render(RenderRequest{
			Reader: bytes.NewReader(src),
			Writer: &out,
			Width:  80,
			Theme:  DefaultTheme(),
		})
	})
	if allocs > 6000 {
		t.Fatalf("too many allocations per Render: got %.2f", allocs)
	}
}
