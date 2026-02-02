package mdf

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"unicode/utf8"
)

func BenchmarkRenderWrappedAgents(b *testing.B) {
	data, err := os.ReadFile("testdata/agents.md")
	if err != nil {
		b.Fatalf("read: %v", err)
	}
	b.ReportAllocs()
	reader := bytes.NewReader(data)
	var out bytes.Buffer
	out.Grow(len(data) * 2)
	for i := 0; i < b.N; i++ {
		reader.Reset(data)
		out.Reset()
		_ = Render(RenderRequest{
			Reader: reader,
			Writer: &out,
			Width:  80,
			Theme:  DefaultTheme(),
		})
	}
}

func BenchmarkStreamRendererAgents(b *testing.B) {
	data, err := os.ReadFile("testdata/agents.md")
	if err != nil {
		b.Fatalf("read: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	reader := bytes.NewReader(data)
	for i := 0; i < b.N; i++ {
		reader.Reset(data)
		_ = Render(RenderRequest{
			Reader: reader,
			Writer: io.Discard,
			Width:  80,
			Theme:  DefaultTheme(),
		})
	}
}

func BenchmarkRenderSampledata(b *testing.B) {
	samples := map[string][]byte{
		"agents":  mustReadSample(b, "testdata/agents.md"),
		"centaur": mustReadSample(b, "testdata/centaur.md"),
		"mdtest":  mustReadSample(b, "testdata/mdtest/TEST.md"),
	}
	widths := []int{50, 60, 80}
	for name, data := range samples {
		data := data
		b.Run(name, func(b *testing.B) {
			for _, width := range widths {
				width := width
				b.Run(intToWidthLabel(width), func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					reader := bytes.NewReader(data)
					for i := 0; i < b.N; i++ {
						reader.Reset(data)
						_ = Render(RenderRequest{
							Reader: reader,
							Writer: io.Discard,
							Width:  width,
							Theme:  DefaultTheme(),
						})
					}
				})
			}
		})
	}
}

func BenchmarkRenderReuse(b *testing.B) {
	samples := map[string][]byte{
		"agents":  mustReadSample(b, "testdata/agents.md"),
		"centaur": mustReadSample(b, "testdata/centaur.md"),
		"mdtest":  mustReadSample(b, "testdata/mdtest/TEST.md"),
	}
	widths := []int{50, 60, 80}
	for name, data := range samples {
		data := data
		b.Run(name, func(b *testing.B) {
			for _, width := range widths {
				width := width
				b.Run(intToWidthLabel(width), func(b *testing.B) {
					theme := DefaultTheme()
					parser := newLiveParser(theme, false)
					stream := NewStreamRenderer(io.Discard, width)
					var v validator
					maxLineBytes, maxLineRunes, maxWordBytes := maxInputMetrics(data)
					parser.reserveTextArena(len(data)*2 + 256)
					parser.reserveLineBuffers(maxLineBytes, maxLineRunes)
					parser.reserveInlineBuffers(maxLineBytes)
					if maxWordBytes > 0 && cap(stream.wordScratch) < maxWordBytes {
						stream.wordScratch = make([]byte, 0, maxWordBytes)
					}
					if maxLineBytes > 0 && cap(stream.prefixBuf) < maxLineBytes {
						stream.prefixBuf = make([]byte, 0, maxLineBytes)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						parser.Reset(theme, false)
						stream.Reset(io.Discard, width)
						if err := streamParseLiveReuse(data, parser, stream, &v); err != nil {
							b.Fatalf("stream parse reuse: %v", err)
						}
					}
				})
			}
		})
	}
}

func BenchmarkStreamSimulateReader(b *testing.B) {
	data := bytes.Repeat([]byte("alpha beta gamma delta epsilon\n"), 200)
	b.ReportAllocs()
	reader := bytes.NewReader(data)
	for i := 0; i < b.N; i++ {
		reader.Reset(data)
		if err := StreamSimulate(StreamSimulateRequest{
			Reader:    reader,
			Writer:    io.Discard,
			Width:     80,
			ChunkSize: 4,
		}); err != nil {
			b.Fatalf("stream simulate: %v", err)
		}
	}
}

func BenchmarkHTTPRender(b *testing.B) {
	data, err := os.ReadFile("testdata/agents.md")
	if err != nil {
		b.Fatalf("read: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer server.Close()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := HTTPRender(context.Background(), HTTPRenderRequest{
			URL:    server.URL,
			Writer: io.Discard,
			Width:  80,
			Theme:  DefaultTheme(),
		}); err != nil {
			b.Fatalf("stream http: %v", err)
		}
	}
}

func mustReadSample(b *testing.B, path string) []byte {
	b.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("read %s: %v", path, err)
	}
	return data
}

func intToWidthLabel(width int) string {
	return "w" + strconv.Itoa(width)
}

func maxInputMetrics(data []byte) (maxLineBytes int, maxLineRunes int, maxWordBytes int) {
	curLineBytes := 0
	curLineRunes := 0
	curWordBytes := 0
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			size = 1
		}
		if r == '\n' {
			if curLineBytes > maxLineBytes {
				maxLineBytes = curLineBytes
			}
			if curLineRunes > maxLineRunes {
				maxLineRunes = curLineRunes
			}
			if curWordBytes > maxWordBytes {
				maxWordBytes = curWordBytes
			}
			curLineBytes = 0
			curLineRunes = 0
			curWordBytes = 0
			i += size
			continue
		}
		curLineBytes += size
		curLineRunes++
		if r == ' ' || r == '\t' || r == '\r' {
			if curWordBytes > maxWordBytes {
				maxWordBytes = curWordBytes
			}
			curWordBytes = 0
		} else {
			curWordBytes += size
		}
		i += size
	}
	if curLineBytes > maxLineBytes {
		maxLineBytes = curLineBytes
	}
	if curLineRunes > maxLineRunes {
		maxLineRunes = curLineRunes
	}
	if curWordBytes > maxWordBytes {
		maxWordBytes = curWordBytes
	}
	return maxLineBytes, maxLineRunes, maxWordBytes
}

func streamParseLiveReuse(data []byte, parser *liveParser, stream *StreamRenderer, v *validator) error {
	v.reset()
	if _, err := v.addBytes(data); err != nil {
		return err
	}
	if err := parser.feedBytes(stream, data); err != nil {
		return err
	}
	parser.finalize(stream)
	return stream.Flush()
}
