package html

import (
	"pkt.systems/mdf"
	"pkt.systems/mdf/pdf"
)

// EmbeddedFont identifies a vendored HTML font bundle.
type EmbeddedFont string

const (
	// EmbeddedFontJetBrainsMono selects the vendored JetBrains Mono variable
	// webfont bundle.
	EmbeddedFontJetBrainsMono EmbeddedFont = "jetbrainsmono"
	// EmbeddedFontHack selects the vendored Hack Nerd Font Mono TTF bundle.
	EmbeddedFontHack EmbeddedFont = "hack"
)

// Config holds HTML rendering settings.
type Config struct {
	// Margin controls page padding in points.
	Margin float64
	// ContentMaxWidthCh constrains the document width in CSS ch units.
	ContentMaxWidthCh float64
	// FontFamily is the CSS font-family name used for body text.
	FontFamily string
	// EmbeddedFont selects the vendored HTML font bundle when explicit font
	// paths or embedded font bytes are not provided.
	EmbeddedFont EmbeddedFont
	// FontSize is the base font size in points.
	FontSize float64
	// LineHeight is the CSS line-height multiplier.
	LineHeight float64
	// RegularFont, BoldFont, ItalicFont, and BoldItalicFont are TTF paths.
	RegularFont, BoldFont, ItalicFont, BoldItalicFont string
	// HeadingFont is an optional TTF path for headings.
	HeadingFont string
	// RegularFontBytes, BoldFontBytes, ItalicFontBytes, and BoldItalicFontBytes
	// embed font data directly. They cannot be mixed with font paths.
	RegularFontBytes, BoldFontBytes, ItalicFontBytes, BoldItalicFontBytes []byte
	// RegularFontFormat, BoldFontFormat, ItalicFontFormat, and
	// BoldItalicFontFormat are CSS font format hints for the corresponding
	// embedded font bytes. They default to "truetype".
	RegularFontFormat, BoldFontFormat, ItalicFontFormat, BoldItalicFontFormat string
	// HeadingFontBytes embeds optional heading TTF data directly.
	HeadingFontBytes []byte
	// HeadingScale contains font-size multipliers for H1 through H6.
	HeadingScale [6]float64
	// IgnoreColors renders text using TextRGB instead of theme colors.
	IgnoreColors bool
	// BackgroundEnabled controls whether BackgroundRGB is emitted.
	BackgroundEnabled bool
	// Boring disables colors and background for plain black-on-transparent output.
	Boring bool
	// BackgroundRGB is the document background color.
	BackgroundRGB [3]int
	// TextRGB is the fallback text color.
	TextRGB [3]int
	// CornerImagePath is an optional PNG or JPEG path floated at the top-right.
	CornerImagePath string
	// CornerImageBytes embeds a corner image directly.
	CornerImageBytes []byte
	// CornerImageMIME is required when CornerImageBytes is set.
	CornerImageMIME string
	// CornerImageMaxWidth and CornerImageMaxHeight constrain the corner image in points.
	CornerImageMaxWidth, CornerImageMaxHeight float64
	// CornerImagePadding controls spacing around the corner image in points.
	CornerImagePadding float64
	// TableBufferMode controls table buffering for HTML tables.
	TableBufferMode mdf.TableBufferMode
	// TableWireMode controls visible table separators. TableWireASCII is unsupported for HTML.
	TableWireMode mdf.TableWireMode
}

// DefaultConfig returns a baseline configuration aligned with the PDF
// renderer's visual defaults.
func DefaultConfig() Config {
	pdfCfg := pdf.DefaultConfig()
	return Config{
		Margin:               pdfCfg.Margin,
		ContentMaxWidthCh:    96,
		FontFamily:           jetBrainsMonoFontFamily,
		EmbeddedFont:         EmbeddedFontJetBrainsMono,
		FontSize:             pdfCfg.FontSize,
		LineHeight:           pdfCfg.LineHeight,
		HeadingScale:         pdfCfg.HeadingScale,
		BackgroundEnabled:    pdfCfg.BackgroundEnabled,
		BackgroundRGB:        pdfCfg.BackgroundRGB,
		TextRGB:              pdfCfg.TextRGB,
		CornerImageMaxWidth:  pdfCfg.CornerImageMaxWidth,
		CornerImageMaxHeight: pdfCfg.CornerImageMaxHeight,
		CornerImagePadding:   pdfCfg.CornerImagePadding,
		TableBufferMode:      mdf.TableBufferFull,
		TableWireMode:        mdf.TableWireLine,
	}
}
