package pdf

import (
	"embed"
	"fmt"
)

// Embedded font metadata for Hack Nerd Font Mono.
const (
	EmbeddedFontFamily         = "HackNerdFontMono"
	EmbeddedRegularFontName    = "HackNerdFontMono-Regular.ttf"
	EmbeddedBoldFontName       = "HackNerdFontMono-Bold.ttf"
	EmbeddedItalicFontName     = "HackNerdFontMono-Italic.ttf"
	EmbeddedBoldItalicFontName = "HackNerdFontMono-BoldItalic.ttf"
)

//go:embed embedded/*.ttf embedded/LICENSE.md
var embeddedFontsFS embed.FS

// EmbeddedHackFont loads an embedded Hack Nerd Font Mono TTF by name.
func EmbeddedHackFont(name string) ([]byte, error) {
	path := "embedded/" + name
	data, err := embeddedFontsFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("embedded font %s missing: %w", name, err)
	}
	return data, nil
}

// EmbeddedHackFonts returns all embedded Hack Nerd Font Mono font bytes.
func EmbeddedHackFonts() (regular, bold, italic, boldItalic []byte, err error) {
	if regular, err = EmbeddedHackFont(EmbeddedRegularFontName); err != nil {
		return nil, nil, nil, nil, err
	}
	if bold, err = EmbeddedHackFont(EmbeddedBoldFontName); err != nil {
		return nil, nil, nil, nil, err
	}
	if italic, err = EmbeddedHackFont(EmbeddedItalicFontName); err != nil {
		return nil, nil, nil, nil, err
	}
	if boldItalic, err = EmbeddedHackFont(EmbeddedBoldItalicFontName); err != nil {
		return nil, nil, nil, nil, err
	}
	return regular, bold, italic, boldItalic, nil
}
