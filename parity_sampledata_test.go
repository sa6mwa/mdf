package mdf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestRenderSampledataParity(t *testing.T) {
	root := "testdata"
	optionsByPath := map[string][]RenderOption{}
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "testdata/future/") {
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk testdata: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("no markdown files found under %s", root)
	}
	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			widths, err := goldenWidthsForFile(root, path)
			if err != nil {
				t.Fatalf("golden widths %s: %v", path, err)
			}
			opts := optionsByPath[path]
			for _, width := range widths {
				goldenPath := goldenStreamPath(path, width)
				want, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatalf("read golden %s: %v", goldenPath, err)
				}
				var out bytes.Buffer
				err = Render(RenderRequest{
					Reader:  bytes.NewReader(src),
					Writer:  &out,
					Width:   width,
					Theme:   DefaultTheme(),
					Options: opts,
				})
				if err != nil {
					t.Fatalf("stream live %s width %d: %v", path, width, err)
				}
				got := out.String()
				if string(want) != got {
					diff := firstDiffContext(string(want), got, 3)
					t.Fatalf("parity mismatch %s width %d\n%s", path, width, diff)
				}
			}
		})
	}
}

func goldenWidthsForFile(root string, mdPath string) ([]int, error) {
	rel, err := filepath.Rel(root, mdPath)
	if err != nil {
		rel = mdPath
	}
	name := strings.TrimSuffix(rel, ".md")
	name = strings.ReplaceAll(filepath.ToSlash(name), "/", "__")
	pattern := filepath.Join(root, name+".w*.golden")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no golden files found for %s", mdPath)
	}
	widths := make([]int, 0, len(matches))
	for _, match := range matches {
		base := filepath.Base(match)
		start := strings.LastIndex(base, ".w")
		if start == -1 {
			continue
		}
		end := strings.LastIndex(base, ".golden")
		if end == -1 || end <= start+2 {
			continue
		}
		widthStr := base[start+2 : end]
		width, err := strconv.Atoi(widthStr)
		if err != nil {
			return nil, fmt.Errorf("parse width from %s: %w", base, err)
		}
		widths = append(widths, width)
	}
	sort.Ints(widths)
	if len(widths) == 0 {
		return nil, fmt.Errorf("no golden widths parsed for %s", mdPath)
	}
	return widths, nil
}

func goldenStreamPath(mdPath string, width int) string {
	rel, err := filepath.Rel("testdata", mdPath)
	if err != nil {
		rel = mdPath
	}
	name := strings.TrimSuffix(rel, ".md")
	name = strings.ReplaceAll(filepath.ToSlash(name), "/", "__")
	return filepath.Join("testdata", fmt.Sprintf("%s.w%d.golden", name, width))
}

func firstDiffContext(want string, got string, ctx int) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	max := len(wantLines)
	if len(gotLines) > max {
		max = len(gotLines)
	}
	diffAt := -1
	for i := 0; i < max; i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			diffAt = i
			break
		}
	}
	if diffAt == -1 {
		return "---want---\n" + want + "\n---got---\n" + got
	}
	start := diffAt - ctx
	if start < 0 {
		start = 0
	}
	end := diffAt + ctx
	if end >= max {
		end = max - 1
	}
	var b strings.Builder
	fmt.Fprintf(&b, "first difference at line %d\n", diffAt+1)
	b.WriteString("---want---\n")
	for i := start; i <= end; i++ {
		line := ""
		if i < len(wantLines) {
			line = wantLines[i]
		}
		fmt.Fprintf(&b, "%5d | %s\n", i+1, line)
	}
	b.WriteString("---got---\n")
	for i := start; i <= end; i++ {
		line := ""
		if i < len(gotLines) {
			line = gotLines[i]
		}
		fmt.Fprintf(&b, "%5d | %s\n", i+1, line)
	}
	return b.String()
}
