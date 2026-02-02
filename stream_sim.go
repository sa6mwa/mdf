package mdf

import (
	"bufio"
	"fmt"
	"io"
	"time"
	"unicode/utf8"
)

// StreamSimulateRequest configures StreamSimulate.
type StreamSimulateRequest struct {
	Reader    io.Reader
	Writer    io.Writer
	Width     int
	ChunkSize int
	Delay     time.Duration
	Options   []RenderOption
}

// StreamSimulate reads plain text from Reader and streams it through a StreamRenderer.
// This is intended for simulating inference token timing over unstyled text.
func StreamSimulate(req StreamSimulateRequest) error {
	if req.Reader == nil {
		return fmt.Errorf("stream simulate: Reader is nil")
	}
	if req.Writer == nil {
		return fmt.Errorf("stream simulate: Writer is nil")
	}
	if req.ChunkSize <= 0 {
		return fmt.Errorf("stream simulate: ChunkSize must be > 0")
	}
	cfg := configPool.Get().(*renderConfig)
	*cfg = renderConfig{}
	for _, opt := range req.Options {
		if opt != nil {
			opt(cfg)
		}
	}
	cfgVal := *cfg
	configPool.Put(cfg)
	stream := streamRendererPool.Get().(*StreamRenderer)
	stream.resetWithConfig(req.Writer, req.Width, cfgVal)
	reader := readerPool.Get().(*bufio.Reader)
	reader.Reset(req.Reader)
	var smallBuf [256]rune
	buf := smallBuf[:0]
	if req.ChunkSize > len(smallBuf) {
		buf = make([]rune, 0, req.ChunkSize)
	}
	var retErr error
	for {
		r, size, err := reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			retErr = fmt.Errorf("stream simulate: read: %w", err)
			goto done
		}
		if r == utf8.RuneError && size == 1 {
			continue
		}
		if isControlRune(r) {
			continue
		}
		buf = append(buf, r)
		if len(buf) >= req.ChunkSize {
			if err := streamSimulateFlush(stream, buf, req.Delay); err != nil {
				retErr = fmt.Errorf("stream simulate: write: %w", err)
				goto done
			}
			buf = buf[:0]
		}
	}
	if len(buf) > 0 {
		if err := streamSimulateFlush(stream, buf, req.Delay); err != nil {
			retErr = fmt.Errorf("stream simulate: write: %w", err)
			goto done
		}
	}
	if err := stream.Flush(); err != nil {
		retErr = err
	}
done:
	stream.Reset(io.Discard, 0)
	streamRendererPool.Put(stream)
	reader.Reset(nil)
	readerPool.Put(reader)
	return retErr
}

func streamSimulateFlush(stream *StreamRenderer, buf []rune, delay time.Duration) error {
	if len(buf) == 0 {
		return nil
	}
	per := time.Duration(0)
	rem := delay
	if delay > 0 {
		per = delay / time.Duration(len(buf))
		rem = delay - per*time.Duration(len(buf))
	}
	first := true
	for _, r := range buf {
		d := per
		if first {
			d += rem
			first = false
		}
		text := ""
		if r >= 0 && r < 128 {
			text = asciiRuneStrings[r]
		} else {
			text = string(r)
		}
		if err := stream.WriteToken(StreamToken{Token: Token{Text: text}, Delay: d}); err != nil {
			return err
		}
	}
	return nil
}
