// Package html renders Markdown to self-contained themed HTML using the mdf
// streaming parser.
//
// The renderer consumes an io.Reader and writes a complete HTML document to an
// io.Writer. It preserves the PDF renderer's theme, font, margin, line-height,
// heading scale, background, optional corner image settings, configurable
// content width, and Markdown table rendering while leaving wrapping to the
// browser so content adapts to the viewport.
//
// Markdown thematic breaks are intentionally consumed as structural separators
// and are not emitted as visible HTML rules.
//
// Example:
//
//	src := strings.NewReader("# Report\n\nHello HTML.\n")
//	cfg := html.DefaultConfig()
//	err := html.Render(html.RenderRequest{
//		Reader: src,
//		Writer: os.Stdout,
//		Config: cfg,
//	})
package html
