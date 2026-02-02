package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"pkt.systems/mdf"
)

func main() {
	widths := []int{50, 60, 80}
	root := "testdata"
	var paths []string
	widthsByBase := map[string][]int{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
			return nil
		}
		if strings.HasSuffix(path, ".golden") {
			if base, width, ok := parseGoldenWidth(root, path); ok {
				widthsByBase[base] = append(widthsByBase[base], width)
			}
		}
		return nil
	})
	if err != nil {
		fatalf("walk %s: %v", root, err)
	}
	if len(paths) == 0 {
		fatalf("no markdown files found under %s", root)
	}
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			fatalf("read %s: %v", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		base := strings.TrimSuffix(rel, ".md")
		base = strings.ReplaceAll(filepath.ToSlash(base), "/", "__")
		useWidths := widthsByBase[base]
		if len(useWidths) == 0 {
			useWidths = widths
		}
		for _, width := range useWidths {
			var out bytes.Buffer
			err := mdf.Render(mdf.RenderRequest{
				Reader: bytes.NewReader(src),
				Writer: &out,
				Width:  width,
				Theme:  mdf.DefaultTheme(),
			})
			if err != nil {
				fatalf("render %s width %d: %v", path, width, err)
			}
			goldenPath := goldenStreamPath(root, path, width)
			if err := os.WriteFile(goldenPath, out.Bytes(), 0o644); err != nil {
				fatalf("write %s: %v", goldenPath, err)
			}
			fmt.Fprintf(os.Stdout, "wrote %s\n", goldenPath)
		}
	}
}

func goldenStreamPath(root string, mdPath string, width int) string {
	rel, err := filepath.Rel(root, mdPath)
	if err != nil {
		rel = mdPath
	}
	name := strings.TrimSuffix(rel, ".md")
	name = strings.ReplaceAll(filepath.ToSlash(name), "/", "__")
	return filepath.Join(root, fmt.Sprintf("%s.w%d.golden", name, width))
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func parseGoldenWidth(root, goldenPath string) (string, int, bool) {
	rel, err := filepath.Rel(root, goldenPath)
	if err != nil {
		return "", 0, false
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasSuffix(rel, ".golden") {
		return "", 0, false
	}
	name := strings.TrimSuffix(rel, ".golden")
	idx := strings.LastIndex(name, ".w")
	if idx == -1 {
		return "", 0, false
	}
	widthPart := name[idx+2:]
	width, err := strconv.Atoi(widthPart)
	if err != nil || width <= 0 {
		return "", 0, false
	}
	base := name[:idx]
	return base, width, true
}
