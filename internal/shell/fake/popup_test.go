package fake

import (
	"errors"
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// pop is a shell.PopupHandler that logs into a win's log with a "pop "
// prefix, so the order against the parent's calls shows.
type pop struct {
	w         *win
	paint     func(c render.Canvas)
	event     func(ev shell.Event) bool
	placed    func(r driver.Rect)
	dismissed func()
}

func (p *pop) Paint(c render.Canvas) {
	p.w.logf("pop paint")
	if p.paint != nil {
		p.paint(c)
	}
}
func (p *pop) Placed(r driver.Rect, scale float64) {
	p.w.logf("pop placed (%g,%g %gx%g) @%g", r.X, r.Y, r.W, r.H, scale)
	if p.placed != nil {
		p.placed(r)
	}
}
func (p *pop) Event(ev shell.Event) bool {
	p.w.logf("pop %s", eventString(ev))
	if p.event != nil {
		return p.event(ev)
	}
	return false
}
func (p *pop) Dismissed() {
	p.w.logf("pop dismissed")
	if p.dismissed != nil {
		p.dismissed()
	}
}
func (p *pop) Closed() { p.w.logf("pop closed") }

func rect(x, y, w, h float64) driver.Rect { return driver.Rect{X: x, Y: y, W: w, H: h} }

func newPopup(t *testing.T, s *Shell, parent shell.Surface, cfg shell.PopupConfig, h shell.PopupHandler) *Popup {
	t.Helper()
	p, err := s.NewPopup(parent, cfg, h)
	if err != nil {
		t.Fatalf("NewPopup(%+v) = %v", cfg, err)
	}
	return p.(*Popup)
}

func wantLog(t *testing.T, w *win, want string) {
	t.Helper()
	if got := w.take(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

// TestPopupLifecycle: the parent checks, the placement in the work area
// with the parent's screen position, Placed before NewPopup returns, the
// paint after the surfaces, Reposition, Close, and closing with the
// parent before its Closed.
func TestPopupLifecycle(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300)})
	ph := &pop{w: &w}
	mustPanic(t, "nil handler", func() { s.NewPopup(f, shell.PopupConfig{}, nil) })
	if _, err := s.NewPopup(nil, shell.PopupConfig{}, ph); !errors.Is(err, shell.ErrPopupParent) {
		t.Errorf("NewPopup(nil parent) = %v, want ErrPopupParent", err)
	}
	other := New()
	if _, err := s.NewPopup(&Surface{shell: other}, shell.PopupConfig{}, ph); !errors.Is(err, shell.ErrPopupParent) {
		t.Errorf("NewPopup(another shell's surface) = %v, want ErrPopupParent", err)
	}
	hidden, _ := s.NewSurface(shell.SurfaceConfig{Size: sz(10, 10)}, &win{})
	if _, err := s.NewPopup(hidden, shell.PopupConfig{}, ph); !errors.Is(err, shell.ErrPopupParent) {
		t.Errorf("NewPopup(hidden parent) = %v, want ErrPopupParent", err)
	}
	hidden.Close()

	// The window at (1000, 500) of a 1440×900 work area: a 200×300 menu
	// over its item at (30, 380) slides up by 50 and left by 70.
	f.UserMove(pt(1000, 500))
	f.SetScale(2)
	w.take()
	cfg := shell.PopupConfig{
		Anchor: rect(380, 380, 60, 20), Size: sz(200, 300), Offset: pt(-10, -40),
		Adjust: shell.AdjustSlideX | shell.AdjustSlideY,
	}
	p := newPopup(t, s, f, cfg, ph)
	wantLog(t, &w, "pop placed (240,100 200x300) @2")
	if p.Rect() != rect(240, 100, 200, 300) || p.Scale() != 2 || p.Placements() != 1 || p.Grab() || p.Translucent() {
		t.Errorf("popup rect %+v @%g, %d placements, grab %v, translucent %v", p.Rect(), p.Scale(), p.Placements(), p.Grab(), p.Translucent())
	}
	if !p.Invalid() || !s.FakeClock().Requested() {
		t.Error("a new popup is not invalidated with a frame requested")
	}
	if got := f.Popups(); len(got) != 1 || got[0] != p || len(s.Popups()) != 1 || s.GrabPopup() != nil {
		t.Errorf("Popups = %v, Shell.Popups = %v, GrabPopup = %v", got, s.Popups(), s.GrabPopup())
	}
	f.Invalidate()
	s.Flush()
	wantLog(t, &w, "paint; pop paint")
	if p.Paints() != 1 || p.LastPaint() == nil || p.PaintErr() != nil || p.Invalid() {
		t.Errorf("after the frame: %d paints, err %v, invalid %v", p.Paints(), p.PaintErr(), p.Invalid())
	}

	// Paint contract: no Reposition, Close or NewPopup during a paint;
	// Invalidate is fine.
	ph.paint = func(render.Canvas) {
		p.Invalidate()
		mustPanic(t, "during SurfaceHandler.Paint or PopupHandler.Paint", func() { p.Reposition(cfg) })
		mustPanic(t, "Popup.Close called during", func() { p.Close() })
		mustPanic(t, "Shell.NewPopup called during", func() { s.NewPopup(f, cfg, &pop{w: &w}) })
	}
	p.Invalidate()
	s.Flush()
	ph.paint = nil
	wantLog(t, &w, "pop paint")
	s.Flush()
	w.take()

	// Reposition: Placed only on a change, a new size invalidates.
	p.Reposition(cfg)
	wantLog(t, &w, "")
	cfg.Offset = pt(-200, -40)
	p.Reposition(cfg)
	wantLog(t, &w, "pop placed (180,100 200x300) @2") // still slid up, now fitting horizontally
	p.Reposition(shell.PopupConfig{Anchor: cfg.Anchor, Size: cfg.Size, Offset: pt(-200, -60), Adjust: cfg.Adjust})
	wantLog(t, &w, "") // slid to the same place: no Placed
	if p.Placements() != 2 || p.Invalid() {
		t.Errorf("%d placements, invalid %v after a move", p.Placements(), p.Invalid())
	}
	cfg.Size = sz(200, 100)
	cfg.Grab = true // ignored
	p.Reposition(cfg)
	wantLog(t, &w, "pop placed (180,300 200x100) @2")
	if !p.Invalid() || p.Grab() || s.GrabPopup() != nil {
		t.Errorf("after a resize: invalid %v, grab %v", p.Invalid(), p.Grab())
	}
	s.Flush()
	wantLog(t, &w, "pop paint")

	// A non-grab popup is not dismissed by a move or a deactivation.
	f.UserMove(pt(0, 0))
	s.Deactivate()
	f.Activate()
	wantLog(t, &w, "key false; key true")
	if p.WasDismissed() {
		t.Error("a popup without Grab was dismissed")
	}

	// Close, then every method does nothing.
	p.SetCursor(shell.CursorPointingHand)
	if p.Cursor() != shell.CursorPointingHand {
		t.Errorf("Cursor = %v", p.Cursor())
	}
	p.Close()
	p.Close()
	p.Invalidate()
	p.Reposition(cfg)
	p.UserDismiss()
	wantLog(t, &w, "pop closed")
	if !p.Closed() || len(f.Popups()) != 0 || len(s.Popups()) != 0 || p.Invalid() {
		t.Errorf("after Close: closed %v, %d popups", p.Closed(), len(s.Popups()))
	}

	// The parent closes its popups, last to first, before its Closed; the
	// popups of another surface stay.
	var w2 win
	f2 := open(t, s, &w2, shell.SurfaceConfig{Size: sz(100, 100)})
	keep := newPopup(t, s, f2, shell.PopupConfig{Size: sz(10, 10)}, &pop{w: &w2})
	a := newPopup(t, s, f, shell.PopupConfig{Size: sz(10, 10)}, &pop{w: &w})
	b := newPopup(t, s, f, shell.PopupConfig{Size: sz(20, 10), Grab: true}, &pop{w: &w})
	w.take()
	f.Close()
	wantLog(t, &w, "pop closed; pop closed; closed")
	if !a.Closed() || !b.Closed() || keep.Closed() || s.GrabPopup() != nil {
		t.Errorf("closed: a %v, b %v, keep %v; grab %v", a.Closed(), b.Closed(), keep.Closed(), s.GrabPopup())
	}
	w2.take()
	s.Quit()
	wantLog(t, &w2, "pop closed; closed")
}

// TestPopupHiddenParent: popups of a minimized parent are not painted, and
// NewPopup refuses a minimized parent.
func TestPopupHiddenParent(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100), Style: driver.StyleResizable})
	p := newPopup(t, s, f, shell.PopupConfig{Size: sz(10, 10)}, &pop{w: &w})
	f.Minimize()
	if _, err := s.NewPopup(f, shell.PopupConfig{}, &pop{w: &w}); !errors.Is(err, shell.ErrPopupParent) {
		t.Errorf("NewPopup(minimized parent) = %v", err)
	}
	s.Flush()
	if p.Paints() != 0 {
		t.Error("the popup of a minimized parent was painted")
	}
	f.Show()
	s.Flush()
	if p.Paints() != 1 {
		t.Errorf("%d paints after the restore, want 1", p.Paints())
	}
}

// TestPopupGrabPointer: a press-drag-release from the parent into a grab
// popup, the pointer routed in the popup's coordinates, an outside press
// eaten and dismissing, and the parent getting the pointer back.
func TestPopupGrabPointer(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300)})
	ph := &pop{w: &w}
	var p *Popup
	// The press on the "button" at (100, 100) opens the menu over it.
	w.event = func(ev shell.Event) bool {
		if e, ok := ev.(shell.PointerEvent); ok && e.Kind == shell.PointerDown && p == nil {
			p = newPopup(t, s, f, shell.PopupConfig{Anchor: rect(90, 95, 60, 20), Size: sz(100, 80), Offset: pt(-10, -20), Grab: true}, ph)
		}
		return false
	}
	f.SetModifiers(driver.ModShift)
	f.Press(pt(100, 100), shell.ButtonPrimary)
	w.event = nil
	wantLog(t, &w, "enter (100,100) m=1; move (100,100) m=1; down (100,100) b=1 held=1 n=1 m=1; "+
		"pop placed (80,75 100x80) @1; up (100,100) b=1 m=1; leave (100,100) m=1")
	if s.GrabPopup() != p || !p.Grab() {
		t.Fatal("no grab popup")
	}
	if pos, inside, held := p.Pointer(); pos != pt(20, 25) || inside || held != shell.ButtonPrimary {
		t.Errorf("popup pointer %v inside %v held %d, want (20,25), false, 1", pos, inside, held)
	}
	if _, inside, held := f.Pointer(); inside || held != 0 {
		t.Errorf("parent pointer inside %v held %d after the grab", inside, held)
	}
	f.SetModifiers(0)
	f.Move(pt(100, 100))                         // no motion, but the popup was never entered
	f.Move(pt(300, 200))                         // outside the popup, button held: a move
	f.Release(pt(120, 130), shell.ButtonPrimary) // back over it: the release picks
	wantLog(t, &w, "pop enter (20,25) held=1; pop move (20,25) held=1; pop move (220,125) held=1; "+
		"pop move (40,55) held=1; pop up (40,55) b=1")

	// Hover, a click on the popup, a scroll over it and outside it.
	s.FakeClock().Advance(DoubleClickTime + 1)
	f.Move(pt(300, 200))
	f.Click(p.ToParent(pt(5, 5)))
	f.Wheel(pt(90, 90), pt(0, 2))
	f.Wheel(pt(10, 10), pt(0, 2)) // outside: dropped (and a leave)
	wantLog(t, &w, "pop paint; pop leave (220,125); pop enter (5,5); pop move (5,5); pop down (5,5) b=1 held=1 n=1; pop up (5,5) b=1 n=1; "+
		"pop move (10,15); pop scroll (10,15) d=(0,2); pop leave (-70,-65)")
	if f.Cursor() != shell.CursorArrow {
		t.Error("the parent's cursor changed")
	}

	// An outside press is eaten and dismisses; its release is eaten too.
	ph.dismissed = func() { p.Close() }
	f.Press(pt(10, 10), shell.ButtonPrimary)
	wantLog(t, &w, "pop dismissed; pop closed")
	if !p.WasDismissed() || s.GrabPopup() != nil {
		t.Error("not dismissed")
	}
	f.Move(pt(12, 12))
	f.Release(pt(12, 12), shell.ButtonPrimary)
	f.Click(pt(20, 20))
	wantLog(t, &w, "enter (12,12); move (12,12); move (20,20); down (20,20) b=1 held=1 n=1; up (20,20) b=1 n=1")
}

// TestPopupGrabKeys: the parent's keys, text and preedits go to the grab
// popup; the parent stays key; Close gives them back.
func TestPopupGrabKeys(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300)})
	f.SetTextInput(shell.TextInput{Enabled: true})
	ph := &pop{w: &w, event: func(ev shell.Event) bool {
		k, ok := ev.(shell.KeyEvent)
		return ok && k.Key == driver.KeyEscape
	}}
	p := newPopup(t, s, f, shell.PopupConfig{Size: sz(100, 80), Grab: true}, ph)
	w.take()
	if f.Key(driver.KeyEscape, 0) != true || f.Key(driver.KeyDown, 0) != false {
		t.Error("the popup's key results were not returned")
	}
	f.Type("a")
	f.Preedit("か", 3)
	f.CommitPreedit("家")
	f.Send(shell.KeyEvent{Down: true, Key: 'x'})
	p.Send(shell.PointerEvent{Kind: shell.PointerMove, Pos: pt(1, 2)})
	wantLog(t, &w, `pop keydown '\x1b'; pop keyup '\x1b'; pop keydown '\uf701'; pop keyup '\uf701'; `+
		`pop keydown 'A'; pop text "a"; pop keyup 'A'; pop preedit "か" @3; pop text "家"; pop preedit end; `+
		`pop keydown 'x'; pop move (1,2)`)
	if !f.IsKey() {
		t.Error("the parent lost the key")
	}
	p.Close()
	f.Key('b', 0)
	wantLog(t, &w, "pop closed; keydown 'B'; keyup 'B'")
}

// TestPopupDismissals: every OS dismissal calls Dismissed once and ends
// the grab, a held button first released on the popup; a second grab
// popup is refused while one is open.
func TestPopupDismissals(t *testing.T) {
	s := started(t)
	var w, w2 win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300), Style: driver.StyleResizable})
	f2 := open(t, s, &w2, shell.SurfaceConfig{Size: sz(100, 100)})
	f.Activate()
	w.take()
	w2.take()
	cases := []struct {
		name    string
		dismiss func()
		log     string // the parent's log after "pop dismissed"
	}{
		{"deactivate", s.Deactivate, "key false"},
		{"another surface", f2.Activate, "key false"},
		{"move", func() { f.UserMove(pt(5, 5)) }, ""},
		{"user resize", func() { f.UserResize(sz(300, 300)) }, "resized 300x300 @1; paint"},
		{"set size", func() { f.SetSize(sz(400, 300)) }, "resized 400x300 @1"},
		{"zoom", f.Zoom, "resized 1440x900 @1"},
		{"minimize", f.Minimize, "key false"},
		{"cancel capture", f.CancelCapture, ""},
		{"user dismiss", nil, ""},
	}
	for _, c := range cases {
		f.Activate()
		if f.Zoomed() {
			f.Zoom()
		}
		w.take()
		p := newPopup(t, s, f, shell.PopupConfig{Size: sz(100, 80), Grab: true}, &pop{w: &w})
		if _, err := s.NewPopup(f2, shell.PopupConfig{Grab: true}, &pop{w: &w2}); !errors.Is(err, shell.ErrPopupGrab) {
			t.Errorf("%s: a second grab popup: %v, want ErrPopupGrab", c.name, err)
		}
		f.Press(pt(10, 10), shell.ButtonPrimary) // held on the popup
		w.take()
		if c.dismiss == nil {
			p.UserDismiss()
		} else {
			c.dismiss()
		}
		want := "pop up (10,10) b=1; pop leave (10,10); pop dismissed"
		if c.log != "" {
			want += "; " + c.log
		}
		if got := w.take(); got != want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.name, got, want)
		}
		p.UserDismiss() // once only
		f.Activate()
		f.Release(pt(10, 10), shell.ButtonPrimary) // not held anywhere any more
		if w.take(); !p.WasDismissed() || s.GrabPopup() != nil || p.Closed() {
			t.Errorf("%s: dismissed %v, grab %v, closed %v", c.name, p.WasDismissed(), s.GrabPopup(), p.Closed())
		}
		p.Close()
		w.take()
	}
	w2.take()
	if len(s.Popups()) != 0 {
		t.Errorf("%d popups left", len(s.Popups()))
	}
}

// TestPopupNoGrab: a popup without Grab takes no input: the parent gets
// every event as without it, the popup's handler none.
func TestPopupNoGrab(t *testing.T) {
	s := started(t)
	var w, wp win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300)})
	p := newPopup(t, s, f, shell.PopupConfig{Anchor: rect(50, 50, 0, 0), Size: sz(100, 40), Offset: pt(0, 20)}, &pop{w: &wp})
	wp.take()
	f.Click(pt(60, 80)) // over the popup
	f.Wheel(pt(60, 80), pt(0, 1))
	f.Key('a', 0)
	wantLog(t, &w, "enter (60,80); move (60,80); down (60,80) b=1 held=1 n=1; up (60,80) b=1 n=1; "+
		"scroll (60,80) d=(0,1); keydown 'A'; keyup 'A'")
	wantLog(t, &wp, "")
	if s.GrabPopup() != nil || p.WasDismissed() {
		t.Error("a popup without Grab grabbed or was dismissed")
	}
}

// TestPopupWorkArea: SetWorkArea and ResizeY: a popup cut to the work
// area of a second screen; SetPopupTranslucent for new popups.
func TestPopupWorkArea(t *testing.T) {
	s := New()
	s.SetWorkArea(rect(-1280, 40, 1280, 984))
	s.SetPopupTranslucent(true)
	if err := s.Start(&app{}); err != nil {
		t.Fatal(err)
	}
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300)})
	f.UserMove(pt(-1000, 20))
	p := newPopup(t, s, f, shell.PopupConfig{
		Anchor: rect(100, 30, 80, 20), Size: sz(120, 400), Offset: pt(0, -100), Adjust: shell.AdjustResizeY,
	}, &pop{w: &w})
	if p.Rect() != rect(100, 20, 120, 310) || !p.Translucent() || s.WorkArea() != rect(-1280, 40, 1280, 984) {
		t.Errorf("rect %+v, translucent %v, work %+v", p.Rect(), p.Translucent(), s.WorkArea())
	}
	if f.Position() != pt(-1000, 20) {
		t.Errorf("Position = %v", f.Position())
	}
	s.Flush()
	if got := w.take(); got != "pop placed (100,20 120x310) @1; pop paint" {
		t.Errorf("log %s", got)
	}
}

// TestCaps: Caps().PopupShadow is true (as Win32) until SetCaps turns it
// off (as gtk4), before and after Start, and the fake is a PopupShell.
func TestCaps(t *testing.T) {
	s := New()
	if !s.Caps().PopupShadow {
		t.Fatal("no shadow initially")
	}
	s.SetCaps(shell.Caps{})
	if s.Caps().PopupShadow {
		t.Fatal("shadow after SetCaps without PopupShadow")
	}
	if err := s.Start(&app{}); err != nil {
		t.Fatal(err)
	}
	s.SetCaps(shell.Caps{PopupShadow: true})
	if !s.Caps().PopupShadow {
		t.Fatal("no shadow after SetCaps with PopupShadow")
	}
	if _, ok := shell.Shell(s).(shell.PopupShell); !ok {
		t.Fatal("the fake is not a PopupShell")
	}
}

// TestPopupGrabTrack: the grab's track area (S5.6d), a strip over the
// parent's caption above a menu: enter and leave for the whole area, no
// leave between the track and the popup below it, moves, presses and
// scrolls over the track delivered in the popup's coordinates (a press
// there neither dismisses nor moves the window), a press outside it eaten
// and dismissing. Reposition keeps the track; an empty track is none, and
// a popup without Grab ignores it.
func TestPopupGrabTrack(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300)})
	caption := rc(0, 0, 400, 28)
	caption.Kind = shell.RegionCaption
	f.SetChromeRegions([]shell.ChromeRegion{caption, rc(80, 4, 200, 20)})
	ph := &pop{w: &w}
	track := rect(80, 0, 200, 28)
	cfg := shell.PopupConfig{Anchor: rect(100, 0, 40, 28), Size: sz(100, 80), Offset: pt(0, 28), Grab: true, Track: track}
	p := newPopup(t, s, f, cfg, ph)
	if p.Config().Track != track {
		t.Errorf("Config().Track = %+v", p.Config().Track)
	}
	f.Move(pt(150, 10)) // the track
	f.Move(pt(150, 40)) // the popup, touching the track: no leave
	f.Move(pt(150, 10)) // back
	f.Move(pt(300, 10)) // the caption right of the track: leave
	f.Move(pt(90, 5))   // the track left of the popup
	f.Click(pt(90, 5))  // delivered, no dismissal
	f.Wheel(pt(270, 27), pt(0, 1))
	f.Move(pt(90, 30)) // below the track, left of the popup: leave
	wantLog(t, &w, "pop placed (100,28 100x80) @1; "+
		"pop enter (50,-18); pop move (50,-18); pop move (50,12); pop move (50,-18); pop leave (200,-18); "+
		"pop enter (-10,-23); pop move (-10,-23); pop down (-10,-23) b=1 held=1 n=1; pop up (-10,-23) b=1 n=1; "+
		"pop move (170,-1); pop scroll (170,-1) d=(0,1); pop leave (-10,2)")
	if p.WasDismissed() || f.Zoomed() {
		t.Fatalf("dismissed %v, zoomed %v", p.WasDismissed(), f.Zoomed())
	}

	// A press-drag from the track into the popup, released there; then a
	// release on the track after a press in the popup.
	s.FakeClock().Advance(DoubleClickTime + 1)
	f.Press(pt(120, 10), shell.ButtonPrimary)
	f.Release(pt(120, 50), shell.ButtonPrimary)
	s.FakeClock().Advance(DoubleClickTime + 1)
	f.Press(pt(120, 50), shell.ButtonPrimary)
	f.Release(pt(200, 10), shell.ButtonPrimary)
	wantLog(t, &w, "pop paint; pop enter (20,-18); pop move (20,-18); pop down (20,-18) b=1 held=1 n=1; pop move (20,22) held=1; pop up (20,22) b=1 n=1; "+
		"pop down (20,22) b=1 held=1 n=1; pop move (100,-18) held=1; pop up (100,-18) b=1 n=1")

	// Reposition keeps the track (a cfg without one changes nothing).
	p.Reposition(shell.PopupConfig{Anchor: rect(160, 0, 40, 28), Size: sz(100, 80), Offset: pt(0, 28)})
	if p.Config().Track != track || p.Rect() != rect(160, 28, 100, 80) {
		t.Errorf("after Reposition: track %+v, rect %+v", p.Config().Track, p.Rect())
	}
	f.Move(pt(90, 5))
	wantLog(t, &w, "pop placed (160,28 100x80) @1; pop move (-70,-23)")

	// Outside the area a press is eaten and dismisses.
	ph.dismissed = func() { p.Close() }
	f.Press(pt(350, 10), shell.ButtonPrimary)
	f.Release(pt(350, 10), shell.ButtonPrimary)
	wantLog(t, &w, "pop leave (190,-18); pop dismissed; pop closed; enter (350,10); move (350,10)")

	// An empty track is none: a press where it would be dismisses.
	for _, bad := range []driver.Rect{{}, rect(80, 0, -200, 28), rect(80, 0, math.NaN(), 28), rect(math.Inf(-1), 0, 200, 28)} {
		ph.dismissed = nil
		cfg.Track = bad
		p = newPopup(t, s, f, cfg, ph)
		f.Press(pt(90, 5), shell.ButtonPrimary)
		f.Release(pt(90, 5), shell.ButtonPrimary)
		if !p.WasDismissed() {
			t.Errorf("track %+v: a press outside the popup did not dismiss", bad)
		}
		p.Close()
		w.take()
	}

	// Without Grab the track is ignored: the parent gets the pointer.
	cfg.Grab, cfg.Track = false, track
	p = newPopup(t, s, f, cfg, &pop{w: &w})
	w.take()
	f.Move(pt(90, 10))
	if got := w.take(); got != "move (90,10)" {
		t.Errorf("without Grab: %s", got)
	}
	if p.Grab() || s.GrabPopup() != nil {
		t.Error("a popup without Grab grabbed")
	}
}

// TestPopupRepositionKeepsPointer: a grab popup moved by Reposition keeps
// the pointer where it is on the screen (S5.6f): a Reposition from inside
// the Enter handler is followed by the Move in the new place's points, and
// so are the later events.
func TestPopupRepositionKeepsPointer(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300)})
	ph := &pop{w: &w}
	cfg := shell.PopupConfig{Anchor: rect(100, 0, 40, 28), Size: sz(100, 80), Offset: pt(0, 28), Grab: true, Track: rect(80, 0, 200, 28)}
	p := newPopup(t, s, f, cfg, ph)
	ph.event = func(ev shell.Event) bool {
		if pe, ok := ev.(shell.PointerEvent); ok && pe.Kind == shell.PointerEnter {
			p.Reposition(shell.PopupConfig{Anchor: rect(160, 0, 40, 28), Size: sz(100, 80), Offset: pt(0, 28)})
		}
		return true
	}
	f.Press(pt(150, 10), shell.ButtonPrimary)
	f.Release(pt(150, 10), shell.ButtonPrimary)
	wantLog(t, &w, "pop placed (100,28 100x80) @1; pop enter (50,-18); pop placed (160,28 100x80) @1; pop move (-10,-18); "+
		"pop down (-10,-18) b=1 held=1 n=1; pop up (-10,-18) b=1 n=1")
}

// TestPopupDeferPlaced (V6): with SetDeferPlaced, Placed comes from the
// loop as on Wayland: not inside NewPopup or Reposition, coalesced, never
// after Close, and the popup is not painted before it.
func TestPopupDeferPlaced(t *testing.T) {
	s := started(t)
	s.SetDeferPlaced(true)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300)})
	p := newPopup(t, s, f, shell.PopupConfig{Anchor: rect(10, 10, 20, 20), Size: sz(50, 40)}, &pop{w: &w})
	if len(w.log) != 0 || !p.PlacedPending() || p.Rect() != rect(10, 10, 50, 40) {
		t.Fatalf("NewPopup: log %v, pending %v, rect %+v", w.log, p.PlacedPending(), p.Rect())
	}
	s.Flush()
	if len(w.log) != 0 || !p.Invalid() {
		t.Fatalf("painted before Placed: %v", w.log)
	}
	p.Reposition(shell.PopupConfig{Anchor: rect(20, 10, 20, 20), Size: sz(60, 40)})
	if s.Step() != 1 {
		t.Fatal("Placed is not one posted call")
	}
	wantLog(t, &w, "pop placed (20,10 60x40) @1")
	if p.PlacedPending() || !s.FakeClock().Requested() {
		t.Fatal("no frame requested for the popup skipped before Placed")
	}
	s.Flush()
	wantLog(t, &w, "pop paint")

	p.Reposition(shell.PopupConfig{Anchor: rect(30, 10, 20, 20), Size: sz(60, 40)})
	p.Close()
	s.Step()
	wantLog(t, &w, "pop closed")
}
