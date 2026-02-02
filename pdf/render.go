package pdf

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"pkt.systems/mdf"
	"pkt.systems/mdf/pdf/gofpdf"
)

// RenderRequest contains inputs for PDF rendering.
type RenderRequest struct {
	Reader io.Reader
	Writer io.Writer
	Theme  mdf.Theme
	Config Config
}

// Render converts Markdown to a themed PDF.
func Render(req RenderRequest) error {
	if req.Reader == nil {
		return fmt.Errorf("pdf render: reader is nil")
	}
	if req.Writer == nil {
		return fmt.Errorf("pdf render: writer is nil")
	}
	cfg := DefaultConfig()
	applyConfig(&cfg, req.Config)
	if cfg.FontFamily == "" || cfg.FontSize <= 0 || cfg.LineHeight <= 0 {
		return fmt.Errorf("pdf render: invalid font configuration")
	}
	hasPath := cfg.RegularFont != "" || cfg.BoldFont != "" || cfg.ItalicFont != ""
	hasBytes := len(cfg.RegularFontBytes) > 0 || len(cfg.BoldFontBytes) > 0 || len(cfg.ItalicFontBytes) > 0
	if hasPath && hasBytes {
		return fmt.Errorf("pdf render: cannot mix font paths with embedded font bytes")
	}
	if hasBytes && (len(cfg.RegularFontBytes) == 0 || len(cfg.BoldFontBytes) == 0 || len(cfg.ItalicFontBytes) == 0) {
		return fmt.Errorf("pdf render: missing embedded font bytes")
	}
	if hasPath && (cfg.RegularFont == "" || cfg.BoldFont == "" || cfg.ItalicFont == "") {
		return fmt.Errorf("pdf render: missing font paths")
	}
	useCoreFont := !hasPath && !hasBytes
	if useCoreFont && !isCoreFont(cfg.FontFamily) {
		return fmt.Errorf("pdf render: core font family required when font paths are empty")
	}
	if cfg.CornerImagePath != "" {
		if err := validateImagePath(cfg.CornerImagePath); err != nil {
			return fmt.Errorf("pdf render: %w", err)
		}
	}
	if cfg.HeadingFont != "" {
		if err := ensureHeadingFont(cfg.HeadingFont); err != nil {
			return fmt.Errorf("pdf render: %w", err)
		}
	}
	if cfg.Boring && cfg.UseOCGPrintView {
		return fmt.Errorf("pdf render: life is too short for doubling down on boring, choose either -boring or -ocg-print-view")
	}
	theme := req.Theme
	if theme == nil {
		theme = mdf.DefaultTheme()
	}
	if cfg.Boring {
		cfg.IgnoreColors = true
		cfg.BackgroundEnabled = false
		cfg.TextRGB = [3]int{0, 0, 0}
	}

	pdf := gofpdf.New("P", "pt", cfg.PageSize, "")
	pdf.SetMargins(cfg.Margin, cfg.Margin, cfg.Margin)
	pdf.SetAutoPageBreak(false, cfg.Margin)
	if hasBytes {
		pdf.AddUTF8FontFromBytes(cfg.FontFamily, "", cfg.RegularFontBytes)
		pdf.AddUTF8FontFromBytes(cfg.FontFamily, "B", cfg.BoldFontBytes)
		pdf.AddUTF8FontFromBytes(cfg.FontFamily, "I", cfg.ItalicFontBytes)
		if len(cfg.BoldItalicFontBytes) > 0 {
			pdf.AddUTF8FontFromBytes(cfg.FontFamily, "BI", cfg.BoldItalicFontBytes)
		}
		if cfg.HeadingFont != "" {
			headingBytes, err := os.ReadFile(cfg.HeadingFont)
			if err != nil {
				return fmt.Errorf("pdf render: heading font missing: %w", err)
			}
			pdf.AddUTF8FontFromBytes(headingFontFamily, "", headingBytes)
			pdf.AddUTF8FontFromBytes(headingFontFamily, "B", headingBytes)
		}
	} else if !useCoreFont {
		fontDir := filepath.Dir(cfg.RegularFont)
		if filepath.Dir(cfg.BoldFont) != fontDir || filepath.Dir(cfg.ItalicFont) != fontDir {
			return fmt.Errorf("pdf render: font paths must be in the same directory")
		}
		if cfg.BoldItalicFont != "" && filepath.Dir(cfg.BoldItalicFont) != fontDir {
			return fmt.Errorf("pdf render: bold-italic font must be in the same directory as body fonts")
		}
		if cfg.HeadingFont != "" && filepath.Dir(cfg.HeadingFont) != fontDir {
			return fmt.Errorf("pdf render: heading font must be in the same directory as body fonts")
		}
		pdf.SetFontLocation(fontDir)
		pdf.AddUTF8Font(cfg.FontFamily, "", filepath.Base(cfg.RegularFont))
		pdf.AddUTF8Font(cfg.FontFamily, "B", filepath.Base(cfg.BoldFont))
		pdf.AddUTF8Font(cfg.FontFamily, "I", filepath.Base(cfg.ItalicFont))
		if cfg.BoldItalicFont != "" {
			pdf.AddUTF8Font(cfg.FontFamily, "BI", filepath.Base(cfg.BoldItalicFont))
		}
		if cfg.HeadingFont != "" {
			base := filepath.Base(cfg.HeadingFont)
			pdf.AddUTF8Font(headingFontFamily, "", base)
			pdf.AddUTF8Font(headingFontFamily, "B", base)
		}
	} else if cfg.HeadingFont != "" {
		fontDir := filepath.Dir(cfg.HeadingFont)
		pdf.SetFontLocation(fontDir)
		base := filepath.Base(cfg.HeadingFont)
		pdf.AddUTF8Font(headingFontFamily, "", base)
		pdf.AddUTF8Font(headingFontFamily, "B", base)
	}
	pdf.SetFont(cfg.FontFamily, "", cfg.FontSize)
	pdf.SetTextColor(cfg.TextRGB[0], cfg.TextRGB[1], cfg.TextRGB[2])
	if err := pdf.Error(); err != nil {
		return fmt.Errorf("pdf render: font setup failed: %w", err)
	}

	charWidth := pdf.GetStringWidth("M")
	if math.IsNaN(charWidth) || charWidth <= 0 {
		return fmt.Errorf("pdf render: invalid font metrics (charWidth=%v)", charWidth)
	}
	pageW, _ := pdf.GetPageSize()
	cols := int((pageW - 2*cfg.Margin) / charWidth)
	if cols < 10 {
		return fmt.Errorf("pdf render: page too narrow for content (cols=%d)", cols)
	}

	cornerImage, err := prepareCornerImage(pdf, cfg)
	if err != nil {
		return err
	}
	layers := pdfLayers{}
	if cfg.UseOCGPrintView {
		layers.enabled = true
		layers.viewBg = pdf.AddLayer("view-bg", true)
		layers.viewText = pdf.AddLayer("view-text", true)
		layers.printText = pdf.AddLayer("print-text", false)
		layers.image = pdf.AddLayer("image", true)
		if cfg.OpenLayerPane {
			pdf.OpenLayerPane()
		}
		pdf.SetLayerViewState(layers.viewBg, gofpdf.LayerUsageOn)
		pdf.SetLayerPrintState(layers.viewBg, gofpdf.LayerUsageOff)
		pdf.SetLayerViewState(layers.viewText, gofpdf.LayerUsageOn)
		pdf.SetLayerPrintState(layers.viewText, gofpdf.LayerUsageOff)
		pdf.SetLayerViewState(layers.printText, gofpdf.LayerUsageOff)
		pdf.SetLayerPrintState(layers.printText, gofpdf.LayerUsageOn)
		pdf.SetLayerViewState(layers.image, gofpdf.LayerUsageOn)
		pdf.SetLayerPrintState(layers.image, gofpdf.LayerUsageOn)
	}
	stream := newPDFStream(pdf, cfg, theme.Styles(), cols, charWidth, cornerImage, layers)
	if err := mdf.Parse(mdf.ParseRequest{
		Reader:  req.Reader,
		Stream:  stream,
		Theme:   theme,
		Options: []mdf.RenderOption{mdf.WithOSC8(true)},
	}); err != nil {
		return fmt.Errorf("pdf render: %w", err)
	}
	if err := pdf.Output(req.Writer); err != nil {
		return fmt.Errorf("pdf render: output: %w", err)
	}
	return nil
}

func applyConfig(dst *Config, src Config) {
	if src.PageSize != "" {
		dst.PageSize = src.PageSize
	}
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
	if src.HeadingScale != [6]float64{} {
		dst.HeadingScale = src.HeadingScale
	}
	if src.IgnoreColors {
		dst.IgnoreColors = src.IgnoreColors
	}
	if !src.BackgroundEnabled && dst.BackgroundEnabled {
		dst.BackgroundEnabled = false
	}
	if src.UseOCGPrintView {
		dst.UseOCGPrintView = src.UseOCGPrintView
	}
	if src.OpenLayerPane {
		dst.OpenLayerPane = src.OpenLayerPane
	}
	if src.Boring {
		dst.Boring = src.Boring
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

func isCoreFont(name string) bool {
	switch name {
	case "Courier", "Helvetica", "Times", "Symbol", "ZapfDingbats":
		return true
	default:
		return false
	}
}

func ensureHeadingFont(path string) error {
	if path == "" {
		return nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".ttf" {
		return fmt.Errorf("heading font must be a .ttf file")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("heading font missing: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("heading font path is a directory")
	}
	return nil
}

func validateImagePath(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return fmt.Errorf("corner image must be PNG or JPEG")
	}
	return nil
}

func imageTypeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "PNG"
	case ".jpg", ".jpeg":
		return "JPG"
	default:
		return ""
	}
}

func prepareCornerImage(pdf *gofpdf.Fpdf, cfg Config) (*cornerImage, error) {
	if cfg.CornerImagePath == "" {
		return nil, nil
	}
	imageType := imageTypeForPath(cfg.CornerImagePath)
	if imageType == "" {
		return nil, fmt.Errorf("pdf render: corner image must be PNG or JPEG")
	}
	opts := gofpdf.ImageOptions{
		ImageType: imageType,
		ReadDpi:   true,
	}
	info := pdf.RegisterImageOptions(cfg.CornerImagePath, opts)
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("pdf render: load corner image: %w", err)
	}
	width, height := info.Extent()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("pdf render: invalid corner image dimensions")
	}
	maxW, maxH := cfg.CornerImageMaxWidth, cfg.CornerImageMaxHeight
	if maxW > 0 || maxH > 0 {
		scale := 1.0
		if maxW > 0 {
			scale = math.Min(scale, maxW/width)
		}
		if maxH > 0 {
			scale = math.Min(scale, maxH/height)
		}
		if scale <= 0 {
			return nil, fmt.Errorf("pdf render: invalid corner image scale")
		}
		width *= scale
		height *= scale
	}
	return &cornerImage{
		path:   cfg.CornerImagePath,
		opts:   opts,
		width:  width,
		height: height,
	}, nil
}
