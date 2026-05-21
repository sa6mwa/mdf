package gofpdf

// appearanceAnnotation stores a page-local annotation backed by a custom
// appearance form XObject.
type appearanceAnnotation struct {
	page       int
	subtype    string
	x1         float64
	y1         float64
	x2         float64
	y2         float64
	bboxW      float64
	bboxH      float64
	flags      int
	appearance []byte
}

// AddAppearanceAnnotation adds a full custom-appearance annotation with an
// explicit subtype and annotation flags.
func (f *Fpdf) AddAppearanceAnnotation(page int, subtype string, flags int, x, y, w, h float64, appearance []byte) {
	if page <= 0 || page >= len(f.pages) || len(appearance) == 0 || subtype == "" {
		return
	}
	x1 := x * f.k
	yTop := pageHeightPtFor(f, page) - y*f.k
	wPt := w * f.k
	hPt := h * f.k
	f.pageAppearanceAnnots[page] = append(f.pageAppearanceAnnots[page], appearanceAnnotation{
		page:       page,
		subtype:    subtype,
		x1:         x1,
		y1:         yTop,
		x2:         x1 + wPt,
		y2:         yTop - hPt,
		bboxW:      wPt,
		bboxH:      hPt,
		flags:      flags,
		appearance: append([]byte(nil), appearance...),
	})
}

// PageContentBytes returns a copy of the raw content stream bytes for the page.
func (f *Fpdf) PageContentBytes(page int) []byte {
	if page <= 0 || page >= len(f.pages) || f.pages[page] == nil {
		return nil
	}
	return append([]byte(nil), f.pages[page].Bytes()...)
}

// pageHeightPtFor returns the target page height in points.
func pageHeightPtFor(f *Fpdf, page int) float64 {
	if pageSize, ok := f.pageSizes[page]; ok {
		return pageSize.Ht
	}
	if f.defOrientation == "L" {
		return f.defPageSize.Wd * f.k
	}
	return f.defPageSize.Ht * f.k
}

// putAppearanceAnnotationRefs emits indirect annotation references for a page.
func (f *Fpdf) putAppearanceAnnotationRefs(out *fmtBuffer, baseObj int, page int) {
	for i := range f.pageAppearanceAnnots[page] {
		out.printf("%d 0 R ", baseObj+4+i*2)
	}
}

// putAppearanceAnnotationObjects writes the appearance form and annotation
// objects associated with a page.
func (f *Fpdf) putAppearanceAnnotationObjects(page int) {
	for _, an := range f.pageAppearanceAnnots[page] {
		compressed := sliceCompress(an.appearance)
		f.newobj()
		appearanceObjNum := f.n
		f.outf("<< /Type /XObject /Subtype /Form /FormType 1 /BBox [0 0 %.2f %.2f] /Resources 2 0 R /Filter /FlateDecode /Length %d >>", an.bboxW, an.bboxH, len(compressed))
		f.putstream(compressed)
		f.out("endobj")

		f.newobj()
		f.outf("<< /Type /Annot /Subtype /%s /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0] /BS << /W 0 >> /F %d /AP << /N %d 0 R >> >>",
			an.subtype, an.x1, an.y1, an.x2, an.y2, an.flags, appearanceObjNum)
		f.out("endobj")
	}
}
