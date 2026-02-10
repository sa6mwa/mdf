package mdf

import "bytes"

const maxFrontMatterProbeBytes = 64 * 1024

type frontMatterFilter struct {
	passthrough bool
	probe       []byte
	probeArr    [4096]byte
}

func (f *frontMatterFilter) reset() {
	f.passthrough = false
	f.probe = f.probeArr[:0]
}

func (f *frontMatterFilter) process(chunk []byte) []byte {
	if f.passthrough || len(chunk) == 0 {
		return chunk
	}
	f.probe = append(f.probe, chunk...)
	out, decided := f.decide(false)
	if !decided && len(f.probe) > maxFrontMatterProbeBytes {
		out = f.probe
		f.passthrough = true
		f.probe = f.probe[:0]
		decided = true
	}
	if decided {
		return out
	}
	return nil
}

func (f *frontMatterFilter) finish() []byte {
	if f.passthrough || len(f.probe) == 0 {
		return nil
	}
	out, _ := f.decide(true)
	return out
}

func (f *frontMatterFilter) decide(eof bool) ([]byte, bool) {
	openLine, openNext, ok := nextLine(f.probe, 0, eof)
	if !ok {
		return nil, false
	}
	delim, isFrontMatter := parseOpeningFrontMatterDelimiter(openLine)
	if !isFrontMatter {
		out := f.probe
		f.passthrough = true
		f.probe = f.probe[:0]
		return out, true
	}

	secondLine, secondNext, ok := nextLine(f.probe, openNext, eof)
	if !ok {
		return nil, false
	}
	if !frontMatterMetadataLikely(secondLine) {
		out := f.probe
		f.passthrough = true
		f.probe = f.probe[:0]
		return out, true
	}

	closeNext, found := findClosingFrontMatterDelimiter(f.probe, secondNext, delim, eof)
	if !found {
		if eof {
			out := f.probe
			f.passthrough = true
			f.probe = f.probe[:0]
			return out, true
		}
		return nil, false
	}
	out := f.probe[closeNext:]
	f.passthrough = true
	f.probe = f.probe[:0]
	return out, true
}

func nextLine(src []byte, start int, eof bool) ([]byte, int, bool) {
	if start > len(src) {
		return nil, 0, false
	}
	if start == len(src) {
		if eof {
			return src[start:], start, true
		}
		return nil, 0, false
	}
	i := bytes.IndexByte(src[start:], '\n')
	if i < 0 {
		if !eof {
			return nil, 0, false
		}
		return trimCR(src[start:]), len(src), true
	}
	lineEnd := start + i
	return trimCR(src[start:lineEnd]), lineEnd + 1, true
}

func parseOpeningFrontMatterDelimiter(line []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(trimBOM(line))
	switch {
	case bytes.Equal(trimmed, []byte("---")):
		return []byte("---"), true
	case bytes.Equal(trimmed, []byte("+++")):
		return []byte("+++"), true
	case bytes.Equal(trimmed, []byte(";;;")):
		return []byte(";;;"), true
	default:
		return nil, false
	}
}

func frontMatterMetadataLikely(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return false
	}
	if bytes.HasPrefix(trimmed, []byte("{")) || bytes.HasPrefix(trimmed, []byte("[")) {
		return true
	}
	if bytes.Contains(trimmed, []byte(":")) || bytes.Contains(trimmed, []byte("=")) {
		return true
	}
	return false
}

func findClosingFrontMatterDelimiter(src []byte, start int, delim []byte, eof bool) (int, bool) {
	for idx := start; idx <= len(src); {
		line, next, ok := nextLine(src, idx, eof)
		if !ok {
			return 0, false
		}
		if bytes.Equal(bytes.TrimSpace(line), delim) {
			return next, true
		}
		if next == idx {
			return 0, false
		}
		idx = next
		if idx == len(src) && !eof {
			return 0, false
		}
	}
	return 0, false
}

func trimCR(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\r' {
		return b[:len(b)-1]
	}
	return b
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
