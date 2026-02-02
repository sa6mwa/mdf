// Package pdf renders Markdown to PDF using the mdf streaming parser.
//
// The renderer consumes an io.Reader and writes a PDF to an io.Writer. It
// supports theme-driven colors, configurable page layout, optional corner
// images, and embedded fonts.
//
// Example:
//
//	src := strings.NewReader("# Report\n\nHello PDF.\n")
//	cfg := pdf.DefaultConfig()
//	cfg.PageSize = "A4"
//	cfg.FontSize = 12
//
//	err := pdf.Render(pdf.RenderRequest{
//		Reader: src,
//		Writer: outFile,
//		Theme:  mdf.DefaultTheme(),
//		Config: cfg,
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//
// For custom fonts, set RegularFont/BoldFont/ItalicFont in Config or provide
// embedded font bytes via RegularFontBytes/BoldFontBytes/ItalicFontBytes.
package pdf
