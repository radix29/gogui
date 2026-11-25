package fake

import (
	"math"
	"slices"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

func TestPointerEnterMoveLeave(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 50)})
	f.Move(pt(200, 10)) // outside, never entered: nothing
	f.Move(pt(10, 10))
	f.Move(pt(10, 10)) // no motion: nothing
	f.Move(pt(20, 10))
	f.Move(pt(100, 10)) // the right edge is outside
	if got, want := w.take(), "enter (10,10); move (10,10); move (20,10); leave (100,10)"; got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if _, inside, _ := f.Pointer(); inside {
		t.Error("inside after leave")
	}
	mustPanic(t, "NaN", func() { f.Move(driver.Point{X: math.NaN()}) })
	mustPanic(t, "nil handler", func() { s.NewSurface(shell.SurfaceConfig{}, nil) })
}

func TestPointerCapture(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 50)})
	f.SetModifiers(driver.ModShift)
	f.Drag(pt(10, 10), pt(150, 80), pt(-5, 20))
	want := "enter (10,10) m=1; move (10,10) m=1; down (10,10) b=1 held=1 n=1 m=1; " +
		"move (-5,20) held=1 m=1; move (150,80) held=1 m=1; up (150,80) b=1 n=1 m=1; leave (150,80) m=1"
	if got := w.take(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	mustPanic(t, "ModPrimary", func() { f.SetModifiers(driver.ModPrimary) })
	f.SetModifiers(0)
	s.FakeClock().Advance(DoubleClickTime + 1) // not a double click with the drag

	// Two buttons: the second press and the first release keep the capture.
	f.Press(pt(10, 10), shell.ButtonPrimary)
	f.Press(pt(10, 10), shell.ButtonSecondary)
	f.Press(pt(10, 10), shell.ButtonSecondary) // held: nothing
	f.Release(pt(300, 10), shell.ButtonPrimary)
	f.Release(pt(300, 10), shell.ButtonPrimary) // not held: only the move (none)
	f.Release(pt(300, 10), shell.ButtonSecondary)
	want = "enter (10,10); move (10,10); down (10,10) b=1 held=1 n=1; down (10,10) b=2 held=3 n=1; " +
		"move (300,10) held=3; up (300,10) b=1 held=2; up (300,10) b=2 n=1; leave (300,10)"
	if got := w.take(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	mustPanic(t, "exactly one button", func() { f.Press(pt(1, 1), shell.ButtonPrimary|shell.ButtonMiddle) })
	mustPanic(t, "exactly one button", func() { f.Press(pt(1, 1), shell.ButtonNone) })

	// A press outside the surface goes elsewhere.
	f.Press(pt(500, 10), shell.ButtonPrimary)
	if w.take() != "" {
		t.Error("press outside delivered")
	}
	// CancelCapture: up for every held button, then leave if outside.
	f.Press(pt(10, 10), shell.ButtonMiddle)
	f.Press(pt(10, 10), shell.ButtonBack)
	f.Move(pt(-20, -20))
	w.take()
	f.CancelCapture()
	if got, want := w.take(), "up (-20,-20) b=4 held=8; up (-20,-20) b=8; leave (-20,-20)"; got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestClickCounts(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 50)})
	clock := s.FakeClock()
	f.Move(pt(10, 10))
	w.take()
	counts := func() []int {
		var ns []int
		for _, l := range w.log {
			if len(l) > 4 && l[:4] == "down" {
				ns = append(ns, int(l[len(l)-1]-'0'))
			}
		}
		w.log = nil
		return ns
	}
	f.DoubleClick(pt(10, 10))
	f.Click(pt(12, 13)) // within the distance: triple
	if got := counts(); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("counts %v", got)
	}
	clock.Advance(DoubleClickTime + 1)
	f.Click(pt(12, 13))
	f.Click(pt(30, 13)) // too far
	f.ClickButton(pt(30, 13), shell.ButtonSecondary)
	f.Click(pt(30, 13)) // other button in between
	if got := counts(); len(got) != 4 || got[0] != 1 || got[1] != 1 || got[2] != 1 || got[3] != 1 {
		t.Fatalf("counts %v", got)
	}
	f.Click(pt(30, 13))
	clock.Advance(DoubleClickTime)
	f.Click(pt(30, 13))
	if got := counts(); len(got) != 2 || got[1] != 3 {
		t.Fatalf("counts at exactly DoubleClickTime: %v", got)
	}
	// Event times come from the clock.
	var times []int64
	w.event = func(ev shell.Event) bool { times = append(times, int64(ev.EventTime())); return false }
	f.Click(pt(30, 13))
	if len(times) != 2 || times[0] != int64(clock.Now()) {
		t.Fatalf("event times %v, now %v", times, clock.Now())
	}
}

func TestCaptionPresses(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300), Style: driver.StyleResizable})
	caption := rc(0, 0, 400, 28)
	caption.Kind = shell.RegionCaption
	zoom := rc(48, 8, 12, 12)
	zoom.Kind = shell.RegionMaximize
	f.SetChromeRegions([]shell.ChromeRegion{caption, zoom})
	f.Click(pt(200, 10))
	if got := w.take(); got != "enter (200,10); move (200,10)" {
		t.Fatalf("caption press delivered: %s", got)
	}
	f.Click(pt(200, 10)) // double-click on the caption zooms
	if !f.Zoomed() || w.take() != "resized 1440x900 @1" {
		t.Fatal("caption double-click must zoom")
	}
	f.ClickButton(pt(200, 10), shell.ButtonSecondary) // only primary goes to the OS
	if got := w.take(); got != "down (200,10) b=2 held=2 n=1; up (200,10) b=2 n=1" {
		t.Fatalf("secondary on caption: %s", got)
	}
	f.Click(pt(50, 10)) // a button region is delivered normally
	if got := w.take(); got != "move (50,10); down (50,10) b=1 held=1 n=1; up (50,10) b=1 n=1" {
		t.Fatalf("button region: %s", got)
	}
}

func TestScroll(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})
	f.Wheel(pt(10, 10), pt(0, -3))
	f.Swipe(pt(10, 10), pt(0, 5), pt(1, 2))
	f.Momentum(pt(0, 8), pt(0, 1))
	f.Swipe(pt(10, 10))
	f.Momentum()
	want := "enter (10,10); move (10,10); scroll (10,10) d=(0,-3); " +
		"scroll (10,10) d=(0,5) precise began; scroll (10,10) d=(1,2) precise changed; scroll (10,10) d=(0,0) precise ended; " +
		"scroll (10,10) d=(0,8) precise momentum began; scroll (10,10) d=(0,1) precise momentum changed; scroll (10,10) d=(0,0) precise momentum ended"
	if got := w.take(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestKeysAndText(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})
	f.Type("aB 1é\n\t\b")
	want := `keydown 'A'; text "a"; keyup 'A'; keydown 'B' m=1; text "B"; keyup 'B' m=1; ` +
		`keydown ' '; text " "; keyup ' '; keydown '1'; text "1"; keyup '1'; keydown 'É'; text "é"; keyup 'É'; ` +
		`keydown '\r'; keyup '\r'; keydown '\t'; keyup '\t'; keydown '\b'; keyup '\b'`
	if got := w.take(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	// A handled key-down drops its text.
	w.event = func(ev shell.Event) bool {
		k, ok := ev.(shell.KeyEvent)
		return ok && k.Down && k.Key == 'X'
	}
	f.Type("xy")
	if got := w.take(); got != `keydown 'X'; keyup 'X'; keydown 'Y'; text "y"; keyup 'Y'` {
		t.Fatalf("handled: %s", got)
	}
	if !f.Key('x', driver.ModControl) || f.Key(driver.KeyEscape, 0) {
		t.Fatal("Key result")
	}
	w.take()
	f.KeyRepeat(driver.KeyDown, driver.ModShift)
	f.Commit("日本")
	if got, want := w.take(), "keydown repeat '\\uf701' m=1; text \"日本\""; got != want {
		t.Fatalf("got %s", got)
	}
	mustPanic(t, "ModPrimary", func() { f.Key('a', driver.ModPrimary) })
	mustPanic(t, "control character", func() { f.Type("\x01") })
	mustPanic(t, "invalid UTF-8", func() { f.Type("\xff") })
	mustPanic(t, "empty text", func() { f.Commit("") })
	mustPanic(t, "control character", func() { f.Commit("a\n") })

	// Key input needs the key surface.
	s.Deactivate()
	mustPanic(t, "not key", func() { f.Type("a") })
	mustPanic(t, "not key", func() { f.Send(shell.TextEvent{Text: "a"}) })
}

func TestTypeStopsWhenFocusMoves(t *testing.T) {
	s := started(t)
	var w1, w2 win
	f1 := open(t, s, &w1, shell.SurfaceConfig{Size: sz(10, 10)})
	f2 := open(t, s, &w2, shell.SurfaceConfig{Size: sz(10, 10)})
	f1.Activate()
	w1.take()
	w2.take()
	w1.event = func(ev shell.Event) bool {
		if k, ok := ev.(shell.KeyEvent); ok && k.Down && k.Key == driver.KeyReturn {
			f2.Show() // Return opens another window
		}
		return false
	}
	f1.Type("\nabc")
	if got := w1.take(); got != `keydown '\r'; key false` || w2.take() != "key true" {
		t.Fatalf("got %s", got)
	}
	// Closing the surface from a key handler ends typing too.
	w2.event = func(shell.Event) bool { f2.Close(); return true }
	f2.Type("ab")
	if got := w2.take(); got != `keydown 'A'; closed` {
		t.Fatalf("got %s", got)
	}
}

func TestInputDroppedWhenHidden(t *testing.T) {
	s := started(t)
	var w win
	sf, _ := s.NewSurface(shell.SurfaceConfig{Size: sz(10, 10)}, &w)
	f := sf.(*Surface)
	f.Click(pt(1, 1))
	f.Wheel(pt(1, 1), pt(0, 1))
	if f.Send(shell.PointerEvent{Kind: shell.PointerMove}) || f.Key('a', 0) {
		t.Fatal("input to a hidden surface")
	}
	f.Show()
	f.Minimize()
	f.Click(pt(1, 1))
	if got := w.take(); got != "key true; key false" {
		t.Fatalf("got %s", got)
	}
}

func TestSend(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(10, 10)})
	s.FakeClock().Advance(1000)
	var times []int64
	w.event = func(ev shell.Event) bool { times = append(times, int64(ev.EventTime())); return true }
	evs := []shell.Event{
		shell.PointerEvent{Kind: shell.PointerEnter},
		shell.ScrollEvent{Delta: pt(0, 1), Time: 7},
		shell.KeyEvent{Down: true, Key: 'A'},
		shell.TextEvent{Text: "a"},
	}
	for _, ev := range evs {
		if !f.Send(ev) {
			t.Fatal("Send result")
		}
	}
	if len(times) != 4 || times[0] != 1000 || times[1] != 7 || times[2] != 1000 || times[3] != 1000 {
		t.Fatalf("times %v", times)
	}
	mustPanic(t, "ModPrimary", func() { f.Send(shell.ScrollEvent{Mods: driver.ModPrimary}) })
	mustPanic(t, "ModPrimary", func() { f.Send(shell.PointerEvent{Mods: driver.ModPrimary}) })
	mustPanic(t, "ModPrimary", func() { f.Send(shell.KeyEvent{Mods: driver.ModPrimary}) })
}

// TestKeyShifted: key-downs carry the fake layout's Shift level (S5.6h),
// US by default, key-ups none.
func TestKeyShifted(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})
	var got []driver.Key
	w.event = func(ev shell.Event) bool {
		if k, ok := ev.(shell.KeyEvent); ok {
			got = append(got, k.Shifted)
		}
		return false
	}
	f.KeyDown('=', driver.ModControl)
	f.KeyRepeat('1', 0)
	f.KeyDown('a', 0)
	f.KeyUp('a', 0)
	f.KeyDown(driver.KeyReturn, 0)
	f.KeyDown('é', 0)
	s.SetShiftLayout(map[driver.Key]driver.Key{'+': '*'})
	f.KeyDown('+', driver.ModControl)
	f.KeyDown('=', 0)
	f.KeyDown('q', 0)
	s.SetShiftLayout(nil)
	f.KeyDown('/', 0)
	want := []driver.Key{'+', '!', 'A', driver.KeyNone, driver.KeyNone, 'É', '*', driver.KeyNone, 'Q', '?'}
	if !slices.Equal(got, want) {
		t.Errorf("Shifted = %q, want %q", got, want)
	}
}

// TestModifierKeys: ModifierDown, ModifierUp and TapModifier send a
// modifier key alone (S5.6i): KeyNone, its Modifier, Mods as after the
// event; a bad modifier panics.
func TestModifierKeys(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})
	var got []shell.KeyEvent
	w.event = func(ev shell.Event) bool {
		if k, ok := ev.(shell.KeyEvent); ok {
			k.Time = 0
			got = append(got, k)
			return k.Down
		}
		return false
	}
	if !f.TapModifier(driver.ModOption) {
		t.Error("TapModifier: press not handled")
	}
	f.ModifierDown(driver.ModControl, 0)
	f.ModifierDown(driver.ModShift, driver.ModControl)
	f.ModifierUp(driver.ModControl, driver.ModControl|driver.ModShift)
	want := []shell.KeyEvent{
		{Down: true, Modifier: driver.ModOption, Mods: driver.ModOption},
		{Modifier: driver.ModOption},
		{Down: true, Modifier: driver.ModControl, Mods: driver.ModControl},
		{Down: true, Modifier: driver.ModShift, Mods: driver.ModControl | driver.ModShift},
		{Modifier: driver.ModControl, Mods: driver.ModShift},
	}
	if !slices.Equal(got, want) {
		t.Errorf("events %+v\nwant %+v", got, want)
	}
	for _, m := range []driver.Modifiers{0, driver.ModShift | driver.ModOption, driver.ModPrimary} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("ModifierDown(%v) did not panic", m)
				}
			}()
			f.ModifierDown(m, m)
		}()
	}
}

// TestActivatingPress (S6.1b): a press into a surface that is not key makes
// it key first and is flagged Activating, its release and the next press
// are not; a press outside the surface activates nothing; a caption press
// activates without being delivered; a press into a blocked surface makes
// the active modal key and delivers nothing.
func TestActivatingPress(t *testing.T) {
	s := started(t)
	var a, b win
	fa := open(t, s, &a, shell.SurfaceConfig{Size: sz(400, 300)})
	fb := open(t, s, &b, shell.SurfaceConfig{Size: sz(400, 300)})
	a.take()
	if s.Key() != fb {
		t.Fatal("the surface shown last is not key")
	}
	fa.Click(pt(500, 10)) // outside: another window's press
	if s.Key() != fb || a.take() != "" {
		t.Fatal("a press outside activated the surface")
	}
	fa.Press(pt(10, 20), shell.ButtonPrimary)
	fa.Move(pt(30, 20))
	fa.Release(pt(30, 20), shell.ButtonPrimary)
	if got := a.take(); got != "enter (10,20); move (10,20); key true; down (10,20) b=1 held=1 n=1 activating; "+
		"move (30,20) held=1; up (30,20) b=1 n=1" {
		t.Fatalf("activating press: %s", got)
	}
	if s.Key() != fa || b.take() != "key false" {
		t.Fatal("the press did not move the key")
	}
	fa.ClickButton(pt(30, 20), shell.ButtonSecondary) // key now: not activating
	if got := a.take(); got != "down (30,20) b=2 held=2 n=1; up (30,20) b=2 n=1" {
		t.Fatalf("press into the key surface: %s", got)
	}

	s.Deactivate() // the app lost the foreground
	a.take()
	fa.ClickButton(pt(30, 20), shell.ButtonSecondary) // any button activates
	if got := a.take(); got != "key true; down (30,20) b=2 held=2 n=2 activating; up (30,20) b=2 n=2" {
		t.Fatalf("secondary activating press: %s", got)
	}

	caption := rc(0, 0, 400, 28)
	caption.Kind = shell.RegionCaption
	fb.SetChromeRegions([]shell.ChromeRegion{caption})
	fb.Click(pt(200, 10))
	if got := b.take(); got != "enter (200,10); move (200,10); key true" || s.Key() != fb {
		t.Fatalf("caption press: %s", got)
	}
	a.take()

	m := newModal(t, s, fa, sz(200, 100), &win{})
	m.Show()
	fb.Activate() // refused: the modal stays key
	a.take()
	b.take()
	s.Deactivate()
	fa.Click(pt(30, 40))
	if got := a.take(); got != "" || s.Key() != m {
		t.Fatalf("press into a blocked surface: %q, key %v", got, s.Key() == m)
	}
}
