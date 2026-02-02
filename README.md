# mdf

Markdown *FAST!* is a high-performance Markdown → ANSI renderer optimized for streaming and terminal reflow, plus a PDF renderer for the same streaming pipeline.

## Design

- Stream parse from an `io.Reader`.
- Emit tokens as soon as decisions are made (no line buffering).
- Wrap only at the final step with ANSI-aware reflow.
- PDF renderer uses the same streaming pipeline: `io.Reader` → tokens → `io.Writer`.

## Demo (ANSI)

```bash
go run ./cmd/mdf-demo -file testdata/agents.md -theme synthwave-84
```

List themes:

```bash
go run ./cmd/mdf-demo -list-themes
```

## SDK: ANSI streaming

```go
f, _ := os.Open("testdata/agents.md")
defer f.Close()

_ = mdf.Render(mdf.RenderRequest{
	Reader:  f,
	Writer:  os.Stdout,
	Width:   80,
	Theme:   mdf.DefaultTheme(),
	Options: []mdf.RenderOption{mdf.WithOSC8(mdf.DetectOSC8Support())},
})
```

## SDK: PDF rendering

```go
f, _ := os.Open("testdata/agents.md")
defer f.Close()

out, _ := os.Create("out.pdf")
defer out.Close()

cfg := pdf.DefaultConfig()
cfg.PageSize = "A4"
cfg.Margin = 36
cfg.FontSize = 12
cfg.LineHeight = 1.4

_ = pdf.Render(pdf.RenderRequest{
	Reader: f,
	Writer: out,
	Theme:  mdf.DefaultTheme(),
	Config: cfg,
})
```

## Streaming pipeline pattern

The core idea is a zero-buffer streaming pipeline:

```
io.Reader  -->  mdf.Parse  -->  token stream  -->  io.Writer
```

You can use `mdf.Render` directly, or plug your own `mdf.Stream` implementation.

## Streaming from OpenAI Responses API (Go)

This example shows a full pipeline from OpenAI streaming → mdf → `io.Writer` (stdout or a scrollbuffer).
The Responses API streams semantic events; the primary text delta event is
`response.output_text.delta`. citeturn0search0turn0search3

```go
package main

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"pkt.systems/mdf"
)

func main() {
	ctx := context.Background()
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY not set")
	}

	// Reader from OpenAI streaming deltas.
	r, err := streamResponses(ctx, apiKey, "Explain streaming markdown in 3 bullets.")
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	// Sink: stdout (or your scrollbuffer writer).
	if err := mdf.Render(mdf.RenderRequest{
		Reader:  r,
		Writer:  os.Stdout,
		Width:   80,
		Theme:   mdf.DefaultTheme(),
		Options: []mdf.RenderOption{mdf.WithOSC8(mdf.DetectOSC8Support())},
	}); err != nil {
		log.Fatal(err)
	}
}

// streamResponses returns an io.Reader that emits response text deltas.
func streamResponses(ctx context.Context, apiKey, input string) (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		client := openai.NewClient(option.WithAPIKey(apiKey))
		params := responses.ResponseNewParams{
			Model: shared.ResponsesModel("gpt-5"),
			Input: responses.ResponseNewParamsInputUnion{
				OfString: openai.String(input),
			},
			Stream:     openai.Bool(true),
			Truncation: responses.ResponseNewParamsTruncationAuto,
		}

		stream := client.Responses.NewStreaming(ctx, params)
		for stream.Next() {
			ev := stream.Current()
			switch v := ev.AsAny().(type) {
			case responses.ResponseTextDeltaEvent:
				if v.Delta != "" {
					_, _ = pw.Write([]byte(v.Delta))
				}
			}
		}
		if err := stream.Err(); err != nil {
			_ = pw.CloseWithError(err)
		}
		_ = stream.Close()
	}()
	return pr, nil
}
```

Notes:
- Streaming events are typed; for text deltas, listen for `response.output_text.delta`. citeturn0search0turn0search1
- Set `stream=true` in the Responses request to enable streaming. citeturn0search1
