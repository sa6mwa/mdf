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
}

type styledRune struct {
	r     rune
	style mdf.Style
}

func newStream(w io.Writer, cfg Config, styles mdf.Styles, cornerImage *embeddedImage) *stream {
	return &stream{
		w:           w,
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
		_, err := io.WriteString(s.w, `<hr class="mdf-thematic-break">`)
		return err
	}
	if tok.Text == "" {
		return nil
	}
	return s.writeText(tok.Text, tok.Style)
}

func (s *stream) Flush() error {
	if s.documentEnded {
		return nil
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
	_, err := io.WriteString(s.w, "\n</main>\n</body>\n</html>\n")
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
	_, err := io.WriteString(s.w, b.String())
	return err
}

func (s *stream) writeCSS(b *strings.Builder) {
	writeFontFace(b, s.cfg.FontFamily, "400", "normal", s.cfg.RegularFontBytes)
	writeFontFace(b, s.cfg.FontFamily, "700", "normal", s.cfg.BoldFontBytes)
	writeFontFace(b, s.cfg.FontFamily, "400", "italic", s.cfg.ItalicFontBytes)
	if len(s.cfg.BoldItalicFontBytes) > 0 {
		writeFontFace(b, s.cfg.FontFamily, "700", "italic", s.cfg.BoldItalicFontBytes)
	}
	if len(s.cfg.HeadingFontBytes) > 0 {
		writeFontFace(b, headingFontFamily, "400", "normal", s.cfg.HeadingFontBytes)
		writeFontFace(b, headingFontFamily, "700", "normal", s.cfg.HeadingFontBytes)
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
	b.WriteString(".mdf-document{box-sizing:border-box;min-height:100vh;width:100%;padding:")
	b.WriteString(formatFloat(s.cfg.Margin))
	b.WriteString("pt;white-space:pre-wrap;overflow-wrap:anywhere;tab-size:4;}\n")
	b.WriteString(".mdf-document a{color:inherit;text-decoration:none;}\n")
	b.WriteString(".mdf-document a:hover{text-decoration:underline;}\n")
	b.WriteString(".mdf-heading{display:inline-block;box-sizing:border-box;max-width:100%;white-space:normal;overflow-wrap:anywhere;padding-left:var(--mdf-heading-indent);text-indent:calc(-1 * var(--mdf-heading-indent));vertical-align:top;}\n")
	b.WriteString(".mdf-line{display:inline-grid;grid-template-columns:max-content minmax(0,1fr);max-width:100%;white-space:normal;overflow-wrap:anywhere;vertical-align:top;}\n")
	b.WriteString(".mdf-prefix{grid-column:1;white-space:pre;}\n")
	b.WriteString(".mdf-content{grid-column:2;min-width:0;white-space:pre-wrap;overflow-wrap:anywhere;}\n")
	b.WriteString(".mdf-corner-image{float:right;max-width:")
	b.WriteString(formatFloat(s.cfg.CornerImageMaxWidth))
	b.WriteString("pt;max-height:")
	b.WriteString(formatFloat(s.cfg.CornerImageMaxHeight))
	b.WriteString("pt;margin-left:")
	b.WriteString(formatFloat(s.cfg.CornerImagePadding))
	b.WriteString("pt;margin-bottom:")
	b.WriteString(formatFloat(s.cfg.CornerImagePadding))
	b.WriteString("pt;object-fit:contain;}\n")
	b.WriteString(".mdf-thematic-break{border:0;border-top:1px solid ")
	b.WriteString(rgbCSS(s.cfg.TextRGB))
	b.WriteString(";margin:1em 0;clear:both;}\n")
}

func writeFontFace(b *strings.Builder, family string, weight string, style string, data []byte) {
	if len(data) == 0 {
		return
	}
	b.WriteString("@font-face{font-family:")
	b.WriteString(cssString(family))
	b.WriteString(";src:url(data:font/ttf;base64,")
	b.WriteString(base64.StdEncoding.EncodeToString(data))
	b.WriteString(") format('truetype');font-weight:")
	b.WriteString(weight)
	b.WriteString(";font-style:")
	b.WriteString(style)
	b.WriteString(";font-display:block;}\n")
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
	_, err := fmt.Fprintf(s.w, `<a href="%s">`, stdhtml.EscapeString(url))
	if err == nil {
		s.linkOpen = true
	}
	return err
}

func (s *stream) closeLink() error {
	if !s.linkOpen {
		return nil
	}
	if err := s.closeSpan(); err != nil {
		return err
	}
	_, err := io.WriteString(s.w, "</a>")
	if err == nil {
		s.linkOpen = false
	}
	return err
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
	for _, r := range text {
		if err := s.writeRune(r, style); err != nil {
			return err
		}
	}
	return nil
}

func (s *stream) writeRune(r rune, style mdf.Style) error {
	if s.headingOpen {
		_, err := io.WriteString(s.w, stdhtml.EscapeString(string(r)))
		return err
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
	_, err := io.WriteString(s.w, stdhtml.EscapeString(string(r)))
	if err == nil && r != '\r' {
		s.atLineStart = false
	}
	return err
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
	_, err := io.WriteString(s.w, "\n")
	if err == nil {
		s.atLineStart = true
	}
	return err
}

func (s *stream) openHeading(marker string, style mdf.Style) error {
	if err := s.closeSpan(); err != nil {
		return err
	}
	attr := s.styleFor(style)
	attr += "--mdf-heading-indent:" + formatFloat(float64(len(marker))) + "ch;"
	_, err := fmt.Fprintf(s.w, `<span class="mdf-heading" style="%s">`, stdhtml.EscapeString(attr))
	if err != nil {
		return err
	}
	_, err = io.WriteString(s.w, stdhtml.EscapeString(marker))
	if err != nil {
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
	_, err := io.WriteString(s.w, "</span>")
	if err == nil {
		s.headingOpen = false
	}
	return err
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
	if _, err := io.WriteString(s.w, `<span class="mdf-line"><span class="mdf-prefix">`); err != nil {
		return err
	}
	for i, item := range probe {
		if i == prefixLen {
			if err := s.closeSpan(); err != nil {
				return err
			}
			if _, err := io.WriteString(s.w, `</span><span class="mdf-content">`); err != nil {
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
		if _, err := io.WriteString(s.w, `</span><span class="mdf-content">`); err != nil {
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
	_, err := io.WriteString(s.w, "</span></span>")
	if err == nil {
		s.lineOpen = false
	}
	return err
}

func (s *stream) writeStyledRune(item styledRune) error {
	if err := s.openSpan(item.style); err != nil {
		return err
	}
	_, err := io.WriteString(s.w, stdhtml.EscapeString(string(item.r)))
	return err
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
	_, err := io.WriteString(s.w, stdhtml.EscapeString(text))
	if err == nil {
		s.atLineStart = false
	}
	return err
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
	_, err := fmt.Fprintf(s.w, `<span style="%s">`, stdhtml.EscapeString(styleAttr))
	if err == nil {
		s.spanOpen = true
		s.currentStyle = styleAttr
	}
	return err
}

func (s *stream) closeSpan() error {
	if !s.spanOpen {
		return nil
	}
	_, err := io.WriteString(s.w, "</span>")
	if err == nil {
		s.spanOpen = false
		s.currentStyle = ""
	}
	return err
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
