package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"pkt.systems/mdf/pdf/internal/pdfgolden"
)

func main() {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		fmt.Fprintln(os.Stderr, "pdftoppm not found in PATH")
		os.Exit(2)
	}
	nicePath, _ := exec.LookPath("nice")
	root, err := pdfgolden.FindTestdataRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find testdata root: %v\n", err)
		os.Exit(1)
	}
	samples, err := pdfgolden.CollectSamples(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "collect samples: %v\n", err)
		os.Exit(1)
	}
	if len(samples) == 0 {
		fmt.Fprintln(os.Stderr, "no testdata markdown files found")
		os.Exit(1)
	}
	fontSizes := []int{18, 12, 9}
	goldenDir := filepath.Join(root, "golden")

	for _, sample := range samples {
		data, err := os.ReadFile(sample.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", sample.Path, err)
			os.Exit(1)
		}
		for _, size := range fontSizes {
			tmpDir, err := os.MkdirTemp("", "mdf-pdf-golden-")
			if err != nil {
				fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
				os.Exit(1)
			}
			pdfPath := filepath.Join(tmpDir, "out.pdf")
			f, err := os.Create(pdfPath)
			if err != nil {
				_ = os.RemoveAll(tmpDir)
				fmt.Fprintf(os.Stderr, "create pdf: %v\n", err)
				os.Exit(1)
			}
			if err := pdfgolden.RenderSamplePDF(f, data, float64(size), root); err != nil {
				_ = f.Close()
				_ = os.RemoveAll(tmpDir)
				fmt.Fprintf(os.Stderr, "render pdf: %v\n", err)
				os.Exit(1)
			}
			if err := f.Close(); err != nil {
				_ = os.RemoveAll(tmpDir)
				fmt.Fprintf(os.Stderr, "close pdf: %v\n", err)
				os.Exit(1)
			}

			prefix := filepath.Join(tmpDir, "page")
			cmd := pdfgolden.PDFToPPMCommand(nicePath, pdfPath, prefix)
			if out, err := cmd.CombinedOutput(); err != nil {
				_ = os.RemoveAll(tmpDir)
				fmt.Fprintf(os.Stderr, "pdftoppm failed: %v\n%s", err, string(out))
				os.Exit(1)
			}
			pages, err := filepath.Glob(prefix + "-*.png")
			if err != nil {
				_ = os.RemoveAll(tmpDir)
				fmt.Fprintf(os.Stderr, "glob pages: %v\n", err)
				os.Exit(1)
			}
			sort.Strings(pages)
			if len(pages) == 0 {
				_ = os.RemoveAll(tmpDir)
				fmt.Fprintln(os.Stderr, "pdftoppm produced no pages")
				os.Exit(1)
			}

			if err := os.MkdirAll(goldenDir, 0o755); err != nil {
				_ = os.RemoveAll(tmpDir)
				fmt.Fprintf(os.Stderr, "mkdir goldens: %v\n", err)
				os.Exit(1)
			}
			pattern := filepath.Join(goldenDir, fmt.Sprintf("%s_fs%d_p*.png", sample.Name, size))
			stale, _ := filepath.Glob(pattern)
			for _, path := range stale {
				_ = os.Remove(path)
			}
			for i, page := range pages {
				dst := filepath.Join(goldenDir, pdfgolden.GoldenName(sample.Name, size, i+1))
				if err := pdfgolden.CopyFile(dst, page); err != nil {
					_ = os.RemoveAll(tmpDir)
					fmt.Fprintf(os.Stderr, "write golden: %v\n", err)
					os.Exit(1)
				}
				fmt.Fprintf(os.Stdout, "wrote %s\n", dst)
			}
			_ = os.RemoveAll(tmpDir)
		}
	}
}
