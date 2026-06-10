package html

import (
	"encoding/base64"
	"fmt"
	stdhtml "html"
	"io"
	"strings"

	"pkt.systems/mdf"
)

const headingFontFamily = "Heading"

type stream struct {
	w             io.Writer
	trace         *mdf.WriteTraceEmitter
	traceText     strings.Builder
	cfg           Config
	styles        mdf.Styles
	cornerImage   *embeddedImage
	styleCache    map[string]string
	spanOpen      bool
	currentStyle  string
	linkOpen      bool
	documentEnded bool
	atLineStart   bool
	headingProbe  strings.Builder
	headingStyle  mdf.Style
	headingActive bool
	headingOpen   bool
	lineProbe     []styledRune
	lineOpen      bool
	linePrefixLen int
	tableStart    mdf.TableStart
	tableRows     []mdf.TableRow
	tableColumns  int
	tableActive   bool
	tableOpen     bool
	tableWrapped  bool
	tableBodyOpen bool
}

type styledRune struct {
	r     rune
	style mdf.Style
}

func newStream(w io.Writer, cfg Config, styles mdf.Styles, cornerImage *embeddedImage, trace mdf.WriteTraceEncoder) *stream {
	return &stream{
		w:           w,
		trace:       mdf.NewWriteTraceEmitter(mdf.WriteTraceFormatHTML, trace),
		cfg:         cfg,
		styles:      styles,
		cornerImage: cornerImage,
		styleCache:  make(map[string]string),
		atLineStart: true,
	}
}

func (s *stream) Width() int {
	return 0
}

func (s *stream) SetWidth(int) {}

func (s *stream) SetWrapIndent(string) {}

func (s *stream) WriteToken(tok mdf.StreamToken) error {
	switch tok.Kind {
	case mdf.TokenLinkStart:
		return s.openLink(tok.LinkURL)
	case mdf.TokenLinkEnd:
		return s.closeLink()
	case mdf.TokenThematicBreak:
		// HTML intentionally consumes thematic breaks as structural separators
		// instead of emitting <hr>, matching the renderer policy tested in
		// TestRenderIntentionallySuppressesThematicBreaks.
		if s.headingProbe.Len() > 0 {
			if err := s.flushHeadingProbe(s.headingStyle); err != nil {
				return err
			}
		}
		if err := s.closeHeading(); err != nil {
			return err
		}
		if err := s.closeSpan(); err != nil {
			return err
		}
		return nil
	}
	if tok.Text == "" {
		return nil
	}
	return s.writeText(tok.Text, tok.Style)
}

func (s *stream) StartTable(table mdf.TableStart) error {
	if err := s.closeInlineForBlock(); err != nil {
		return err
	}
	s.tableStart = table
	s.tableRows = s.tableRows[:0]
	s.tableColumns = 0
	s.tableActive = true
	s.tableOpen = false
	s.tableWrapped = false
	s.tableBodyOpen = false
	return nil
}

func (s *stream) WriteTableRow(row mdf.TableRow) error {
	if !s.tableActive {
		if err := s.StartTable(mdf.TableStart{}); err != nil {
			return err
		}
	}
	if s.cfg.TableBufferMode == mdf.TableBufferRow {
		return s.writeRowBufferedTable(row)
	}
	s.tableRows = append(s.tableRows, row)
	return nil
}

func (s *stream) EndTable() error {
	if !s.tableActive {
		return nil
	}
	if s.cfg.TableBufferMode == mdf.TableBufferFull || !s.tableOpen {
		if err := s.writeFullTable(s.tableRows); err != nil {
			return err
		}
	} else if s.tableBodyOpen {
		if err := s.writeEmissionString("</tbody>"); err != nil {
			return err
		}
	}
	if s.tableOpen {
		if err := s.writeEmissionString("</table>\n"); err != nil {
			return err
		}
	}
	if s.tableWrapped {
		if err := s.writeEmissionString("</div>"); err != nil {
			return err
		}
	}
	s.tableRows = s.tableRows[:0]
	s.tableColumns = 0
	s.tableActive = false
	s.tableOpen = false
	s.tableWrapped = false
	s.tableBodyOpen = false
	s.atLineStart = true
	return nil
}

func (s *stream) closeInlineForBlock() error {
	if s.headingProbe.Len() > 0 {
		if err := s.flushHeadingProbe(s.headingStyle); err != nil {
			return err
		}
	}
	if err := s.closeHeading(); err != nil {
		return err
	}
	if err := s.closeSpan(); err != nil {
		return err
	}
	if err := s.closeHangingLine(); err != nil {
		return err
	}
	if err := s.closeLink(); err != nil {
		return err
	}
	if !s.atLineStart {
		if err := s.writeEmissionString("\n"); err != nil {
			return err
		}
		s.atLineStart = true
	}
	return nil
}

func (s *stream) Flush() error {
	if s.documentEnded {
		return nil
	}
	if s.tableActive {
		if err := s.EndTable(); err != nil {
			return err
		}
	}
	if s.headingProbe.Len() > 0 {
		if err := s.flushHeadingProbe(s.headingStyle); err != nil {
			return err
		}
	}
	if err := s.closeHeading(); err != nil {
		return err
	}
	if err := s.closeSpan(); err != nil {
		return err
	}
	if err := s.closeLink(); err != nil {
		return err
	}
	if err := s.flushTraceText(); err != nil {
		return err
	}
	err := s.writeEmissionString("\n</main>\n</body>\n</html>\n")
	s.documentEnded = true
	return err
}

func (s *stream) writeDocumentStart() error {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>mdf</title>\n<style>\n")
	s.writeCSS(&b)
	b.WriteString("</style>\n</head>\n<body>\n<main class=\"mdf-document\">")
	if s.cornerImage != nil {
		b.WriteString(`<img class="mdf-corner-image" alt="" src="data:`)
		b.WriteString(stdhtml.EscapeString(s.cornerImage.mime))
		b.WriteString(";base64,")
		b.WriteString(s.cornerImage.data)
		b.WriteString("\">\n")
	}
	return s.writeEmissionString(b.String())
}

func (s *stream) writeCSS(b *strings.Builder) {
	writeFontFace(b, s.cfg.FontFamily, "400", "normal", s.cfg.RegularFontBytes, s.cfg.RegularFontFormat)
	writeFontFace(b, s.cfg.FontFamily, "700", "normal", s.cfg.BoldFontBytes, s.cfg.BoldFontFormat)
	writeFontFace(b, s.cfg.FontFamily, "400", "italic", s.cfg.ItalicFontBytes, s.cfg.ItalicFontFormat)
	if len(s.cfg.BoldItalicFontBytes) > 0 {
		writeFontFace(b, s.cfg.FontFamily, "700", "italic", s.cfg.BoldItalicFontBytes, s.cfg.BoldItalicFontFormat)
	}
	if len(s.cfg.HeadingFontBytes) > 0 {
		writeFontFace(b, headingFontFamily, "400", "normal", s.cfg.HeadingFontBytes, "truetype")
		writeFontFace(b, headingFontFamily, "700", "normal", s.cfg.HeadingFontBytes, "truetype")
	}
	bg := "transparent"
	if s.cfg.BackgroundEnabled {
		bg = rgbCSS(s.cfg.BackgroundRGB)
	}
	b.WriteString(":root{color-scheme:dark light;}\n")
	b.WriteString("html,body{margin:0;min-height:100%;background:")
	b.WriteString(bg)
	b.WriteString(";}\n")
	b.WriteString("body{color:")
	b.WriteString(rgbCSS(s.cfg.TextRGB))
	b.WriteString(";font-family:")
	b.WriteString(cssString(s.cfg.FontFamily))
	b.WriteString(",monospace;font-size:")
	b.WriteString(formatFloat(s.cfg.FontSize))
	b.WriteString("pt;line-height:")
	b.WriteString(formatFloat(s.cfg.LineHeight))
	b.WriteString(";}\n")
	b.WriteString(".mdf-document{--mdf-content-max-width:")
	b.WriteString(formatFloat(s.cfg.ContentMaxWidthCh))
	b.WriteString("ch;--mdf-page-padding-block:")
	b.WriteString(formatFloat(s.cfg.Margin))
	b.WriteString("pt;--mdf-page-padding-inline:")
	b.WriteString(formatFloat(s.cfg.Margin))
	b.WriteString("pt;box-sizing:border-box;min-height:100vh;width:min(100%,var(--mdf-content-max-width));margin-inline:auto;padding-block:var(--mdf-page-padding-block);padding-inline:clamp(1rem,4vw,var(--mdf-page-padding-inline));white-space:pre-wrap;overflow-wrap:anywhere;tab-size:4;}\n")
	b.WriteString(".mdf-document a{color:inherit;text-decoration:none;}\n")
	b.WriteString(".mdf-document a:hover{text-decoration:underline;}\n")
	b.WriteString(".mdf-heading{display:inline-block;box-sizing:border-box;max-width:100%;white-space:normal;overflow-wrap:anywhere;padding-left:var(--mdf-heading-indent);text-indent:calc(-1 * var(--mdf-heading-indent));vertical-align:top;}\n")
	b.WriteString(".mdf-line{display:inline-grid;grid-template-columns:max-content minmax(0,1fr);max-width:100%;white-space:normal;overflow-wrap:anywhere;vertical-align:top;}\n")
	b.WriteString(".mdf-prefix{grid-column:1;white-space:pre;}\n")
	b.WriteString(".mdf-content{grid-column:2;min-width:0;white-space:pre-wrap;overflow-wrap:anywhere;}\n")
	b.WriteString(".mdf-table-block{display:grid;grid-template-columns:max-content max-content;column-gap:0;align-items:start;margin:1em 0;clear:both;}\n")
	b.WriteString(".mdf-table-block .mdf-table{margin:0;}\n")
	b.WriteString(".mdf-corner-image{float:right;max-width:")
	b.WriteString(formatFloat(s.cfg.CornerImageMaxWidth))
	b.WriteString("pt;max-height:")
	b.WriteString(formatFloat(s.cfg.CornerImageMaxHeight))
	b.WriteString("pt;margin-left:")
	b.WriteString(formatFloat(s.cfg.CornerImagePadding))
	b.WriteString("pt;margin-bottom:")
	b.WriteString(formatFloat(s.cfg.CornerImagePadding))
	b.WriteString("pt;object-fit:contain;}\n")
	tableWire := rgbCSS(parseANSIPrefix(s.styles.TableWire.Prefix, [3]int{64, 64, 64}).color)
	if s.cfg.IgnoreColors {
		tableWire = rgbCSS(s.cfg.TextRGB)
	}
	b.WriteString(".mdf-table{--mdf-table-wire:")
	b.WriteString(tableWire)
	b.WriteString(";white-space:normal;border-collapse:collapse;margin:1em 0;clear:both;}\n")
	b.WriteString(".mdf-table th,.mdf-table td{vertical-align:top;overflow-wrap:anywhere;white-space:pre-wrap;}\n")
	b.WriteString(".mdf-table th{")
	b.WriteString(s.styleFor(s.styles.TableHeader))
	b.WriteString("overflow-wrap:normal;word-break:normal;}\n")
	b.WriteString(".mdf-table-bordered th,.mdf-table-bordered td{border:1px solid var(--mdf-table-wire);padding:.2em 1ch;}\n")
	b.WriteString(".mdf-table-space{border-collapse:collapse;border-spacing:0;}\n")
	b.WriteString(".mdf-table-space th,.mdf-table-space td{border:0;padding:0 1ch;}\n")
	b.WriteString(".mdf-table-space th:first-child,.mdf-table-space td:first-child{padding-left:0;}\n")
	b.WriteString(".mdf-table-space th:last-child,.mdf-table-space td:last-child{padding-right:0;}\n")
}

func (s *stream) writeRowBufferedTable(row mdf.TableRow) error {
	if !s.tableOpen {
		s.tableRows = append(s.tableRows, row)
		if len(s.tableRows) < 2 {
			return nil
		}
		s.tableColumns = htmlTableColumnCount(s.tableRows, s.tableStart.Alignments)
		if err := s.openTable(); err != nil {
			return err
		}
		if s.tableRows[0].Header {
			if err := s.writeEmissionString("<thead>"); err != nil {
				return err
			}
			if err := s.writeHTMLTableRow(s.tableRows[0], s.tableColumns); err != nil {
				return err
			}
			if err := s.writeEmissionString("</thead><tbody>"); err != nil {
				return err
			}
			s.tableBodyOpen = true
			if err := s.writeHTMLTableRow(s.tableRows[1], s.tableColumns); err != nil {
				return err
			}
		} else {
			if err := s.writeEmissionString("<tbody>"); err != nil {
				return err
			}
			s.tableBodyOpen = true
			for _, buffered := range s.tableRows {
				if err := s.writeHTMLTableRow(buffered, s.tableColumns); err != nil {
					return err
				}
			}
		}
		s.tableRows = s.tableRows[:0]
		return nil
	}
	if !s.tableBodyOpen && !row.Header {
		if err := s.writeEmissionString("<tbody>"); err != nil {
			return err
		}
		s.tableBodyOpen = true
	}
	return s.writeHTMLTableRow(row, s.tableColumns)
}

func (s *stream) writeFullTable(rows []mdf.TableRow) error {
	if len(rows) == 0 {
		return nil
	}
	if err := s.openTable(); err != nil {
		return err
	}
	s.tableColumns = htmlTableColumnCount(rows, s.tableStart.Alignments)
	idx := 0
	if rows[0].Header {
		if err := s.writeEmissionString("<thead>"); err != nil {
			return err
		}
		if err := s.writeHTMLTableRow(rows[0], s.tableColumns); err != nil {
			return err
		}
		if err := s.writeEmissionString("</thead>"); err != nil {
			return err
		}
		idx = 1
	}
	if idx < len(rows) {
		if err := s.writeEmissionString("<tbody>"); err != nil {
			return err
		}
		s.tableBodyOpen = true
		for ; idx < len(rows); idx++ {
			if err := s.writeHTMLTableRow(rows[idx], s.tableColumns); err != nil {
				return err
			}
		}
		if err := s.writeEmissionString("</tbody>"); err != nil {
			return err
		}
		s.tableBodyOpen = false
	}
	return nil
}

func (s *stream) openTable() error {
	if len(s.tableStart.Prefix) > 0 {
		if err := s.writeEmissionString(`<div class="mdf-table-block"><span class="mdf-prefix">`); err != nil {
			return err
		}
		if err := s.writeHTMLTablePrefix(s.tableStart.Prefix); err != nil {
			return err
		}
		if err := s.writeEmissionString(`</span>`); err != nil {
			return err
		}
		s.tableWrapped = true
	}
	class := "mdf-table mdf-table-bordered"
	if s.cfg.TableWireMode == mdf.TableWireSpace {
		class = "mdf-table mdf-table-space"
	}
	if err := s.writeEmissionString(fmt.Sprintf(`<table class="%s">`, class)); err != nil {
		return err
	}
	s.tableOpen = true
	return nil
}

func (s *stream) writeHTMLTablePrefix(prefix []mdf.TablePrefixSegment) error {
	for _, segment := range prefix {
		style := s.styleFor(segment.Style)
		if style == "" {
			if err := s.writeEmissionString(stdhtml.EscapeString(segment.Text)); err != nil {
				return err
			}
			continue
		}
		if err := s.writeEmissionString(fmt.Sprintf(`<span style="%s">%s</span>`, stdhtml.EscapeString(style), stdhtml.EscapeString(segment.Text))); err != nil {
			return err
		}
	}
	return nil
}

func htmlTableColumnCount(rows []mdf.TableRow, alignments []mdf.TableAlignment) int {
	if len(alignments) > 0 {
		return len(alignments)
	}
	cols := 0
	for _, row := range rows {
		if len(row.Cells) > cols {
			cols = len(row.Cells)
		}
	}
	if cols == 0 {
		return 1
	}
	return cols
}

func (s *stream) writeHTMLTableRow(row mdf.TableRow, cols int) error {
	if err := s.writeEmissionString("<tr>"); err != nil {
		return err
	}
	tag := "td"
	if row.Header {
		tag = "th"
	}
	if cols <= 0 {
		cols = htmlTableColumnCount([]mdf.TableRow{row}, s.tableStart.Alignments)
	}
	for i := 0; i < cols; i++ {
		align := "left"
		if i < len(s.tableStart.Alignments) {
			align = htmlTableAlign(s.tableStart.Alignments[i])
		}
		if err := s.writeEmissionString(fmt.Sprintf(`<%s style="text-align:%s;">`, tag, align)); err != nil {
			return err
		}
		if i < len(row.Cells) {
			if err := s.writeHTMLTableCellContent(row.Cells[i], row.Header); err != nil {
				return err
			}
			if len(s.tableStart.Alignments) == 0 && i == cols-1 {
				for _, extra := range row.Cells[cols:] {
					if err := s.writeEmissionString(" | "); err != nil {
						return err
					}
					if err := s.writeHTMLTableCellContent(extra, row.Header); err != nil {
						return err
					}
				}
			}
		}
		if err := s.writeEmissionString(fmt.Sprintf(`</%s>`, tag)); err != nil {
			return err
		}
	}
	return s.writeEmissionString("</tr>")
}

func (s *stream) writeHTMLTableCellContent(cell mdf.TableCell, header bool) error {
	if len(cell.Tokens) == 0 {
		return s.writeEmissionString(stdhtml.EscapeString(cell.Text))
	}
	currentStyle := ""
	spanOpen := false
	closeSpan := func() error {
		if !spanOpen {
			return nil
		}
		if err := s.writeEmissionString("</span>"); err != nil {
			return err
		}
		spanOpen = false
		currentStyle = ""
		return nil
	}
	for _, tok := range cell.Tokens {
		if tok.Kind == mdf.TokenLinkStart {
			if err := closeSpan(); err != nil {
				return err
			}
			if err := s.writeEmissionString(fmt.Sprintf(`<a href="%s">`, stdhtml.EscapeString(tok.LinkURL))); err != nil {
				return err
			}
			continue
		}
		if tok.Kind == mdf.TokenLinkEnd {
			if err := closeSpan(); err != nil {
				return err
			}
			if err := s.writeEmissionString("</a>"); err != nil {
				return err
			}
			continue
		}
		if tok.Text == "" {
			continue
		}
		styleAttr := s.tableCellTokenStyle(tok.Style, header)
		if styleAttr != currentStyle {
			if err := closeSpan(); err != nil {
				return err
			}
			if styleAttr != "" {
				if err := s.writeEmissionString(fmt.Sprintf(`<span style="%s">`, stdhtml.EscapeString(styleAttr))); err != nil {
					return err
				}
				spanOpen = true
				currentStyle = styleAttr
			}
		}
		if err := s.writeEmissionString(stdhtml.EscapeString(tok.Text)); err != nil {
			return err
		}
	}
	return closeSpan()
}

func (s *stream) tableCellTokenStyle(style mdf.Style, header bool) string {
	if style.Prefix == "" {
		return ""
	}
	if !header || s.styles.TableHeader.Prefix == "" {
		return s.styleFor(style)
	}
	headerAttrs := parseANSIPrefix(s.styles.TableHeader.Prefix, s.cfg.TextRGB)
	inlineAttrs := parseANSIPrefix(style.Prefix, s.cfg.TextRGB)
	if s.cfg.IgnoreColors {
		headerAttrs.color = s.cfg.TextRGB
	}
	var b strings.Builder
	b.WriteString("color:")
	b.WriteString(rgbCSS(headerAttrs.color))
	b.WriteByte(';')
	if headerAttrs.bold || inlineAttrs.bold {
		b.WriteString("font-weight:700;")
	}
	if headerAttrs.italic || inlineAttrs.italic {
		b.WriteString("font-style:italic;")
	}
	if headerAttrs.underline || inlineAttrs.underline {
		b.WriteString("text-decoration:underline;")
	}
	return b.String()
}

func htmlTableAlign(align mdf.TableAlignment) string {
	switch align {
	case mdf.TableAlignRight:
		return "right"
	case mdf.TableAlignCenter:
		return "center"
	default:
		return "left"
	}
}

func writeFontFace(b *strings.Builder, family string, weight string, style string, data []byte, format string) {
	if len(data) == 0 {
		return
	}
	mime := fontMIMEForFormat(format)
	if mime == "" {
		mime = "font/ttf"
	}
	if format == "" {
		format = "truetype"
	}
	b.WriteString("@font-face{font-family:")
	b.WriteString(cssString(family))
	b.WriteString(";src:url(data:")
	b.WriteString(mime)
	b.WriteString(";base64,")
	b.WriteString(base64.StdEncoding.EncodeToString(data))
	b.WriteString(") format('")
	b.WriteString(format)
	b.WriteString("');font-weight:")
	b.WriteString(weight)
	b.WriteString(";font-style:")
	b.WriteString(style)
	b.WriteString(";font-display:block;}\n")
}

func fontMIMEForFormat(format string) string {
	switch format {
	case "woff2":
		return "font/woff2"
	case "woff":
		return "font/woff"
	case "opentype":
		return "font/otf"
	case "", "truetype":
		return "font/ttf"
	default:
		return ""
	}
}

func (s *stream) openLink(url string) error {
	if err := s.closeSpan(); err != nil {
		return err
	}
	if s.linkOpen {
		if err := s.closeLink(); err != nil {
			return err
		}
	}
	if err := s.writeEmissionString(fmt.Sprintf(`<a href="%s">`, stdhtml.EscapeString(url))); err != nil {
		return err
	}
	s.linkOpen = true
	return nil
}

func (s *stream) closeLink() error {
	if !s.linkOpen {
		return nil
	}
	if err := s.closeSpan(); err != nil {
		return err
	}
	if err := s.writeEmissionString("</a>"); err != nil {
		return err
	}
	s.linkOpen = false
	return nil
}

func (s *stream) writeText(text string, style mdf.Style) error {
	text = strings.ReplaceAll(text, "\u00A0", " ")
	for text != "" {
		idx := strings.IndexByte(text, '\n')
		if idx == -1 {
			return s.writeTextSegment(text, style)
		}
		if idx > 0 {
			if err := s.writeTextSegment(text[:idx], style); err != nil {
				return err
			}
		}
		if err := s.writeNewline(); err != nil {
			return err
		}
		text = text[idx+1:]
	}
	return nil
}

func (s *stream) writeTextSegment(text string, style mdf.Style) error {
	if text == "" {
		return nil
	}
	for i, r := range text {
		if !s.atLineStart && !s.headingActive && !s.headingOpen && len(s.lineProbe) == 0 {
			return s.writeTextContent(text[i:], style)
		}
		if err := s.writeRune(r, style); err != nil {
			return err
		}
	}
	return nil
}

func (s *stream) writeRune(r rune, style mdf.Style) error {
	if s.headingOpen {
		return s.writeEmissionString(stdhtml.EscapeString(string(r)))
	}
	if s.atLineStart && !s.headingActive && r == '#' && s.headingLevel(style) > 0 {
		s.headingStyle = style
		s.headingActive = true
	}
	if s.atLineStart && s.headingActive {
		s.headingProbe.WriteRune(r)
		probe := s.headingProbe.String()
		if isHeadingMarkerProbe(probe) {
			return nil
		}
		if isCompleteHeadingMarker(probe) {
			return s.openHeading(probe, s.headingStyle)
		}
		return s.flushHeadingProbe(style)
	}
	if s.atLineStart {
		return s.writeLineStartRune(r, style)
	}
	if err := s.openSpan(style); err != nil {
		return err
	}
	if err := s.writeEmissionString(stdhtml.EscapeString(string(r))); err != nil {
		return err
	}
	if r != '\r' {
		s.atLineStart = false
	}
	return nil
}

func (s *stream) writeTextContent(text string, style mdf.Style) error {
	if text == "" {
		return nil
	}
	if err := s.openSpan(style); err != nil {
		return err
	}
	if err := s.writeEmissionString(stdhtml.EscapeString(text)); err != nil {
		return err
	}
	if text != "\r" {
		s.atLineStart = false
	}
	return nil
}

func (s *stream) writeNewline() error {
	if s.headingProbe.Len() > 0 {
		if err := s.flushHeadingProbe(s.headingStyle); err != nil {
			return err
		}
	}
	if len(s.lineProbe) > 0 {
		if err := s.flushLineProbe(); err != nil {
			return err
		}
	}
	if s.headingOpen {
		if err := s.closeHeading(); err != nil {
			return err
		}
	} else if err := s.closeSpan(); err != nil {
		return err
	}
	if err := s.closeHangingLine(); err != nil {
		return err
	}
	if err := s.writeEmissionString("\n"); err != nil {
		return err
	}
	s.atLineStart = true
	return nil
}

func (s *stream) openHeading(marker string, style mdf.Style) error {
	if err := s.closeSpan(); err != nil {
		return err
	}
	attr := s.styleFor(style)
	attr += "--mdf-heading-indent:" + formatFloat(float64(len(marker))) + "ch;"
	if err := s.writeEmissionString(fmt.Sprintf(`<span class="mdf-heading" style="%s">`, stdhtml.EscapeString(attr))); err != nil {
		return err
	}
	if err := s.writeEmissionString(stdhtml.EscapeString(marker)); err != nil {
		return err
	}
	s.headingOpen = true
	s.atLineStart = false
	s.headingProbe.Reset()
	s.headingStyle = mdf.Style{}
	s.headingActive = false
	return nil
}

func (s *stream) closeHeading() error {
	if !s.headingOpen {
		return nil
	}
	if err := s.writeEmissionString("</span>"); err != nil {
		return err
	}
	s.headingOpen = false
	return nil
}

func (s *stream) writeLineStartRune(r rune, style mdf.Style) error {
	s.lineProbe = append(s.lineProbe, styledRune{r: r, style: style})
	prefixLen, pending := hangingPrefixLen(s.lineProbeText())
	if pending {
		return nil
	}
	s.linePrefixLen = prefixLen
	return s.flushLineProbe()
}

func (s *stream) flushLineProbe() error {
	probe := s.lineProbe
	s.lineProbe = nil
	prefixLen := s.linePrefixLen
	s.linePrefixLen = 0
	if prefixLen > 0 {
		if err := s.openPrefixedLine(probe, prefixLen); err != nil {
			return err
		}
	} else {
		for _, item := range probe {
			if err := s.writeStyledRune(item); err != nil {
				return err
			}
		}
	}
	if len(probe) > 0 {
		s.atLineStart = false
	}
	return nil
}

func (s *stream) lineProbeText() string {
	var b strings.Builder
	for _, item := range s.lineProbe {
		b.WriteRune(item.r)
	}
	return b.String()
}

func (s *stream) openPrefixedLine(probe []styledRune, prefixLen int) error {
	if prefixLen <= 0 || s.lineOpen {
		return nil
	}
	if err := s.closeSpan(); err != nil {
		return err
	}
	if err := s.writeEmissionString(`<span class="mdf-line"><span class="mdf-prefix">`); err != nil {
		return err
	}
	for i, item := range probe {
		if i == prefixLen {
			if err := s.closeSpan(); err != nil {
				return err
			}
			if err := s.writeEmissionString(`</span><span class="mdf-content">`); err != nil {
				return err
			}
			s.lineOpen = true
		}
		if err := s.writeStyledRune(item); err != nil {
			return err
		}
	}
	if !s.lineOpen {
		if err := s.closeSpan(); err != nil {
			return err
		}
		if err := s.writeEmissionString(`</span><span class="mdf-content">`); err != nil {
			return err
		}
		s.lineOpen = true
	}
	return nil
}

func (s *stream) closeHangingLine() error {
	if !s.lineOpen {
		return nil
	}
	if err := s.closeSpan(); err != nil {
		return err
	}
	if err := s.writeEmissionString("</span></span>"); err != nil {
		return err
	}
	s.lineOpen = false
	return nil
}

func (s *stream) writeStyledRune(item styledRune) error {
	if err := s.openSpan(item.style); err != nil {
		return err
	}
	return s.writeEmissionString(stdhtml.EscapeString(string(item.r)))
}

func (s *stream) flushHeadingProbe(style mdf.Style) error {
	text := s.headingProbe.String()
	s.headingProbe.Reset()
	s.headingStyle = mdf.Style{}
	s.headingActive = false
	if text == "" {
		return nil
	}
	if err := s.openSpan(style); err != nil {
		return err
	}
	if err := s.writeEmissionString(stdhtml.EscapeString(text)); err != nil {
		return err
	}
	s.atLineStart = false
	return nil
}

func isHeadingMarkerProbe(text string) bool {
	if text == "" || len(text) > 6 {
		return false
	}
	for _, r := range text {
		if r != '#' {
			return false
		}
	}
	return true
}

func isCompleteHeadingMarker(text string) bool {
	if len(text) < 2 || len(text) > 7 || text[len(text)-1] != ' ' {
		return false
	}
	for _, r := range text[:len(text)-1] {
		if r != '#' {
			return false
		}
	}
	return true
}

func hangingPrefixLen(text string) (int, bool) {
	runes := []rune(text)
	if len(runes) == 0 {
		return 0, true
	}
	i := 0
	for i < len(runes) && runes[i] == ' ' {
		i++
	}
	if i == len(runes) {
		return 0, true
	}
	hadQuote := false
	for {
		if i >= len(runes) {
			return 0, true
		}
		if runes[i] != '>' {
			break
		}
		hadQuote = true
		i++
		if i >= len(runes) {
			return 0, true
		}
		if runes[i] != ' ' {
			return 0, false
		}
		i++
		for i < len(runes) && runes[i] == ' ' {
			i++
		}
		if i == len(runes) {
			return 0, true
		}
	}
	if markerPrefixLen, pending := listPrefixLen(runes[i:]); pending {
		return 0, true
	} else if markerPrefixLen > 0 {
		return i + markerPrefixLen, false
	}
	if hadQuote {
		return i, false
	}
	if i > 0 {
		return i, false
	}
	return 0, false
}

func listPrefixLen(runes []rune) (int, bool) {
	if len(runes) == 0 {
		return 0, true
	}
	if runes[0] == '-' || runes[0] == '+' || runes[0] == '*' {
		return unorderedListPrefixLen(runes)
	}
	if runes[0] >= '0' && runes[0] <= '9' {
		return orderedListPrefixLen(runes)
	}
	return 0, false
}

func unorderedListPrefixLen(runes []rune) (int, bool) {
	if len(runes) == 1 {
		return 0, true
	}
	if runes[1] != ' ' {
		return 0, false
	}
	if len(runes) == 2 {
		return 0, true
	}
	if runes[2] != '[' {
		return 2, false
	}
	if len(runes) < 6 {
		return 0, true
	}
	if isTaskState(runes[3]) && runes[4] == ']' && runes[5] == ' ' {
		return 6, false
	}
	return 2, false
}

func orderedListPrefixLen(runes []rune) (int, bool) {
	i := 0
	for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
		i++
	}
	if i == len(runes) {
		return 0, true
	}
	if runes[i] != '.' && runes[i] != ')' {
		return 0, false
	}
	if i+1 == len(runes) {
		return 0, true
	}
	if runes[i+1] != ' ' {
		return 0, false
	}
	if i+2 == len(runes) {
		return 0, true
	}
	return i + 2, false
}

func isTaskState(r rune) bool {
	return r == ' ' || r == 'x' || r == 'X'
}

func (s *stream) openSpan(style mdf.Style) error {
	if s.headingOpen {
		return nil
	}
	styleAttr := s.styleFor(style)
	if s.spanOpen && styleAttr == s.currentStyle {
		return nil
	}
	if err := s.closeSpan(); err != nil {
		return err
	}
	if styleAttr == "" {
		return nil
	}
	if err := s.writeEmissionString(fmt.Sprintf(`<span style="%s">`, stdhtml.EscapeString(styleAttr))); err != nil {
		return err
	}
	s.spanOpen = true
	s.currentStyle = styleAttr
	return nil
}

func (s *stream) closeSpan() error {
	if !s.spanOpen {
		return nil
	}
	if err := s.writeEmissionString("</span>"); err != nil {
		return err
	}
	s.spanOpen = false
	s.currentStyle = ""
	return nil
}

func (s *stream) writeEmissionString(text string) error {
	if text == "" {
		return nil
	}
	n, err := io.WriteString(s.w, text)
	if n > 0 && s.trace != nil {
		if traceErr := s.traceHTMLString(text[:n]); traceErr != nil && err == nil {
			return traceErr
		}
	}
	return err
}

func (s *stream) traceHTMLString(text string) error {
	if text == "" {
		return nil
	}
	if !isHTMLTextTracePayload(text) {
		if err := s.flushTraceText(); err != nil {
			return err
		}
		return s.trace.EmitString(text)
	}
	for _, r := range text {
		if r == ' ' || r == '\t' {
			if err := s.flushTraceText(); err != nil {
				return err
			}
			if err := s.trace.EmitString(string(r)); err != nil {
				return err
			}
			continue
		}
		s.traceText.WriteRune(r)
	}
	return nil
}

func (s *stream) flushTraceText() error {
	if s.trace == nil || s.traceText.Len() == 0 {
		return nil
	}
	text := s.traceText.String()
	s.traceText.Reset()
	return s.trace.EmitString(text)
}

func isHTMLTextTracePayload(text string) bool {
	return !strings.ContainsAny(text, "<>\n\r")
}

func (s *stream) styleFor(style mdf.Style) string {
	headingLevel := s.headingLevel(style)
	key := style.Prefix
	if headingLevel > 0 {
		key += "#h" + formatFloat(float64(headingLevel))
	}
	if cached, ok := s.styleCache[key]; ok {
		return cached
	}
	size := 0.0
	if headingLevel > 0 {
		size = s.cfg.FontSize * s.cfg.HeadingScale[headingLevel-1]
	}
	attr := styleAttr(style.Prefix, s.cfg.TextRGB, s.cfg.IgnoreColors, size)
	if headingLevel > 0 {
		if len(s.cfg.HeadingFontBytes) > 0 {
			attr += "font-family:" + cssString(headingFontFamily) + "," + cssString(s.cfg.FontFamily) + ",monospace;"
		}
		attr += "font-weight:700;"
	}
	s.styleCache[key] = attr
	return attr
}

func (s *stream) headingLevel(style mdf.Style) int {
	for i := 0; i < len(s.styles.Heading); i++ {
		if style.Prefix == s.styles.Heading[i].Prefix {
			return i + 1
		}
	}
	return 0
}

func cssString(value string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}
