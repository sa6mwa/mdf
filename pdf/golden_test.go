package pdf_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"pkt.systems/mdf/pdf/internal/pdfgolden"
)

func TestPDFGoldens(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not found in PATH")
	}
	nicePath, _ := exec.LookPath("nice")
	root, err := pdfgolden.FindModuleRoot()
	if err != nil {
		t.Fatalf("find module root: %v", err)
	}
	samples, err := pdfgolden.CollectSamples(root)
	if err != nil {
		t.Fatalf("collect samples: %v", err)
	}
	if len(samples) == 0 {
		t.Fatalf("no testdata markdown files found")
	}
	fontSizes := []int{18, 12, 9}
	goldenDir := filepath.Join(root, "testdata", "golden")

	for _, sample := range samples {
		data, err := os.ReadFile(sample.Path)
		if err != nil {
			t.Fatalf("read %s: %v", sample.Path, err)
		}
		for _, size := range fontSizes {
			t.Run(fmt.Sprintf("%s-fs%d", sample.Name, size), func(t *testing.T) {
				tmpDir := t.TempDir()
				pdfPath := filepath.Join(tmpDir, "out.pdf")
				f, err := os.Create(pdfPath)
				if err != nil {
					t.Fatalf("create pdf: %v", err)
				}
				if err := pdfgolden.RenderSamplePDF(f, data, float64(size), root); err != nil {
					_ = f.Close()
					t.Fatalf("render pdf: %v", err)
				}
				if err := f.Close(); err != nil {
					t.Fatalf("close pdf: %v", err)
				}

				prefix := filepath.Join(tmpDir, "page")
				cmd := pdfgolden.PDFToPPMCommand(nicePath, pdfPath, prefix)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("pdftoppm failed: %v\n%s", err, string(out))
				}
				pages, err := filepath.Glob(prefix + "-*.png")
				if err != nil {
					t.Fatalf("glob pages: %v", err)
				}
				sort.Strings(pages)
				if len(pages) == 0 {
					t.Fatalf("pdftoppm produced no pages")
				}

				for i, page := range pages {
					want := filepath.Join(goldenDir, pdfgolden.GoldenName(sample.Name, size, i+1))
					if _, err := os.Stat(want); err != nil {
						t.Fatalf("missing golden %s (run \"go generate ./...\" to regenerate)", want)
					}
					if err := pdfgolden.ComparePNG(page, want); err != nil {
						t.Fatalf("pdf golden mismatch: %v", err)
					}
				}
			})
		}
	}
}
