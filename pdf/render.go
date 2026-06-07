package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"pkt.systems/mdf"
	"pkt.systems/mdf/pdf/gofpdf"
)

const (
	annotationFlagPrint  = 1 << 2
	annotationFlagNoView = 1 << 5
)

// RenderRequest contains inputs for PDF rendering.
type RenderRequest struct {
	// Reader supplies Markdown input. Render reads it incrementally.
	Reader io.Reader
	// Writer receives the generated PDF bytes.
	Writer io.Writer
	// Theme controls semantic styles. If nil, mdf.DefaultTheme is used.
	Theme mdf.Theme
	// Config controls page layout, fonts, colors, images, and tables.
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
	if cfg.UseOCGPrintView {
		return renderPrintViewAnnotations(req.Reader, req.Writer, theme, cfg, hasBytes, useCoreFont)
	}
	cfg = normalizedRenderConfig(cfg)
	pdf, stream, err := buildPDFRenderer(theme, cfg, hasBytes, useCoreFont, rendererOptions{
		drawCornerImage: true,
	})
	if err != nil {
		return err
	}
	if err := parseIntoStream(req.Reader, stream, theme); err != nil {
		return err
	}
	if err := pdf.Output(req.Writer); err != nil {
		return fmt.Errorf("pdf render: output: %w", err)
	}
	return nil
}

// renderPrintViewAnnotations emits a single PDF that shows themed content on
// screen and boring content in print/export pipelines.
func renderPrintViewAnnotations(reader io.Reader, writer io.Writer, theme mdf.Theme, cfg Config, hasBytes bool, useCoreFont bool) error {
	if !cfg.BackgroundEnabled {
		return renderPrintOnlySplit(reader, writer, theme, cfg, hasBytes, useCoreFont)
	}
	return renderScreenOnlySplit(reader, writer, theme, cfg, hasBytes, useCoreFont)
}

func renderScreenOnlySplit(reader io.Reader, writer io.Writer, theme mdf.Theme, cfg Config, hasBytes bool, useCoreFont bool) error {
	printCfg := normalizedRenderConfig(cfg)
	printCfg.UseOCGPrintView = false
	printCfg.OpenLayerPane = false
	printCfg.Boring = true
	printCfg = normalizedRenderConfig(printCfg)
	printCfg.BackgroundEnabled = true
	printCfg.BackgroundRGB = [3]int{255, 255, 255}

	viewCfg := normalizedRenderConfig(cfg)
	viewCfg.UseOCGPrintView = false
	viewCfg.OpenLayerPane = false
	viewCfg.BackgroundEnabled = false

	printPDF, printStream, err := buildPDFRenderer(theme, printCfg, hasBytes, useCoreFont, rendererOptions{
		drawCornerImage: true,
	})
	if err != nil {
		return err
	}
	viewPDF, viewStream, err := buildPDFRenderer(theme, viewCfg, hasBytes, useCoreFont, rendererOptions{
		drawCornerImage:              true,
		flattenCornerImageOnBackdrop: true,
		cornerImageBackdropRGB:       viewCfg.BackgroundRGB,
	})
	if err != nil {
		return err
	}
	if viewStream.cornerImage != nil && printStream.cornerImage != nil && viewStream.cornerImage.name != printStream.cornerImage.name {
		if _, err := registerFlattenedCornerImage(printPDF, viewStream.cornerImage.name, cfg.CornerImagePath, viewCfg.BackgroundRGB); err != nil {
			return err
		}
		if err := printPDF.Error(); err != nil {
			return fmt.Errorf("pdf render: load corner image: %w", err)
		}
	}
	if err := parseIntoStream(reader, &fanoutStream{streams: []mdf.Stream{printStream, viewStream}}, theme); err != nil {
		return err
	}
	if printPDF.PageCount() != viewPDF.PageCount() {
		return fmt.Errorf("pdf render: print/view page count mismatch (%d != %d)", printPDF.PageCount(), viewPDF.PageCount())
	}
	for page := 1; page <= printPDF.PageCount(); page++ {
		content := viewPDF.PageContentBytes(page)
		if len(content) == 0 {
			continue
		}
		pageW, pageH, _ := printPDF.PageSize(page)
		appearance := wrapAppearanceContent(pageW, pageH, viewCfg.BackgroundRGB, content)
		printPDF.AddAppearanceAnnotation(
			page,
			"Square",
			0,
			0,
			0,
			pageW,
			pageH,
			appearance,
		)
	}
	if err := printPDF.Output(writer); err != nil {
		return fmt.Errorf("pdf render: output: %w", err)
	}
	return nil
}

func renderPrintOnlySplit(reader io.Reader, writer io.Writer, theme mdf.Theme, cfg Config, hasBytes bool, useCoreFont bool) error {
	viewCfg := normalizedRenderConfig(cfg)
	viewCfg.UseOCGPrintView = false
	viewCfg.OpenLayerPane = false

	printCfg := normalizedRenderConfig(cfg)
	printCfg.UseOCGPrintView = false
	printCfg.OpenLayerPane = false
	printCfg.Boring = true
	printCfg = normalizedRenderConfig(printCfg)
	printCfg.BackgroundEnabled = true
	printCfg.BackgroundRGB = [3]int{255, 255, 255}

	viewPDF, viewStream, err := buildPDFRenderer(theme, viewCfg, hasBytes, useCoreFont, rendererOptions{
		drawCornerImage: false,
	})
	if err != nil {
		return err
	}
	var viewCornerPDF *gofpdf.Fpdf
	if cfg.CornerImagePath != "" {
		viewCornerPDF, _, err = buildPDFRenderer(theme, viewCfg, hasBytes, useCoreFont, rendererOptions{
			drawCornerImage: true,
		})
		if err != nil {
			return err
		}
	}
	printPDF, printStream, err := buildPDFRenderer(theme, printCfg, hasBytes, useCoreFont, rendererOptions{
		drawCornerImage: true,
	})
	if err != nil {
		return err
	}
	if err := parseIntoStream(reader, &fanoutStream{streams: []mdf.Stream{viewStream, printStream}}, theme); err != nil {
		return err
	}
	if viewPDF.PageCount() != printPDF.PageCount() {
		return fmt.Errorf("pdf render: print/view page count mismatch (%d != %d)", printPDF.PageCount(), viewPDF.PageCount())
	}
	if viewCornerPDF != nil {
		content := viewCornerPDF.PageContentBytes(1)
		if len(content) > 0 {
			pageW, pageH, _ := viewPDF.PageSize(1)
			viewPDF.AddAppearanceAnnotation(
				1,
				"Square",
				0,
				0,
				0,
				pageW,
				pageH,
				content,
			)
		}
	}
	for page := 1; page <= printPDF.PageCount(); page++ {
		content := printPDF.PageContentBytes(page)
		if len(content) == 0 {
			continue
		}
		pageW, pageH, _ := viewPDF.PageSize(page)
		appearance := wrapAppearanceContent(pageW, pageH, printCfg.BackgroundRGB, content)
		viewPDF.AddAppearanceAnnotation(
			page,
			"Square",
			annotationFlagPrint|annotationFlagNoView,
			0,
			0,
			pageW,
			pageH,
			appearance,
		)
	}
	if err := viewPDF.Output(writer); err != nil {
		return fmt.Errorf("pdf render: output: %w", err)
	}
	return nil
}

// wrapAppearanceContent paints a white page-sized backdrop before replaying a
// themed page content stream inside an annotation appearance.
func wrapAppearanceContent(pageW, pageH float64, bgRGB [3]int, content []byte) []byte {
	if len(content) == 0 {
		return nil
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "q %.3f %.3f %.3f rg 0 0 %.2f %.2f re f Q\n", float64(bgRGB[0])/255.0, float64(bgRGB[1])/255.0, float64(bgRGB[2])/255.0, pageW, pageH)
	out.WriteString("q\n")
	out.Write(content)
	if content[len(content)-1] != '\n' {
		out.WriteByte('\n')
	}
	out.WriteString("Q\n")
	return out.Bytes()
}

type rendererOptions struct {
	drawCornerImage              bool
	flattenCornerImageOnBackdrop bool
	cornerImageBackdropRGB       [3]int
}

// normalizedRenderConfig applies the implicit boring-mode PDF defaults.
func normalizedRenderConfig(cfg Config) Config {
	if cfg.Boring {
		cfg.IgnoreColors = true
		cfg.BackgroundEnabled = false
		cfg.TextRGB = [3]int{0, 0, 0}
	}
	return cfg
}

// buildPDFRenderer constructs a configured PDF document and matching token stream.
func buildPDFRenderer(theme mdf.Theme, cfg Config, hasBytes bool, useCoreFont bool, opts rendererOptions) (*gofpdf.Fpdf, *pdfStream, error) {
	pdf := gofpdf.New("P", "pt", cfg.PageSize, "")
	pdf.SetMargins(cfg.Margin, cfg.Margin, cfg.Margin)
	pdf.SetAutoPageBreak(false, cfg.Margin)
	if err := configureFonts(pdf, cfg, hasBytes, useCoreFont); err != nil {
		return nil, nil, err
	}
	pdf.SetFont(cfg.FontFamily, "", cfg.FontSize)
	pdf.SetTextColor(cfg.TextRGB[0], cfg.TextRGB[1], cfg.TextRGB[2])
	if err := pdf.Error(); err != nil {
		return nil, nil, fmt.Errorf("pdf render: font setup failed: %w", err)
	}

	charWidth := pdf.GetStringWidth("M")
	if math.IsNaN(charWidth) || charWidth <= 0 {
		return nil, nil, fmt.Errorf("pdf render: invalid font metrics (charWidth=%v)", charWidth)
	}
	pageW, _ := pdf.GetPageSize()
	cols := int((pageW - 2*cfg.Margin) / charWidth)
	if cols < 10 {
		return nil, nil, fmt.Errorf("pdf render: page too narrow for content (cols=%d)", cols)
	}
	cornerImage, err := prepareCornerImage(pdf, cfg, opts)
	if err != nil {
		return nil, nil, err
	}
	if cornerImage != nil {
		cornerImage.draw = opts.drawCornerImage
	}
	stream := newPDFStream(pdf, cfg, theme.Styles(), cols, charWidth, cornerImage, pdfLayers{})
	return pdf, stream, nil
}

// configureFonts loads either embedded fonts, file-backed fonts, or core fonts.
func configureFonts(pdf *gofpdf.Fpdf, cfg Config, hasBytes bool, useCoreFont bool) error {
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
		return nil
	}
	if !useCoreFont {
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
		return nil
	}
	if cfg.HeadingFont != "" {
		fontDir := filepath.Dir(cfg.HeadingFont)
		pdf.SetFontLocation(fontDir)
		base := filepath.Base(cfg.HeadingFont)
		pdf.AddUTF8Font(headingFontFamily, "", base)
		pdf.AddUTF8Font(headingFontFamily, "B", base)
	}
	return nil
}

// parseIntoStream drives the streaming Markdown parser into the provided stream.
func parseIntoStream(reader io.Reader, stream mdf.Stream, theme mdf.Theme) error {
	if err := mdf.Parse(mdf.ParseRequest{
		Reader:  reader,
		Stream:  stream,
		Theme:   theme,
		Options: []mdf.RenderOption{mdf.WithOSC8(true)},
	}); err != nil {
		return fmt.Errorf("pdf render: %w", err)
	}
	return nil
}

// fanoutStream mirrors parser tokens into multiple output streams.
type fanoutStream struct {
	streams []mdf.Stream
}

func (f *fanoutStream) WriteToken(tok mdf.StreamToken) error {
	for _, stream := range f.streams {
		if err := stream.WriteToken(tok); err != nil {
			return err
		}
	}
	return nil
}

func (f *fanoutStream) StartTable(table mdf.TableStart) error {
	for _, stream := range f.streams {
		tableStream, ok := stream.(mdf.TableStream)
		if !ok {
			continue
		}
		if err := tableStream.StartTable(table); err != nil {
			return err
		}
	}
	return nil
}

func (f *fanoutStream) WriteTableRow(row mdf.TableRow) error {
	for _, stream := range f.streams {
		tableStream, ok := stream.(mdf.TableStream)
		if !ok {
			continue
		}
		if err := tableStream.WriteTableRow(row); err != nil {
			return err
		}
	}
	return nil
}

func (f *fanoutStream) EndTable() error {
	for _, stream := range f.streams {
		tableStream, ok := stream.(mdf.TableStream)
		if !ok {
			continue
		}
		if err := tableStream.EndTable(); err != nil {
			return err
		}
	}
	return nil
}

func (f *fanoutStream) Flush() error {
	for _, stream := range f.streams {
		if err := stream.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func (f *fanoutStream) Width() int {
	if len(f.streams) == 0 {
		return 0
	}
	return f.streams[0].Width()
}

func (f *fanoutStream) SetWidth(width int) {
	for _, stream := range f.streams {
		stream.SetWidth(width)
	}
}

func (f *fanoutStream) SetWrapIndent(indent string) {
	for _, stream := range f.streams {
		stream.SetWrapIndent(indent)
	}
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
	switch src.TableBufferMode {
	case mdf.TableBufferFull, mdf.TableBufferRow:
		dst.TableBufferMode = src.TableBufferMode
	}
	switch src.TableWireMode {
	case mdf.TableWireLine, mdf.TableWireASCII, mdf.TableWireSpace:
		dst.TableWireMode = src.TableWireMode
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

func prepareCornerImage(pdf *gofpdf.Fpdf, cfg Config, renderOpts rendererOptions) (*cornerImage, error) {
	if cfg.CornerImagePath == "" {
		return nil, nil
	}
	imageType := imageTypeForPath(cfg.CornerImagePath)
	if imageType == "" {
		return nil, fmt.Errorf("pdf render: corner image must be PNG or JPEG")
	}
	imageOpts := gofpdf.ImageOptions{
		ImageType: imageType,
		ReadDpi:   true,
	}
	imageName := cfg.CornerImagePath
	info := pdf.RegisterImageOptions(cfg.CornerImagePath, imageOpts)
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("pdf render: load corner image: %w", err)
	}
	width, height := info.Extent()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("pdf render: invalid corner image dimensions")
	}
	var err error
	registeredOpts := imageOpts
	if renderOpts.flattenCornerImageOnBackdrop && imageType == "PNG" {
		imageName = cfg.CornerImagePath + "#screen"
		_, err = registerFlattenedCornerImage(pdf, imageName, cfg.CornerImagePath, renderOpts.cornerImageBackdropRGB)
		if err != nil {
			return nil, err
		}
		registeredOpts = gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}
		if err := pdf.Error(); err != nil {
			return nil, fmt.Errorf("pdf render: load corner image: %w", err)
		}
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
		name:   imageName,
		opts:   registeredOpts,
		width:  width,
		height: height,
		draw:   true,
	}, nil
}

func registerFlattenedCornerImage(pdf *gofpdf.Fpdf, name, path string, bgRGB [3]int) (*gofpdf.ImageInfoType, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("pdf render: load corner image: %w", err)
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("pdf render: decode corner image: %w", err)
	}
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, &image.Uniform{C: color.RGBA{R: uint8(bgRGB[0]), G: uint8(bgRGB[1]), B: uint8(bgRGB[2]), A: 255}}, image.Point{}, draw.Src)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Over)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("pdf render: encode corner image: %w", err)
	}
	return pdf.RegisterImageOptionsReader(name, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, &buf), nil
}
