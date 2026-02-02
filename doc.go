// Package mdf renders Markdown to ANSI for terminal display.
//
// This package is built for streaming: it parses incrementally from an io.Reader
// and emits a style-aware ANSI token stream that is wrapped only at the final
// output step. The renderer avoids buffering full documents and can handle
// unbounded streams as they arrive.
//
// Core properties:
//   - Streaming-first parsing from io.Reader
//   - Width-independent render tokens; wrap/reflow is last
//   - Low allocations in hot paths
//   - Theme-driven styling via ANSI prefixes
//
// Example:
//
//	reader := strings.NewReader("# Hello\n\nMarkdown in, ANSI out.\n")
//	err := mdf.Render(mdf.RenderRequest{
//		Reader: reader,
//		Writer: os.Stdout,
//		Width:  80,
//		Theme:  mdf.DefaultTheme(),
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//
// The renderer can be customized using RenderOptions such as OSC 8 hyperlink
// support.
package mdf
