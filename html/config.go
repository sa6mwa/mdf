package html

import "pkt.systems/mdf/pdf"

// Config holds HTML rendering settings.
type Config struct {
	Margin               float64
	FontFamily           string
	FontSize             float64
	LineHeight           float64
	RegularFont          string
	BoldFont             string
	ItalicFont           string
	BoldItalicFont       string
	HeadingFont          string
	RegularFontBytes     []byte
	BoldFontBytes        []byte
	ItalicFontBytes      []byte
	BoldItalicFontBytes  []byte
	HeadingFontBytes     []byte
	HeadingScale         [6]float64
	IgnoreColors         bool
	BackgroundEnabled    bool
	Boring               bool
	BackgroundRGB        [3]int
	TextRGB              [3]int
	CornerImagePath      string
	CornerImageBytes     []byte
	CornerImageMIME      string
	CornerImageMaxWidth  float64
	CornerImageMaxHeight float64
	CornerImagePadding   float64
}

// DefaultConfig returns a baseline configuration aligned with the PDF
// renderer's visual defaults.
func DefaultConfig() Config {
	pdfCfg := pdf.DefaultConfig()
	return Config{
		Margin:               pdfCfg.Margin,
		FontFamily:           pdf.EmbeddedFontFamily,
		FontSize:             pdfCfg.FontSize,
		LineHeight:           pdfCfg.LineHeight,
		HeadingScale:         pdfCfg.HeadingScale,
		BackgroundEnabled:    pdfCfg.BackgroundEnabled,
		BackgroundRGB:        pdfCfg.BackgroundRGB,
		TextRGB:              pdfCfg.TextRGB,
		CornerImageMaxWidth:  pdfCfg.CornerImageMaxWidth,
		CornerImageMaxHeight: pdfCfg.CornerImageMaxHeight,
		CornerImagePadding:   pdfCfg.CornerImagePadding,
	}
}
