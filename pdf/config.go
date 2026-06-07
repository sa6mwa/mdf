package pdf

import "pkt.systems/mdf"

// Config holds PDF rendering settings.
type Config struct {
	// PageSize is the PDF page size name, such as "A4" or "Letter".
	PageSize string
	// Margin is the page margin in points.
	Margin float64
	// FontFamily is the PDF font family name.
	FontFamily string
	// FontSize is the base font size in points.
	FontSize float64
	// LineHeight is the line-height multiplier.
	LineHeight float64
	// RegularFont, BoldFont, ItalicFont, and BoldItalicFont are TTF paths.
	RegularFont, BoldFont, ItalicFont, BoldItalicFont string
	// HeadingFont is an optional TTF path for headings.
	HeadingFont string
	// RegularFontBytes, BoldFontBytes, ItalicFontBytes, and BoldItalicFontBytes
	// embed TTF font data directly. They cannot be mixed with font paths.
	RegularFontBytes, BoldFontBytes, ItalicFontBytes, BoldItalicFontBytes []byte
	// HeadingScale contains font-size multipliers for H1 through H6.
	HeadingScale [6]float64
	// IgnoreColors renders text using TextRGB instead of theme colors.
	IgnoreColors bool
	// BackgroundEnabled controls whether BackgroundRGB is painted.
	BackgroundEnabled bool
	// UseOCGPrintView is a legacy compatibility name for the single-file
	// split screen/print PDF mode. The implementation is annotation-based.
	UseOCGPrintView bool
	// OpenLayerPane is a legacy compatibility option from the old OCG-based
	// implementation. It has no effect in annotation-based split mode.
	OpenLayerPane bool
	// Boring disables colors and background for plain print-like output.
	Boring bool
	// BackgroundRGB is the themed page background color.
	BackgroundRGB [3]int
	// TextRGB is the fallback text color.
	TextRGB [3]int
	// CornerImagePath is an optional PNG or JPEG path floated at the top-right.
	CornerImagePath string
	// CornerImageMaxWidth and CornerImageMaxHeight constrain the corner image in points.
	CornerImageMaxWidth, CornerImageMaxHeight float64
	// CornerImagePadding controls spacing around the corner image in points.
	CornerImagePadding float64
	// TableBufferMode controls table buffering for PDF tables.
	TableBufferMode mdf.TableBufferMode
	// TableWireMode controls visible table separators.
	TableWireMode mdf.TableWireMode
}

const headingFontFamily = "Heading"

// DefaultConfig returns a baseline configuration.
func DefaultConfig() Config {
	return Config{
		PageSize:   "A4",
		Margin:     36,
		FontFamily: "HackNerdFontMono",
		FontSize:   12,
		LineHeight: 1.4,
		HeadingScale: [6]float64{
			1.9,
			1.6,
			1.3,
			1.0,
			1.0,
			1.0,
		},
		BackgroundEnabled:    true,
		BackgroundRGB:        [3]int{0, 0, 0},
		TextRGB:              [3]int{220, 220, 220},
		CornerImageMaxWidth:  96,
		CornerImageMaxHeight: 96,
		CornerImagePadding:   8,
		TableBufferMode:      mdf.TableBufferFull,
		TableWireMode:        mdf.TableWireLine,
	}
}
