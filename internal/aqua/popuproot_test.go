package aqua

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// popupProbe is a probe that paints a solid fill and can show a cursor.
type popupProbe struct {
	probe
	cursor shell.Cursor
}

func newPopupProbe(log *[]string, name string, handles ...string) *popupProbe {
	p := &popupProbe{probe: probe{name: name, log: log, handles: map[string]bool{}}}
	p.Init(p)
	for _, k := range handles {
		p.handles[k] = true
	}
	return p
}

func (p *popupProbe) Paint(c render.Canvas) {
	c.FillRoundedRect(p.Bounds(), render.Radii{}, render.Solid(semantic(driver.ColorControlAccent, p.Window().Appearance())))
}

func (p *popupProbe) Cursor() shell.Cursor { return p.cursor }

// popupWindow opens a titled window with a probe "win" covering its
// content and the pointer over it.
func popupWindow(t *testing.T, log *[]string) (*Window, *fake.Surface, *fake.Shell, *probe) {
	t.Helper()
	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "P", Size: driver.Size{W: 300, H: 200}, Style: allStyles})
	win := newProbe(log, "win", 0, 0, 300, 200, "down", "key")
	w.content.SetChildren(win)
	sf.Move(driver.Point{X: 50, Y: 100})
	*log = nil
	return w, sf, s, win
}

// openTestPopup opens a popup with content c at (20, 40) of the content
// area, 100×60, and records Placed and Closed in log.
func openTestPopup(t *testing.T, w *Window, c Element, grab bool, log *[]string) *popupRoot {
	t.Helper()
	p, err := w.openPopup(popupOptions{
		Config: shell.PopupConfig{
			Anchor: driver.Rect{X: 20, Y: 40 + w.titleBarHeight(), W: 10, H: 10},
			Size:   driver.Size{W: 100, H: 60},
			Grab:   grab,
		},
		Content: c,
		Placed: func(r driver.Rect, scale float64) {
			*log = append(*log, "placed "+rectString(r))
		},
		Closed: func() { *log = append(*log, "closed") },
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func rectString(r driver.Rect) string {
	return fmt.Sprintf("%g %g %g %g", r.X, r.Y, r.W, r.H)
}

func ftoa(v float64) string { return fmt.Sprintf("%g", v) }

func itoa(v int) string { return fmt.Sprint(v) }

func TestPopupRootOpenPaintClose(t *testing.T) {
	var log []string
	w, sf, s, _ := popupWindow(t, &log)
	c := newPopupProbe(&log, "menu")
	p := openTestPopup(t, w, c, false, &log)
	tb := w.titleBarHeight()
	checkLog(t, &log, "placed 20 "+ftoa(40+tb)+" 100 60")

	if got := sf.Popups(); len(got) != 1 || got[0] != p.Popup() {
		t.Fatalf("shell popups %v", got)
	}
	if p.Frame() != (driver.Rect{W: 100, H: 60}) || c.Frame() != p.Frame() {
		t.Fatalf("frames root %v content %v", p.Frame(), c.Frame())
	}
	if c.Window() != w || p.Rect() != (driver.Rect{X: 20, Y: 40 + tb, W: 100, H: 60}) || p.Scale() != 1 {
		t.Fatalf("window %p rect %v scale %v", c.Window(), p.Rect(), p.Scale())
	}
	if p.Translucent() || p.Grab() || w.GrabPopupOpen() {
		t.Fatal("translucent, grab or grab open")
	}

	fp := p.Popup().(*fake.Popup)
	s.FakeClock().Flush()
	if fp.Paints() != 1 || fp.PaintErr() != nil || !strings.Contains(fp.Dump(), "fillRRect (0,0,100,60)") {
		t.Fatalf("paints %d err %v dump:\n%s", fp.Paints(), fp.PaintErr(), fp.Dump())
	}
	// An invalidation in the popup's tree repaints the popup only.
	paints := sf.Paints()
	c.Invalidate()
	s.FakeClock().Flush()
	if fp.Paints() != 2 || sf.Paints() != paints {
		t.Fatalf("popup paints %d, surface paints %d (was %d)", fp.Paints(), sf.Paints(), paints)
	}

	// An appearance change repaints it, with WindowStateEvent.
	a := w.Appearance()
	a.Dark = true
	w.SetAppearance(a)
	checkLog(t, &log, "win state key=true dark=true", "menu state key=true dark=true")
	if !fp.Invalid() {
		t.Fatal("appearance change did not invalidate the popup")
	}
	s.FakeClock().Flush()
	if !strings.Contains(fp.Dump(), "fillRRect (0,0,100,60)") || fp.Paints() != 3 {
		t.Fatalf("paints %d", fp.Paints())
	}

	// Reposition places it again.
	p.Reposition(shell.PopupConfig{Anchor: driver.Rect{X: 30, Y: 50}, Size: driver.Size{W: 80, H: 40}})
	checkLog(t, &log, "placed 30 50 80 40")
	if c.Frame() != (driver.Rect{W: 80, H: 40}) {
		t.Fatalf("content frame %v", c.Frame())
	}

	p.Close()
	checkLog(t, &log, "closed")
	if !p.Closed() || !fp.Closed() || c.Parent() != nil || p.Content() != nil || len(w.popups) != 0 {
		t.Fatalf("after Close: closed %v/%v parent %v", p.Closed(), fp.Closed(), c.Parent())
	}
	p.Close()
	p.Reposition(shell.PopupConfig{})
	if len(log) != 0 || p.Translucent() {
		t.Fatalf("closed popup acted: %v", log)
	}

	// The content can be reused for the next popup.
	p2 := openTestPopup(t, w, c, false, &log)
	if c.Parent() != Element(p2) || len(log) != 1 || p2.Content() != Element(c) {
		t.Fatalf("reuse: parent %v log %v", c.Parent(), log)
	}
}

func TestPopupRootTranslucent(t *testing.T) {
	var log []string
	w, _, s, _ := popupWindow(t, &log)
	s.SetPopupTranslucent(true)
	p := openTestPopup(t, w, newPopupProbe(&log, "menu"), false, &log)
	if !p.Translucent() {
		t.Fatal("not translucent")
	}
}

func TestPopupRootGrabInput(t *testing.T) {
	var log []string
	w, sf, _, win := popupWindow(t, &log)
	win.SetFocusable(true)
	if !win.Focus() {
		t.Fatal("window focus")
	}
	log = nil
	c := newPopupProbe(&log, "menu", "down", "up", "key", "move")
	c.SetFocusable(true)
	c.cursor = shell.CursorPointingHand
	p := openTestPopup(t, w, c, true, &log)
	tb := w.titleBarHeight()
	// Opening cancels the window's hover; the shell's capture cancel is
	// dropped.
	checkLog(t, &log, "placed 20 "+ftoa(40+tb)+" 100 60", "win leave 50,"+ftoa(100-tb))
	if !w.GrabPopupOpen() || len(w.router.HoverChain()) != 0 {
		t.Fatalf("grab open %v hover %v", w.GrabPopupOpen(), w.router.HoverChain())
	}

	// Pointer events reach the popup's tree in its points.
	sf.Move(driver.Point{X: 30, Y: 50 + tb})
	checkLog(t, &log, "menu enter 10,10", "menu move 10,10")
	if got := p.Popup().(*fake.Popup).Cursor(); got != shell.CursorPointingHand {
		t.Fatalf("popup cursor %v", got)
	}
	sf.Press(driver.Point{X: 31, Y: 50 + tb}, shell.ButtonPrimary)
	sf.Release(driver.Point{X: 32, Y: 50 + tb}, shell.ButtonPrimary)
	checkLog(t, &log, "menu move 11,10", "menu down 11,10 x1", "menu move 12,10", "menu up 12,10 x1")

	// Keys go to the popup's tree, not the window's: to its root without
	// a focus (nothing handles them there), then to its focus.
	if sf.KeyDown(driver.KeyDown, 0) {
		t.Fatal("key handled without a popup focus")
	}
	sf.KeyUp(driver.KeyDown, 0)
	checkLog(t, &log)
	if !c.Focus() || c.FocusState() != FocusedActive {
		t.Fatalf("popup focus %v", c.FocusState())
	}
	checkLog(t, &log, "menu focus true")
	if win.FocusState() != FocusedActive {
		t.Fatalf("window focus %v", win.FocusState())
	}
	if !sf.KeyDown(driver.KeyDown, 0) {
		t.Fatal("key not handled by the popup focus")
	}
	sf.KeyUp(driver.KeyDown, 0)
	checkLog(t, &log, "menu key "+itoa(int(driver.KeyDown))+" true", "menu key "+itoa(int(driver.KeyDown))+" false")
	sf.KeyDown(driver.KeyReturn, 0)
	checkLog(t, &log, "menu key "+itoa(int(driver.KeyReturn))+" true")

	// An outside press dismisses and closes it; the content loses its
	// hover and focus as it is detached.
	sf.Press(driver.Point{X: 250, Y: 150}, shell.ButtonPrimary)
	checkLog(t, &log, "menu leave 230,"+ftoa(150-40-tb), "menu focus false", "closed")
	if w.GrabPopupOpen() || !p.Closed() || len(sf.Popups()) != 0 {
		t.Fatal("not closed by the outside press")
	}
	sf.Release(driver.Point{X: 250, Y: 150}, shell.ButtonPrimary)
	sf.Move(driver.Point{X: 251, Y: 150})
	checkLog(t, &log, "win enter 250,"+ftoa(150-tb), "win move 250,"+ftoa(150-tb), "win move 251,"+ftoa(150-tb))
}

// TestPopupRootOpenFromPress opens a grab popup from a press handler, as a
// pop-up button does: the press captures nothing in the window, the
// pressed element gets PointerCancel, and the release (press-drag-release)
// reaches the popup's tree although its press went to the window.
func TestPopupRootOpenFromPress(t *testing.T) {
	var log []string
	w, sf, _, win := popupWindow(t, &log)
	c := newPopupProbe(&log, "menu", "up")
	var p *popupRoot
	win.onEvent = func(ev Event) {
		if pe, ok := ev.(PointerEvent); ok && pe.Kind == PointerDown {
			p = openTestPopup(t, w, c, true, &log)
		}
	}
	tb := w.titleBarHeight()
	sf.Press(driver.Point{X: 25, Y: 45 + tb}, shell.ButtonPrimary)
	checkLog(t, &log,
		"win move 25,"+ftoa(45), "win down 25,"+ftoa(45)+" x1",
		"placed 20 "+ftoa(40+tb)+" 100 60",
		"win leave 25,"+ftoa(45), "win cancel 25,"+ftoa(45))
	if w.router.Captured() != nil {
		t.Fatal("the window captured the press")
	}
	sf.Move(driver.Point{X: 40, Y: 60 + tb})
	sf.Release(driver.Point{X: 40, Y: 60 + tb}, shell.ButtonPrimary)
	checkLog(t, &log, "menu enter 20,20", "menu move 20,20", "menu up 20,20")
	if p.Closed() {
		t.Fatal("closed")
	}
}

func TestPopupRootDismissed(t *testing.T) {
	var log []string
	w, _, _, _ := popupWindow(t, &log)
	p := openTestPopup(t, w, newPopupProbe(&log, "menu"), true, &log)
	log = nil
	p.Popup().(*fake.Popup).UserDismiss()
	checkLog(t, &log, "closed")
	if !p.Closed() || w.GrabPopupOpen() {
		t.Fatal("Dismissed did not close")
	}
}

func TestPopupRootClosedWithWindow(t *testing.T) {
	for _, tc := range []struct {
		name string
		do   func(w *Window, sf *fake.Surface, s *fake.Shell)
	}{
		{"close", func(w *Window, _ *fake.Surface, _ *fake.Shell) { w.Close() }},
		{"user close", func(_ *Window, sf *fake.Surface, _ *fake.Shell) { sf.UserClose() }},
		{"minimize", func(_ *Window, sf *fake.Surface, _ *fake.Shell) { sf.Minimize() }},
		{"deactivate", func(_ *Window, _ *fake.Surface, s *fake.Shell) {
			other, err := s.NewSurface(shell.SurfaceConfig{Size: driver.Size{W: 10, H: 10}}, nopSurface{})
			if err != nil {
				panic(err)
			}
			other.Show()
		}},
		{"quit", func(_ *Window, _ *fake.Surface, s *fake.Shell) { s.Quit() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log []string
			w, sf, s, _ := popupWindow(t, &log)
			grab := openTestPopup(t, w, newPopupProbe(&log, "menu"), true, &log)
			tip := openTestPopup(t, w, newPopupProbe(&log, "tip"), false, &log)
			log = nil
			tc.do(w, sf, s)
			if !grab.Closed() || !tip.Closed() || len(w.popups) != 0 || w.GrabPopupOpen() {
				t.Fatalf("popups open after %s", tc.name)
			}
			n := 0
			for _, l := range log {
				if l == "closed" {
					n++
				}
			}
			if n != 2 {
				t.Fatalf("closed %d times: %v", n, log)
			}
		})
	}
}

// nopSurface is a shell.SurfaceHandler that does nothing.
type nopSurface struct{}

func (nopSurface) Paint(render.Canvas)              {}
func (nopSurface) Resized(driver.Size, float64)     {}
func (nopSurface) CloseRequested() bool             { return true }
func (nopSurface) Closed()                          {}
func (nopSurface) KeyChanged(bool)                  {}
func (nopSurface) Event(shell.Event) (handled bool) { return false }

func TestPopupRootErrors(t *testing.T) {
	var log []string
	w, _, s, _ := popupWindow(t, &log)
	openTestPopup(t, w, newPopupProbe(&log, "menu"), true, &log)
	c := newPopupProbe(&log, "second")
	_, err := w.openPopup(popupOptions{Config: shell.PopupConfig{Grab: true}, Content: c})
	if !errors.Is(err, shell.ErrPopupGrab) || c.Parent() != nil {
		t.Fatalf("second grab: %v, parent %v", err, c.Parent())
	}
	// A non-grab popup may open beside it.
	if _, err := w.openPopup(popupOptions{Content: c}); err != nil {
		t.Fatal(err)
	}

	// A hidden window: the shell refuses, the content stays detached.
	h, err := NewWindow(s, services(s), driver.WindowConfig{Size: driver.Size{W: 50, H: 50}}, &winLog{})
	if err != nil {
		t.Fatal(err)
	}
	d := newPopupProbe(&log, "d")
	if _, err := h.openPopup(popupOptions{Content: d}); !errors.Is(err, shell.ErrPopupParent) || d.Parent() != nil {
		t.Fatalf("hidden window: %v", err)
	}
	h.Close()
	if _, err := h.openPopup(popupOptions{Content: d}); !errors.Is(err, shell.ErrPopupParent) {
		t.Fatalf("closed window: %v", err)
	}
}

// TestRouterCancelPointer: CancelPointer ends a capture with PointerCancel
// and the hover with PointerLeave; a stray release reaches the hover chain
// only with SetStrayReleases.
func TestRouterCancelPointer(t *testing.T) {
	var log []string
	root := newProbe(&log, "root", 0, 0, 100, 100)
	a := newProbe(&log, "a", 10, 10, 50, 50, "down", "up")
	root.AddChild(a)
	r := NewRouter(root)
	r.Event(move(20, 20))
	r.Event(down(20, 20))
	log = nil
	r.CancelPointer()
	checkLog(t, &log, "a cancel 10,10", "a leave 10,10", "root leave 20,20")
	if r.Captured() != nil || len(r.HoverChain()) != 0 {
		t.Fatal("state left after CancelPointer")
	}
	// The release of the cancelled press goes nowhere by default.
	if r.Event(up(20, 20)) {
		t.Fatal("stray release handled")
	}
	checkLog(t, &log, "root enter 20,20", "a enter 10,10")
	r.SetStrayReleases(true)
	if !r.Event(up(21, 20)) {
		t.Fatal("stray release not delivered")
	}
	checkLog(t, &log, "a up 11,10 x1")
}

// TestPopupRootDeferredPlaced (V6): with Placed from the loop, as on
// Wayland, the popup lays out and paints only once it arrives, and a popup
// closed before it gets none.
func TestPopupRootDeferredPlaced(t *testing.T) {
	var log []string
	w, _, s, _ := popupWindow(t, &log)
	s.SetDeferPlaced(true)
	c := newPopupProbe(&log, "menu")
	p := openTestPopup(t, w, c, true, &log)
	fp := p.Popup().(*fake.Popup)
	checkLog(t, &log, "win leave 50,"+ftoa(100-w.titleBarHeight())) // the grab took the pointer, no Placed yet
	s.FakeClock().Flush()
	if fp.Paints() != 0 {
		t.Fatal("painted before Placed")
	}
	s.Step()
	tb := w.titleBarHeight()
	checkLog(t, &log, "placed 20 "+ftoa(40+tb)+" 100 60")
	if c.Frame() != (driver.Rect{W: 100, H: 60}) {
		t.Fatalf("content frame %v", c.Frame())
	}
	s.FakeClock().Flush()
	if fp.Paints() != 1 || fp.PaintErr() != nil {
		t.Fatalf("paints %d err %v", fp.Paints(), fp.PaintErr())
	}
	p.Close()
	checkLog(t, &log, "closed")

	p = openTestPopup(t, w, c, false, &log)
	p.Close()
	s.Step()
	checkLog(t, &log, "closed")
}

// TestPopupRootClosedInsideNewPopup: a popup closed by a handler from
// inside the shell's NewPopup (here its first Placed) also closes the
// shell popup, so a grab popup is not orphaned (B12).
func TestPopupRootClosedInsideNewPopup(t *testing.T) {
	for _, grab := range []bool{true, false} {
		t.Run(fmt.Sprint("grab=", grab), func(t *testing.T) {
			var log []string
			w, _, s, _ := popupWindow(t, &log)
			c := newPopupProbe(&log, "menu")
			p, err := w.openPopup(popupOptions{
				Config: shell.PopupConfig{
					Anchor: driver.Rect{X: 20, Y: 40 + w.titleBarHeight(), W: 10, H: 10},
					Size:   driver.Size{W: 100, H: 60},
					Grab:   grab,
				},
				Content: c,
				Placed:  func(driver.Rect, float64) { parentOf(c).(*popupRoot).Close() },
				Closed:  func() { log = append(log, "closed") },
			})
			if err != nil {
				t.Fatal(err)
			}
			if !p.Closed() || w.GrabPopupOpen() || len(w.popups) != 0 {
				t.Fatal("aqua popup still open")
			}
			if fp := p.Popup().(*fake.Popup); !fp.Closed() || s.GrabPopup() != nil {
				t.Fatal("shell popup orphaned")
			}
			if n := strings.Count(strings.Join(log, "\n"), "closed"); n != 1 {
				t.Fatalf("Closed called %d times", n)
			}
		})
	}
}

// noPopupShell hides the fake's NewPopup: a shell that is no
// shell.PopupShell (R10).
type noPopupShell struct{ shell.Shell }

// TestOpenPopupWithoutPopupShell: on a shell without popups openPopup
// fails with errNoPopups and leaves the content detached; the window
// itself works.
func TestOpenPopupWithoutPopupShell(t *testing.T) {
	s := startShell(t)
	w, err := NewWindow(noPopupShell{s}, services(s), driver.WindowConfig{Size: driver.Size{W: 80, H: 60}}, &winLog{})
	if err != nil {
		t.Fatal(err)
	}
	w.Show()
	paint(t, s, w.Surface().(*fake.Surface))
	var log []string
	c := newPopupProbe(&log, "menu")
	for _, grab := range []bool{false, true} {
		if _, err := w.openPopup(popupOptions{Config: shell.PopupConfig{Grab: grab}, Content: c}); !errors.Is(err, errNoPopups) || c.Parent() != nil {
			t.Fatalf("grab %v: %v, parent %v", grab, err, c.Parent())
		}
	}
	if len(s.Popups()) != 0 {
		t.Fatal("a shell popup opened")
	}
}
