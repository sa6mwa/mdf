package html

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"pkt.systems/mdf"
	"pkt.systems/mdf/pdf"
)

// RenderRequest contains inputs for HTML rendering.
type RenderRequest struct {
	Reader io.Reader
	Writer io.Writer
	Theme  mdf.Theme
	Config Config
}

// Render converts Markdown to a self-contained themed HTML document.
func Render(req RenderRequest) error {
	if req.Reader == nil {
		return fmt.Errorf("html render: reader is nil")
	}
	if req.Writer == nil {
		return fmt.Errorf("html render: writer is nil")
	}
	cfg := DefaultConfig()
	applyConfig(&cfg, req.Config)
	if cfg.FontFamily == "" || cfg.FontSize <= 0 || cfg.LineHeight <= 0 {
		return fmt.Errorf("html render: invalid font configuration")
	}
	if cfg.Boring {
		cfg.IgnoreColors = true
		cfg.BackgroundEnabled = false
		cfg.TextRGB = [3]int{0, 0, 0}
	}
	if err := resolveFonts(&cfg); err != nil {
		return err
	}
	cornerImage, err := resolveCornerImage(cfg)
	if err != nil {
		return err
	}
	theme := req.Theme
	if theme == nil {
		theme = mdf.DefaultTheme()
	}
	stream := newStream(req.Writer, cfg, theme.Styles(), cornerImage)
	if err := stream.writeDocumentStart(); err != nil {
		return fmt.Errorf("html render: %w", err)
	}
	if err := mdf.Parse(mdf.ParseRequest{
		Reader:  req.Reader,
		Stream:  stream,
		Theme:   theme,
		Options: []mdf.RenderOption{mdf.WithOSC8(true)},
	}); err != nil {
		return fmt.Errorf("html render: %w", err)
	}
	return nil
}

func applyConfig(dst *Config, src Config) {
	if src.Margin > 0 {
		dst.Margin = src.Margin
	}
	if src.FontFamily != "" {
		dst.FontFamily = src.FontFamily
	}
	if src.FontSize > 0 {
		dst.FontSize = src.FontSize
	}
	if src.LineHeight > 0 {
		dst.LineHeight = src.LineHeight
	}
	if src.RegularFont != "" {
		dst.RegularFont = src.RegularFont
	}
	if src.BoldFont != "" {
		dst.BoldFont = src.BoldFont
	}
	if src.ItalicFont != "" {
		dst.ItalicFont = src.ItalicFont
	}
	if src.BoldItalicFont != "" {
		dst.BoldItalicFont = src.BoldItalicFont
	}
	if src.HeadingFont != "" {
		dst.HeadingFont = src.HeadingFont
	}
	if len(src.RegularFontBytes) > 0 {
		dst.RegularFontBytes = src.RegularFontBytes
	}
	if len(src.BoldFontBytes) > 0 {
		dst.BoldFontBytes = src.BoldFontBytes
	}
	if len(src.ItalicFontBytes) > 0 {
		dst.ItalicFontBytes = src.ItalicFontBytes
	}
	if len(src.BoldItalicFontBytes) > 0 {
		dst.BoldItalicFontBytes = src.BoldItalicFontBytes
	}
	if len(src.HeadingFontBytes) > 0 {
		dst.HeadingFontBytes = src.HeadingFontBytes
	}
	if src.HeadingScale != [6]float64{} {
		dst.HeadingScale = src.HeadingScale
	}
	if src.IgnoreColors {
		dst.IgnoreColors = true
	}
	if src.Boring {
		dst.Boring = true
	}
	if src.BackgroundRGB != [3]int{} {
		dst.BackgroundRGB = src.BackgroundRGB
	}
	if src.TextRGB != [3]int{} {
		dst.TextRGB = src.TextRGB
	}
	if src.CornerImagePath != "" {
		dst.CornerImagePath = src.CornerImagePath
	}
	if len(src.CornerImageBytes) > 0 {
		dst.CornerImageBytes = src.CornerImageBytes
	}
	if src.CornerImageMIME != "" {
		dst.CornerImageMIME = src.CornerImageMIME
	}
	if src.CornerImageMaxWidth > 0 {
		dst.CornerImageMaxWidth = src.CornerImageMaxWidth
	}
	if src.CornerImageMaxHeight > 0 {
		dst.CornerImageMaxHeight = src.CornerImageMaxHeight
	}
	if src.CornerImagePadding > 0 {
		dst.CornerImagePadding = src.CornerImagePadding
	}
}

func resolveFonts(cfg *Config) error {
	hasPath := cfg.RegularFont != "" || cfg.BoldFont != "" || cfg.ItalicFont != ""
	hasBytes := len(cfg.RegularFontBytes) > 0 || len(cfg.BoldFontBytes) > 0 || len(cfg.ItalicFontBytes) > 0
	if hasPath && hasBytes {
		return fmt.Errorf("html render: cannot mix font paths with embedded font bytes")
	}
	if hasBytes && (len(cfg.RegularFontBytes) == 0 || len(cfg.BoldFontBytes) == 0 || len(cfg.ItalicFontBytes) == 0) {
		return fmt.Errorf("html render: missing embedded font bytes")
	}
	if hasPath && (cfg.RegularFont == "" || cfg.BoldFont == "" || cfg.ItalicFont == "") {
		return fmt.Errorf("html render: missing font paths")
	}
	if hasPath {
		var err error
		if cfg.RegularFontBytes, err = readFont("regular font", cfg.RegularFont); err != nil {
			return err
		}
		if cfg.BoldFontBytes, err = readFont("bold font", cfg.BoldFont); err != nil {
			return err
		}
		if cfg.ItalicFontBytes, err = readFont("italic font", cfg.ItalicFont); err != nil {
			return err
		}
		if cfg.BoldItalicFont != "" {
			if cfg.BoldItalicFontBytes, err = readFont("bold-italic font", cfg.BoldItalicFont); err != nil {
				return err
			}
		}
	}
	if !hasPath && !hasBytes {
		regular, bold, italic, boldItalic, err := pdf.EmbeddedHackFonts()
		if err != nil {
			return fmt.Errorf("html render: embedded fonts: %w", err)
		}
		cfg.FontFamily = pdf.EmbeddedFontFamily
		cfg.RegularFontBytes = regular
		cfg.BoldFontBytes = bold
		cfg.ItalicFontBytes = italic
		cfg.BoldItalicFontBytes = boldItalic
	}
	if cfg.HeadingFont != "" {
		heading, err := readFont("heading font", cfg.HeadingFont)
		if err != nil {
			return err
		}
		cfg.HeadingFontBytes = heading
	}
	return nil
}

func readFont(label string, path string) ([]byte, error) {
	if strings.ToLower(filepath.Ext(path)) != ".ttf" {
		return nil, fmt.Errorf("html render: %s must be a .ttf file", label)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("html render: %s missing: %w", label, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("html render: %s path is a directory", label)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("html render: read %s: %w", label, err)
	}
	return data, nil
}

type embeddedImage struct {
	mime string
	data string
}

func resolveCornerImage(cfg Config) (*embeddedImage, error) {
	if len(cfg.CornerImageBytes) > 0 {
		mime := strings.TrimSpace(cfg.CornerImageMIME)
		if mime == "" {
			return nil, fmt.Errorf("html render: corner image MIME type required with image bytes")
		}
		return &embeddedImage{
			mime: mime,
			data: base64.StdEncoding.EncodeToString(cfg.CornerImageBytes),
		}, nil
	}
	if cfg.CornerImagePath == "" {
		return nil, nil
	}
	mime := imageMIMEForPath(cfg.CornerImagePath)
	if mime == "" {
		return nil, fmt.Errorf("html render: corner image must be PNG or JPEG")
	}
	data, err := os.ReadFile(cfg.CornerImagePath)
	if err != nil {
		return nil, fmt.Errorf("html render: read corner image: %w", err)
	}
	return &embeddedImage{
		mime: mime,
		data: base64.StdEncoding.EncodeToString(data),
	}, nil
}

func imageMIMEForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return ""
	}
}
