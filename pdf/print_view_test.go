package pdf_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"pkt.systems/mdf"
	"pkt.systems/mdf/pdf"
	"pkt.systems/mdf/pdf/internal/pdfgolden"
)

func TestPrintViewHeadingInPrintedOutput(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not found in PATH")
	}
	if _, err := exec.LookPath("pdftocairo"); err != nil {
		t.Skip("pdftocairo not found in PATH")
	}
	root, err := pdfgolden.FindTestdataRoot()
	if err != nil {
		t.Fatalf("find testdata root: %v", err)
	}

	cfg := pdf.DefaultConfig()
	cfg.UseOCGPrintView = true
	cfg.CornerImagePath = filepath.Join(root, "ocg_corner.png")
	reg, bold, italic, boldItalic, err := pdf.EmbeddedHackFonts()
	if err != nil {
		t.Fatalf("embedded fonts: %v", err)
	}
	cfg.FontFamily = pdf.EmbeddedFontFamily
	cfg.RegularFontBytes = reg
	cfg.BoldFontBytes = bold
	cfg.ItalicFontBytes = italic
	cfg.BoldItalicFontBytes = boldItalic

	var out bytes.Buffer
	markdown := "# Heading\n\nBody text.\n"
	src := strings.NewReader(markdown)
	err = pdf.Render(pdf.RenderRequest{
		Reader: src,
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("render pdf: %v", err)
	}

	goldenDir := filepath.Join(root, "golden")
	viewPNG, err := renderPrintViewPNG(out.Bytes())
	if err != nil {
		t.Fatalf("render view png: %v", err)
	}
	printPNG, err := flattenAndRenderPrintViewPNG(out.Bytes())
	if err != nil {
		t.Fatalf("render print png: %v", err)
	}
	boringPNG, err := renderBoringPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render boring png: %v", err)
	}
	if err := comparePNGTolerant(viewPNG, filepath.Join(goldenDir, "ocg_print_view_view_p1.png"), 0.002); err != nil {
		t.Fatalf("view golden mismatch: %v", err)
	}
	if err := comparePNGTolerant(printPNG, boringPNG, 0.003); err != nil {
		t.Fatalf("flattened print mismatch: %v", err)
	}
}

func TestPrintViewTransparentCornerImageMatchesThemedView(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not found in PATH")
	}
	tmpDir, err := os.MkdirTemp("", "mdf-print-view-corner-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cornerPath := filepath.Join(tmpDir, "corner.png")
	if err := writeTransparentCornerPNG(cornerPath); err != nil {
		t.Fatalf("write corner image: %v", err)
	}

	cfg := pdf.DefaultConfig()
	cfg.PageSize = "A4"
	cfg.Margin = 36
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	cfg.LineHeight = 1.4
	cfg.CornerImagePath = cornerPath
	markdown := "# Heading\n\nBody text that should stay clear of the corner image.\n"

	splitPNG, err := renderSplitViewPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render split view png: %v", err)
	}
	themedPNG, err := renderThemedPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render themed png: %v", err)
	}
	if err := comparePNGTolerant(splitPNG, themedPNG, 0.002); err != nil {
		t.Fatalf("split view raster mismatch: %v", err)
	}
}

func TestPrintViewWithoutBackgroundMatchesThemedViewAndBoringPrint(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not found in PATH")
	}
	if _, err := exec.LookPath("pdftocairo"); err != nil {
		t.Skip("pdftocairo not found in PATH")
	}

	cfg := pdf.DefaultConfig()
	cfg.PageSize = "A4"
	cfg.Margin = 36
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	cfg.LineHeight = 1.4
	cfg.BackgroundEnabled = false
	cfg.BackgroundRGB = [3]int{255, 0, 0}
	cfg.TextRGB = [3]int{32, 32, 32}
	markdown := "# Heading\n\nBody text should look like the non-split view but print in the boring layout.\n\n> quoted text with themed fill\n\n`inline code` and [a colored link](https://example.com)\n"

	splitPDF, err := renderSplitViewPDF(markdown, cfg)
	if err != nil {
		t.Fatalf("render split view pdf: %v", err)
	}
	splitPNG, err := renderPrintViewPNG(splitPDF)
	if err != nil {
		t.Fatalf("render split view png: %v", err)
	}
	printPNG, err := flattenAndRenderPrintViewPNG(splitPDF)
	if err != nil {
		t.Fatalf("render split print png: %v", err)
	}
	themedPNG, err := renderThemedPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render themed png: %v", err)
	}
	boringPNG, err := renderBoringPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render boring png: %v", err)
	}
	if err := comparePNGTolerant(splitPNG, themedPNG, 0.002); err != nil {
		t.Fatalf("split no-background view mismatch: %v", err)
	}
	if err := comparePNGTolerant(printPNG, boringPNG, 0.007); err != nil {
		t.Fatalf("split no-background print mismatch: %v", err)
	}
}

func TestPrintViewWithoutBackgroundTransparentCornerMatchesBoringPrint(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not found in PATH")
	}
	if _, err := exec.LookPath("pdftocairo"); err != nil {
		t.Skip("pdftocairo not found in PATH")
	}
	tmpDir, err := os.MkdirTemp("", "mdf-no-bg-print-corner-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cornerPath := filepath.Join(tmpDir, "corner.png")
	if err := writeTransparentCornerPNG(cornerPath); err != nil {
		t.Fatalf("write corner image: %v", err)
	}

	cfg := pdf.DefaultConfig()
	cfg.PageSize = "A4"
	cfg.Margin = 36
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	cfg.LineHeight = 1.4
	cfg.BackgroundEnabled = false
	cfg.CornerImagePath = cornerPath
	cfg.TextRGB = [3]int{32, 32, 32}
	markdown := "# Heading\n\nBody text should print with a single corner image, not a double-darkened transparent overlay.\n"

	splitPDF, err := renderSplitViewPDF(markdown, cfg)
	if err != nil {
		t.Fatalf("render split view pdf: %v", err)
	}
	splitPNG, err := renderPrintViewPNG(splitPDF)
	if err != nil {
		t.Fatalf("render split view png: %v", err)
	}
	printPNG, err := flattenAndRenderPrintViewPNG(splitPDF)
	if err != nil {
		t.Fatalf("render split print png: %v", err)
	}
	themedPNG, err := renderThemedPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render themed png: %v", err)
	}
	boringPNG, err := renderBoringPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render boring png: %v", err)
	}
	if err := comparePNGTolerant(splitPNG, themedPNG, 0.002); err != nil {
		t.Fatalf("split no-background transparent-corner view mismatch: %v", err)
	}
	if err := comparePNGTolerant(printPNG, boringPNG, 0.005); err != nil {
		t.Fatalf("split no-background transparent-corner print mismatch: %v", err)
	}
}

func TestRenderWithoutBackgroundTransparentCornerIgnoresBackgroundRGB(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not found in PATH")
	}
	tmpDir, err := os.MkdirTemp("", "mdf-no-bg-corner-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cornerPath := filepath.Join(tmpDir, "corner.png")
	if err := writeTransparentCornerPNG(cornerPath); err != nil {
		t.Fatalf("write corner image: %v", err)
	}

	cfg := pdf.DefaultConfig()
	cfg.PageSize = "A4"
	cfg.Margin = 36
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	cfg.LineHeight = 1.4
	cfg.BackgroundEnabled = false
	cfg.CornerImagePath = cornerPath
	cfg.TextRGB = [3]int{32, 32, 32}
	markdown := "# Heading\n\nBody text.\n"

	cfg.BackgroundRGB = [3]int{0, 0, 0}
	blackPNG, err := renderThemedPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render themed png with black background rgb: %v", err)
	}
	cfg.BackgroundRGB = [3]int{255, 0, 0}
	redPNG, err := renderThemedPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render themed png with red background rgb: %v", err)
	}
	if err := comparePNGTolerant(blackPNG, redPNG, 0.002); err != nil {
		t.Fatalf("ordinary no-background render changed with background rgb: %v", err)
	}
}

func TestPrintViewTransparentCornerImageWithDPIMatchesThemedView(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not found in PATH")
	}
	tmpDir, err := os.MkdirTemp("", "mdf-print-view-corner-dpi-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cornerPath := filepath.Join(tmpDir, "corner-dpi.png")
	if err := writeTransparentCornerPNGWithDPI(cornerPath, 144); err != nil {
		t.Fatalf("write dpi corner image: %v", err)
	}

	cfg := pdf.DefaultConfig()
	cfg.PageSize = "A4"
	cfg.Margin = 36
	cfg.FontFamily = "Courier"
	cfg.FontSize = 12
	cfg.LineHeight = 1.4
	cfg.CornerImagePath = cornerPath
	cfg.CornerImageMaxWidth = 0
	cfg.CornerImageMaxHeight = 0
	cfg.TextRGB = [3]int{32, 32, 32}
	markdown := "# Heading\n\nBody text that should wrap consistently next to the DPI-sized corner image and stay aligned across split view rendering.\n"

	splitPNG, err := renderSplitViewPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render split view png: %v", err)
	}
	themedPNG, err := renderThemedPNG(markdown, cfg)
	if err != nil {
		t.Fatalf("render themed png: %v", err)
	}
	if err := comparePNGTolerant(splitPNG, themedPNG, 0.002); err != nil {
		t.Fatalf("split view dpi corner mismatch: %v", err)
	}
}

func renderPrintViewPNG(pdfData []byte) (string, error) {
	tmpDir, err := os.MkdirTemp("", "mdf-print-view-test-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	pdfPath := filepath.Join(tmpDir, "out.pdf")
	if err := os.WriteFile(pdfPath, pdfData, 0o644); err != nil {
		return "", err
	}
	prefix := filepath.Join(tmpDir, "page")
	nicePath, _ := exec.LookPath("nice")
	cmd := pdfgolden.PDFToPPMCommand(nicePath, pdfPath, prefix)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdftoppm failed: %v\n%s", err, string(out))
	}
	pages, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return "", err
	}
	sort.Strings(pages)
	if len(pages) == 0 {
		return "", fmt.Errorf("pdftoppm produced no pages")
	}
	dst, err := os.CreateTemp("", "mdf-print-view-page-*.png")
	if err != nil {
		return "", err
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	if err := pdfgolden.CopyFile(dst.Name(), pages[0]); err != nil {
		return "", err
	}
	return dst.Name(), nil
}

func flattenAndRenderPrintViewPNG(pdfData []byte) (string, error) {
	tmpDir, err := os.MkdirTemp("", "mdf-print-view-flat-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	srcPath := filepath.Join(tmpDir, "src.pdf")
	if err := os.WriteFile(srcPath, pdfData, 0o644); err != nil {
		return "", err
	}
	flatPath := filepath.Join(tmpDir, "flat.pdf")
	cmd := pdfgolden.PDFToCairoPDFCommand(srcPath, flatPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdftocairo failed: %v\n%s", err, string(out))
	}
	flatData, err := os.ReadFile(flatPath)
	if err != nil {
		return "", err
	}
	return renderPrintViewPNG(flatData)
}

func renderSplitViewPNG(markdown string, cfg pdf.Config) (string, error) {
	out, err := renderSplitViewPDF(markdown, cfg)
	if err != nil {
		return "", err
	}
	return renderPrintViewPNG(out)
}

func renderSplitViewPDF(markdown string, cfg pdf.Config) ([]byte, error) {
	cfg.UseOCGPrintView = true
	var out bytes.Buffer
	err := pdf.Render(pdf.RenderRequest{
		Reader: strings.NewReader(markdown),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: cfg,
	})
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func renderThemedPNG(markdown string, cfg pdf.Config) (string, error) {
	cfg.UseOCGPrintView = false
	cfg.Boring = false
	var out bytes.Buffer
	err := pdf.Render(pdf.RenderRequest{
		Reader: strings.NewReader(markdown),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: cfg,
	})
	if err != nil {
		return "", err
	}
	return renderPrintViewPNG(out.Bytes())
}

func renderBoringPNG(markdown string, cfg pdf.Config) (string, error) {
	cfg.UseOCGPrintView = false
	cfg.Boring = true
	var out bytes.Buffer
	err := pdf.Render(pdf.RenderRequest{
		Reader: strings.NewReader(markdown),
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: cfg,
	})
	if err != nil {
		return "", err
	}
	return renderPrintViewPNG(out.Bytes())
}

func comparePNGTolerant(gotPath, wantPath string, maxRatio float64) error {
	got, err := loadPNG(gotPath)
	if err != nil {
		return fmt.Errorf("load %s: %w", gotPath, err)
	}
	want, err := loadPNG(wantPath)
	if err != nil {
		return fmt.Errorf("load %s: %w", wantPath, err)
	}
	if got.Bounds() != want.Bounds() {
		return fmt.Errorf("bounds mismatch got=%v want=%v", got.Bounds(), want.Bounds())
	}
	diff := 0
	total := got.Bounds().Dx() * got.Bounds().Dy()
	for y := got.Bounds().Min.Y; y < got.Bounds().Max.Y; y++ {
		for x := got.Bounds().Min.X; x < got.Bounds().Max.X; x++ {
			r1, g1, b1, a1 := got.At(x, y).RGBA()
			r2, g2, b2, a2 := want.At(x, y).RGBA()
			if !rgbaClose(r1, r2) || !rgbaClose(g1, g2) || !rgbaClose(b1, b2) || !rgbaClose(a1, a2) {
				diff++
			}
		}
	}
	if diff == 0 {
		return nil
	}
	ratio := float64(diff) / float64(total)
	if ratio > maxRatio {
		return fmt.Errorf("pixel diff ratio %.4f exceeds %.4f", ratio, maxRatio)
	}
	return nil
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func rgbaClose(a, b uint32) bool {
	av := int(a >> 8)
	bv := int(b >> 8)
	if av < bv {
		return bv-av <= 2
	}
	return av-bv <= 2
}

func writeTransparentCornerPNG(path string) error {
	return writeTransparentCornerPNGWithDPI(path, 0)
}

func writeTransparentCornerPNGWithDPI(path string, dpi int) error {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.NRGBA{R: 220, G: 32, B: 32, A: 128})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	data := buf.Bytes()
	if dpi > 0 {
		withDPI, err := injectPNGPhysicalDPI(data, dpi)
		if err != nil {
			return err
		}
		data = withDPI
	}
	return os.WriteFile(path, data, 0o644)
}

func injectPNGPhysicalDPI(data []byte, dpi int) ([]byte, error) {
	const pngHeaderLen = 8
	if len(data) < pngHeaderLen {
		return nil, fmt.Errorf("png too short")
	}
	if !bytes.Equal(data[:pngHeaderLen], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return nil, fmt.Errorf("invalid png header")
	}
	if dpi <= 0 {
		return append([]byte(nil), data...), nil
	}
	pixelsPerMeter := uint32(math.Round(float64(dpi) * 39.3701))
	chunkData := make([]byte, 9)
	binary.BigEndian.PutUint32(chunkData[0:4], pixelsPerMeter)
	binary.BigEndian.PutUint32(chunkData[4:8], pixelsPerMeter)
	chunkData[8] = 1

	chunkType := []byte("pHYs")
	chunk := make([]byte, 4+4+len(chunkData)+4)
	binary.BigEndian.PutUint32(chunk[0:4], uint32(len(chunkData)))
	copy(chunk[4:8], chunkType)
	copy(chunk[8:8+len(chunkData)], chunkData)
	binary.BigEndian.PutUint32(chunk[8+len(chunkData):], crc32.ChecksumIEEE(append(chunkType, chunkData...)))

	offset := pngHeaderLen
	if len(data) < offset+8 {
		return nil, fmt.Errorf("png missing ihdr")
	}
	length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	chunkEnd := offset + 8 + length + 4
	if chunkEnd > len(data) {
		return nil, fmt.Errorf("png ihdr chunk truncated")
	}
	out := make([]byte, 0, len(data)+len(chunk))
	out = append(out, data[:chunkEnd]...)
	out = append(out, chunk...)
	out = append(out, data[chunkEnd:]...)
	return out, nil
}
