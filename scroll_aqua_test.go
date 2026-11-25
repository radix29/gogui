package gogui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	fakeshell "github.com/radix29/gogui/internal/shell/fake"
)

// The notes sidebar scrolling on the aqua driver through the fake shell
// (PLAN.md S3.5d): wheel steps, swipes with momentum, the edges, latching,
// paint culling and the a11y frames following the offset.

// notesEnv is the notes window (its sidebar: 30 checkboxes, document
// 180 × 736 in a 451 pt viewport, so the offset runs 0…285).
type notesEnv struct {
	t       *testing.T
	app     *App
	sh      *fakeshell.Shell
	sf      *fakeshell.Surface
	clk     *fakeshell.Clock
	aw      *aqua.Window
	sidebar *ScrollView
	sv      *aqua.ScrollView
	boxes   []*Checkbox
	scrolls []Point // OnScroll
	in      driver.Point
	y0      float64 // checkbox 1 at offset 0
	editor  driver.Point
}

const notesMax = 736 - 451

func runNotes(t *testing.T, body func(e *notesEnv)) {
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		e := &notesEnv{t: t, app: app, sh: sh, clk: sh.FakeClock()}
		settings := VStack(8)
		for i := 1; i <= 30; i++ {
			c := NewCheckbox(fmt.Sprintf("Option %d", i))
			e.boxes = append(e.boxes, c)
			settings.Add(c)
		}
		e.sidebar = NewScrollView(settings.SetPadding(12)).SetHasVerticalScroller(true).SetMinWidth(180)
		e.sidebar.OnScroll(func(p Point) { e.scrolls = append(e.scrolls, p) })
		editor := NewTextView().SetFont(SystemFont(13))
		win := app.NewWindow(WindowConfig{
			Title: "Notes", Size: Size{W: 720, H: 480},
			Style: Titled | Closable | Miniaturizable | Resizable,
		})
		win.SetContent(VStack(0,
			HStack(0, e.sidebar, NewSeparator().SetVertical(true), editor),
			NewSeparator(),
			HStack(8, NewLabel("stats"), Spacer()).SetPadding(6),
		))
		showExample(app, win)
		e.aw = win.p.(*aqua.Window)
		e.sf = e.aw.Surface().(*fakeshell.Surface)
		e.sv = e.sidebar.peerWidget().(*aqua.ScrollView)
		tree := aqua.AccessibilityTree(e.aw.Root())
		area := tree.Find(a11y.RoleScrollArea, "")
		ed := tree.Find(a11y.RoleTextArea, "")
		if area == nil || ed == nil {
			t.Fatalf("no scroll area or text area:\n%s", tree.Dump())
		}
		f := area.Frame
		e.in = driver.Point{X: f.X + f.W/2, Y: f.Y + f.H/2}
		e.editor = driver.Point{X: ed.Frame.X + ed.Frame.W/2, Y: ed.Frame.Y + ed.Frame.H/2}
		if e.sv.MaxOffset().Y != notesMax {
			t.Fatalf("max offset %v, want %d", e.sv.MaxOffset(), notesMax)
		}
		e.y0 = e.boxY(1)
		body(e)
	})
}

// settle runs frames until the timeline is idle (at most 10 s).
func (e *notesEnv) settle() {
	e.t.Helper()
	d := e.app.drv.(*aqua.Driver)
	for i := 0; i < 600 && (d.Timeline().Running() > 0 || e.clk.Requested()); i++ {
		e.clk.Advance(fakeshell.FrameInterval)
	}
	if d.Timeline().Running() > 0 {
		e.t.Fatalf("still scrolling at %v", e.sidebar.Offset())
	}
	drain(e.app)
}

// boxY returns the window y of checkbox i (from 1) in the a11y tree.
func (e *notesEnv) boxY(i int) float64 {
	e.t.Helper()
	n := aqua.AccessibilityTree(e.aw.Root()).Find(a11y.RoleCheckBox, fmt.Sprintf("Option %d", i))
	if n == nil {
		e.t.Fatalf("no checkbox %d", i)
	}
	return n.Frame.Y
}

// docY returns the document host's y in the viewport (minus the shown
// offset, overscroll included).
func (e *notesEnv) docY() float64 {
	return e.boxY(1) - e.y0
}

// painted reports whether the last paint drew checkbox i's title.
func (e *notesEnv) painted(i int) bool {
	e.clk.Flush()
	return strings.Contains(e.sf.Dump(), fmt.Sprintf("%q", fmt.Sprintf("Option %d", i)))
}

func TestNotesWheelOnAqua(t *testing.T) {
	runNotes(t, func(e *notesEnv) {
		y1 := e.boxY(1)
		e.sf.Wheel(e.in, driver.Point{Y: -3}) // three lines down the document
		e.settle()
		want := 3 * theme.Sequoia.Scroll.WheelLineHeight
		if got := e.sidebar.Offset(); got != (Point{Y: want}) {
			t.Fatalf("offset after the wheel %v, want %g", got, want)
		}
		if len(e.scrolls) == 0 || e.scrolls[len(e.scrolls)-1] != (Point{Y: want}) {
			t.Errorf("OnScroll %v", e.scrolls)
		}
		if got := e.boxY(1); got != y1-want {
			t.Errorf("a11y frame at %g, want %g", got, y1-want)
		}
		// Up past the top edge: the wheel clamps, then is not taken.
		e.sf.Wheel(e.in, driver.Point{Y: 10})
		e.settle()
		if e.sidebar.Offset().Y != 0 {
			t.Errorf("offset %v after scrolling up", e.sidebar.Offset())
		}
		if e.sf.Send(shell.ScrollEvent{Pos: e.in, Delta: driver.Point{Y: 1}}) {
			t.Error("wheel past the top edge handled")
		}
		// Over the editor the sidebar does not scroll.
		e.sf.Wheel(e.editor, driver.Point{Y: -3})
		e.settle()
		if e.sidebar.Offset().Y != 0 {
			t.Errorf("the editor's wheel scrolled the sidebar to %v", e.sidebar.Offset())
		}
		// The horizontal axis has no scroller: its delta is not taken.
		if e.sf.Send(shell.ScrollEvent{Pos: e.in, Delta: driver.Point{X: -3}}) {
			t.Error("horizontal wheel handled")
		}
	})
}

func TestNotesSwipeOnAqua(t *testing.T) {
	runNotes(t, func(e *notesEnv) {
		// A slow drag scrolls 1:1 and stops (the fingers rest before the
		// Win32 end): no momentum.
		e.sf.TimedSwipe(e.in, 10*time.Millisecond, driver.Point{Y: -10}, driver.Point{Y: -10})
		if got := e.sidebar.Offset().Y; got != 20 {
			t.Fatalf("drag: offset %g, want 20", got)
		}
		e.settle()
		if got := e.sidebar.Offset().Y; !(got > 20) {
			t.Fatalf("fling: no momentum (offset %g)", got)
		}
		at := e.sidebar.Offset().Y
		n := len(e.scrolls)
		if n < 3 || e.scrolls[n-1].Y != at {
			t.Errorf("OnScroll %d calls, last %v (offset %g)", n, e.scrolls[n-1:], at)
		}

		// A fast fling into the end: overscroll, then back to the edge.
		var deltas []driver.Point
		for range 8 {
			deltas = append(deltas, driver.Point{Y: -40})
		}
		minDoc := 0.0
		e.sf.TimedSwipe(e.in, 10*time.Millisecond, deltas...)
		for range 300 {
			e.clk.Advance(fakeshell.FrameInterval)
			minDoc = min(minDoc, e.docY())
		}
		e.settle()
		if e.sidebar.Offset().Y != notesMax || e.docY() != -notesMax || !(minDoc < -notesMax) {
			t.Errorf("fling into the end: offset %v, doc at %g, deepest %g", e.sidebar.Offset(), e.docY(), minDoc)
		}
		if e.scrolls[len(e.scrolls)-1].Y != notesMax {
			t.Errorf("last OnScroll %v", e.scrolls[len(e.scrolls)-1])
		}

		// Dragged past the top and released: rubber-band, spring back.
		e.sidebar.SetOffset(Point{})
		e.sf.Send(shell.ScrollEvent{Pos: e.in, Delta: driver.Point{Y: 60}, Precise: true, Phase: shell.PhaseBegan})
		if !(e.docY() > 0 && e.docY() < 60) || e.sidebar.Offset().Y != 0 {
			t.Errorf("overscroll: doc at %g, offset %v", e.docY(), e.sidebar.Offset())
		}
		e.clk.Advance(time.Second)
		e.sf.Send(shell.ScrollEvent{Pos: e.in, Precise: true, Phase: shell.PhaseEnded})
		e.settle()
		if e.docY() != 0 {
			t.Errorf("spring-back to %g", e.docY())
		}
	})
}

func TestNotesLatchingOnAqua(t *testing.T) {
	runNotes(t, func(e *notesEnv) {
		ev := func(p driver.Point, d float64, ph shell.ScrollPhase) bool {
			return e.sf.Send(shell.ScrollEvent{Pos: p, Delta: driver.Point{Y: d}, Precise: true, Phase: ph})
		}
		if !ev(e.in, -10, shell.PhaseBegan) {
			t.Fatal("began not handled")
		}
		// The pointer moves over the editor: the gesture stays.
		ev(e.editor, -10, shell.PhaseChanged)
		ev(e.editor, -10, shell.PhaseChanged)
		if got := e.sidebar.Offset().Y; got != 30 {
			t.Errorf("latched gesture: offset %g, want 30", got)
		}
		e.clk.Advance(time.Second)
		ev(e.editor, 0, shell.PhaseEnded)
		e.settle()
		// OS momentum after the end (GTK) goes to the sidebar too.
		ev(e.in, -5, shell.PhaseBegan)
		ev(e.in, 0, shell.PhaseEnded)
		e.sf.Move(e.editor)
		e.sf.Momentum(driver.Point{Y: -7}, driver.Point{Y: -3})
		if got := e.sidebar.Offset().Y; got != 45 {
			t.Errorf("OS momentum: offset %g, want 45", got)
		}
		// A gesture over the editor does not scroll the sidebar.
		e.sf.Swipe(e.editor, driver.Point{Y: -10})
		e.settle()
		if got := e.sidebar.Offset().Y; got != 45 {
			t.Errorf("editor gesture moved the sidebar to %g", got)
		}
	})
}

func TestNotesCullingOnAqua(t *testing.T) {
	runNotes(t, func(e *notesEnv) {
		if !e.painted(1) || e.painted(30) {
			t.Fatal("at the top: option 1 not painted or option 30 painted")
		}
		e.sidebar.SetOffset(Point{Y: notesMax})
		if e.painted(1) || !e.painted(30) {
			t.Error("at the end: option 1 painted or option 30 not painted")
		}
		if len(e.scrolls) != 0 {
			t.Errorf("SetOffset called OnScroll: %v", e.scrolls)
		}
		// SetOffset during momentum stops it.
		e.sidebar.SetOffset(Point{})
		e.sf.TimedSwipe(e.in, 10*time.Millisecond, driver.Point{Y: -30}, driver.Point{Y: -30}, driver.Point{Y: -30})
		e.clk.Advance(3 * fakeshell.FrameInterval)
		e.sidebar.SetOffset(Point{Y: 100})
		e.settle()
		if got := e.sidebar.Offset().Y; got != 100 {
			t.Errorf("SetOffset during momentum: %g", got)
		}
	})
}
