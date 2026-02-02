package mdf

import (
	"sort"
	"strings"

	"pkt.systems/mdf/internal/palette"
)

// Style describes a terminal style as an ANSI prefix sequence.
type Style struct {
	Prefix string
}

// Styles groups the semantic styles used by the renderer.
type Styles struct {
	Text           Style
	Heading        [6]Style
	Emphasis       Style
	Strong         Style
	EmphasisStrong Style
	CodeInline     Style
	CodeBlock      Style
	Quote          Style
	ListMarker     Style
	LinkText       Style
	LinkURL        Style
	ThematicBreak  Style
}

// Theme provides named styles for Markdown rendering.
type Theme interface {
	Name() string
	Styles() Styles
}

type theme struct {
	name   string
	styles Styles
}

func (t theme) Name() string   { return t.name }
func (t theme) Styles() Styles { return t.styles }

// NewTheme returns a Theme from a Styles definition.
func NewTheme(name string, styles Styles) Theme {
	return theme{name: name, styles: styles}
}

func style(prefixes ...string) Style {
	var b strings.Builder
	for _, p := range prefixes {
		if p != "" {
			b.WriteString(p)
		}
	}
	return Style{Prefix: b.String()}
}

func stylesFromPalette(p palette.Palette) Styles {
	return Styles{
		Text:           style(p.Text),
		Heading:        [6]Style{style(p.H1), style(p.H2), style(p.H3), style(p.H4), style(p.H5), style(p.H6)},
		Emphasis:       style(palette.Italic, p.Emphasis),
		Strong:         style(palette.Bold, p.Strong),
		EmphasisStrong: style(palette.Bold, palette.Italic, p.EmphasisStrong),
		CodeInline:     style(p.CodeInline),
		CodeBlock:      style(p.CodeBlock),
		Quote:          style(p.Quote),
		ListMarker:     style(p.ListMarker),
		LinkText:       style(palette.Underline, p.LinkText),
		LinkURL:        style(p.LinkURL),
		ThematicBreak:  style(p.ThematicBreak),
	}
}

var builtinThemes = map[string]Theme{
	"default":             theme{name: "default", styles: stylesFromPalette(palette.PaletteDefault)},
	"outrun-electric":     theme{name: "outrun-electric", styles: stylesFromPalette(palette.PaletteOutrunElectric)},
	"iosvkem":             theme{name: "iosvkem", styles: stylesFromPalette(palette.PaletteDoomIosvkem)},
	"gruvbox":             theme{name: "gruvbox", styles: stylesFromPalette(palette.PaletteDoomGruvbox)},
	"dracula":             theme{name: "dracula", styles: stylesFromPalette(palette.PaletteDoomDracula)},
	"nord":                theme{name: "nord", styles: stylesFromPalette(palette.PaletteDoomNord)},
	"tokyo-night":         theme{name: "tokyo-night", styles: stylesFromPalette(palette.PaletteTokyoNight)},
	"solarized-nightfall": theme{name: "solarized-nightfall", styles: stylesFromPalette(palette.PaletteSolarizedNightfall)},
	"catppuccin-mocha":    theme{name: "catppuccin-mocha", styles: stylesFromPalette(palette.PaletteCatppuccinMocha)},
	"gruvbox-light":       theme{name: "gruvbox-light", styles: stylesFromPalette(palette.PaletteGruvboxLight)},
	"monokai-vibrant":     theme{name: "monokai-vibrant", styles: stylesFromPalette(palette.PaletteMonokaiVibrant)},
	"one-dark-aurora":     theme{name: "one-dark-aurora", styles: stylesFromPalette(palette.PaletteOneDarkAurora)},
	"synthwave-84":        theme{name: "synthwave-84", styles: stylesFromPalette(palette.PaletteSynthwave84)},
	"kanagawa":            theme{name: "kanagawa", styles: stylesFromPalette(palette.PaletteKanagawa)},
	"rose-pine":           theme{name: "rose-pine", styles: stylesFromPalette(palette.PaletteRosePine)},
	"rose-pine-dawn":      theme{name: "rose-pine-dawn", styles: stylesFromPalette(palette.PaletteRosePineDawn)},
	"everforest":          theme{name: "everforest", styles: stylesFromPalette(palette.PaletteEverforest)},
	"everforest-light":    theme{name: "everforest-light", styles: stylesFromPalette(palette.PaletteEverforestLight)},
	"night-owl":           theme{name: "night-owl", styles: stylesFromPalette(palette.PaletteNightOwl)},
	"ayu-mirage":          theme{name: "ayu-mirage", styles: stylesFromPalette(palette.PaletteAyuMirage)},
	"ayu-light":           theme{name: "ayu-light", styles: stylesFromPalette(palette.PaletteAyuLight)},
	"one-light":           theme{name: "one-light", styles: stylesFromPalette(palette.PaletteOneLight)},
	"one-dark":            theme{name: "one-dark", styles: stylesFromPalette(palette.PaletteOneDark)},
	"solarized-light":     theme{name: "solarized-light", styles: stylesFromPalette(palette.PaletteSolarizedLight)},
	"solarized-dark":      theme{name: "solarized-dark", styles: stylesFromPalette(palette.PaletteSolarizedDark)},
	"github-light":        theme{name: "github-light", styles: stylesFromPalette(palette.PaletteGithubLight)},
	"github-dark":         theme{name: "github-dark", styles: stylesFromPalette(palette.PaletteGithubDark)},
	"papercolor-light":    theme{name: "papercolor-light", styles: stylesFromPalette(palette.PalettePapercolorLight)},
	"papercolor-dark":     theme{name: "papercolor-dark", styles: stylesFromPalette(palette.PalettePapercolorDark)},
	"oceanic-next":        theme{name: "oceanic-next", styles: stylesFromPalette(palette.PaletteOceanicNext)},
	"horizon":             theme{name: "horizon", styles: stylesFromPalette(palette.PaletteHorizon)},
	"palenight":           theme{name: "palenight", styles: stylesFromPalette(palette.PalettePalenight)},
}

// AvailableThemes returns the names of built-in themes.
func AvailableThemes() []string {
	names := make([]string, 0, len(builtinThemes))
	for name := range builtinThemes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ThemeByName returns a built-in theme by name.
func ThemeByName(name string) (Theme, bool) {
	if name == "" {
		return builtinThemes["default"], true
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	theme, ok := builtinThemes[normalized]
	return theme, ok
}

// DefaultTheme returns the default built-in theme.
func DefaultTheme() Theme {
	return builtinThemes["default"]
}
