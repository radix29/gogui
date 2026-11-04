//go:build windows

package win32

import (
	"errors"
	"log"
	"math"

	"github.com/radix29/gogui/assets/fonts"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/fake"
	"github.com/radix29/gogui/internal/shell/win32/dwrite"
)

// The text system (PLAN.md S2.5d, S2.5e): render.TextSystem on DirectWrite.
//
// Layouts are made in DIPs, which are the logical points, with the natural
// measuring mode, so measurement is the same at every scale and on every
// monitor; crispness comes from the device context's DPI (S2.5f). A layout
// is measured once, in NewLayout, and kept as plain Go data: its native
// IDWriteTextLayout is released right away, because TextLayout has no
// Release and nothing may rely on finalizers (PLAN.md §7.2). The line and
// cluster geometry is read then too (textgeom.go), so HitTest, CaretRect
// and SelectionRects make no native calls; drawing (S2.5f) rebuilds a
// native layout from the stored text, font and width. Only the text formats
// are cached, per resolved font, and live until the shell shuts down.
//
// The embedded fonts (S2.5g) are the .ttf/.otf files of assets/fonts: one
// in-memory collection is built from all of them with the first layout.
// When there are none, or any of them fails to load, there is no embedded
// collection and the UI font is Segoe UI (the reason is logged once with
// GOGUI_DEBUG=1).

const (
	// noWrapWidth is the width of the layout box of text that does not wrap.
	noWrapWidth = 1e7
	// minFontSize and maxFontSize clamp the sizes DirectWrite is given.
	minFontSize = 0.1
	maxFontSize = 2048
	// maxFormats is the size of the format cache before it is emptied.
	maxFormats = 128
	// textLocale is the BCP 47 name of every format.
	textLocale = "en-us"
)

// Font families the system fonts resolve to.
const (
	familyUIWin11  = "Segoe UI Variable Text"
	familyUIWin10  = "Segoe UI"
	familyMono     = "Consolas"
	familyMonoLast = "Courier New"
	familyEmbedded = "Inter"
)

// errTextReleased: a layout is drawn after its text system was released.
var errTextReleased = errors.New("win32: text: the text system is released")

// textSystem implements render.TextSystem. It is used on the UI thread. The
// DirectWrite objects are created with the first layout; when that fails, or
// after release, layouts come from shell/fake's fixed-width metrics (the
// first failure is logged), so text never takes the application down.
type textSystem struct {
	q *mainthread.Queue

	inited, closed, failed bool
	factory                dwrite.Factory
	system                 dwrite.FontCollection
	fonts                  *dwrite.MemoryFonts   // the embedded fonts; nil when there are none or they failed
	embedded               dwrite.FontCollection // fonts.Collection, or nil
	uiFamily               string                // the system UI family
	formats                map[fontKey]dwrite.TextFormat

	// fontFiles returns the files of the embedded collection (fonts.Files;
	// the tests' hook), embeddedUI is the family the empty family resolves
	// to when the collection has it (familyEmbedded), and debugf logs why
	// the embedded fonts are not used (log.Printf with GOGUI_DEBUG=1, else
	// nil).
	fontFiles  func() []fonts.File
	embeddedUI string
	debugf     func(format string, args ...any)
}

var _ render.TextSystem = (*textSystem)(nil)

func newTextSystem(q *mainthread.Queue) *textSystem {
	ts := &textSystem{q: q, fontFiles: fonts.Files, embeddedUI: familyEmbedded}
	if debugChecks {
		ts.debugf = log.Printf
	}
	return ts
}

// fontKey identifies a resolved font.
type fontKey struct {
	family   string
	embedded bool // family is in the embedded collection
	size     float32
	weight   dwrite.FontWeight
	italic   bool
}

// init creates the factory, looks up the system UI family and builds the
// embedded collection.
func (ts *textSystem) init() error {
	if ts.inited {
		return nil
	}
	f, err := dwrite.CreateFactory()
	if err != nil {
		return err
	}
	sys, err := f.SystemFontCollection(false)
	if err != nil {
		f.Release()
		return err
	}
	ts.factory, ts.system = f, sys
	ts.uiFamily = ts.firstInstalled(familyUIWin11, familyUIWin10)
	ts.formats = make(map[fontKey]dwrite.TextFormat)
	ts.loadEmbedded()
	ts.inited = true
	return nil
}

// loadEmbedded builds the embedded collection from ts.fontFiles. Without
// files, or when any of them fails, there is none (said once by debugf:
// init runs once per text system).
func (ts *textSystem) loadEmbedded() {
	files := ts.fontFiles()
	if len(files) == 0 {
		if ts.debugf != nil {
			ts.debugf("win32: text: no embedded fonts in assets/fonts, using %s", ts.uiFamily)
		}
		return
	}
	data := make([][]byte, len(files))
	names := make([]string, len(files))
	for i, f := range files {
		data[i], names[i] = f.Data, f.Name
	}
	mf, err := ts.factory.CreateMemoryFonts(data)
	if err != nil {
		if ts.debugf != nil {
			ts.debugf("win32: text: embedded fonts %v not used, using %s: %v", names, ts.uiFamily, err)
		}
		return
	}
	ts.fonts, ts.embedded = mf, mf.Collection
}

// firstInstalled returns the first of names the system collection has, or
// the last one.
func (ts *textSystem) firstInstalled(names ...string) string {
	for _, n := range names {
		if hasFamily(ts.system, n) {
			return n
		}
	}
	return names[len(names)-1]
}

// hasFamily reports whether c (which may be nil) has the family name.
func hasFamily(c dwrite.FontCollection, name string) bool {
	if c.IsNil() {
		return false
	}
	_, ok, err := c.FindFamilyName(name)
	return err == nil && ok
}

// release gives back the formats, the collections and the factory, and
// reports a text format that still has references to leak. The shared
// system collection and factory are not checked: DirectWrite's caches keep
// references of their own on them (S2.5c). Later layouts come from the fake
// metrics.
func (ts *textSystem) release(leak func(what string, left uint32)) {
	ts.closed = true
	if !ts.inited {
		return
	}
	for _, tf := range ts.formats {
		if n := tf.Release(); n != 0 {
			leak("text format", n)
		}
	}
	ts.formats = nil
	ts.fonts.Release() // after the formats: the loader must outlive them
	ts.fonts, ts.embedded = nil, dwrite.FontCollection{}
	ts.system.Release()
	ts.factory.Release()
	ts.inited = false
}

// resolve maps f to a font that exists. An empty family is the UI font
// (Inter when the embedded collection has it, else Segoe UI Variable or
// Segoe UI) or, when monospaced, Consolas. A named family is looked up in
// the embedded collection, then the system one, and falls back to the UI
// font. DirectWrite's own fallback covers glyphs a font lacks.
func (ts *textSystem) resolve(f render.Font) fontKey {
	k := fontKey{
		size:   float32(min(max(f.EffectiveSize(driver.ControlSizeRegular), minFontSize), maxFontSize)),
		weight: dwrite.FontWeight(min(max(int(f.EffectiveWeight()), 1), 999)),
		italic: f.Italic,
	}
	switch {
	case f.Family != "" && hasFamily(ts.embedded, f.Family):
		k.family, k.embedded = f.Family, true
	case f.Family != "" && hasFamily(ts.system, f.Family):
		k.family = f.Family
	case f.Family == "" && f.Monospaced:
		k.family = ts.firstInstalled(familyMono, familyMonoLast)
	case hasFamily(ts.embedded, ts.embeddedUI):
		k.family, k.embedded = ts.embeddedUI, true
	default:
		k.family = ts.uiFamily
	}
	return k
}

// format returns the cached text format of k.
func (ts *textSystem) format(k fontKey) (dwrite.TextFormat, error) {
	if tf, ok := ts.formats[k]; ok {
		return tf, nil
	}
	if len(ts.formats) >= maxFormats { // layouts hold no native objects, so all can go
		for key, tf := range ts.formats {
			tf.Release()
			delete(ts.formats, key)
		}
	}
	coll, style := ts.system, dwrite.FONT_STYLE_NORMAL
	if k.embedded {
		coll = ts.embedded
	}
	if k.italic {
		style = dwrite.FONT_STYLE_ITALIC
	}
	tf, err := ts.factory.CreateTextFormat(k.family, coll, k.weight, style, dwrite.FONT_STRETCH_NORMAL, k.size, textLocale)
	if err != nil {
		return dwrite.TextFormat{}, err
	}
	ts.formats[k] = tf
	return tf, nil
}

// NewLayout implements render.TextSystem.
func (ts *textSystem) NewLayout(text string, font render.Font, maxWidth float64) render.TextLayout {
	ts.q.Check("TextSystem.NewLayout")
	if !ts.closed {
		l, err := ts.layout(text, font, maxWidth)
		if err == nil {
			return l
		}
		if !ts.failed {
			ts.failed = true
			log.Printf("win32: text: DirectWrite failed, using fixed-width metrics: %v", err)
		}
	}
	return fake.NewTextSystem().NewLayout(text, font, maxWidth)
}

// wrapWidth returns the layout box width for maxWidth and whether the text
// wraps: only a positive, finite width does (NaN fails the first test).
func wrapWidth(maxWidth float64) (float32, bool) {
	if !(maxWidth > 0) || math.IsInf(maxWidth, 1) {
		return noWrapWidth, false
	}
	return float32(min(maxWidth, noWrapWidth)), true
}

// native makes the DirectWrite layout of u in font, wrapped at maxWidth
// (wrapWidth), and returns it with its format (cached; not to release).
// NewLayout measures it, the canvas draws it (S2.5f).
func (ts *textSystem) native(u []uint16, font render.Font, maxWidth float64) (dwrite.TextLayout, dwrite.TextFormat, error) {
	tf, err := ts.format(ts.resolve(font))
	if err != nil {
		return dwrite.TextLayout{}, dwrite.TextFormat{}, err
	}
	w, wrap := wrapWidth(maxWidth)
	dl, err := ts.factory.CreateTextLayout(u, tf, w, noWrapWidth)
	if err != nil {
		return dwrite.TextLayout{}, dwrite.TextFormat{}, err
	}
	mode := dwrite.WORD_WRAPPING_NO_WRAP
	if wrap {
		mode = dwrite.WORD_WRAPPING_WRAP
	}
	// The format is shared, so the wrapping is set on the layout only.
	if err := dl.SetWordWrapping(mode); err != nil {
		dl.Release()
		return dwrite.TextLayout{}, dwrite.TextFormat{}, err
	}
	return dl, tf, nil
}

func (ts *textSystem) layout(text string, font render.Font, maxWidth float64) (*layout, error) {
	if err := ts.init(); err != nil {
		return nil, err
	}
	u, idx := utf16Of(text)
	dl, tf, err := ts.native(u, font, maxWidth)
	if err != nil {
		return nil, err
	}
	defer dl.Release()
	m, err := dl.Metrics()
	if err != nil {
		return nil, err
	}
	lines, err := dl.LineMetrics()
	if err != nil {
		return nil, err
	}
	clusters, err := dl.ClusterMetrics()
	if err != nil {
		return nil, err
	}
	var box func(pos uint32) (float32, float32, error)
	if m.MaxBidiReorderingDepth > 1 { // mixed directions: DirectWrite places each cluster
		box = func(pos uint32) (float32, float32, error) {
			x, _, hm, err := dl.HitTestTextPosition(pos, false)
			if hm.Length == 0 { // the text's end or an empty line: a caret, no box
				return x, 0, err
			}
			return hm.Left, hm.Width, err
		}
	}
	l := &layout{ts: ts, text: text, font: font, maxWidth: maxWidth, idx: idx, lines: max(int(m.LineCount), 1),
		size: render.Size{W: float64(m.Width), H: float64(m.Height)}}
	if l.geom, err = newGeometry(len(u), lines, clusters, box, newlineWidth(tf.FontSize())); err != nil {
		return nil, err
	}
	l.baseline, l.lineHeight = float64(lines[0].Baseline), float64(lines[0].Height)
	return l, nil
}

// layout is a render.TextLayout: the measurements of one DirectWrite
// layout, kept as Go data (see the top of this file).
type layout struct {
	ts         *textSystem // the system that made it (DrawText rebuilds the native layout there)
	text       string
	font       render.Font
	maxWidth   float64  // as given to NewLayout
	idx        indexMap // UTF-8 ↔ UTF-16 of text
	geom       geometry // lines and clusters for the queries (textgeom.go)
	size       render.Size
	baseline   float64 // of the first line
	lineHeight float64 // of the first line
	lines      int
}

var _ render.TextLayout = (*layout)(nil)

func (l *layout) Text() string      { return l.text }
func (l *layout) Font() render.Font { return l.font }
func (l *layout) Size() render.Size { return l.size }
func (l *layout) Baseline() float64 { return l.baseline }
func (l *layout) LineCount() int    { return l.lines }

// HitTest implements render.TextLayout.
func (l *layout) HitTest(p render.Point) (int, bool) {
	u, trailing := l.geom.hitTest(p.X, p.Y)
	return l.idx.byteOffset(u), trailing
}

// CaretRect implements render.TextLayout.
func (l *layout) CaretRect(index int) render.Rect {
	return l.geom.caret(l.geom.clusterStart(l.idx.unit(index)))
}

// SelectionRects implements render.TextLayout.
func (l *layout) SelectionRects(start, end int) []render.Rect {
	a, b := l.geom.clusterStart(l.idx.unit(start)), l.geom.clusterStart(l.idx.unit(end))
	return l.geom.selection(min(a, b), max(a, b))
}

// normalize returns index clamped to the text and moved to its cluster's
// start, as a UTF-8 offset.
func (l *layout) normalize(index int) int {
	return l.idx.byteOffset(l.geom.clusterStart(l.idx.unit(index)))
}

// native rebuilds the layout's DirectWrite layout for drawing (S2.5f); the
// caller releases it. It fails once the text system is released.
func (l *layout) native() (dwrite.TextLayout, error) {
	if l.ts.closed || !l.ts.inited {
		return dwrite.TextLayout{}, errTextReleased
	}
	u, _ := utf16Of(l.text)
	dl, _, err := l.ts.native(u, l.font, l.maxWidth)
	return dl, err
}
