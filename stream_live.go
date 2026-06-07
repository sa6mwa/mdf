package mdf

import (
	"bufio"
	"fmt"
	"io"
	"sync"
	"unicode/utf8"
)

var streamRendererPool = sync.Pool{
	New: func() any {
		return &StreamRenderer{}
	},
}

var parserPool = sync.Pool{
	New: func() any {
		return &liveParser{}
	},
}

var readerPool = sync.Pool{
	New: func() any {
		return bufio.NewReaderSize(nil, 4096)
	},
}

var configPool = sync.Pool{
	New: func() any {
		return &renderConfig{}
	},
}

// RenderRequest configures streaming Markdown rendering to ANSI.
type RenderRequest struct {
	// Reader supplies Markdown input. Render reads it incrementally.
	Reader io.Reader
	// Writer receives ANSI-rendered Markdown.
	Writer io.Writer
	// Width is the target terminal width in cells. Values <= 0 disable wrapping.
	Width int
	// Theme controls semantic ANSI styles. If nil, DefaultTheme is used.
	Theme Theme
	// Options configures optional renderer behavior such as OSC 8 links and tables.
	Options []RenderOption
}

// ParseRequest configures streaming Markdown parsing into a token sink.
type ParseRequest struct {
	// Reader supplies Markdown input. Parse reads it incrementally.
	Reader io.Reader
	// Stream receives parsed tokens. If it also implements TableStream, tables
	// are emitted as structured table events.
	Stream Stream
	// Theme controls semantic styles attached to emitted tokens. If nil,
	// DefaultTheme is used.
	Theme Theme
	// Options configures parser/render metadata such as OSC 8 link tokens.
	Options []RenderOption
}

// Render streams Markdown from Reader to Writer as ANSI terminal output.
func Render(req RenderRequest) error {
	if req.Reader == nil {
		return fmt.Errorf("render: reader is nil")
	}
	if req.Writer == nil {
		return fmt.Errorf("render: writer is nil")
	}
	cfg := configPool.Get().(*renderConfig)
	*cfg = renderConfig{}
	for _, opt := range req.Options {
		if opt != nil {
			opt(cfg)
		}
	}
	normalizeRenderConfig(cfg)
	cfgVal := *cfg
	configPool.Put(cfg)
	stream := streamRendererPool.Get().(*StreamRenderer)
	stream.resetWithConfig(req.Writer, req.Width, cfgVal)
	err := Parse(ParseRequest{
		Reader:  req.Reader,
		Stream:  stream,
		Theme:   req.Theme,
		Options: req.Options,
	})
	stream.Reset(io.Discard, 0)
	streamRendererPool.Put(stream)
	return err
}

// Parse streams Markdown from Reader and writes parsed tokens to Stream.
func Parse(req ParseRequest) error {
	if req.Reader == nil {
		return fmt.Errorf("parse: reader is nil")
	}
	if req.Stream == nil {
		return fmt.Errorf("parse: stream is nil")
	}
	cfg := configPool.Get().(*renderConfig)
	*cfg = renderConfig{}
	for _, opt := range req.Options {
		if opt != nil {
			opt(cfg)
		}
	}
	normalizeRenderConfig(cfg)
	cfgVal := *cfg
	configPool.Put(cfg)
	theme := req.Theme
	if theme == nil {
		theme = DefaultTheme()
	}
	parser := parserPool.Get().(*liveParser)
	reader := readerPool.Get().(*bufio.Reader)
	parser.Reset(theme, cfgVal.osc8)
	reader.Reset(req.Reader)
	buf := parser.readBufArr[:]
	var tailBuf [utf8.UTFMax]byte
	tailLen := 0
	var cleanBuf [4096]byte
	var retErr error
	for {
		n, err := reader.Read(buf[:])
		if n > 0 {
			chunk := buf[:n]
			if tailLen > 0 {
				need := utf8.UTFMax - tailLen
				if need > len(chunk) {
					need = len(chunk)
				}
				var smallBuf [utf8.UTFMax * 2]byte
				combined := smallBuf[:tailLen+need]
				copy(combined, tailBuf[:tailLen])
				copy(combined[tailLen:], chunk[:need])
				var smallOut [utf8.UTFMax * 2]byte
				clean, rest := sanitizeBytes(smallOut[:], combined)
				if len(clean) > 0 {
					filtered := parser.frontMatter.process(clean)
					if len(filtered) > 0 {
						err := parser.feedBytes(req.Stream, filtered)
						if err != nil {
							retErr = fmt.Errorf("parse: %w", err)
							goto done
						}
					}
				}
				tailLen = copy(tailBuf[:], rest)
				chunk = chunk[need:]
			}
			if len(chunk) > 0 {
				clean, rest := sanitizeBytes(cleanBuf[:len(chunk)], chunk)
				if len(clean) > 0 {
					filtered := parser.frontMatter.process(clean)
					if len(filtered) > 0 {
						err := parser.feedBytes(req.Stream, filtered)
						if err != nil {
							retErr = fmt.Errorf("parse: %w", err)
							goto done
						}
					}
				}
				tailLen = copy(tailBuf[:], rest)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			retErr = fmt.Errorf("parse: read: %w", err)
			goto done
		}
	}
	if trailing := parser.frontMatter.finish(); len(trailing) > 0 {
		if err := parser.feedBytes(req.Stream, trailing); err != nil {
			retErr = fmt.Errorf("parse: %w", err)
			goto done
		}
	}
	if err := parser.finalize(req.Stream); err != nil {
		retErr = fmt.Errorf("parse: %w", err)
		goto done
	}
	if err := req.Stream.Flush(); err != nil {
		retErr = err
	}
done:
	parserPool.Put(parser)
	readerPool.Put(reader)
	return retErr
}

func (p *liveParser) feedBytes(stream Stream, data []byte) error {
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			data = data[1:]
			continue
		}
		if isControlRune(r) {
			data = data[size:]
			continue
		}
		if err := p.feedRune(stream, r); err != nil {
			return err
		}
		data = data[size:]
	}
	return nil
}
