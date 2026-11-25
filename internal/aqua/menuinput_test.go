package aqua

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// menuEnv is a menu in a grab popup of a shown driver window (so with the
// frame clock's timeline), for the interaction tests (S5.5i).
type menuEnv struct {
	t   *testing.T
	d   *Driver
	s   *fake.Shell
	w   *Window
	sf  *fake.Surface
	m   *menu
	p   *popupRoot
	log []string // "pick i", "cancel"
}

// runMenu runs f on a started driver with a shown 300×200 window and a
// menu of items, not yet open.
func runMenu(t *testing.T, items []menuItem, f func(e *menuEnv)) {
	t.Helper()
	sh := fake.New()
	d := New(sh)
	h := &appLog{}
	h.onReady = func() {
		defer d.Quit()
		dw, err := d.NewWindow(driver.WindowConfig{Size: driver.Size{W: 300, H: 200}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		w := dw.(*Window)
		w.Show()
		sh.FakeClock().Advance(0)
		e := &menuEnv{t: t, d: d, s: sh, w: w, sf: w.Surface().(*fake.Surface), m: newMenu(sh.TextSystem())}
		e.m.SetItems(items)
		fit := e.m.FittingSize(driver.Size{W: 1e6, H: 1e6})
		e.m.SetFrame(driver.Rect{W: fit.W, H: fit.H}) // where it opens, for a press before
		e.m.SetActions(func(i int) {
			e.log = append(e.log, fmt.Sprint("pick ", i))
			e.p.Close()
		}, func() {
			e.log = append(e.log, "cancel")
			e.p.Close()
		})
		f(e)
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}

// open opens the menu at (20, 40) of the content area, h points high (0:
// its fitting height), and begins its interaction as op says.
func (e *menuEnv) open(h float64, op menuOpening) {
	e.t.Helper()
	size := e.m.FittingSize(driver.Size{W: 1e6, H: 1e6})
	if h > 0 {
		size.H = h
	}
	p, err := e.w.openPopup(popupOptions{
		Config: shell.PopupConfig{
			Anchor: driver.Rect{X: 20, Y: 40 + e.w.titleBarHeight(), W: 10, H: 10},
			Size:   size,
			Grab:   true,
		},
		Content: e.m,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	e.p = p
	e.m.SetHighlighted(-1) // the last pick stays highlighted
	e.m.Begin(op)
}

// at returns the surface point of q in the menu's points: the popup opens
// at the anchor's top-left corner.
func (e *menuEnv) at(q driver.Point) driver.Point {
	return driver.Point{X: 20 + q.X, Y: 40 + e.w.titleBarHeight() + q.Y}
}

// item returns the surface point in the middle of item i.
func (e *menuEnv) item(i int) driver.Point {
	r := e.m.ItemRect(i)
	return e.at(driver.Point{X: r.X + r.W/2, Y: r.Y + r.H/2})
}

// outside is a surface point off the popup.
func (e *menuEnv) outside() driver.Point { return e.at(driver.Point{X: -10, Y: 5}) }

func (e *menuEnv) now() time.Duration { return e.s.FakeClock().Now() }

// wantLog checks the reports since the last call.
func (e *menuEnv) wantLog(what string, want ...string) {
	e.t.Helper()
	if !slices.Equal(e.log, want) {
		e.t.Errorf("%s: reports %q, want %q", what, e.log, want)
	}
	e.log = nil
}

// wantHighlight checks the highlighted item.
func (e *menuEnv) wantHighlight(what string, i int) {
	e.t.Helper()
	if got := e.m.Highlighted(); got != i {
		e.t.Errorf("%s: highlighted %d, want %d", what, got, i)
	}
}

// finishBlink runs the pick's blink to its report.
func (e *menuEnv) finishBlink() { e.s.FakeClock().Advance(menuBlinkOff + menuBlinkOn) }

// TestMenuHover: the enabled item under the pointer is highlighted, none
// over a disabled item, a separator, the padding or off the menu; a
// keyboard highlight stays under a still pointer.
func TestMenuHover(t *testing.T) {
	runMenu(t, sampleMenuItems, func(e *menuEnv) {
		e.open(0, menuOpening{})
		if !e.m.Tracking() || e.m.FocusState() != FocusedActive {
			t.Fatalf("tracking %v, focus %v", e.m.Tracking(), e.m.FocusState())
		}
		for _, c := range []struct {
			at   driver.Point
			want int
		}{
			{e.item(0), 0}, {e.item(1), 1}, {e.item(2), -1}, {e.item(5), 5}, {e.item(3), -1},
			{e.item(4), -1}, {e.item(0), 0}, {e.at(pt(30, 1)), -1}, {e.item(1), 1}, {e.outside(), -1},
		} {
			e.sf.Move(c.at)
			e.wantHighlight(fmt.Sprintf("pointer at %v", c.at), c.want)
		}
		e.sf.Move(e.item(5))
		e.sf.Key(driver.KeyUp, 0)
		e.wantHighlight("Up from 5", 1)
		e.sf.Move(e.item(5)) // no movement: the keyboard's highlight stays
		e.wantHighlight("still pointer", 1)
		e.sf.Move(e.item(5).Add(driver.Point{X: 1}))
		e.wantHighlight("moved pointer", 5)
		e.wantLog("hover")
	})
}

// TestMenuClick: a menu opened by a quick press stays open at its release
// for a click; a click on a disabled item, a separator or the padding does
// nothing, on an enabled item it blinks and picks; a press inside released
// outside cancels.
func TestMenuClick(t *testing.T) {
	runMenu(t, sampleMenuItems, func(e *menuEnv) {
		start := e.item(1)
		e.sf.Move(start)
		e.sf.Press(start, shell.ButtonPrimary)
		e.open(0, menuOpening{Press: true, Time: e.now()})
		e.s.FakeClock().Advance(menuReleaseDelay / 2)
		e.sf.Release(start, shell.ButtonPrimary)
		if e.p.Closed() || !e.m.Tracking() {
			t.Fatal("a quick release closed the menu")
		}
		e.wantLog("quick release")
		for _, i := range []int{2, 4} {
			e.sf.Click(e.item(i))
		}
		e.sf.Click(e.at(pt(30, 1)))
		e.wantLog("clicks on what cannot be picked")
		if e.p.Closed() {
			t.Fatal("closed by a click on a disabled item")
		}

		// The pick blinks: off, on, then the report.
		e.sf.Click(e.item(5))
		e.wantHighlight("blink off", -1)
		e.wantLog("blink")
		if got := blinkTimers(e.s); !slices.Equal(got, []time.Duration{menuBlinkOff}) {
			t.Errorf("blink timers %v", got)
		}
		// Input during the blink is eaten.
		e.sf.Move(e.item(0))
		if e.sf.Key(driver.KeyEscape, 0) != true {
			t.Error("Escape during the blink not eaten")
		}
		e.s.FakeClock().Advance(menuBlinkOff)
		e.wantHighlight("blink on", 5)
		e.wantLog("blink on")
		e.s.FakeClock().Advance(menuBlinkOn)
		e.wantLog("after the blink", "pick 5")
		if !e.p.Closed() || e.m.Tracking() || len(e.s.Timers()) != 0 {
			t.Errorf("after the pick: closed %v, tracking %v, timers %v", e.p.Closed(), e.m.Tracking(), e.s.Timers())
		}

		// Reopened by a key: a press inside released outside cancels.
		e.open(0, menuOpening{})
		e.sf.Press(e.item(0), shell.ButtonPrimary)
		e.wantHighlight("pressed", 0)
		e.sf.Move(e.outside())
		e.wantHighlight("dragged off", -1)
		e.sf.Release(e.outside(), shell.ButtonPrimary)
		e.wantLog("released outside", "cancel")
		if !e.p.Closed() {
			t.Error("not closed")
		}
	})
}

// TestMenuPressDragRelease: the opening press dragged over an item and
// released picks it; released late without a drag it picks too; dragged
// off the menu and released outside it cancels; over a separator nothing
// happens and the menu stays for a click.
func TestMenuPressDragRelease(t *testing.T) {
	runMenu(t, sampleMenuItems, func(e *menuEnv) {
		openPressed := func(at driver.Point) {
			e.sf.Move(at)
			e.sf.Press(at, shell.ButtonPrimary)
			e.open(0, menuOpening{Press: true, Time: e.now()})
		}

		// Dragged to item 5 and released at once: a pick.
		openPressed(e.item(0))
		e.sf.Move(e.item(1))
		e.sf.Move(e.item(5))
		e.wantHighlight("dragged", 5)
		e.sf.Release(e.item(5), shell.ButtonPrimary)
		e.finishBlink()
		e.wantLog("drag", "pick 5")

		// Held still past the delay, then released: a pick.
		openPressed(e.item(1))
		e.s.FakeClock().Advance(menuReleaseDelay)
		e.sf.Release(e.item(1), shell.ButtonPrimary)
		e.finishBlink()
		e.wantLog("held", "pick 1")

		// A move within the slop is no drag.
		openPressed(e.item(1))
		e.sf.Move(e.item(1).Add(driver.Point{X: 2}))
		e.sf.Release(e.item(1).Add(driver.Point{X: 2}), shell.ButtonPrimary)
		e.finishBlink()
		e.wantLog("within the slop")
		if e.p.Closed() {
			t.Fatal("closed by a release within the slop")
		}
		e.sf.Key(driver.KeyEscape, 0)
		e.wantLog("Escape", "cancel")

		// Dragged off the menu and released outside: a cancel.
		openPressed(e.item(0))
		e.sf.Move(e.item(1))
		e.sf.Move(e.outside())
		e.wantHighlight("off the menu", -1)
		e.sf.Release(e.outside(), shell.ButtonPrimary)
		e.wantLog("released outside", "cancel")
		if !e.p.Closed() {
			t.Fatal("not closed by the release outside")
		}

		// Released late outside without a drag: a cancel.
		openPressed(e.item(0))
		e.sf.Move(e.outside())
		e.s.FakeClock().Advance(menuReleaseDelay)
		e.sf.Release(e.outside(), shell.ButtonPrimary)
		e.wantLog("late outside", "cancel")

		// Dragged to the separator: the menu stays, and a click picks.
		openPressed(e.item(0))
		e.sf.Move(e.item(4))
		e.sf.Release(e.item(4), shell.ButtonPrimary)
		e.wantLog("separator")
		if e.p.Closed() {
			t.Fatal("closed by a release on the separator")
		}
		e.sf.Click(e.item(0))
		e.finishBlink()
		e.wantLog("click after", "pick 0")
	})
}

// TestMenuKeyboard: Up and Down skip what cannot be picked and stop at
// the ends, Home and End go to them, Return and Space pick, Escape and
// Return without a highlight cancel.
func TestMenuKeyboard(t *testing.T) {
	runMenu(t, sampleMenuItems, func(e *menuEnv) {
		e.open(0, menuOpening{})
		for _, c := range []struct {
			key  driver.Key
			want int
		}{
			{driver.KeyDown, 0}, {driver.KeyDown, 1}, {driver.KeyDown, 5}, {driver.KeyDown, 5},
			{driver.KeyUp, 1}, {driver.KeyUp, 0}, {driver.KeyUp, 0},
			{driver.KeyEnd, 5}, {driver.KeyHome, 0}, {driver.KeyPageDown, 5}, {driver.KeyPageUp, 0},
		} {
			if !e.sf.Key(c.key, 0) {
				t.Errorf("key %v not handled", c.key)
			}
			e.wantHighlight(fmt.Sprintf("key %v", c.key), c.want)
		}
		if e.sf.Key(driver.KeyLeft, 0) {
			t.Error("Left handled")
		}
		e.sf.Key(driver.KeyReturn, 0)
		e.finishBlink()
		e.wantLog("Return", "pick 0")

		e.open(0, menuOpening{})
		e.sf.Key(driver.KeyUp, 0)
		e.wantHighlight("Up from none", 5)
		e.sf.Type(" ")
		e.finishBlink()
		e.wantLog("Space", "pick 5")

		e.open(0, menuOpening{})
		e.sf.Key(driver.KeyEscape, 0)
		e.wantLog("Escape", "cancel")

		e.open(0, menuOpening{})
		e.sf.Key(driver.KeyReturn, 0)
		e.wantLog("Return without a highlight", "cancel")
	})
}

// TestMenuTypeSelect: typed characters select by a case-insensitive
// prefix of the enabled titles, the prefix growing within the reset time;
// a Space while it grows is typed, later it picks.
func TestMenuTypeSelect(t *testing.T) {
	items := []menuItem{{Title: "Apple"}, {Title: "Banana"}, {Title: "Blueberry"}, {Title: "Cherry", Disabled: true}, {Title: "Cranberry"}, {Title: "C D"}}
	runMenu(t, items, func(e *menuEnv) {
		e.open(0, menuOpening{})
		clk := e.s.FakeClock()
		e.sf.Type("b")
		e.wantHighlight("b", 1)
		clk.Advance(menuTypeSelectReset / 2)
		e.sf.Type("L")
		e.wantHighlight("bL", 2)
		clk.Advance(menuTypeSelectReset)
		e.sf.Type("c")
		e.wantHighlight("c, past the disabled Cherry", 4)
		e.sf.Type("x")
		e.wantHighlight("no match", 4)
		clk.Advance(menuTypeSelectReset)
		e.sf.Type("c d") // the Space is typed
		e.wantHighlight("c d", 5)
		e.wantLog("typing")
		clk.Advance(menuTypeSelectReset)
		e.sf.Type(" ")
		e.finishBlink()
		e.wantLog("Space after the reset", "pick 5")
	})
}

// TestMenuPickReduceMotion: with Reduce Motion the pick is reported at
// once, without a blink.
func TestMenuPickReduceMotion(t *testing.T) {
	runMenu(t, sampleMenuItems, func(e *menuEnv) {
		a := e.s.Appearance()
		a.ReduceMotion = true
		e.s.SetAppearance(a)
		e.open(0, menuOpening{})
		e.sf.Key(driver.KeyDown, 0)
		e.sf.Key(driver.KeyReturn, 0)
		e.wantLog("Reduce Motion", "pick 0")
		if len(e.s.Timers()) != 0 {
			t.Errorf("timers %v", e.s.Timers())
		}
	})
}

// TestMenuClosedWhileBlinking: a popup closed during the blink (a
// dismissal) reports nothing and leaves no timer.
func TestMenuClosedWhileBlinking(t *testing.T) {
	runMenu(t, sampleMenuItems, func(e *menuEnv) {
		e.open(0, menuOpening{})
		e.sf.Key(driver.KeyDown, 0)
		e.sf.Key(driver.KeyReturn, 0)
		e.p.Popup().(*fake.Popup).UserDismiss()
		if !e.p.Closed() || e.m.Tracking() || len(e.s.Timers()) != 0 {
			t.Errorf("closed %v, tracking %v, timers %v", e.p.Closed(), e.m.Tracking(), e.s.Timers())
		}
		e.finishBlink()
		e.wantLog("dismissed")
	})
}

// numberedItems returns n items "Item 0" … "Item n-1".
func numberedItems(n int) []menuItem {
	items := make([]menuItem, n)
	for i := range items {
		items[i].Title = fmt.Sprint("Item ", i)
	}
	return items
}

// TestMenuScrollArrows: a panel lower than its items shows the down arrow,
// whose hover scrolls on the frame clock until the end, where it goes and
// the up arrow shows; no frames are left after; the arrows hold no item.
func TestMenuScrollArrows(t *testing.T) {
	runMenu(t, numberedItems(30), func(e *menuEnv) {
		mt := &tokens.Menu
		e.open(150, menuOpening{})
		m, clk := e.m, e.s.FakeClock()
		max := m.ContentHeight() - 150
		if m.MaxScroll() != max || m.ScrollOffset() != 0 {
			t.Fatalf("max scroll %v (want %v), offset %v", m.MaxScroll(), max, m.ScrollOffset())
		}
		down := m.ArrowRect(1)
		if !m.ArrowRect(-1).Empty() || down != (driver.Rect{Y: 150 - mt.ScrollArrowHeight, W: m.Panel().W, H: mt.ScrollArrowHeight}) {
			t.Fatalf("arrows %v, %v", m.ArrowRect(-1), down)
		}
		mid := driver.Point{X: down.X + down.W/2, Y: down.Y + down.H/2}
		if m.ItemAt(mid) != -1 {
			t.Error("an item under the down arrow")
		}
		e.sf.Move(e.at(mid))
		e.wantHighlight("on the arrow", -1)
		if !clk.Requested() {
			t.Fatal("no frame for the arrow")
		}
		clk.Advance(500 * time.Millisecond)
		got := m.ScrollOffset()
		if want := menuArrowSpeed / 2; math.Abs(got-want) > menuArrowSpeed*fake.FrameInterval.Seconds() {
			t.Errorf("half a second on the arrow: scrolled %v, want about %v", got, want)
		}
		if m.ArrowRect(-1).Empty() {
			t.Error("no up arrow once scrolled")
		}
		// Off the arrow it stops.
		e.sf.Move(e.item(10))
		clk.Advance(time.Second) // the frame already requested
		if n := clk.Advance(time.Second); n != 0 || m.ScrollOffset() != got {
			t.Errorf("off the arrow: %d frames, offset %v (was %v)", n, m.ScrollOffset(), got)
		}
		// Back on it, to the end: the down arrow goes, the item now under
		// the pointer is highlighted, no frames left.
		e.sf.Move(e.at(mid))
		clk.Advance(10 * time.Second)
		if m.ScrollOffset() != max || !m.ArrowRect(1).Empty() {
			t.Fatalf("at the end: offset %v, down arrow %v", m.ScrollOffset(), m.ArrowRect(1))
		}
		e.wantHighlight("at the end", 29)
		if n := clk.Advance(time.Second); n != 0 {
			t.Errorf("%d frames at the end", n)
		}
		// The up arrow scrolls back.
		up := m.ArrowRect(-1)
		e.sf.Move(e.at(driver.Point{X: up.W / 2, Y: up.Y + up.H/2}))
		clk.Advance(10 * time.Second)
		if m.ScrollOffset() != 0 || !m.ArrowRect(-1).Empty() {
			t.Errorf("back at the top: offset %v", m.ScrollOffset())
		}
		e.wantHighlight("at the top", 0)
		e.wantLog("arrows")
	})
}

// TestMenuScrollWheelAndKeys: the wheel scrolls at once by wheel lines,
// clamped, the highlight following the pointer; the keyboard scrolls its
// highlight into view between the arrows; a picked item after a scroll is
// the one under the pointer.
func TestMenuScrollWheelAndKeys(t *testing.T) {
	runMenu(t, numberedItems(30), func(e *menuEnv) {
		e.open(150, menuOpening{})
		m := e.m
		at := e.item(2)
		e.sf.Move(at)
		e.wantHighlight("before the wheel", 2)
		e.sf.Wheel(at, driver.Point{Y: -2})
		if want := 2 * tokens.Scroll.WheelLineHeight; m.ScrollOffset() != want {
			t.Errorf("wheel offset %v, want %v", m.ScrollOffset(), want)
		}
		if i := m.ItemAt(driver.Point{X: at.X - e.p.Rect().X, Y: at.Y - e.p.Rect().Y}); m.Highlighted() != i || i <= 2 {
			t.Errorf("after the wheel: highlighted %d, under the pointer %d", m.Highlighted(), i)
		}
		e.sf.Wheel(at, driver.Point{Y: 100})
		if m.ScrollOffset() != 0 {
			t.Errorf("wheel past the top: %v", m.ScrollOffset())
		}

		// The keyboard: End scrolls to the end, Home back; each Down and
		// Up shows its item whole between the arrows, scrolling only when
		// it was not.
		e.sf.Key(driver.KeyEnd, 0)
		if m.ScrollOffset() != m.MaxScroll() {
			t.Errorf("End: offset %v", m.ScrollOffset())
		}
		e.sf.Key(driver.KeyHome, 0)
		if m.ScrollOffset() != 0 {
			t.Errorf("Home: offset %v", m.ScrollOffset())
		}
		shown := func(i int) bool {
			r, a := m.ItemRect(i), m.itemsArea()
			return r.Y >= a.Y && r.Y+r.H <= a.Y+a.H
		}
		for _, k := range []driver.Key{driver.KeyDown, driver.KeyUp} {
			for range 29 {
				before := m.ScrollOffset()
				next := m.Highlighted() + 1
				if k == driver.KeyUp {
					next -= 2
				}
				was := shown(next)
				e.sf.Key(k, 0)
				if m.Highlighted() != next || !shown(next) {
					t.Fatalf("key %v to %d: highlighted %d, shown %v", k, next, m.Highlighted(), shown(next))
				}
				if was && m.ScrollOffset() != before {
					t.Errorf("key %v to the shown %d scrolled %v → %v", k, next, before, m.ScrollOffset())
				}
			}
		}
		if m.ScrollOffset() != 0 {
			t.Errorf("Up to the first: offset %v", m.ScrollOffset())
		}
		e.wantLog("scrolling")
	})
}

// TestPopupRootStrayReleaseOutside: the opening press released off the
// popup reaches its content with Over false.
func TestPopupRootStrayReleaseOutside(t *testing.T) {
	var log []string
	w, sf, _, win := popupWindow(t, &log)
	c := newPopupProbe(&log, "menu", "up")
	win.onEvent = func(ev Event) {
		if pe, ok := ev.(PointerEvent); ok && pe.Kind == PointerDown {
			openTestPopup(t, w, c, true, &log)
		}
	}
	tb := w.titleBarHeight()
	sf.Press(driver.Point{X: 25, Y: 45 + tb}, shell.ButtonPrimary)
	log = nil
	sf.Move(driver.Point{X: 200, Y: 150 + tb})
	sf.Release(driver.Point{X: 200, Y: 150 + tb}, shell.ButtonPrimary)
	checkLog(t, &log, "menu up 180,110 off")
}

// TestMenuTypeSelectRepeat (review R5): in a menu, as in the menu strip,
// the same letter again steps to the next enabled item it starts,
// wrapping, instead of matching nothing.
func TestMenuTypeSelectRepeat(t *testing.T) {
	items := []menuItem{{Title: "Banana"}, {Title: "Apple"}, {Title: "Blueberry"}, {Title: "Berry", Disabled: true}}
	runMenu(t, items, func(e *menuEnv) {
		e.open(0, menuOpening{})
		e.sf.Type("b")
		e.wantHighlight("b", 0)
		e.sf.Type("b")
		e.wantHighlight("b again", 2)
		e.sf.Type("b")
		e.wantHighlight("b again, past the disabled Berry, wrapping", 0)
	})
}
