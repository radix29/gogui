package gogui

import (
	"fmt"
	"math"
	"testing"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
)

// The notes sidebar on the aqua driver through the fake shell (PLAN.md
// S3.5g): Tab keeps the focused checkbox in view with the least scroll,
// and the paging keys bubble from the focused checkbox to the sidebar.

// focused returns the index (from 1) of the focused checkbox, or 0.
func (e *notesEnv) focused() int {
	f := e.aw.Router().Focused()
	for i, c := range e.boxes {
		if aqua.ElementOf(c.p) == f {
			return i + 1
		}
	}
	return 0
}

// viewTop returns the window y of the sidebar's viewport.
func (e *notesEnv) viewTop() float64 {
	e.t.Helper()
	area := aqua.AccessibilityTree(e.aw.Root()).Find(a11y.RoleScrollArea, "")
	if area == nil {
		e.t.Fatal("no scroll area")
	}
	return area.Frame.Y
}

// boxFrame returns checkbox i's a11y frame (window coordinates).
func (e *notesEnv) boxFrame(i int) driver.Rect {
	e.t.Helper()
	n := aqua.AccessibilityTree(e.aw.Root()).Find(a11y.RoleCheckBox, fmt.Sprintf("Option %d", i))
	if n == nil {
		e.t.Fatalf("no checkbox %d", i)
	}
	return n.Frame
}

// wantRevealed checks that checkbox i, grown by the focus ring, lies in
// the viewport, and that the offset moved from `from` by the least amount
// that does it (the reveal of ScrollView).
func (e *notesEnv) wantRevealed(i int, from float64) {
	e.t.Helper()
	outset, width := aqua.FocusRingBand(1)
	g := outset + width
	top, h := e.viewTop(), e.sv.ViewportSize().H
	off := e.sidebar.Offset().Y
	f := e.boxFrame(i)
	if f.Y-g < top || f.Y+f.H+g > top+h {
		e.t.Errorf("checkbox %d at y %g…%g (ring %g) outside the viewport %g…%g (offset %g)",
			i, f.Y, f.Y+f.H, g, top, top+h, off)
	}
	// In the document: lo…hi including the ring.
	lo := f.Y - top + off - g
	hi := f.Y + f.H - top + off + g
	want := from
	if hi > want+h {
		want = hi - h
	}
	if lo < want {
		want = lo
	}
	want = math.Max(0, math.Min(notesMax, want))
	if off != want {
		e.t.Errorf("checkbox %d: offset %g → %g, want the least scroll to %g", i, from, off, want)
	}
	if off != from && (len(e.scrolls) == 0 || e.scrolls[len(e.scrolls)-1] != (Point{Y: off})) {
		e.t.Errorf("checkbox %d: OnScroll %v after scrolling to %g", i, e.scrolls, off)
	}
}

func TestNotesTabScrollsOnAqua(t *testing.T) {
	runNotes(t, func(e *notesEnv) {
		r := e.aw.Router()
		if n := len(r.FocusOrder()); n != 31 {
			t.Fatalf("Tab order has %d elements, want the 30 checkboxes and the editor", n)
		}
		// The window starts with the first Tab stop focused: checkbox 1.
		if e.focused() != 1 {
			t.Fatalf("initial focus %T, want checkbox 1", r.Focused())
		}
		e.wantRevealed(1, 0)
		moved := 0
		for i := 2; i <= 30; i++ {
			from := e.sidebar.Offset().Y
			if !e.sf.Key(driver.KeyTab, 0) {
				t.Fatalf("Tab %d not handled", i)
			}
			drain(e.app)
			if e.focused() != i {
				t.Fatalf("Tab %d focused checkbox %d", i, e.focused())
			}
			e.wantRevealed(i, from)
			if e.sidebar.Offset().Y != from {
				moved++
			}
		}
		// The least scroll leaves the document padding (12) beyond the
		// ring unrevealed at both ends.
		outset, width := aqua.FocusRingBand(1)
		edge := 12 - (outset + width)
		if !(edge > 0) {
			t.Fatalf("focus ring %g wider than the padding", outset+width)
		}
		if e.sidebar.Offset().Y != notesMax-edge || moved == 0 || moved == 29 {
			t.Errorf("at checkbox 30: offset %v (want %g), moved on %d Tabs (want some, not all)",
				e.sidebar.Offset(), notesMax-edge, moved)
		}
		n := len(e.scrolls)

		// Tab on to the editor: the sidebar stays.
		e.sf.Key(driver.KeyTab, 0)
		drain(e.app)
		if e.focused() != 0 || e.sidebar.Offset().Y != notesMax-edge || len(e.scrolls) != n {
			t.Errorf("Tab to the editor: focus %d, offset %v, %d OnScroll", e.focused(), e.sidebar.Offset(), len(e.scrolls)-n)
		}
		// Shift-Tab back to 30, then up to 1.
		for i := 30; i >= 1; i-- {
			from := e.sidebar.Offset().Y
			e.sf.Key(driver.KeyTab, driver.ModShift)
			drain(e.app)
			if e.focused() != i {
				t.Fatalf("Shift-Tab to %d focused checkbox %d", i, e.focused())
			}
			e.wantRevealed(i, from)
		}
		if e.sidebar.Offset().Y != edge {
			t.Errorf("back at checkbox 1: offset %v, want %g", e.sidebar.Offset(), edge)
		}

		// Focus (a click or Focus) reveals too; clearing the focus does not.
		from := e.sidebar.Offset().Y
		e.boxes[19].Focus()
		drain(e.app)
		e.wantRevealed(20, from)
		at, n := e.sidebar.Offset(), len(e.scrolls)
		r.SetFocus(nil)
		drain(e.app)
		if e.sidebar.Offset() != at || len(e.scrolls) != n {
			t.Errorf("clearing the focus scrolled to %v", e.sidebar.Offset())
		}
	})
}

func TestNotesPageKeysOnAqua(t *testing.T) {
	runNotes(t, func(e *notesEnv) {
		// A shorter window so a page is less than the scroll range.
		if !e.sf.UserResize(driver.Size{W: 720, H: 300}) {
			t.Fatal("resize refused")
		}
		drain(e.app)
		h := e.sv.ViewportSize().H
		max := e.sv.MaxOffset().Y
		page := h - theme.Sequoia.Scroll.PageOverlap
		if !(page > 0 && 2*page < max && max < 3*page) {
			t.Fatalf("viewport %g, max %g, page %g: want max between two and three pages", h, max, page)
		}
		e.boxes[0].Focus()
		drain(e.app)
		if e.focused() != 1 || e.sidebar.Offset().Y != 0 {
			t.Fatalf("focus %d, offset %v", e.focused(), e.sidebar.Offset())
		}
		key := func(k driver.Key, mods driver.Modifiers, handled bool, want float64) {
			t.Helper()
			n := len(e.scrolls)
			if got := e.sf.Key(k, mods); got != handled {
				t.Errorf("key %v handled %v, want %v", k, got, handled)
			}
			e.settle()
			if got := e.sidebar.Offset().Y; got != want {
				t.Errorf("key %v: offset %g, want %g", k, got, want)
			}
			if handled && (len(e.scrolls) == n || e.scrolls[len(e.scrolls)-1] != (Point{Y: want})) {
				t.Errorf("key %v: OnScroll %v", k, e.scrolls[n:])
			}
		}
		key(driver.KeyPageDown, 0, true, page)
		key(driver.KeyPageDown, 0, true, 2*page)
		key(driver.KeyPageDown, 0, true, max) // clamped at the end
		key(driver.KeyPageDown, 0, false, max)
		key(driver.KeyPageUp, 0, true, max-page)
		key(driver.KeyPageUp, 0, true, max-2*page)
		key(driver.KeyPageUp, 0, true, 0)
		key(driver.KeyPageUp, 0, false, 0)
		key(driver.KeyEnd, 0, true, max)
		key(driver.KeyHome, 0, true, 0)
		key(driver.KeyHome, 0, false, 0)
		key(driver.KeyPageDown, driver.ModControl, false, 0) // a modifier leaves it alone

		// The focus stays on checkbox 1 (scrolled out of view at the end).
		key(driver.KeyEnd, 0, true, max)
		if e.focused() != 1 {
			t.Errorf("paging moved the focus to %d", e.focused())
		}
		// The page key animates like a wheel step: halfway it is moving.
		e.sf.Key(driver.KeyHome, 0)
		e.clk.Advance(theme.Sequoia.Scroll.WheelStepDuration / 2)
		if got := e.sidebar.Offset().Y; !(got > 0 && got < max) {
			t.Errorf("Home halfway: offset %g", got)
		}
		e.settle()

		// Space on the focused checkbox toggles it and does not page.
		e.sidebar.SetOffset(Point{})
		drain(e.app)
		toggled := 0
		e.boxes[0].OnChange(func(bool) { toggled++ })
		n := len(e.scrolls)
		if !e.sf.Key(driver.KeySpace, 0) {
			t.Error("Space not handled")
		}
		e.settle()
		if !e.boxes[0].Checked() || toggled != 1 {
			t.Errorf("Space: checked %v, %d OnChange", e.boxes[0].Checked(), toggled)
		}
		if e.sidebar.Offset().Y != 0 || len(e.scrolls) != n {
			t.Errorf("Space paged to %v", e.sidebar.Offset())
		}
		e.sf.Key(driver.KeySpace, driver.ModShift)
		e.settle()
		if e.sidebar.Offset().Y != 0 {
			t.Errorf("Shift-Space paged to %v", e.sidebar.Offset())
		}
	})
}
