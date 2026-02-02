package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"golang.org/x/term"
	"pkt.systems/mdf"
	"pkt.systems/mdf/pdf"
	"pkt.systems/version"
)

const (
	defaultThemeName = "default"
	defaultWidth     = 80
	defaultChunkSize = 3
	defaultDelay     = 20 * time.Millisecond
)

func init() {
	version.SetDefaultModule("pkt.systems/mdf")
}

func main() {
	var (
		simulate          bool
		simChunkSize      int
		simDelay          time.Duration
		themeName         string
		widthFlag         int
		osc8Flag          string
		listThemes        bool
		outPath           string
		boring            bool
		pdfMode           bool
		pdfPageSize       string
		pdfMargin         float64
		pdfLineHeight     float64
		pdfFontSize       float64
		pdfH1Scale        float64
		pdfH2Scale        float64
		pdfH3Scale        float64
		pdfOCGPrintView   bool
		pdfRegularFont    string
		pdfBoldFont       string
		pdfItalicFont     string
		pdfBoldItalicFont string
		pdfHeadingFont    string
		pdfCornerImage    string
		pdfCornerMaxW     float64
		pdfCornerMaxH     float64
		pdfCornerPadding  float64
	)

	pdfDefaults := pdf.DefaultConfig()
	flags := pflag.NewFlagSet("mdf", pflag.ExitOnError)
	flags.BoolVar(&simulate, "simulate", false, "Stream simulator (use default delay and chunk size)")
	flags.IntVar(&simChunkSize, "simulate-chunk", defaultChunkSize, "Max bytes per stream chunk")
	flags.DurationVar(&simDelay, "simulate-delay", defaultDelay, "Delay per stream chunk")
	flags.StringVarP(&themeName, "theme", "t", defaultThemeName, "Theme name")
	flags.IntVarP(&widthFlag, "width", "w", 0, "Output width override (0 uses terminal width if available)")
	flags.StringVarP(&osc8Flag, "osc8", "8", "auto", "OSC8 hyperlinks: auto|on|off")
	flags.BoolVar(&listThemes, "list-themes", false, "List available themes")
	flags.StringVarP(&outPath, "output", "o", "", "Output file instead of stdout")
	flags.BoolVarP(&boring, "boring", "b", false, "Generate non-ANSI output or boring PDF")
	flags.BoolVar(&pdfMode, "pdf", false, "Generate a PDF instead of ANSI output")
	flags.StringVar(&pdfBoldFont, "pdf-bold-font", "", "TTF path for bold font")
	flags.StringVar(&pdfItalicFont, "pdf-italic-font", "", "TTF path for italic font")
	flags.StringVar(&pdfRegularFont, "pdf-regular-font", "", "TTF path for regular font")
	flags.StringVar(&pdfBoldItalicFont, "pdf-bold-italic-font", "", "TTF path for bold-italic font")
	flags.StringVar(&pdfHeadingFont, "pdf-heading-font", "", "TTF path for heading font (overrides body font)")
	flags.BoolVar(&pdfOCGPrintView, "pdf-ocg-print-view", false, "Enable OCG view/print layers (themed view, boring print)")
	flags.StringVar(&pdfPageSize, "pdf-page-size", pdfDefaults.PageSize, "PDF page size")
	flags.Float64Var(&pdfMargin, "pdf-margin", pdfDefaults.Margin, "Page margin in points")
	flags.Float64Var(&pdfLineHeight, "pdf-line-height", pdfDefaults.LineHeight, "Line height multiplier")
	flags.Float64Var(&pdfFontSize, "pdf-font-size", pdfDefaults.FontSize, "Base font size in points")
	flags.Float64Var(&pdfH1Scale, "pdf-h1-scale", pdfDefaults.HeadingScale[0], "Scale factor for H1 headings")
	flags.Float64Var(&pdfH2Scale, "pdf-h2-scale", pdfDefaults.HeadingScale[1], "Scale factor for H2 headings")
	flags.Float64Var(&pdfH3Scale, "pdf-h3-scale", pdfDefaults.HeadingScale[2], "Scale factor for H3 headings")
	flags.StringVar(&pdfCornerImage, "corner-image", "", "Corner image path (PNG or JPEG)")
	flags.Float64Var(&pdfCornerMaxW, "corner-image-max-width", pdfDefaults.CornerImageMaxWidth, "Corner image max width in points")
	flags.Float64Var(&pdfCornerMaxH, "corner-image-max-height", pdfDefaults.CornerImageMaxHeight, "Corner image max height in points")
	flags.Float64Var(&pdfCornerPadding, "corner-image-padding", pdfDefaults.CornerImagePadding, "Corner image padding in points")

	flags.SetInterspersed(true)
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, version.Module(), version.Current())
		fmt.Fprintf(os.Stderr, "Usage: mdf [flags] [inputs...]\n")
		fmt.Fprintln(os.Stderr, "\nIf no input is provided, Markdown is read from stdin.")
		fmt.Fprintln(os.Stderr, "\nFlags:")
		flags.PrintDefaults()
	}

	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	if listThemes {
		printThemes()
		return
	}

	args := flags.Args()
	reader, closer, err := openInputs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open input: %v\n", err)
		os.Exit(1)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	if simulate {
		reader = &slowReader{r: reader, delay: simDelay, maxChunk: simChunkSize}
	}

	if !pdfMode && outPath != "" && strings.HasSuffix(strings.ToLower(outPath), ".pdf") {
		fmt.Fprintf(os.Stderr, "warning: output %q ends with .pdf; enabling --pdf\n", outPath)
		pdfMode = true
	}

	writer, closeOut, err := resolveOutput(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open output: %v\n", err)
		os.Exit(1)
	}
	if closeOut != nil {
		defer func() { _ = closeOut.Close() }()
	}

	theme, ok := mdf.ThemeByName(themeName)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown theme %q\n\n", themeName)
		printThemes()
		os.Exit(2)
	}

	if pdfMode {
		if isTerminal(writer) {
			fmt.Fprintln(os.Stderr, "refusing to write PDF to terminal; use -o/--output")
			os.Exit(2)
		}
		if err := renderPDF(reader, writer, theme, boring, pdfConfig{
			pageSize:       pdfPageSize,
			margin:         pdfMargin,
			lineHeight:     pdfLineHeight,
			fontSize:       pdfFontSize,
			h1Scale:        pdfH1Scale,
			h2Scale:        pdfH2Scale,
			h3Scale:        pdfH3Scale,
			ocgPrintView:   pdfOCGPrintView,
			regularFont:    pdfRegularFont,
			boldFont:       pdfBoldFont,
			italicFont:     pdfItalicFont,
			boldItalicFont: pdfBoldItalicFont,
			headingFont:    pdfHeadingFont,
			cornerImage:    pdfCornerImage,
			cornerMaxW:     pdfCornerMaxW,
			cornerMaxH:     pdfCornerMaxH,
			cornerPadding:  pdfCornerPadding,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "render pdf: %v\n", err)
			os.Exit(1)
		}
		return
	}

	width := resolveWidth(widthFlag)
	osc8, err := resolveOSC8(osc8Flag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --osc8 %q: %v\n", osc8Flag, err)
		os.Exit(2)
	}
	if boring {
		theme = boringTheme()
	}
	if err := mdf.Render(mdf.RenderRequest{
		Reader:  reader,
		Writer:  writer,
		Width:   width,
		Theme:   theme,
		Options: []mdf.RenderOption{mdf.WithOSC8(osc8)},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "render: %v\n", err)
		os.Exit(1)
	}
}

type pdfConfig struct {
	pageSize       string
	margin         float64
	lineHeight     float64
	fontSize       float64
	h1Scale        float64
	h2Scale        float64
	h3Scale        float64
	ocgPrintView   bool
	regularFont    string
	boldFont       string
	italicFont     string
	boldItalicFont string
	headingFont    string
	cornerImage    string
	cornerMaxW     float64
	cornerMaxH     float64
	cornerPadding  float64
}

func renderPDF(r io.Reader, w io.Writer, theme mdf.Theme, boring bool, cfgIn pdfConfig) error {
	cfg := pdf.DefaultConfig()
	cfg.PageSize = defaultIf(cfgIn.pageSize, cfg.PageSize)
	if cfgIn.margin > 0 {
		cfg.Margin = cfgIn.margin
	}
	if cfgIn.lineHeight > 0 {
		cfg.LineHeight = cfgIn.lineHeight
	}
	if cfgIn.fontSize > 0 {
		cfg.FontSize = cfgIn.fontSize
	}
	if cfgIn.h1Scale > 0 {
		cfg.HeadingScale[0] = cfgIn.h1Scale
	}
	if cfgIn.h2Scale > 0 {
		cfg.HeadingScale[1] = cfgIn.h2Scale
	}
	if cfgIn.h3Scale > 0 {
		cfg.HeadingScale[2] = cfgIn.h3Scale
	}
	cfg.UseOCGPrintView = cfgIn.ocgPrintView
	if cfgIn.cornerImage != "" {
		cfg.CornerImagePath = cfgIn.cornerImage
	}
	if cfgIn.cornerMaxW > 0 {
		cfg.CornerImageMaxWidth = cfgIn.cornerMaxW
	}
	if cfgIn.cornerMaxH > 0 {
		cfg.CornerImageMaxHeight = cfgIn.cornerMaxH
	}
	if cfgIn.cornerPadding > 0 {
		cfg.CornerImagePadding = cfgIn.cornerPadding
	}
	cfg.Boring = boring

	reg, bold, italic := strings.TrimSpace(cfgIn.regularFont), strings.TrimSpace(cfgIn.boldFont), strings.TrimSpace(cfgIn.italicFont)
	if reg != "" || bold != "" || italic != "" {
		if reg == "" || bold == "" || italic == "" {
			return fmt.Errorf("pdf fonts: regular, bold, and italic fonts must all be provided")
		}
		reg = normalizePath(reg)
		bold = normalizePath(bold)
		italic = normalizePath(italic)
		if err := ensureFont(reg); err != nil {
			return fmt.Errorf("regular font: %w", err)
		}
		if err := ensureFont(bold); err != nil {
			return fmt.Errorf("bold font: %w", err)
		}
		if err := ensureFont(italic); err != nil {
			return fmt.Errorf("italic font: %w", err)
		}
		cfg.FontFamily = "mdf"
		cfg.RegularFont = reg
		cfg.BoldFont = bold
		cfg.ItalicFont = italic
		if cfgIn.boldItalicFont != "" {
			boldItalic := normalizePath(cfgIn.boldItalicFont)
			if err := ensureFont(boldItalic); err != nil {
				return fmt.Errorf("bold-italic font: %w", err)
			}
			cfg.BoldItalicFont = boldItalic
		}
	} else {
		regBytes, boldBytes, italicBytes, boldItalicBytes, err := pdf.EmbeddedHackFonts()
		if err != nil {
			return fmt.Errorf("embedded fonts: %w", err)
		}
		cfg.FontFamily = pdf.EmbeddedFontFamily
		cfg.RegularFontBytes = regBytes
		cfg.BoldFontBytes = boldBytes
		cfg.ItalicFontBytes = italicBytes
		cfg.BoldItalicFontBytes = boldItalicBytes
	}
	if cfgIn.headingFont != "" {
		heading := normalizePath(cfgIn.headingFont)
		if err := ensureFont(heading); err != nil {
			return fmt.Errorf("heading font: %w", err)
		}
		cfg.HeadingFont = heading
	}

	return pdf.Render(pdf.RenderRequest{
		Reader: r,
		Writer: w,
		Theme:  theme,
		Config: cfg,
	})
}

func defaultIf(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func printThemes() {
	names := mdf.AvailableThemes()
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintln(os.Stdout, name)
	}
}

func resolveWidth(width int) int {
	if width > 0 {
		return width
	}
	return terminalWidth(defaultWidth)
}

func terminalWidth(fallback int) int {
	fd := int(os.Stdout.Fd())
	if term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil && w > 0 {
			return w
		}
	}
	if value := os.Getenv("COLUMNS"); value != "" {
		if w, err := strconvAtoi(value); err == nil && w > 0 {
			return w
		}
	}
	return fallback
}

func resolveOSC8(mode string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		return mdf.DetectOSC8Support(), nil
	case "on", "true", "1", "yes":
		return true, nil
	case "off", "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("expected auto|on|off")
	}
}

func boringTheme() mdf.Theme {
	return mdf.NewTheme("boring", mdf.Styles{})
}

type inputSource struct {
	open func() (io.Reader, io.Closer, error)
}

type multiInputReader struct {
	sources   []inputSource
	idx       int
	cur       io.Reader
	curCloser io.Closer
	closed    bool
}

func (m *multiInputReader) Read(p []byte) (int, error) {
	for {
		if m.closed {
			return 0, io.EOF
		}
		if m.cur == nil {
			if m.idx >= len(m.sources) {
				m.closed = true
				return 0, io.EOF
			}
			reader, closer, err := m.sources[m.idx].open()
			if err != nil {
				return 0, err
			}
			m.cur = reader
			m.curCloser = closer
			m.idx++
		}
		n, err := m.cur.Read(p)
		if n > 0 {
			return n, nil
		}
		if err == io.EOF {
			if m.curCloser != nil {
				_ = m.curCloser.Close()
			}
			m.cur = nil
			m.curCloser = nil
			continue
		}
		if err != nil {
			return 0, err
		}
	}
}

func (m *multiInputReader) Close() error {
	m.closed = true
	if m.curCloser != nil {
		return m.curCloser.Close()
	}
	return nil
}

func openInputs(args []string) (io.Reader, io.Closer, error) {
	if len(args) == 0 {
		return os.Stdin, nil, nil
	}
	sources := make([]inputSource, 0, len(args))
	for _, raw := range args {
		src, err := makeInputSource(raw)
		if err != nil {
			return nil, nil, err
		}
		sources = append(sources, src)
	}
	return &multiInputReader{sources: sources}, nil, nil
}

func makeInputSource(raw string) (inputSource, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return inputSource{}, fmt.Errorf("empty input argument")
	}
	u, err := url.Parse(raw)
	if err == nil && u.Scheme != "" {
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			return inputSource{open: func() (io.Reader, io.Closer, error) {
				return openURL(raw)
			}}, nil
		case "file":
			path := u.Path
			if path == "" {
				path = u.Host
			}
			if unescaped, err := url.PathUnescape(path); err == nil {
				path = unescaped
			}
			return inputSource{open: func() (io.Reader, io.Closer, error) {
				return openFile(path)
			}}, nil
		}
	}
	return inputSource{open: func() (io.Reader, io.Closer, error) {
		return openFile(raw)
	}}, nil
}

func openURL(raw string) (io.Reader, io.Closer, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, raw, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("http %s: %s", raw, resp.Status)
	}
	return resp.Body, resp.Body, nil
}

func openFile(path string) (io.Reader, io.Closer, error) {
	clean := normalizePath(path)
	f, err := os.Open(clean)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}

func resolveOutput(path string) (io.Writer, io.Closer, error) {
	if strings.TrimSpace(path) == "" {
		return os.Stdout, nil, nil
	}
	clean := normalizePath(path)
	dir := filepath.Dir(clean)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, err
		}
	}
	f, err := os.Create(clean)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}

func normalizePath(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				path = home
			} else {
				path = filepath.Join(home, path[2:])
			}
		}
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		return abs
	}
	return path
}

func ensureFont(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	if !strings.HasSuffix(strings.ToLower(info.Name()), ".ttf") {
		return fmt.Errorf("expected .ttf font file")
	}
	return nil
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func strconvAtoi(value string) (int, error) {
	var n int
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, fmt.Errorf("invalid int")
		}
		n = n*10 + int(value[i]-'0')
	}
	return n, nil
}

type slowReader struct {
	r        io.Reader
	delay    time.Duration
	maxChunk int
}

func (s *slowReader) Read(p []byte) (int, error) {
	if s.maxChunk > 0 && len(p) > s.maxChunk {
		p = p[:s.maxChunk]
	}
	n, err := s.r.Read(p)
	if n > 0 && s.delay > 0 {
		time.Sleep(s.delay)
	}
	return n, err
}
