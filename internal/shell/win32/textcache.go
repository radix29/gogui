//go:build windows

package win32

import "github.com/radix29/gogui/internal/shell/win32/dwrite"

// The drawn-layout cache (PLAN.md S5.4k): native IDWriteTextLayouts kept
// across frames, so that text drawn again is not shaped again. NewLayout
// puts the native layout it measured there (a paragraph just edited or
// scrolled in is drawn right after it is made), and DrawText takes it
// from there, rebuilding it from the layout's text, font and width when it
// is gone. A layout keeps no native object itself (text.go), so the cache
// is the one owner: it is bounded (maxDrawn layouts, the least recently
// made or drawn first out) and is emptied when the text system is
// released, so no finalizer is needed. A layout drawn in the current frame
// is never evicted (Direct2D may hold it until EndDraw), so a frame that
// draws more than maxDrawn layouts holds them all until the next eviction
// after it. The keys are Go pointers in a Go map; nothing native refers to
// them.

// maxDrawn is the number of native layouts the cache keeps: a TextView's
// visible paragraphs with room for the rest of a window.
const maxDrawn = 256

// drawnLayout is a cached native layout of l and the frame that last drew
// it (0: none), linked into the cache's recency list (prev, next).
type drawnLayout struct {
	nl         dwrite.TextLayout
	l          *layout
	frame      uint64
	prev, next *drawnLayout
}

// drawnCache is the text system's drawn-layout cache. lru is the sentinel
// of the recency list (lru.next the most recently made or drawn layout,
// lru.prev the least; review P17: an eviction takes the tail instead of
// scanning the map). frame counts the frames that drew text (nextFrame);
// made counts the native layouts DrawText had to build and visits the
// entries evictions looked at (tests).
type drawnCache struct {
	m      map[*layout]*drawnLayout
	lru    drawnLayout
	frame  uint64
	made   int
	visits int
}

// nextFrame starts a frame that draws text: the layouts drawn from here on
// belong to it.
func (ts *textSystem) nextFrame() { ts.drawn.frame++ }

// drawable returns l's native layout for drawing in the current frame,
// from the cache or rebuilt (and cached, keepDrawn). The cache keeps the
// reference; the caller must not release it.
func (ts *textSystem) drawable(l *layout) (dwrite.TextLayout, error) {
	c := &ts.drawn
	e := c.m[l]
	if e == nil {
		nl, err := l.native()
		if err != nil {
			return dwrite.TextLayout{}, err
		}
		c.made++
		e = ts.keepDrawn(l, nl)
	}
	c.touch(e)
	e.frame = c.frame
	return e.nl, nil
}

// touch moves e to the front of the recency list.
func (c *drawnCache) touch(e *drawnLayout) {
	if c.lru.next == e {
		return
	}
	c.unlink(e)
	e.prev, e.next = &c.lru, c.lru.next
	e.next.prev = e
	c.lru.next = e
}

// unlink takes e out of the recency list (if it is in it).
func (c *drawnCache) unlink(e *drawnLayout) {
	if e.prev != nil {
		e.prev.next, e.next.prev = e.next, e.prev
	}
	e.prev, e.next = nil, nil
}

// keepDrawn caches nl as l's native layout, evicting the least recently
// used layouts not drawn in the current frame beyond maxDrawn, and returns
// its entry. The cache takes over the reference.
func (ts *textSystem) keepDrawn(l *layout, nl dwrite.TextLayout) *drawnLayout {
	c := &ts.drawn
	if c.m == nil {
		c.m = map[*layout]*drawnLayout{}
		c.lru.prev, c.lru.next = &c.lru, &c.lru
	}
	e := &drawnLayout{nl: nl, l: l}
	c.m[l] = e
	c.touch(e)
	for len(c.m) > maxDrawn && ts.evictDrawn() {
	}
	return e
}

// evictDrawn releases the least recently used layout that was not drawn
// in the current frame, and reports whether there was one. The new
// layout keepDrawn just put in front is not drawn yet, but is the last
// candidate: there is always an older one while the cache is over
// maxDrawn, unless every older one is drawn in this frame.
func (ts *textSystem) evictDrawn() bool {
	c := &ts.drawn
	for e := c.lru.prev; e != &c.lru && e != c.lru.next; e = e.prev {
		c.visits++
		if e.frame == 0 || e.frame < c.frame {
			c.unlink(e)
			e.nl.Release()
			delete(c.m, e.l)
			return true
		}
	}
	return false
}

// releaseDrawn releases every cached layout; each release must be the
// last reference, else leak gets the rest.
func (ts *textSystem) releaseDrawn(leak leakFunc) {
	c := &ts.drawn
	for l, e := range c.m {
		leak.releaseLast("IDWriteTextLayout", &e.nl.Ptr)
		c.unlink(e)
		delete(c.m, l)
	}
}
