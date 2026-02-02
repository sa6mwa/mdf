package pdf

// Config holds PDF rendering settings.
type Config struct {
	PageSize             string
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
	HeadingScale         [6]float64
	IgnoreColors         bool
	BackgroundEnabled    bool
	UseOCGPrintView      bool
	OpenLayerPane        bool
	Boring               bool
	BackgroundRGB        [3]int
	TextRGB              [3]int
	CornerImagePath      string
	CornerImageMaxWidth  float64
	CornerImageMaxHeight float64
	CornerImagePadding   float64
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
	}
}
