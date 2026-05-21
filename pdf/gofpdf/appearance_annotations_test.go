package gofpdf

import (
	"bytes"
	"testing"
)

func TestAddAppearanceAnnotationUsesLandscapePageHeight(t *testing.T) {
	pdf := New("L", "pt", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.AddAppearanceAnnotation(1, "Square", 0, 0, 0, 100, 50, []byte("q\nQ\n"))

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("output pdf: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("/Rect [0.00 595.28 100.00 545.28]")) {
		t.Fatalf("expected landscape annotation rect to use page height 595.28pt, got %q", out.Bytes())
	}
}

func TestAddAppearanceAnnotationUsesCustomPageHeightUnits(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPageFormat("P", SizeType{Wd: 100, Ht: 200})
	pdf.AddAppearanceAnnotation(1, "Square", 0, 0, 0, 10, 20, []byte("q\nQ\n"))

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("output pdf: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("/Rect [0.00 566.93 28.35 510.24]")) {
		t.Fatalf("expected custom-size annotation rect to use 200mm page height, got %q", out.Bytes())
	}
}

func TestAppearanceAnnotationsPreserveInternalLinkAndBookmarkTargets(t *testing.T) {
	pdf := New("P", "pt", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.AddAppearanceAnnotation(1, "Square", 0, 0, 0, 10, 10, []byte("q\nQ\n"))
	link := pdf.AddLink()
	pdf.Link(10, 10, 20, 10, link)
	pdf.AddPage()
	pdf.SetLink(link, 20, 2)
	pdf.Bookmark("Page 2", 0, 20)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("output pdf: %v", err)
	}
	want := []byte("/Dest [7 0 R /XYZ 0 821.89 null]")
	if count := bytes.Count(out.Bytes(), want); count != 2 {
		t.Fatalf("expected two page-2 destinations at object 7, got %d in %q", count, out.Bytes())
	}
	if bytes.Contains(out.Bytes(), []byte("/Dest [5 0 R /XYZ 0 821.89 null]")) {
		t.Fatalf("unexpected stale destination pointing at page-1 appearance object in %q", out.Bytes())
	}
}
