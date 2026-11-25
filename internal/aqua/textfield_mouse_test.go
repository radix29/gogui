package aqua

import (
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// Mouse selection in TextField and SecureField and the router's cursor
// (PLAN.md S5.2h), through the fake shell's pointer input.

// fieldPoint returns the surface point on the field's text line where the
// caret for the editor offset off is drawn, by the scroll as last placed;
// dx moves it right.
func fieldPoint(tf *TextField, off int, dx float64) driver.Point {
	v := tf.view
	r, pad, room := v.textArea()
	l := v.textLayout()
	o := v.origin(l, r, pad, room)
	x := o.X + l.CaretRect(v.shownOffset(off)).X + dx
	return ToRoot(v, driver.Point{X: x, Y: r.Y + r.H/2})
}

// apart lets the double-click time pass, so the next press counts as a
// single click.
func apart(s *fake.Shell) { s.FakeClock().Advance(time.Second) }

// TestTextFieldClick: a click focuses the field (keeping no Tab
// select-all), places the caret at the nearest boundary, fires nothing,
// and repaints.
func TestTextFieldClick(t *testing.T) {
	h := &fieldLog{}
	tf, w, sf, s := fieldWindow(t, false, h)
	tf.SetText("one two three")
	sf.Click(fieldPoint(tf, 5, 0))
	if !tf.view.HasFocus() || selOf(tf) != "one two three|5,5" {
		t.Fatalf("click: focus %v, %s", tf.view.HasFocus(), selOf(tf))
	}
	if w.Router().Captured() != nil {
		t.Error("capture kept after the release")
	}
	s.Flush()
	for _, c := range []struct {
		dx   float64
		off  int
		want string
	}{
		{0.4 * charW(tf), 5, "5,5"}, // the leading half of a cluster: the caret before it
		{0.6 * charW(tf), 5, "6,6"},
		{-0.4 * charW(tf), 5, "5,5"}, // the trailing half: after it
		{-0.6 * charW(tf), 5, "4,4"},
		{3, 13, "13,13"}, // right of the text: its end
		{-3, 0, "0,0"},   // in the padding before it: its start
	} {
		apart(s)
		sf.Click(fieldPoint(tf, c.off, c.dx))
		if got := selOf(tf); got != "one two three|"+c.want {
			t.Errorf("click at %d%+.1f: %s, want %s", c.off, c.dx, got, c.want)
		}
		if !sf.Invalid() {
			t.Errorf("click at %d%+.1f did not repaint", c.off, c.dx)
		}
		s.Flush()
	}
	if len(h.events) != 0 {
		t.Errorf("clicks fired %q", h.events)
	}
	// A click on a field focused by Tab (all selected) places the caret.
	w.Router().SetFocus(nil)
	w.Router().FocusNext(false)
	if selOf(tf) != "one two three|0,13" {
		t.Fatalf("Tab focus: %s", selOf(tf))
	}
	apart(s)
	sf.Click(fieldPoint(tf, 3, 0))
	if got := selOf(tf); got != "one two three|3,3" {
		t.Errorf("click after Tab: %s", got)
	}
	// An empty field: the caret at 0, also with a placeholder.
	tf.SetText("")
	tf.SetPlaceholder("Name")
	apart(s)
	sf.Click(fieldPoint(tf, 0, 30))
	if got := selOf(tf); got != "|0,0" {
		t.Errorf("click in an empty field: %s", got)
	}
}

// charW is the fake text system's width of one cluster in tf's font.
func charW(tf *TextField) float64 { return fake.RuneWidth(tf.TextFont()) }

// TestTextFieldClickAligned: centred and trailing text hits where it is
// drawn.
func TestTextFieldClickAligned(t *testing.T) {
	for _, a := range []driver.TextAlignment{driver.AlignCenter, driver.AlignTrailing} {
		tf, _, sf, _ := fieldWindow(t, false, nil)
		tf.SetText("abc")
		tf.SetAlignment(a)
		sf.Click(fieldPoint(tf, 1, 0))
		if got := selOf(tf); got != "abc|1,1" {
			t.Errorf("alignment %v: %s", a, got)
		}
	}
}

// TestTextFieldShiftClick: Shift-click extends the selection to the
// click, keeping its far end as AppKit does; inside it keeps the anchor.
func TestTextFieldShiftClick(t *testing.T) {
	tf, w, sf, s := fieldWindow(t, false, nil)
	tf.SetText("one two three")
	sf.Click(fieldPoint(tf, 2, 0))
	for _, c := range []struct {
		off  int
		want string
	}{
		{8, "2,8"},   // after the caret
		{0, "8,0"},   // before the selection: the far end (8) is the anchor
		{11, "0,11"}, // after it: the start is
		{5, "0,5"},   // inside: the anchor stays
	} {
		apart(s)
		sf.SetModifiers(shf)
		sf.Click(fieldPoint(tf, c.off, 0))
		sf.SetModifiers(0)
		if got := selOf(tf); got != "one two three|"+c.want {
			t.Errorf("Shift-click at %d: %s, want %s", c.off, got, c.want)
		}
	}
	// Shift-click on an unfocused field: it is focused and extends from
	// the kept caret.
	w.Router().SetFocus(nil)
	tf.Editor().SetSelection(4, 4)
	apart(s)
	sf.SetModifiers(shf)
	sf.Click(fieldPoint(tf, 7, 0))
	sf.SetModifiers(0)
	if got := selOf(tf); got != "one two three|4,7" || !tf.view.HasFocus() {
		t.Errorf("Shift-click unfocused: %s, focus %v", got, tf.view.HasFocus())
	}
}

// TestTextFieldDrag: a drag selects by clusters from the press, both
// ways, and the capture keeps it going outside the field.
func TestTextFieldDrag(t *testing.T) {
	h := &fieldLog{}
	tf, w, sf, s := fieldWindow(t, false, h)
	tf.SetText("one two")
	sf.Drag(fieldPoint(tf, 1, 0), fieldPoint(tf, 5, 0), fieldPoint(tf, 3, 0))
	if got := selOf(tf); got != "one two|1,5" {
		t.Errorf("drag right: %s", got)
	}
	apart(s)
	sf.Press(fieldPoint(tf, 5, 0), shell.ButtonPrimary)
	if w.Router().Captured() != tf.view {
		t.Fatal("a press does not capture")
	}
	sf.Move(fieldPoint(tf, 2, 0))
	if got := selOf(tf); got != "one two|5,2" {
		t.Errorf("drag left: %s", got)
	}
	// Below and above the field: still the one line.
	p := fieldPoint(tf, 6, 0)
	sf.Move(driver.Point{X: p.X, Y: p.Y + 30})
	if got := selOf(tf); got != "one two|5,6" {
		t.Errorf("drag below: %s", got)
	}
	sf.Move(driver.Point{X: p.X, Y: 2})
	if got := selOf(tf); got != "one two|5,6" {
		t.Errorf("drag above (over the title bar): %s", got)
	}
	sf.Release(fieldPoint(tf, 0, 0), shell.ButtonPrimary)
	if got := selOf(tf); got != "one two|5,0" {
		t.Errorf("release: %s", got)
	}
	// Moves after the release select nothing.
	sf.Move(fieldPoint(tf, 7, 0))
	if got := selOf(tf); got != "one two|5,0" {
		t.Errorf("move after the release: %s", got)
	}
	if len(h.events) != 0 {
		t.Errorf("drags fired %q", h.events)
	}
}

// TestTextFieldDragPastEdge: dragged past either edge of an overflowing
// field, the head goes to the text scrolled out there and every move
// scrolls further, up to the ends.
func TestTextFieldDragPastEdge(t *testing.T) {
	tf, _, sf, sh := fieldWindow(t, false, nil)
	s := strings.Repeat("abcdefghij", 5) // 50 clusters, far wider than the field
	tf.SetText(s)
	r, pad, room := tf.view.textArea()
	if tf.view.textLayout().Size().W <= room {
		t.Fatal("the text fits")
	}
	sf.Press(fieldPoint(tf, 2, 0), shell.ButtonPrimary)
	if tf.view.ScrollX() != 0 {
		t.Fatalf("scroll %v after the press", tf.view.ScrollX())
	}
	right := ToRoot(tf.view, driver.Point{X: r.X + pad + room + 10, Y: r.Y + r.H/2})
	prev, prevScroll := tf.Editor().Head(), 0.0
	for i := 0; tf.Editor().Head() < len(s); i++ {
		if i > 50 {
			t.Fatal("dragging right never reaches the end")
		}
		sf.Move(driver.Point{X: right.X + float64(i%2)/2, Y: right.Y}) // a still pointer sends no moves
		head, scroll := tf.Editor().Head(), tf.view.ScrollX()
		if head <= prev || scroll <= prevScroll {
			t.Fatalf("move %d past the right edge: head %d (was %d), scroll %v (was %v)", i, head, prev, scroll, prevScroll)
		}
		prev, prevScroll = head, scroll
	}
	if tf.Editor().Anchor() != 2 || tf.view.ScrollX() != tf.view.textLayout().Size().W-room {
		t.Errorf("at the end: anchor %d, scroll %v", tf.Editor().Anchor(), tf.view.ScrollX())
	}
	// Back inside: the head follows the pointer, nothing scrolls.
	scroll := tf.view.ScrollX()
	inside := ToRoot(tf.view, driver.Point{X: r.X + pad + room/2, Y: r.Y + r.H/2})
	sf.Move(inside)
	if tf.view.ScrollX() != scroll || tf.Editor().Head() >= len(s) {
		t.Errorf("back inside: head %d, scroll %v (was %v)", tf.Editor().Head(), tf.view.ScrollX(), scroll)
	}
	// Past the left edge, down to the start.
	left := ToRoot(tf.view, driver.Point{X: r.X - 10, Y: r.Y + r.H/2})
	prev, prevScroll = tf.Editor().Head(), tf.view.ScrollX()
	for i := 0; tf.Editor().Head() > 0; i++ {
		if i > 50 {
			t.Fatal("dragging left never reaches the start")
		}
		sf.Move(driver.Point{X: left.X - float64(i%2)/2, Y: left.Y})
		head, scroll := tf.Editor().Head(), tf.view.ScrollX()
		if head >= prev || scroll >= prevScroll && scroll != 0 {
			t.Fatalf("move %d past the left edge: head %d (was %d), scroll %v (was %v)", i, head, prev, scroll, prevScroll)
		}
		prev, prevScroll = head, scroll
	}
	sf.Release(left, shell.ButtonPrimary)
	if got := selOf(tf); got != s+"|2,0" || tf.view.ScrollX() != 0 {
		t.Errorf("at the start: %s, scroll %v", got, tf.view.ScrollX())
	}
	// The paint keeps the scroll the drag made.
	sh.Flush()
	if tf.view.ScrollX() != 0 {
		t.Errorf("scroll %v after the paint", tf.view.ScrollX())
	}
}

// TestTextFieldMultiClick: a double-click selects the word, whitespace
// or cluster there and drags by words; a triple-click selects all and a
// drag after it changes nothing.
func TestTextFieldMultiClick(t *testing.T) {
	tf, _, sf, s := fieldWindow(t, false, nil)
	const text = "one two, three"
	tf.SetText(text)
	for _, c := range []struct {
		off  int
		dx   float64
		want string
	}{
		{5, 0, "4,7"},   // in "two"
		{4, 0.2, "4,7"}, // its first cluster
		{0, 0.2, "0,3"},
		{7, 0.2, "7,8"},   // the comma alone
		{8, 0.2, "8,9"},   // the space
		{13, 0.2, "9,14"}, // the last cluster
	} {
		apart(s)
		sf.DoubleClick(fieldPoint(tf, c.off, c.dx))
		if got := selOf(tf); got != text+"|"+c.want {
			t.Errorf("double-click at %d%+.1f: %s, want %s", c.off, c.dx, got, c.want)
		}
	}
	// Drag by words: forward from "two" to "three", back to "one".
	apart(s)
	p := fieldPoint(tf, 5, 0)
	sf.Click(p)
	sf.Press(p, shell.ButtonPrimary)
	if got := selOf(tf); got != text+"|4,7" {
		t.Fatalf("second press: %s", got)
	}
	sf.Move(fieldPoint(tf, 11, 0))
	if got := selOf(tf); got != text+"|4,14" {
		t.Errorf("word drag forward: %s", got)
	}
	sf.Move(fieldPoint(tf, 6, 0)) // back inside the first word
	if got := selOf(tf); got != text+"|4,7" {
		t.Errorf("word drag back to the first word: %s", got)
	}
	sf.Move(fieldPoint(tf, 1, 0))
	if got := selOf(tf); got != text+"|7,0" {
		t.Errorf("word drag backward: %s", got)
	}
	sf.Release(fieldPoint(tf, 1, 0), shell.ButtonPrimary)

	// Triple-click: everything; a drag after it keeps everything.
	apart(s)
	p = fieldPoint(tf, 5, 0)
	sf.DoubleClick(p)
	sf.Press(p, shell.ButtonPrimary)
	if got := selOf(tf); got != text+"|0,14" {
		t.Errorf("triple-click: %s", got)
	}
	sf.Move(fieldPoint(tf, 2, 0))
	sf.Release(fieldPoint(tf, 2, 0), shell.ButtonPrimary)
	if got := selOf(tf); got != text+"|0,14" {
		t.Errorf("drag after a triple-click: %s", got)
	}
	// A fourth click still selects all.
	sf.Click(p)
	if got := selOf(tf); got != text+"|0,14" {
		t.Errorf("fourth click: %s", got)
	}
	// Clicks far apart do not count as a double-click.
	apart(s)
	sf.Click(fieldPoint(tf, 1, 0))
	sf.Click(fieldPoint(tf, 12, 0))
	if got := selOf(tf); got != text+"|12,12" {
		t.Errorf("two clicks apart: %s", got)
	}
}

// TestTextFieldSecureMouse: a SecureField maps the bullets back to the
// real text, and a double-click selects the whole text.
func TestTextFieldSecureMouse(t *testing.T) {
	tf, _, sf, s := fieldWindow(t, true, nil)
	const text = "añ b🇩🇪c" // clusters at 0, 1, 3, 4, 5, 13
	tf.SetText(text)
	for _, c := range []struct{ bullets, want int }{{0, 0}, {1, 1}, {2, 3}, {3, 4}, {4, 5}, {5, 13}, {6, 14}} {
		apart(s)
		// Each bullet is one cluster of the shown text.
		v := tf.view
		r, pad, room := v.textArea()
		l := v.textLayout()
		o := v.origin(l, r, pad, room)
		x := o.X + l.CaretRect(c.bullets*len(bullet)).X
		sf.Click(ToRoot(v, driver.Point{X: x, Y: r.Y + r.H/2}))
		if tf.Editor().Head() != c.want || tf.Editor().Anchor() != c.want {
			t.Errorf("click before bullet %d: %s, want %d", c.bullets, selOf(tf), c.want)
		}
	}
	apart(s)
	sf.DoubleClick(fieldPoint(tf, 1, 0))
	if got := selOf(tf); got != text+"|0,14" {
		t.Errorf("double-click in a secure field: %s", got)
	}
	apart(s)
	sf.Drag(fieldPoint(tf, 1, 0), fieldPoint(tf, 5, 0))
	if got := selOf(tf); got != text+"|1,5" {
		t.Errorf("drag in a secure field: %s", got)
	}
}

// TestTextFieldMouseStates: read-only fields select with the mouse;
// disabled fields and other buttons do nothing.
func TestTextFieldMouseStates(t *testing.T) {
	t.Run("read-only", func(t *testing.T) {
		h := &fieldLog{}
		tf, _, sf, s := fieldWindow(t, false, h)
		tf.SetText("one two")
		tf.SetEditable(false)
		sf.Drag(fieldPoint(tf, 1, 0), fieldPoint(tf, 6, 0))
		if got := selOf(tf); got != "one two|1,6" || !tf.view.HasFocus() {
			t.Errorf("drag: %s, focus %v", got, tf.view.HasFocus())
		}
		apart(s)
		sf.DoubleClick(fieldPoint(tf, 1, 0))
		if got := selOf(tf); got != "one two|0,3" || len(h.events) != 0 {
			t.Errorf("double-click: %s, events %q", got, h.events)
		}
		if sf.Cursor() != shell.CursorIBeam {
			t.Errorf("cursor %v over a read-only field", sf.Cursor())
		}
	})
	t.Run("disabled", func(t *testing.T) {
		tf, w, sf, _ := fieldWindow(t, false, nil)
		tf.SetText("one two")
		tf.Editor().SetSelection(2, 2)
		tf.SetEnabled(false)
		sf.Drag(fieldPoint(tf, 1, 0), fieldPoint(tf, 6, 0))
		if got := selOf(tf); got != "one two|2,2" || w.Router().Focused() != nil {
			t.Errorf("disabled: %s, focus %v", got, w.Router().Focused())
		}
		if sf.Cursor() != shell.CursorArrow || tf.view.Cursor() != shell.CursorArrow {
			t.Errorf("cursor %v over a disabled field (its own %v)", sf.Cursor(), tf.view.Cursor())
		}
		// Sent directly, the field refuses the press.
		if tf.view.HandleEvent(PointerEvent{Kind: PointerDown, Button: shell.ButtonPrimary, Clicks: 1, Over: true}) {
			t.Error("a disabled field took a press")
		}
	})
	t.Run("secondary", func(t *testing.T) {
		tf, w, sf, _ := fieldWindow(t, false, nil)
		tf.SetText("one two")
		sf.ClickButton(fieldPoint(tf, 3, 0), shell.ButtonSecondary)
		if w.Router().Focused() != nil || selOf(tf) != "one two|7,7" {
			t.Errorf("secondary click: %s, focus %v", selOf(tf), w.Router().Focused())
		}
		// A secondary press during a drag is swallowed and does not end it.
		sf.Press(fieldPoint(tf, 1, 0), shell.ButtonPrimary)
		sf.ClickButton(fieldPoint(tf, 2, 0), shell.ButtonSecondary)
		sf.Move(fieldPoint(tf, 5, 0))
		sf.Release(fieldPoint(tf, 5, 0), shell.ButtonPrimary)
		if got := selOf(tf); got != "one two|1,5" {
			t.Errorf("drag with a secondary click: %s", got)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		tf, _, sf, s := fieldWindow(t, false, nil)
		tf.SetText("one two")
		sf.Press(fieldPoint(tf, 1, 0), shell.ButtonPrimary)
		sf.CancelCapture()
		sf.Move(fieldPoint(tf, 5, 0))
		if got := selOf(tf); got != "one two|1,1" {
			t.Errorf("move after a cancelled capture: %s", got)
		}
		// Disabled mid-drag: PointerCancel, the selection made so far kept.
		apart(s)
		sf.Press(fieldPoint(tf, 1, 0), shell.ButtonPrimary)
		sf.Move(fieldPoint(tf, 3, 0))
		tf.SetEnabled(false)
		tf.SetEnabled(true)
		sf.Move(fieldPoint(tf, 6, 0))
		sf.Release(fieldPoint(tf, 6, 0), shell.ButtonPrimary)
		if got := selOf(tf); got != "one two|1,3" || tf.view.drag.on {
			t.Errorf("disabled mid-drag: %s, dragging %v", got, tf.view.drag.on)
		}
	})
}

// TestTextFieldClickMovesFocus: clicking a second field moves the focus;
// the first keeps its selection and stops showing it.
func TestTextFieldClickMovesFocus(t *testing.T) {
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 300, H: 100}, Style: allStyles})
	box := d.NewContainer()
	w.SetContent(box)
	a := d.NewTextField(false, nil).(*TextField)
	b := d.NewTextField(false, nil).(*TextField)
	box.SetChildren([]driver.Widget{a, b})
	a.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 22})
	b.SetFrame(driver.Rect{X: 150, Y: 10, W: 100, H: 22})
	a.SetText("alpha")
	b.SetText("beta")
	sf.Drag(fieldPoint(a, 1, 0), fieldPoint(a, 3, 0))
	apart(s)
	sf.Click(fieldPoint(b, 2, 0))
	if w.Router().Focused() != b.view || selOf(a) != "alpha|1,3" || selOf(b) != "beta|2,2" {
		t.Errorf("focus %v, a %s, b %s", w.Router().Focused(), selOf(a), selOf(b))
	}
}

// TestTextFieldCursor: the I-beam over an enabled field, the arrow
// elsewhere, frozen during a drag, updated when the field is disabled,
// hidden or removed under a still pointer, and set on the surface only
// on a change.
func TestTextFieldCursor(t *testing.T) {
	tf, w, sf, _ := fieldWindow(t, false, nil)
	tf.SetText("one two")
	in, out := fieldPoint(tf, 2, 0), driver.Point{X: 150, Y: 80}
	if sf.Cursor() != shell.CursorArrow {
		t.Fatalf("initial cursor %v", sf.Cursor())
	}
	sf.Move(in)
	if sf.Cursor() != shell.CursorIBeam || w.Router().Cursor() != shell.CursorIBeam {
		t.Errorf("over the field: %v", sf.Cursor())
	}
	sf.Move(out)
	if sf.Cursor() != shell.CursorArrow {
		t.Errorf("off the field: %v", sf.Cursor())
	}
	// Dragged out of the field: the I-beam stays until the release.
	sf.Press(in, shell.ButtonPrimary)
	sf.Move(out)
	if sf.Cursor() != shell.CursorIBeam {
		t.Errorf("dragged out: %v", sf.Cursor())
	}
	sf.Release(out, shell.ButtonPrimary)
	if sf.Cursor() != shell.CursorArrow {
		t.Errorf("released outside: %v", sf.Cursor())
	}
	// A press outside that nothing takes captures nothing: the shape
	// follows the pointer in.
	sf.Press(out, shell.ButtonPrimary)
	sf.Move(in)
	if sf.Cursor() != shell.CursorIBeam {
		t.Errorf("dragged in without a capture: %v", sf.Cursor())
	}
	sf.Release(in, shell.ButtonPrimary)
	if sf.Cursor() != shell.CursorIBeam {
		t.Errorf("released over the field: %v", sf.Cursor())
	}
	// Disabled, hidden or removed under the still pointer.
	tf.SetEnabled(false)
	if sf.Cursor() != shell.CursorArrow {
		t.Errorf("disabled under the pointer: %v", sf.Cursor())
	}
	tf.SetEnabled(true)
	w.Router().RefreshHover()
	if sf.Cursor() != shell.CursorIBeam {
		t.Errorf("enabled again after RefreshHover: %v", sf.Cursor())
	}
	tf.SetHidden(true)
	if sf.Cursor() != shell.CursorArrow {
		t.Errorf("hidden under the pointer: %v", sf.Cursor())
	}
	tf.SetHidden(false)
	w.Router().RefreshHover()
	// Disabled during a drag: the capture is cancelled and the shape
	// follows the hover again.
	sf.Press(in, shell.ButtonPrimary)
	tf.SetEnabled(false)
	if sf.Cursor() != shell.CursorArrow {
		t.Errorf("disabled mid-drag: %v", sf.Cursor())
	}
	sf.Release(in, shell.ButtonPrimary)
	tf.SetEnabled(true)
	// Leaving the window: the arrow.
	sf.Move(in)
	sf.Move(driver.Point{X: 500, Y: 500})
	if sf.Cursor() != shell.CursorArrow {
		t.Errorf("outside the window: %v", sf.Cursor())
	}
}

// cursorElem is an element with a settable cursor.
type cursorElem struct {
	Base
	c shell.Cursor
}

func (e *cursorElem) Cursor() shell.Cursor { return e.c }

// TestRouterCursor: the router applies the deepest hovered element's
// Cursor only on a change, RefreshCursor re-reads it, and a nil function
// applies nothing.
func TestRouterCursor(t *testing.T) {
	root := &Base{}
	root.Init(root)
	root.SetFrame(driver.Rect{W: 100, H: 100})
	child := &cursorElem{c: shell.CursorPointingHand}
	child.Init(child)
	child.SetFrame(driver.Rect{X: 10, Y: 10, W: 20, H: 20})
	root.AddChild(child)
	r := NewRouter(root)
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 15, Y: 15}}) // no function yet
	if r.Cursor() != shell.CursorPointingHand {
		t.Fatalf("cursor %v without a function", r.Cursor())
	}
	var set []shell.Cursor
	r.SetCursorFunc(func(c shell.Cursor) { set = append(set, c) })
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 16, Y: 16}})
	r.RefreshCursor()
	if len(set) != 0 {
		t.Errorf("applied %v without a change", set)
	}
	child.c = shell.CursorCrosshair
	r.RefreshCursor()
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 50, Y: 50}})
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 60, Y: 60}})
	r.Event(shell.PointerEvent{Kind: shell.PointerLeave, Pos: driver.Point{X: 60, Y: 60}})
	if want := []shell.Cursor{shell.CursorCrosshair, shell.CursorArrow}; !equalCursors(set, want) {
		t.Errorf("applied %v, want %v", set, want)
	}
}

func equalCursors(a, b []shell.Cursor) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
