//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// pangoNative is the pangoLayouter over Pango (text.go): one PangoContext
// of the GTK thread's default PangoCairo font map. It is used on the GTK
// thread.
type pangoNative struct {
	fm  uintptr // PangoFontMap *, transfer none (owned by Pango, per thread)
	ctx uintptr // PangoContext *, the text system's reference
	// folded is the font map's family names in lower case, listed by the
	// first hasFamily that pango_font_map_get_family doesn't answer.
	folded map[string]bool
}

var _ pangoLayouter = (*pangoNative)(nil)

// openPango opens the text system's Pango (textSystem.open): the context,
// with glyph positions not rounded to whole pixels and metrics not hinted
// (cairo font options with CAIRO_HINT_METRICS_OFF; the default counts as
// hinted and rounds advances and line heights to whole logical pixels),
// so measurement doesn't depend on the scale (GTK's own contexts do the
// same with gtk-hint-font-metrics off, its default).
func openPango() (pangoLayouter, error) {
	fm := gtk.PangoCairo.FontMapGetDefault()
	if fm == 0 {
		return nil, errors.New("gtk4: text: pango_cairo_font_map_get_default returned NULL")
	}
	ctx := gtk.Pango.FontMapCreateContext(fm)
	if ctx == 0 {
		return nil, errors.New("gtk4: text: pango_font_map_create_context returned NULL")
	}
	gtk.Pango.ContextSetRoundGlyphPositions(ctx, gtk.False)
	opts := gtk.Cairo.FontOptionsCreate()
	gtk.Cairo.FontOptionsSetHintMetrics(opts, gtk.HintMetricsOff)
	gtk.PangoCairo.ContextSetFontOptions(ctx, opts) // copies opts
	gtk.Cairo.FontOptionsDestroy(opts)
	return &pangoNative{fm: fm, ctx: ctx}, nil
}

func (p *pangoNative) release() {
	if p.ctx != 0 {
		gtk.GObject.ObjectUnref(p.ctx)
		p.ctx = 0
	}
	p.folded = nil
}

// uiFamily reads gtk-font-name ("Cantarell 11") from the default
// GtkSettings through a GValue in g_malloc memory and returns its family.
func (p *pangoNative) uiFamily() string {
	st := gtk.Gtk.SettingsGetDefault()
	if st == 0 {
		return ""
	}
	v := gtk.GLib.Malloc0(unsafe.Sizeof(gtk.GValue{}))
	defer gtk.GLib.Free(v)
	gtk.GObject.ValueInit(v, gtk.TypeString)
	defer gtk.GObject.ValueUnset(v)
	gtk.GObject.ObjectGetProperty(st, "gtk-font-name", v)
	name := ffi.GoString(gtk.GObject.ValueGetString(v))
	if name == "" {
		return ""
	}
	d := gtk.Pango.FontDescriptionFromString(name)
	if d == 0 {
		return ""
	}
	defer gtk.Pango.FontDescriptionFree(d)
	return ffi.GoString(gtk.Pango.FontDescriptionGetFamily(d))
}

// hasFamily asks pango_font_map_get_family, then, as it may compare case
// sensitively, the font map's family names in lower case.
func (p *pangoNative) hasFamily(name string) bool {
	if p.ctx == 0 {
		return false
	}
	if gtk.Pango.FontMapGetFamily(p.fm, name) != 0 {
		return true
	}
	if p.folded == nil {
		p.folded = p.familyNames()
	}
	return p.folded[strings.ToLower(name)]
}

// familyNames lists the font map's families, in lower case.
func (p *pangoNative) familyNames() map[string]bool {
	var arr uintptr
	var n int32
	gtk.Pango.FontMapListFamilies(p.fm, &arr, &n)
	names := make(map[string]bool, max(n, 0))
	if arr == 0 {
		return names
	}
	defer gtk.GLib.Free(arr)
	for i := range uintptr(max(n, 0)) { // arr: n PangoFontFamily pointers in g_malloc memory
		f := ffi.ReadUintptr(arr, i*unsafe.Sizeof(uintptr(0)))
		names[strings.ToLower(ffi.GoString(gtk.Pango.FontFamilyGetName(f)))] = true
	}
	return names
}

// newLayout returns a PangoLayout of text in f wrapped at width (the
// caller unrefs it), or 0. NewLayout measures it; DrawText draws one
// (layout, S4.2i). Every paragraph is laid out left to right as its base
// direction (auto_dir off, as DirectWrite's default reading direction),
// so lines start at x = 0 also under a width.
func (p *pangoNative) newLayout(text string, f pangoFont, width int32) uintptr {
	if p.ctx == 0 {
		return 0
	}
	l := gtk.Pango.LayoutNew(p.ctx)
	if l == 0 {
		return 0
	}
	d := gtk.Pango.FontDescriptionNew()
	gtk.Pango.FontDescriptionSetFamily(d, f.family)
	gtk.Pango.FontDescriptionSetAbsoluteSize(d, f.size)
	gtk.Pango.FontDescriptionSetWeight(d, f.weight)
	style := gtk.PangoStyleNormal
	if f.italic {
		style = gtk.PangoStyleItalic
	}
	gtk.Pango.FontDescriptionSetStyle(d, style)
	gtk.Pango.LayoutSetFontDescription(l, d) // copies d
	gtk.Pango.FontDescriptionFree(d)
	gtk.Pango.LayoutSetAutoDir(l, gtk.False)
	gtk.Pango.LayoutSetWrap(l, gtk.PangoWrapWordChar)
	gtk.Pango.LayoutSetWidth(l, width)
	gtk.Pango.LayoutSetText(l, text, int32(len(text)))
	return l
}

// pangoLayout is a PangoLayout made for drawing (S4.2i); release drops
// the text system's reference.
type pangoLayout uintptr

func (l pangoLayout) release() { gtk.GObject.ObjectUnref(uintptr(l)) }

// fontTarget is a font map with the FcConfig it reads its fonts from.
type fontTarget struct{ fm, cfg uintptr }

// addedFonts are the font files added to each font map and FcConfig in
// this process. Both outlive a text system, so a later one (another Run,
// a test) adds only files that are new. A new FcConfig drops the app
// fonts (GTK calls FcInitReinitialize when fontconfig's files change, a
// font installed while the app runs), so a text system made after that
// adds them again; one made before keeps its Inter family, which Pango
// then substitutes. It is used on the GTK thread.
var addedFonts = map[fontTarget]map[string]bool{}

// addFonts adds the app font's files to the font map (appfont.go): with
// pango_font_map_add_font_file on Pango 1.56 and later, else as
// fontconfig application fonts of the map's FcConfig followed by
// pango_fc_font_map_config_changed, so the map sees them. The map's
// FcConfig is its own or, without one (GTK's default map), fontconfig's
// current one; both paths add to it.
func (p *pangoNative) addFonts(paths []string) error {
	if p.ctx == 0 {
		return errors.New("gtk4: text: the text system is released")
	}
	cfg := gtk.PangoFc.FontMapGetConfig(p.fm)
	if cfg == 0 {
		cfg = gtk.Fc.ConfigGetCurrent()
	}
	if cfg == 0 {
		return errors.New("gtk4: text: FcConfigGetCurrent returned NULL")
	}
	key := fontTarget{p.fm, cfg}
	done := addedFonts[key]
	if done == nil {
		done = make(map[string]bool)
		addedFonts[key] = done
	}
	var todo []string
	for _, path := range paths {
		if done[path] {
			continue
		}
		if strings.IndexByte(path, 0) >= 0 {
			return fmt.Errorf("gtk4: text: font path %q has a NUL byte", path)
		}
		todo = append(todo, path)
	}
	if len(todo) == 0 {
		return nil
	}
	if gtk.Pango.FontMapAddFontFile != nil {
		for _, path := range todo {
			var gerr uintptr
			if gtk.Pango.FontMapAddFontFile(p.fm, path, &gerr) == gtk.False {
				return fmt.Errorf("gtk4: text: pango_font_map_add_font_file(%s): %s", path, takeGError(gerr))
			}
			done[path] = true
		}
		return nil
	}
	var err error
	for _, path := range todo {
		if gtk.Fc.ConfigAppFontAddFile(cfg, path) == gtk.False {
			err = fmt.Errorf("gtk4: text: FcConfigAppFontAddFile(%s) failed", path)
			break
		}
		done[path] = true
	}
	gtk.PangoFc.FontMapConfigChanged(p.fm) // also after a failure: the files added so far
	return err
}

// takeGError returns the message of gerr (a GError *, or 0) and frees it.
func takeGError(gerr uintptr) string {
	if gerr == 0 {
		return "no error message"
	}
	defer gtk.GLib.ErrorFree(gerr)
	return ffi.GoString(ffi.ReadUintptr(gerr, unsafe.Offsetof(gtk.GError{}.Message)))
}

// layout returns the PangoLayout measure measured for the same arguments,
// for DrawText.
func (p *pangoNative) layout(text string, f pangoFont, width int32) (nativeLayout, error) {
	l := p.newLayout(text, f, width)
	if l == 0 {
		return nil, errors.New("gtk4: text: pango_layout_new failed")
	}
	return pangoLayout(l), nil
}

// measure lays text out and reads its extents and its geometry
// (textgeom.go) in linear time: the lines from one pass of the layout's
// iterator, the character boxes from a second (pango_layout_index_to_pos
// per cluster would be quadratic: it finds the line and the character
// offset from the text's start each time), and the clusters from the
// cursor positions of its log attrs.
func (p *pangoNative) measure(text string, f pangoFont, width int32) (pangoMetrics, error) {
	l := p.newLayout(text, f, width)
	if l == 0 {
		return pangoMetrics{}, errors.New("gtk4: text: pango_layout_new failed")
	}
	defer gtk.GObject.ObjectUnref(l)
	var m pangoMetrics
	gtk.Pango.LayoutGetSize(l, &m.width, &m.height)
	m.baseline = gtk.Pango.LayoutGetBaseline(l)
	var err error
	if m.lines, err = layoutLines(l); err != nil {
		return pangoMetrics{}, err
	}
	boxes, err := charBoxes(l, len(text))
	if err != nil {
		return pangoMetrics{}, err
	}
	if m.clusters, err = layoutClusters(l, text, m.lines, boxes); err != nil {
		return pangoMetrics{}, err
	}
	return m, nil
}

// layoutLines reads the lines of l from its iterator. A line's end x is
// relative to the line, so its logical x is added (0 for gogui's
// left-aligned LTR-based layouts, but kept in layout coordinates as the
// character boxes are).
func layoutLines(l uintptr) ([]pangoLine, error) {
	it := gtk.Pango.LayoutGetIter(l)
	if it == 0 {
		return nil, errors.New("gtk4: text: pango_layout_get_iter returned NULL")
	}
	defer gtk.Pango.LayoutIterFree(it)
	var lines []pangoLine
	for {
		line := gtk.Pango.LayoutIterGetLineReadonly(it)
		if line == 0 {
			return nil, errors.New("gtk4: text: a layout line is NULL")
		}
		var pl pangoLine
		var logical gtk.PangoRectangle
		pl.start = gtk.Pango.LayoutLineGetStartIndex(line)
		pl.length = gtk.Pango.LayoutLineGetLength(line)
		gtk.Pango.LayoutIterGetLineYrange(it, &pl.top, &pl.bottom)
		pl.baseline = gtk.Pango.LayoutIterGetBaseline(it)
		gtk.Pango.LayoutIterGetLineExtents(it, nil, &logical)
		gtk.Pango.LayoutLineIndexToX(line, pl.start+pl.length, gtk.False, &pl.endX)
		pl.endX += logical.X
		lines = append(lines, pl)
		if gtk.Pango.LayoutIterNextLine(it) == gtk.False {
			return lines, nil
		}
	}
}

// charBox is the logical box of one character, in layout coordinates.
type charBox struct {
	left, width int32 // width ≥ 0
	rtl, ok     bool  // ok: the character is in a run (a paragraph delimiter is not)
}

// charBoxes returns the box of every character of l, indexed by its first
// byte (n bytes of text), from one pass of the layout's iterator; the
// direction is its run's bidi level.
func charBoxes(l uintptr, n int) ([]charBox, error) {
	it := gtk.Pango.LayoutGetIter(l)
	if it == 0 {
		return nil, errors.New("gtk4: text: pango_layout_get_iter returned NULL")
	}
	defer gtk.Pango.LayoutIterFree(it)
	boxes := make([]charBox, n)
	for {
		if run := gtk.Pango.LayoutIterGetRunReadonly(it); run != 0 { // NULL at a line's end
			if i := int(gtk.Pango.LayoutIterGetIndex(it)); i >= 0 && i < n {
				var r gtk.PangoRectangle
				gtk.Pango.LayoutIterGetCharExtents(it, &r)
				if r.Width < 0 {
					r.X, r.Width = r.X+r.Width, -r.Width
				}
				boxes[i] = charBox{left: r.X, width: r.Width, rtl: gtk.RunLevel(run)&1 != 0, ok: true}
			}
		}
		if gtk.Pango.LayoutIterNextChar(it) == gtk.False {
			return boxes, nil
		}
	}
}

// layoutClusters returns the clusters of l, the layout of text: one per
// cursor position before the text's end and one at every line's start
// (Pango's line starts are cursor positions, but the geometry relies on
// it), each the union of its characters' boxes, in the direction of its
// first. A cluster without a box (a paragraph delimiter) sits, without
// width, at the end of the one before it.
func layoutClusters(l uintptr, text string, lines []pangoLine, boxes []charBox) ([]pangoCluster, error) {
	var n int32
	attrs := gtk.Pango.LayoutGetLogAttrsReadonly(l, &n)
	chars := utf8.RuneCountInString(text)
	if attrs == 0 {
		return nil, errors.New("gtk4: text: pango_layout_get_log_attrs_readonly returned NULL")
	}
	if int(n) != chars+1 {
		return nil, fmt.Errorf("gtk4: text: %d log attrs for %d characters", n, chars)
	}
	var (
		cl       []pangoCluster
		cur      pangoCluster
		lo, hi   int32 // cur's extent so far
		dir, has bool  // cur's direction, and whether it has a box yet
		prevEnd  int32 // the trailing edge of the cluster before cur
		li       int   // the next line whose start may begin a cluster
	)
	flush := func() {
		switch {
		case !has:
			cur.x, cur.width = prevEnd, 0
		case dir:
			cur.x, cur.width, prevEnd = hi, lo-hi, lo
		default:
			cur.x, cur.width, prevEnd = lo, hi-lo, hi
		}
		cl = append(cl, cur)
	}
	i := 0
	for c := range chars { // attrs: n PangoLogAttrs in the layout's memory
		a := uint32(ffi.ReadInt32(attrs, uintptr(c)*gtk.LogAttrSize))
		lineStart := false
		for li < len(lines) && int(lines[li].start) <= i {
			lineStart = lineStart || int(lines[li].start) == i
			li++
		}
		if c == 0 || a&gtk.LogAttrIsCursorPosition != 0 || lineStart {
			if c > 0 {
				flush()
			}
			cur, has = pangoCluster{start: int32(i), white: a&gtk.LogAttrIsWhite != 0}, false
		}
		if b := boxes[i]; b.ok {
			if !has {
				lo, hi, dir, has = b.left, b.left+b.width, b.rtl, true
			} else {
				lo, hi = min(lo, b.left), max(hi, b.left+b.width)
			}
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
	}
	if chars > 0 {
		flush()
	}
	return cl, nil
}
