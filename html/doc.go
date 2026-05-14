// Package html renders Markdown to self-contained themed HTML using the mdf
// streaming parser.
//
// The renderer consumes an io.Reader and writes a complete HTML document to an
// io.Writer. It preserves the PDF renderer's theme, font, margin, line-height,
// heading scale, background, and optional corner image settings while leaving
// wrapping to the browser so content adapts to the viewport.
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
