package pdfgolden

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png" // register PNG decoder
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"pkt.systems/mdf"
	"pkt.systems/mdf/pdf"
	"pkt.systems/mdf/pdf/testdata"
	"sort"
	"strings"
)

const (
	pdfGoldenDPI      = "96"
	pdfGoldenTol      = 2
	pdfGoldenMaxRatio = 0.0005
)

// Sample identifies a markdown input used for PDF golden testing.
type Sample struct {
	Path string
	Name string
}

// FindTestdataRoot locates the pdf testdata module root.
func FindTestdataRoot() (string, error) {
	return testdata.Root()
}

// CollectSamples finds markdown samples eligible for PDF goldens.
func CollectSamples(root string) ([]Sample, error) {
	var samples []Sample
	sampleRoot := root
	err := filepath.WalkDir(sampleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel, relErr := filepath.Rel(sampleRoot, path)
			if relErr == nil {
				rel = filepath.ToSlash(rel)
				if rel == "future" || strings.HasPrefix(rel, "future/") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(sampleRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !allowedSample(rel) {
			return nil
		}
		base := strings.TrimSuffix(rel, filepath.Ext(rel))
		name := strings.ReplaceAll(base, "/", "__")
		samples = append(samples, Sample{Path: path, Name: name})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Name < samples[j].Name
	})
	return samples, nil
}

func allowedSample(rel string) bool {
	switch rel {
	case "replay.md",
		"lazyblockquote.md",
		"misreadings.md",
		"OBAF.md",
		"mdtest/TEST.md":
		return true
	}
	if strings.HasPrefix(rel, "parity/") || strings.HasPrefix(rel, "parity__") {
		return true
	}
	return false
}

// PDFToPPMCommand returns the pdftoppm command used to rasterize PDFs.
func PDFToPPMCommand(nicePath, pdfPath, prefix string) *exec.Cmd {
	if nicePath != "" {
		return exec.Command(nicePath, "-n", "10", "pdftoppm", "-png", "-r", pdfGoldenDPI, pdfPath, prefix)
	}
	return exec.Command("pdftoppm", "-png", "-r", pdfGoldenDPI, pdfPath, prefix)
}

// PDFToCairoPDFCommand returns the pdftocairo command used to flatten PDFs.
func PDFToCairoPDFCommand(pdfPath, outPath string) *exec.Cmd {
	return exec.Command("pdftocairo", "-pdf", pdfPath, outPath)
}

// RenderSamplePDF renders markdown input into a PDF for golden comparison.
func RenderSamplePDF(w io.Writer, data []byte, fontSize float64, root string) error {
	cfg := pdf.DefaultConfig()
	cfg.FontSize = fontSize
	cfg.PageSize = "A4"
	reg, bold, italic, boldItalic, err := pdf.EmbeddedHackFonts()
	if err != nil {
		return err
	}
	cfg.FontFamily = pdf.EmbeddedFontFamily
	cfg.RegularFontBytes = reg
	cfg.BoldFontBytes = bold
	cfg.ItalicFontBytes = italic
	cfg.BoldItalicFontBytes = boldItalic
	req := pdf.RenderRequest{
		Reader: bytes.NewReader(data),
		Writer: w,
		Theme:  mdf.DefaultTheme(),
		Config: cfg,
	}
	return pdf.Render(req)
}

// GoldenName formats a golden PNG filename.
func GoldenName(name string, size int, page int) string {
	return fmt.Sprintf("%s_fs%d_p%d.png", name, size, page)
}

// CopyFile copies src to dst, creating parent directories as needed.
func CopyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// ComparePNG compares two PNGs and returns an error if they differ beyond tolerance.
func ComparePNG(gotPath, wantPath string) error {
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
	total := (got.Bounds().Dx() * got.Bounds().Dy())
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
	if ratio > pdfGoldenMaxRatio {
		return fmt.Errorf("pixel diff ratio %.4f exceeds %.4f", ratio, pdfGoldenMaxRatio)
	}
	return fmt.Errorf("pixel diff ratio %.4f exceeds 0 (tolerance %.4f)", ratio, pdfGoldenMaxRatio)
}

func rgbaClose(a, b uint32) bool {
	av := int(a >> 8)
	bv := int(b >> 8)
	if av < bv {
		return bv-av <= pdfGoldenTol
	}
	return av-bv <= pdfGoldenTol
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
