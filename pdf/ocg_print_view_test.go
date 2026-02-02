package pdf_test

import (
	"bytes"
	"fmt"
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

func TestOCGPrintViewHeadingInPrintLayer(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not found in PATH")
	}
	root, err := pdfgolden.FindModuleRoot()
	if err != nil {
		t.Fatalf("find module root: %v", err)
	}

	cfg := pdf.DefaultConfig()
	cfg.UseOCGPrintView = true
	cfg.CornerImagePath = filepath.Join(root, "testdata", "ocg_corner.png")
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
	src := strings.NewReader("# Heading\n\nBody text.\n")
	err = pdf.Render(pdf.RenderRequest{
		Reader: src,
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("render pdf: %v", err)
	}

	goldenDir := filepath.Join(root, "testdata", "golden")
	viewPNG, err := renderOCGPNG(out.Bytes())
	if err != nil {
		t.Fatalf("render view png: %v", err)
	}
	printData, err := pdfgolden.ApplyPrintOCGVisibility(out.Bytes())
	if err != nil {
		t.Fatalf("apply print visibility: %v", err)
	}
	printPNG, err := renderOCGPNG(printData)
	if err != nil {
		t.Fatalf("render print png: %v", err)
	}
	if err := pdfgolden.ComparePNG(viewPNG, filepath.Join(goldenDir, "ocg_print_view_view_p1.png")); err != nil {
		t.Fatalf("view golden mismatch: %v", err)
	}
	if err := pdfgolden.ComparePNG(printPNG, filepath.Join(goldenDir, "ocg_print_view_print_p1.png")); err != nil {
		t.Fatalf("print golden mismatch: %v", err)
	}
}

func renderOCGPNG(pdfData []byte) (string, error) {
	tmpDir, err := os.MkdirTemp("", "mdf-ocg-test-")
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
	dst, err := os.CreateTemp("", "mdf-ocg-page-*.png")
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
