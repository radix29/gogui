package aqua

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// Keyboard paging and scroll-to-focused (PLAN.md S3.5g). In scrollRun's
// window the viewport is 100 pt high and the offset runs 0…300; a page is
// 100 − PageOverlap, an arrow step WheelLineHeight.

// textElem is a focusable element of FocusKind FocusText that takes no
// keys.
type textElem struct{ Base }

func newTextElem() *textElem {
	e := &textElem{}
	e.Init(e)
	e.SetFocusable(true)
	e.SetFocusKind(FocusText)
	return e
}

// addCheckbox adds a checkbox framed at r to the document.
func (e *scrollEnv) addCheckbox(r driver.Rect) *Checkbox {
	cb := e.d.NewCheckbox(nil).(*Checkbox)
	cb.SetTitle("Option")
	e.doc.AddChild(cb.view)
	cb.SetFrame(r)
	return cb
}

// settle runs the frame clock past a key step.
func (e *scrollEnv) settle() { e.clk.Advance(2 * tokens.Scroll.WheelStepDuration) }

func page() float64 { return 100 - tokens.Scroll.PageOverlap }

// wantY checks the vertical offset at rest and the last Scrolled.
func (e *scrollEnv) wantY(t *testing.T, what string, y float64) {
	t.Helper()
	e.settle()
	if got := e.sv.Offset().Y; got != y {
		t.Errorf("%s: offset %v, want %v", what, got, y)
	}
	if got := e.sv.y.Offset(); got != y {
		t.Errorf("%s: shown offset %v, want %v", what, got, y)
	}
	if y != 0 || len(e.h.scrolled) > 0 {
		if n := len(e.h.scrolled); n == 0 || e.h.scrolled[n-1].Y != y {
			t.Errorf("%s: Scrolled %v, last want %v", what, e.h.scrolled, y)
		}
	}
}

func TestScrollKeysPaging(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		cb := e.addCheckbox(driver.Rect{X: 10, Y: 10, W: 100, H: 20})
		if !cb.view.Focus() {
			t.Fatal("checkbox not focused")
		}
		e.settle()
		if !e.sf.Key(driver.KeyPageDown, 0) {
			t.Fatal("Page Down not taken")
		}
		// Animated like a wheel step.
		e.clk.Advance(tokens.Scroll.WheelStepDuration / 2)
		if y := e.sv.y.Offset(); !(y > 0 && y < page()) {
			t.Errorf("mid-step offset %v", y)
		}
		if e.vbar().Hidden() {
			t.Error("overlay scroller not shown")
		}
		e.wantY(t, "Page Down", page())
		// Repeats add up from the running step's target.
		e.sf.KeyDown(driver.KeyPageDown, 0)
		e.sf.KeyRepeat(driver.KeyPageDown, 0)
		e.sf.KeyUp(driver.KeyPageDown, 0)
		e.wantY(t, "two more Page Downs", 3*page())
		if !e.sf.Key(driver.KeyPageDown, 0) {
			t.Fatal("Page Down before the end not taken")
		}
		e.wantY(t, "Page Down to the end", 300)
		if e.sf.Key(driver.KeyPageDown, 0) {
			t.Error("Page Down at the end taken")
		}
		e.sf.Key(driver.KeyPageUp, 0)
		e.wantY(t, "Page Up", 300-page())
		e.sf.Key(driver.KeyHome, 0)
		e.wantY(t, "Home", 0)
		if e.sf.Key(driver.KeyHome, 0) || e.sf.Key(driver.KeyPageUp, 0) {
			t.Error("Home or Page Up at the start taken")
		}
		e.sf.Key(driver.KeyEnd, 0)
		e.wantY(t, "End", 300)
		// Modifiers leave the keys alone.
		for _, m := range []driver.Modifiers{driver.ModShift, driver.ModControl, driver.ModOption} {
			if e.sf.Key(driver.KeyHome, m) {
				t.Errorf("Home with mods %v taken", m)
			}
		}
		e.wantY(t, "Home with modifiers", 300)
		// Arrows need the scroll view itself focused.
		if e.sf.Key(driver.KeyUp, 0) {
			t.Error("Up taken with a checkbox focused")
		}
		e.wantY(t, "Up with a checkbox focused", 300)
	})
}

func TestScrollKeysSpace(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		cb := e.addCheckbox(driver.Rect{X: 10, Y: 10, W: 100, H: 20})
		cb.view.Focus()
		e.settle()
		// Space on a focused checkbox toggles it and does not page.
		if !e.sf.Key(driver.KeySpace, 0) {
			t.Fatal("Space not taken")
		}
		if cb.State() != driver.StateOn {
			t.Errorf("Space did not toggle the checkbox: %v", cb.State())
		}
		e.wantY(t, "Space on a checkbox", 0)
		if e.sf.Key(driver.KeySpace, driver.ModShift) {
			t.Error("Shift-Space taken with a checkbox focused")
		}
		if cb.State() != driver.StateOn {
			t.Errorf("Shift-Space changed the checkbox: %v", cb.State())
		}

		// A focused FocusText element keeps Space for its text (S5.2g):
		// the key goes on unpaged, and a real field types it.
		te := newTextElem()
		e.doc.AddChild(te)
		te.SetFrame(driver.Rect{X: 10, Y: 40, W: 100, H: 20})
		if !te.Focus() {
			t.Fatal("text element not focused")
		}
		e.settle()
		if e.sf.Key(driver.KeySpace, 0) || e.sf.Key(driver.KeySpace, driver.ModShift) {
			t.Error("Space taken with a text element focused")
		}
		e.wantY(t, "Space on a text element", 0)
		tf := e.d.NewTextField(false, nil).(*TextField)
		e.doc.AddChild(tf.view)
		tf.SetFrame(driver.Rect{X: 10, Y: 70, W: 100, H: 22})
		tf.view.Focus()
		e.settle()
		e.sf.Type("a b")
		if tf.Text() != "a b" {
			t.Errorf("the field typed %q", tf.Text())
		}
		e.wantY(t, "Space in a field", 0)

		// The focused scroll view itself pages on Space.
		e.sv.view.SetFocusable(true)
		if !e.sv.view.Focus() {
			t.Fatal("scroll view not focused")
		}
		if !e.sf.Key(driver.KeySpace, 0) {
			t.Fatal("Space not taken")
		}
		e.wantY(t, "Space", page())
		e.sf.Key(driver.KeySpace, driver.ModShift)
		e.wantY(t, "Shift-Space", 0)
		if e.sf.Key(driver.KeySpace, driver.ModControl) {
			t.Error("Ctrl-Space taken")
		}
		if cb.State() != driver.StateOn {
			t.Errorf("paging changed the checkbox: %v", cb.State())
		}
	})
}

func TestScrollKeysArrows(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		line := tokens.Scroll.WheelLineHeight
		e.sv.view.SetFocusable(true)
		if !e.sv.view.Focus() {
			t.Fatal("scroll view not focused")
		}
		if !e.sf.Key(driver.KeyDown, 0) {
			t.Fatal("Down not taken")
		}
		e.wantY(t, "Down", line)
		e.sf.Key(driver.KeyDown, 0)
		e.wantY(t, "Down twice", 2*line)
		e.sf.Key(driver.KeyUp, 0)
		e.wantY(t, "Up", line)
		// No horizontal scroller: Left and Right do nothing.
		if e.sf.Key(driver.KeyRight, 0) || e.sf.Key(driver.KeyLeft, 0) {
			t.Error("Left or Right taken without a horizontal scroller")
		}
		// The scroll view itself focused: Space pages.
		if !e.sf.Key(driver.KeySpace, 0) {
			t.Fatal("Space not taken")
		}
		e.wantY(t, "Space", line+page())
		e.sv.SetOffset(driver.Point{})
		if e.sf.Key(driver.KeyUp, 0) {
			t.Error("Up at the start taken")
		}
	})
}

func TestScrollKeysReduceMotion(t *testing.T) {
	scrollRun(t, driver.Appearance{ReduceMotion: true}, func(e *scrollEnv) {
		cb := e.addCheckbox(driver.Rect{X: 10, Y: 10, W: 100, H: 20})
		cb.view.Focus()
		e.sf.Key(driver.KeyPageDown, 0)
		if got := e.sv.y.Offset(); got != page() {
			t.Errorf("Page Down under Reduce Motion: offset %v, want a jump to %v", got, page())
		}
		if n := len(e.h.scrolled); n != 1 || e.h.scrolled[0].Y != page() {
			t.Errorf("Scrolled %v", e.h.scrolled)
		}
	})
}

// TestScrollKeysPressedScroller: a held knob keeps the keys from moving
// the offset.
func TestScrollKeysPressedScroller(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		cb := e.addCheckbox(driver.Rect{X: 10, Y: 10, W: 100, H: 20})
		cb.view.Focus()
		e.sf.Move(e.at(inStrip, 10))
		e.sf.Press(e.at(inStrip, 10), shell.ButtonPrimary)
		if e.sf.Key(driver.KeyPageDown, 0) {
			t.Error("Page Down taken during a knob press")
		}
		e.sf.Release(e.at(inStrip, 10), shell.ButtonPrimary)
		e.wantY(t, "after the press", 0)
	})
}

func TestScrollToFocused(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		outset, width := FocusRingBand(1)
		g := outset + width
		var cbs []*Checkbox
		for i := range 10 {
			cbs = append(cbs, e.addCheckbox(driver.Rect{X: 10, Y: 10 + 38*float64(i), W: 100, H: 20}))
		}
		r := e.w.Router()
		visible := func(what string, cb *Checkbox) {
			t.Helper()
			f := cb.view.Frame()
			off := e.sv.Offset().Y
			if f.Y-g < off || f.MaxY()+g > off+100 {
				t.Errorf("%s: %v (ring %v) outside the viewport at %v", what, f, g, off)
			}
		}
		// Tab forward through all: each focused one in view, scrolled by
		// the least amount (its ring's bottom at the viewport's bottom).
		for i, cb := range cbs {
			if !r.FocusNext(false) || r.Focused() != Element(cb.view) {
				t.Fatalf("Tab %d: focus %v", i, r.Focused())
			}
			visible("Tab", cb)
			want := max(0, cb.view.Frame().MaxY()+g-100)
			if got := e.sv.Offset().Y; got != want {
				t.Errorf("Tab %d: offset %v, want %v", i, got, want)
			}
			if got := e.sv.y.Offset(); got != want {
				t.Errorf("Tab %d: no jump, shown offset %v", i, got)
			}
		}
		if n := len(e.h.scrolled); n == 0 || e.h.scrolled[n-1].Y != e.sv.Offset().Y {
			t.Errorf("Scrolled %v", e.h.scrolled)
		}
		// Shift-Tab back: the ring's top at the viewport's top.
		for i := len(cbs) - 2; i >= 0; i-- {
			r.FocusNext(true)
			visible("Shift-Tab", cbs[i])
		}
		if got, want := e.sv.Offset().Y, max(0, cbs[0].view.Frame().Y-g); got != want {
			t.Errorf("back at the first: offset %v, want %v", got, want)
		}
		// A visible element scrolls nothing.
		e.sv.SetOffset(driver.Point{Y: 50})
		e.h.scrolled = nil
		cbs[2].view.Focus() // y 86…106, ring 83…109: inside 50…150
		if got := e.sv.Offset().Y; got != 50 || len(e.h.scrolled) != 0 {
			t.Errorf("visible: offset %v, Scrolled %v", got, e.h.scrolled)
		}
		// Clearing the focus scrolls nothing.
		r.SetFocus(nil)
		if got := e.sv.Offset().Y; got != 50 {
			t.Errorf("cleared focus: offset %v", got)
		}
		// Through the sf: Tab is the router's FocusNext.
		e.sv.SetOffset(driver.Point{})
		r.SetFocus(cbs[len(cbs)-2].view)
		e.sf.Key(driver.KeyTab, 0)
		visible("Tab key", cbs[len(cbs)-1])
	})
}

// TestScrollToFocusedTall: an element taller than the viewport shows its
// top.
func TestScrollToFocusedTall(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		te := newTextElem()
		e.doc.AddChild(te)
		te.SetFrame(driver.Rect{X: 10, Y: 200, W: 100, H: 150})
		te.Focus()
		outset, width := FocusRingBand(1)
		if got, want := e.sv.Offset().Y, 200-(outset+width); got != want {
			t.Errorf("offset %v, want %v", got, want)
		}
	})
}

// TestScrollToFocusedNested: an inner scroll view in the document of an
// outer one; the inner one scrolls first, then the outer one brings the
// element (now at its inner position) into its own viewport.
func TestScrollToFocusedNested(t *testing.T) {
	scrollRun(t, driver.Appearance{}, func(e *scrollEnv) {
		inner := e.d.NewScrollView(&svLog{}).(*ScrollView)
		idoc := e.d.NewContainer().(*Container)
		inner.SetDocument(idoc)
		inner.SetHasVerticalScroller(true)
		e.doc.SetChildren([]driver.Widget{inner})
		inner.SetFrame(driver.Rect{Y: 200, W: 150, H: 60})
		inner.SetDocumentSize(driver.Size{W: 150, H: 300})
		idoc.SetFrame(driver.Rect{W: 150, H: 300})
		te := newTextElem()
		idoc.AddChild(te)
		te.SetFrame(driver.Rect{X: 10, Y: 150, W: 100, H: 20})
		if !te.Focus() {
			t.Fatal("not focused")
		}
		outset, width := FocusRingBand(1)
		g := outset + width
		if got, want := inner.Offset().Y, 170+g-60; got != want {
			t.Errorf("inner offset %v, want %v", got, want)
		}
		// In the outer document its ring now ends at the inner viewport's
		// bottom, 260.
		if got, want := e.sv.Offset().Y, 260.0-100; got != want {
			t.Errorf("outer offset %v, want %v", got, want)
		}
		// Page Down from inside goes to the inner scroll view first; at its
		// end it bubbles to the outer one.
		e.sf.Key(driver.KeyEnd, 0)
		e.settle()
		if got := inner.Offset().Y; got != 240 {
			t.Errorf("End: inner offset %v, want 240", got)
		}
		if got := e.sv.Offset().Y; got != 160 {
			t.Errorf("End: outer moved to %v", got)
		}
		e.sf.Key(driver.KeyPageDown, 0)
		e.settle()
		if got := e.sv.Offset().Y; got != 250 {
			t.Errorf("Page Down at the inner end: outer offset %v, want 250", got)
		}
	})
}
