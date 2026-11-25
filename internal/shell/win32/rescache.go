//go:build windows

package win32

import (
	"cmp"
	"slices"

	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
)

// The device's drawing resources (review P3): solid brushes by color,
// gradient stop collections by stops, gradient brushes by stops and end
// points, and stroke styles by style, kept across paints on the gpu, so a
// steady frame creates none of them (they were per canvas before, and a
// canvas per paint). They are device-bound and go with the device
// (gpu.release, also on a lost device).
//
// Bounds (resCache.trim, after every paint's EndDraw): an entry unused for
// resKeep paints goes (4 s of a 60 Hz animation, as the shadow cache's);
// beyond resMax entries of a kind the least recently used go. A stop
// collection is only needed to make gradient brushes, so it goes as soon
// as no cached gradient brush uses it, and every gradient brush hit marks
// its collection used too.
//
// Lookups don't allocate: the color and stroke keys are plain values (a
// custom dash array's key string is interned), and a gradient's stops key
// is built into a scratch buffer and looked up as m[string(buf)], which Go
// does without a copy; the gradient brush's key holds the collection
// entry. The stop and dash arrays are built only on a miss.

const (
	resKeep = 240
	resMax  = 256
)

// resEntry is one cached resource and the last paint that used it.
type resEntry[T any] struct {
	r     T
	used  uint64
	refs  int                                   // a stop collection: the cached gradient brushes over it
	stops *resEntry[d2d.GradientStopCollection] // a gradient brush: its collection
}

// gradKey identifies a cached gradient brush.
type gradKey struct {
	stops      *resEntry[d2d.GradientStopCollection]
	start, end d2d.Point2F
}

// strokeKey identifies a cached stroke style.
type strokeKey struct {
	cap        d2d.CapStyle
	join       d2d.LineJoin
	miter      float32
	offset     float32
	dashes     string // the custom dashes' float32 bits, interned
	dashStyled bool
}

// resCache is the device's drawing resources.
type resCache struct {
	paint   uint64 // counts the canvases (newCanvas)
	solids  map[d2d.ColorF]*resEntry[d2d.SolidColorBrush]
	stops   map[string]*resEntry[d2d.GradientStopCollection]
	grads   map[gradKey]*resEntry[d2d.LinearGradientBrush]
	strokes map[strokeKey]*resEntry[d2d.StrokeStyle]
	dashes  map[string]string // interned strokeKey.dashes

	key     []byte             // scratch: a stops or dashes key
	stopBuf []d2d.GradientStop // scratch: the stops of a new collection
	dashBuf []float32          // scratch: the dashes of a new stroke style
	made    int                // resources created (tests)
}

// begin starts a paint (newCanvas).
func (rc *resCache) begin() {
	rc.paint++
	if rc.solids == nil {
		rc.solids = map[d2d.ColorF]*resEntry[d2d.SolidColorBrush]{}
		rc.stops = map[string]*resEntry[d2d.GradientStopCollection]{}
		rc.grads = map[gradKey]*resEntry[d2d.LinearGradientBrush]{}
		rc.strokes = map[strokeKey]*resEntry[d2d.StrokeStyle]{}
		rc.dashes = map[string]string{}
	}
}

// hit marks e used by this paint and returns its resource.
func hit[T any](rc *resCache, e *resEntry[T]) T {
	e.used = rc.paint
	return e.r
}

// add caches r under k in m, used by this paint.
func add[K comparable, T any](rc *resCache, m map[K]*resEntry[T], k K, r T) *resEntry[T] {
	e := &resEntry[T]{r: r, used: rc.paint}
	m[k] = e
	rc.made++
	return e
}

// trim drops what the bounds rule out (the file comment). Gradient
// brushes go before the collections they hold.
func (rc *resCache) trim(last func(what string, p *com.Ptr)) {
	trimKind(rc, rc.solids, func(e *resEntry[d2d.SolidColorBrush]) {
		last("ID2D1SolidColorBrush", &e.r.Ptr)
	})
	trimKind(rc, rc.grads, func(e *resEntry[d2d.LinearGradientBrush]) {
		last("ID2D1LinearGradientBrush", &e.r.Ptr)
		e.stops.refs--
	})
	for k, e := range rc.stops {
		if e.refs == 0 {
			last("ID2D1GradientStopCollection", &e.r.Ptr)
			delete(rc.stops, k)
		}
	}
	trimKind(rc, rc.strokes, func(e *resEntry[d2d.StrokeStyle]) {
		last("ID2D1StrokeStyle1", &e.r.Ptr)
	})
	if len(rc.dashes) > resMax {
		clear(rc.dashes) // the keys keep their strings; equal ones compare equal
	}
}

// trimKind drops m's stale entries, then its least recently used beyond
// resMax, through drop.
func trimKind[K comparable, T any](rc *resCache, m map[K]*resEntry[T], drop func(e *resEntry[T])) {
	del := func(k K, e *resEntry[T]) {
		drop(e)
		delete(m, k)
	}
	for k, e := range m {
		if e.used+resKeep <= rc.paint {
			del(k, e)
		}
	}
	if len(m) <= resMax {
		return
	}
	type aged struct {
		k    K
		used uint64
	}
	all := make([]aged, 0, len(m))
	for k, e := range m {
		all = append(all, aged{k, e.used})
	}
	slices.SortFunc(all, func(a, b aged) int { return cmp.Compare(a.used, b.used) })
	for _, a := range all[:len(all)-resMax] {
		del(a.k, m[a.k])
	}
}

// release frees every entry (the device's release).
func (rc *resCache) release(last func(what string, p *com.Ptr)) {
	for k, e := range rc.grads {
		last("ID2D1LinearGradientBrush", &e.r.Ptr)
		delete(rc.grads, k)
	}
	for k, e := range rc.stops {
		last("ID2D1GradientStopCollection", &e.r.Ptr)
		delete(rc.stops, k)
	}
	for k, e := range rc.solids {
		last("ID2D1SolidColorBrush", &e.r.Ptr)
		delete(rc.solids, k)
	}
	for k, e := range rc.strokes {
		last("ID2D1StrokeStyle1", &e.r.Ptr)
		delete(rc.strokes, k)
	}
	clear(rc.dashes)
}

// len returns the number of cached entries of each kind (tests).
func (rc *resCache) len() (solids, stops, grads, strokes int) {
	return len(rc.solids), len(rc.stops), len(rc.grads), len(rc.strokes)
}

// usedNow returns the number of m's entries this paint used (tests).
func usedNow[K comparable, T any](rc *resCache, m map[K]*resEntry[T]) int {
	n := 0
	for _, e := range m {
		if e.used == rc.paint {
			n++
		}
	}
	return n
}
