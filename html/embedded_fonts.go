package html

import (
	"embed"
	"fmt"

	"pkt.systems/mdf/pdf"
)

const (
	jetBrainsMonoFontFamily = "JetBrains Mono"
)

const (
	jetBrainsMonoRegularFontName = "JetBrainsMono[wght].woff2"
	jetBrainsMonoItalicFontName  = "JetBrainsMono-Italic[wght].woff2"
)

//go:embed embedded/*.woff2 embedded/OFL.txt
var embeddedFontsFS embed.FS

type embeddedFontFace struct {
	data   []byte
	format string
}

type embeddedFontBundle struct {
	family     string
	regular    embeddedFontFace
	bold       embeddedFontFace
	italic     embeddedFontFace
	boldItalic embeddedFontFace
}

func loadEmbeddedFontBundle(name EmbeddedFont) (embeddedFontBundle, error) {
	switch normalizeEmbeddedFont(name) {
	case EmbeddedFontJetBrainsMono:
		return loadJetBrainsMonoFontBundle()
	case EmbeddedFontHack:
		return loadHackFontBundle()
	default:
		return embeddedFontBundle{}, fmt.Errorf("html render: unsupported embedded font %q", name)
	}
}

func normalizeEmbeddedFont(name EmbeddedFont) EmbeddedFont {
	if name == "" {
		return EmbeddedFontJetBrainsMono
	}
	return name
}

func loadJetBrainsMonoFontBundle() (embeddedFontBundle, error) {
	regular, err := embeddedFontsFS.ReadFile("embedded/" + jetBrainsMonoRegularFontName)
	if err != nil {
		return embeddedFontBundle{}, fmt.Errorf("html render: embedded font %s missing: %w", jetBrainsMonoRegularFontName, err)
	}
	italic, err := embeddedFontsFS.ReadFile("embedded/" + jetBrainsMonoItalicFontName)
	if err != nil {
		return embeddedFontBundle{}, fmt.Errorf("html render: embedded font %s missing: %w", jetBrainsMonoItalicFontName, err)
	}
	regularFace := embeddedFontFace{data: regular, format: "woff2"}
	italicFace := embeddedFontFace{data: italic, format: "woff2"}
	return embeddedFontBundle{
		family:     jetBrainsMonoFontFamily,
		regular:    regularFace,
		bold:       regularFace,
		italic:     italicFace,
		boldItalic: italicFace,
	}, nil
}

func loadHackFontBundle() (embeddedFontBundle, error) {
	regular, bold, italic, boldItalic, err := pdf.EmbeddedHackFonts()
	if err != nil {
		return embeddedFontBundle{}, fmt.Errorf("html render: embedded fonts: %w", err)
	}
	return embeddedFontBundle{
		family: pdf.EmbeddedFontFamily,
		regular: embeddedFontFace{
			data:   regular,
			format: "truetype",
		},
		bold: embeddedFontFace{
			data:   bold,
			format: "truetype",
		},
		italic: embeddedFontFace{
			data:   italic,
			format: "truetype",
		},
		boldItalic: embeddedFontFace{
			data:   boldItalic,
			format: "truetype",
		},
	}, nil
}
