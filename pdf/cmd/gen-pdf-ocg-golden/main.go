package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"pkt.systems/mdf"
	"pkt.systems/mdf/pdf"
	"pkt.systems/mdf/pdf/internal/pdfgolden"
)

func main() {
	root, err := pdfgolden.FindModuleRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find module root: %v\n", err)
		os.Exit(1)
	}
	var out bytes.Buffer
	cfg, err := ocgConfig(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "embedded fonts: %v\n", err)
		os.Exit(1)
	}
	src := strings.NewReader("# Heading\n\nBody text.\n")
	if err := pdf.Render(pdf.RenderRequest{
		Reader: src,
		Writer: &out,
		Theme:  mdf.DefaultTheme(),
		Config: cfg,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "render pdf: %v\n", err)
		os.Exit(1)
	}

	viewPDF := out.Bytes()
	printPDF, err := pdfgolden.ApplyPrintOCGVisibility(viewPDF)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apply print visibility: %v\n", err)
		os.Exit(1)
	}
	goldenDir := filepath.Join(root, "testdata", "golden")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir goldens: %v\n", err)
		os.Exit(1)
	}
	if err := writeOCGPNG(viewPDF, filepath.Join(goldenDir, "ocg_print_view_view_p1.png")); err != nil {
		fmt.Fprintf(os.Stderr, "write view golden: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "wrote %s\n", filepath.Join(goldenDir, "ocg_print_view_view_p1.png"))
	if err := writeOCGPNG(printPDF, filepath.Join(goldenDir, "ocg_print_view_print_p1.png")); err != nil {
		fmt.Fprintf(os.Stderr, "write print golden: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "wrote %s\n", filepath.Join(goldenDir, "ocg_print_view_print_p1.png"))
}

func ocgConfig(root string) (pdf.Config, error) {
	cfg := pdf.DefaultConfig()
	cfg.UseOCGPrintView = true
	cfg.CornerImagePath = filepath.Join(root, "testdata", "ocg_corner.png")
	reg, bold, italic, boldItalic, err := pdf.EmbeddedHackFonts()
	if err != nil {
		return pdf.Config{}, err
	}
	cfg.FontFamily = pdf.EmbeddedFontFamily
	cfg.RegularFontBytes = reg
	cfg.BoldFontBytes = bold
	cfg.ItalicFontBytes = italic
	cfg.BoldItalicFontBytes = boldItalic
	return cfg, nil
}

func writeOCGPNG(pdfData []byte, dst string) error {
	tmpDir, err := os.MkdirTemp("", "mdf-ocg-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	pdfPath := filepath.Join(tmpDir, "out.pdf")
	if err := os.WriteFile(pdfPath, pdfData, 0o644); err != nil {
		return err
	}
	prefix := filepath.Join(tmpDir, "page")
	nicePath, _ := exec.LookPath("nice")
	cmd := pdfgolden.PDFToPPMCommand(nicePath, pdfPath, prefix)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pdftoppm failed: %v\n%s", err, string(out))
	}
	pages, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return err
	}
	sort.Strings(pages)
	if len(pages) == 0 {
		return fmt.Errorf("pdftoppm produced no pages")
	}
	return pdfgolden.CopyFile(dst, pages[0])
}
