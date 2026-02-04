package pdfgolden

import (
	"bytes"
	"errors"
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
	"regexp"
	"sort"
	"strconv"
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

// ApplyPrintViewState replaces OCG /ViewState values with /PrintState values.
func ApplyPrintViewState(data []byte) ([]byte, error) {
	out := make([]byte, 0, len(data))
	i := 0
	for {
		pos := bytes.Index(data[i:], []byte("/Usage"))
		if pos == -1 {
			out = append(out, data[i:]...)
			break
		}
		pos += i
		out = append(out, data[i:pos]...)
		start := bytes.Index(data[pos:], []byte("<<"))
		if start == -1 {
			out = append(out, data[pos:]...)
			break
		}
		start += pos
		end := matchDictEnd(data, start)
		if end == -1 {
			out = append(out, data[pos:]...)
			break
		}
		usage := data[start : end+2]
		modified := rewriteViewState(usage)
		out = append(out, data[pos:start]...)
		out = append(out, modified...)
		i = end + 2
	}
	return out, nil
}

// ApplyPrintOCGVisibility rewrites OCG visibility to match print state.
func ApplyPrintOCGVisibility(data []byte) ([]byte, error) {
	onObjs, offObjs, err := collectOCGPrintStates(data)
	if err != nil {
		return nil, err
	}
	rewritten, err := ApplyPrintViewState(data)
	if err != nil {
		return nil, err
	}
	onArr := formatObjRefArray(onObjs)
	offArr := formatObjRefArray(offObjs)
	dict, dAbsStart, dEnd, err := ocgDefaultDict(rewritten)
	if err != nil {
		return nil, err
	}
	updated := dict
	printDict, err := extractSubDict(dict, "/Print")
	if err != nil {
		return nil, err
	}
	updated, err = replaceSubDict(updated, "/View", printDict)
	if err != nil {
		return nil, err
	}
	updated = replaceTopLevelArray(updated, "/ON", onArr)
	updated = replaceTopLevelArray(updated, "/OFF", offArr)
	updated = replaceTopLevelName(updated, "/BaseState", "/ON")
	out := make([]byte, 0, len(rewritten)-len(dict)+len(updated))
	out = append(out, rewritten[:dAbsStart]...)
	out = append(out, updated...)
	out = append(out, rewritten[dEnd+2:]...)
	return rebuildXref(out)
}

func matchDictEnd(data []byte, start int) int {
	depth := 0
	for i := start; i+1 < len(data); i++ {
		if data[i] == '<' && data[i+1] == '<' {
			depth++
			i++
			continue
		}
		if data[i] == '>' && data[i+1] == '>' {
			depth--
			if depth == 0 {
				return i
			}
			i++
			continue
		}
	}
	return -1
}

func rewriteViewState(usage []byte) []byte {
	viewIdx := bytes.Index(usage, []byte("/ViewState"))
	printIdx := bytes.Index(usage, []byte("/PrintState"))
	if viewIdx == -1 || printIdx == -1 {
		return usage
	}
	viewValueStart := viewIdx + len("/ViewState")
	printValueStart := printIdx + len("/PrintState")
	viewValue, viewStart, viewEnd := parseName(usage, viewValueStart)
	printValue, _, _ := parseName(usage, printValueStart)
	if viewValue == "" || printValue == "" || viewStart == -1 || viewEnd == -1 {
		return usage
	}
	out := make([]byte, 0, len(usage))
	out = append(out, usage[:viewStart]...)
	out = append(out, printValue...)
	out = append(out, usage[viewEnd:]...)
	return out
}

func ocgDefaultDict(data []byte) ([]byte, int, int, error) {
	ocProps := bytes.Index(data, []byte("/OCProperties"))
	if ocProps == -1 {
		return nil, 0, 0, errors.New("missing /OCProperties")
	}
	startDict := bytes.Index(data[ocProps:], []byte("<<"))
	if startDict == -1 {
		return nil, 0, 0, errors.New("missing /OCProperties dict")
	}
	startDict += ocProps
	endDict := matchDictEnd(data, startDict)
	if endDict == -1 {
		return nil, 0, 0, errors.New("unterminated /OCProperties dict")
	}
	block := data[startDict : endDict+2]
	dIdx := bytes.Index(block, []byte("/D"))
	if dIdx == -1 {
		return nil, 0, 0, errors.New("missing /D in /OCProperties")
	}
	dStart := bytes.Index(block[dIdx:], []byte("<<"))
	if dStart == -1 {
		return nil, 0, 0, errors.New("missing /D dict")
	}
	dStart += dIdx
	dAbsStart := startDict + dStart
	dEnd := matchDictEnd(data, dAbsStart)
	if dEnd == -1 {
		return nil, 0, 0, errors.New("unterminated /D dict")
	}
	return data[dAbsStart : dEnd+2], dAbsStart, dEnd, nil
}

func extractSubDict(dict []byte, key string) ([]byte, error) {
	idx := bytes.Index(dict, []byte(key))
	if idx == -1 {
		return nil, fmt.Errorf("missing %s dict", key)
	}
	start := bytes.Index(dict[idx:], []byte("<<"))
	if start == -1 {
		return nil, fmt.Errorf("missing %s content", key)
	}
	start += idx
	end := matchDictEnd(dict, start)
	if end == -1 {
		return nil, fmt.Errorf("unterminated %s dict", key)
	}
	return dict[start : end+2], nil
}

func replaceSubDict(dict []byte, key string, sub []byte) ([]byte, error) {
	idx := bytes.Index(dict, []byte(key))
	if idx == -1 {
		return nil, fmt.Errorf("missing %s dict", key)
	}
	start := bytes.Index(dict[idx:], []byte("<<"))
	if start == -1 {
		return nil, fmt.Errorf("missing %s content", key)
	}
	start += idx
	end := matchDictEnd(dict, start)
	if end == -1 {
		return nil, fmt.Errorf("unterminated %s dict", key)
	}
	out := make([]byte, 0, len(dict)-((end+2)-start)+len(sub))
	out = append(out, dict[:start]...)
	out = append(out, sub...)
	out = append(out, dict[end+2:]...)
	return out, nil
}

func replaceTopLevelArray(dict []byte, key string, value []byte) []byte {
	idx := findTopLevelKey(dict, key)
	if idx == -1 {
		insert := []byte(key + " ")
		out := make([]byte, 0, len(dict)+len(insert)+len(value)+1)
		if len(dict) >= 2 && dict[0] == '<' && dict[1] == '<' {
			out = append(out, dict[:2]...)
			out = append(out, ' ')
			out = append(out, insert...)
			out = append(out, value...)
			out = append(out, ' ')
			out = append(out, dict[2:]...)
			return out
		}
		out = append(out, dict...)
		out = append(out, ' ')
		out = append(out, insert...)
		out = append(out, value...)
		return out
	}
	arrStart := bytes.IndexByte(dict[idx:], '[')
	if arrStart == -1 {
		return dict
	}
	arrStart += idx
	arrEnd := bytes.IndexByte(dict[arrStart:], ']')
	if arrEnd == -1 {
		return dict
	}
	arrEnd += arrStart
	out := make([]byte, 0, len(dict)-(arrEnd+1-arrStart)+len(value))
	out = append(out, dict[:arrStart]...)
	out = append(out, value...)
	out = append(out, dict[arrEnd+1:]...)
	return out
}

func replaceTopLevelName(dict []byte, key string, value string) []byte {
	idx := findTopLevelKey(dict, key)
	if idx == -1 {
		if len(dict) >= 2 && dict[0] == '<' && dict[1] == '<' {
			insert := []byte(key + " " + value + " ")
			out := make([]byte, 0, len(dict)+len(insert))
			out = append(out, dict[:2]...)
			out = append(out, ' ')
			out = append(out, insert...)
			out = append(out, dict[2:]...)
			return out
		}
		return dict
	}
	start := idx + len(key)
	name, nameStart, nameEnd := parseName(dict, start)
	if name == "" || nameStart == -1 || nameEnd == -1 {
		return dict
	}
	out := make([]byte, 0, len(dict)-len(name)+len(value))
	out = append(out, dict[:nameStart]...)
	out = append(out, []byte(value)...)
	out = append(out, dict[nameEnd:]...)
	return out
}

func findTopLevelKey(dict []byte, key string) int {
	depth := 0
	for i := 0; i+1 < len(dict); i++ {
		if dict[i] == '<' && dict[i+1] == '<' {
			depth++
			i++
			continue
		}
		if dict[i] == '>' && dict[i+1] == '>' {
			depth--
			i++
			continue
		}
		if depth == 1 && bytes.HasPrefix(dict[i:], []byte(key)) {
			return i
		}
	}
	return -1
}

func collectOCGPrintStates(data []byte) ([]int, []int, error) {
	re := regexp.MustCompile(`(?m)^(\d+)\s+\d+\s+obj`)
	indices := re.FindAllSubmatchIndex(data, -1)
	if len(indices) == 0 {
		return nil, nil, errors.New("no objects found")
	}
	var on []int
	var off []int
	for i, m := range indices {
		if len(m) < 4 {
			continue
		}
		objNum, err := strconv.Atoi(string(data[m[2]:m[3]]))
		if err != nil {
			continue
		}
		start := m[0]
		end := len(data)
		if i+1 < len(indices) {
			end = indices[i+1][0]
		}
		endObj := bytes.Index(data[start:end], []byte("endobj"))
		if endObj == -1 {
			continue
		}
		obj := data[start : start+endObj]
		if !bytes.Contains(obj, []byte("/Type /OCG")) {
			continue
		}
		state := extractPrintState(obj)
		switch state {
		case "/ON":
			on = append(on, objNum)
		case "/OFF":
			off = append(off, objNum)
		default:
			return nil, nil, fmt.Errorf("missing /PrintState for OCG %d", objNum)
		}
	}
	if len(on) == 0 && len(off) == 0 {
		return nil, nil, errors.New("no OCG print states found")
	}
	sort.Ints(on)
	sort.Ints(off)
	return on, off, nil
}

func extractPrintState(obj []byte) string {
	idx := bytes.Index(obj, []byte("/PrintState"))
	if idx == -1 {
		return ""
	}
	state, _, _ := parseName(obj, idx+len("/PrintState"))
	return state
}

func formatObjRefArray(objs []int) []byte {
	var b bytes.Buffer
	b.WriteByte('[')
	for _, obj := range objs {
		fmt.Fprintf(&b, "%d 0 R ", obj)
	}
	b.WriteByte(']')
	return b.Bytes()
}

func parseName(data []byte, start int) (string, int, int) {
	i := start
	for i < len(data) && (data[i] == ' ' || data[i] == '\n' || data[i] == '\r' || data[i] == '\t') {
		i++
	}
	if i >= len(data) || data[i] != '/' {
		return "", -1, -1
	}
	j := i + 1
	for j < len(data) && isNameChar(data[j]) {
		j++
	}
	return string(data[i:j]), i, j
}

func rebuildXref(data []byte) ([]byte, error) {
	startxref := bytes.LastIndex(data, []byte("startxref"))
	if startxref == -1 {
		return nil, errors.New("missing startxref")
	}
	xrefIdx := bytes.LastIndex(data[:startxref], []byte("xref"))
	if xrefIdx == -1 {
		return nil, errors.New("missing xref")
	}
	body := data[:xrefIdx]
	trailerDict, err := extractTrailerDict(data[xrefIdx:])
	if err != nil {
		return nil, err
	}
	offsets, maxObj, err := collectObjectOffsets(body)
	if err != nil {
		return nil, err
	}
	root := extractTrailerRef(trailerDict, "/Root")
	info := extractTrailerRef(trailerDict, "/Info")
	id := extractTrailerID(trailerDict)

	var out bytes.Buffer
	out.Write(body)
	xrefOffset := out.Len()
	out.WriteString("xref\n")
	out.WriteString(fmt.Sprintf("0 %d\n", maxObj+1))
	out.WriteString("0000000000 65535 f \n")
	for i := 1; i <= maxObj; i++ {
		if off, ok := offsets[i]; ok {
			out.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
		} else {
			out.WriteString("0000000000 65535 f \n")
		}
	}
	out.WriteString("trailer\n<< ")
	out.WriteString(fmt.Sprintf("/Size %d", maxObj+1))
	if root != "" {
		out.WriteString(" /Root ")
		out.WriteString(root)
	}
	if info != "" {
		out.WriteString(" /Info ")
		out.WriteString(info)
	}
	if id != "" {
		out.WriteString(" /ID ")
		out.WriteString(id)
	}
	out.WriteString(" >>\nstartxref\n")
	out.WriteString(strconv.Itoa(xrefOffset))
	out.WriteString("\n%%EOF\n")
	return out.Bytes(), nil
}

func collectObjectOffsets(data []byte) (map[int]int, int, error) {
	re := regexp.MustCompile(`(?m)^(\d+)\s+\d+\s+obj`)
	matches := re.FindAllSubmatchIndex(data, -1)
	if len(matches) == 0 {
		return nil, 0, errors.New("no objects found")
	}
	offsets := make(map[int]int, len(matches))
	maxObj := 0
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		objNum, err := strconv.Atoi(string(data[m[2]:m[3]]))
		if err != nil {
			continue
		}
		offsets[objNum] = m[0]
		if objNum > maxObj {
			maxObj = objNum
		}
	}
	if maxObj == 0 {
		return nil, 0, errors.New("invalid object table")
	}
	return offsets, maxObj, nil
}

func extractTrailerDict(data []byte) (string, error) {
	trailerIdx := bytes.Index(data, []byte("trailer"))
	if trailerIdx == -1 {
		return "", errors.New("missing trailer")
	}
	start := bytes.Index(data[trailerIdx:], []byte("<<"))
	if start == -1 {
		return "", errors.New("missing trailer dict")
	}
	start += trailerIdx
	end := matchDictEnd(data, start)
	if end == -1 {
		return "", errors.New("unterminated trailer dict")
	}
	return string(data[start : end+2]), nil
}

func extractTrailerRef(dict string, key string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(key) + `\s+(\d+\s+\d+\s+R)`)
	m := re.FindStringSubmatch(dict)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func extractTrailerID(dict string) string {
	re := regexp.MustCompile(`/ID\s*(\[[^\]]+\])`)
	m := re.FindStringSubmatch(dict)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func isNameChar(b byte) bool {
	switch b {
	case ' ', '\n', '\r', '\t', '/', '<', '>', '[', ']', '(', ')':
		return false
	default:
		return true
	}
}
