package fake

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

func TestRunUntilQuit(t *testing.T) {
	s := New()
	var order []int
	s.Post(func() { order = append(order, 0) }) // before Run: waits for it
	a := &app{}
	var w win
	a.onReady = func() {
		if _, err := s.NewSurface(shell.SurfaceConfig{Size: sz(100, 50)}, &w); err != nil {
			t.Error(err)
		}
		s.Post(func() {
			order = append(order, 1)
			s.Quit()
			s.Post(func() { order = append(order, 99) }) // after Quit: dropped
		})
	}
	done := make(chan error, 1)
	go func() { done <- s.Run(a) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after Quit")
	}
	if a.ready != 1 || len(order) != 2 || order[0] != 0 || order[1] != 1 {
		t.Fatalf("ready %d order %v", a.ready, order)
	}
	if w.take() != "closed" {
		t.Error("Quit did not close the surface")
	}
	if !s.Queue().Closed() {
		t.Error("queue open after Run")
	}
	if err := s.Run(a); !errors.Is(err, ErrRunTwice) {
		t.Errorf("second Run: %v", err)
	}
	if err := s.Start(a); !errors.Is(err, ErrRunTwice) {
		t.Errorf("Start after Run: %v", err)
	}
}

func TestRunPostFromGoroutines(t *testing.T) {
	const producers, per = 8, 500
	s := New()
	got := make([][]int, producers)
	total, synced := 0, false
	maybeQuit := func() {
		if total == producers*per && synced {
			s.Quit()
		}
	}
	a := &app{onReady: func() {
		var wg sync.WaitGroup
		for p := range producers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range per {
					s.Post(func() {
						got[p] = append(got[p], i)
						total++
						maybeQuit()
					})
				}
			}()
		}
		// DoSync from a goroutine runs on the UI goroutine.
		go func() {
			ok := s.Queue().DoSync(func() {
				if !s.Queue().OnUIThread() {
					t.Error("DoSync off the UI goroutine")
				}
				synced = true
				maybeQuit()
			})
			if !ok {
				t.Error("DoSync did not run")
			}
		}()
	}}
	if err := s.Run(a); err != nil {
		t.Fatal(err)
	}
	for p, seq := range got {
		if len(seq) != per {
			t.Fatalf("producer %d: %d of %d", p, len(seq), per)
		}
		for i, v := range seq {
			if v != i {
				t.Fatalf("producer %d out of order at %d", p, i)
			}
		}
	}
}

func TestRunPanicReleasesDoSync(t *testing.T) {
	s := New()
	released := make(chan bool, 1)
	a := &app{onReady: func() {
		go func() { released <- s.Queue().DoSync(func() {}) }()
		s.Post(func() { panic("boom") })
	}}
	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Fatalf("recovered %v", r)
			}
		}()
		s.Run(a)
	}()
	select {
	case <-released: // true or false: it ran before the panic or was given up
	case <-time.After(10 * time.Second):
		t.Fatal("DoSync still blocked after Run panicked")
	}
}

func TestStartStep(t *testing.T) {
	s := New()
	s.SetAppearance(driver.Appearance{Dark: true, Accent: driver.DefaultAccent})
	s.SetScreen(sz(800, 600), 2)
	n := 0
	s.Post(func() { n++ })
	a := &app{}
	if err := s.Start(a); err != nil {
		t.Fatal(err)
	}
	if a.ready != 1 || n != 0 {
		t.Fatal("Start ran posted work or not Ready")
	}
	// Ready hands over the text system and the clock (R10).
	if a.services.Text != s.TextSystem() || a.services.Clock != s.Clock() || a.services.Clock == nil {
		t.Fatalf("Ready services %+v", a.services)
	}
	s.Post(func() { n++; s.Post(func() { n += 10 }) })
	if got := s.Step(); got != 2 || n != 2 {
		t.Fatalf("Step ran %d, n = %d", got, n)
	}
	if got := s.Step(); got != 1 || n != 12 {
		t.Fatalf("second Step ran %d, n = %d (re-posted work waits a Step)", got, n)
	}
	if s.Step() != 0 {
		t.Fatal("Step on an empty queue")
	}
	if !s.Appearance().Dark || len(a.appearances) != 0 {
		t.Fatal("initial appearance")
	}
	s.SetAppearance(s.Appearance())
	light := driver.DefaultAppearance()
	s.SetAppearance(light)
	if len(a.appearances) != 1 || a.appearances[0] != light {
		t.Fatalf("AppearanceChanged %v", a.appearances)
	}
	if s.TextSystem() != s.TextSystem() || s.Clock() != s.Clock() || s.FakeClock() != s.Clock() {
		t.Fatal("TextSystem and Clock must return the same value")
	}
	sf, _ := s.NewSurface(shell.SurfaceConfig{Size: sz(10, 10)}, &win{})
	if sf.Scale() != 2 {
		t.Errorf("scale %g, want the screen's", sf.Scale())
	}
	s.Quit()
	if !s.Quitting() || len(s.Surfaces()) != 0 {
		t.Error("Quit")
	}
	s.Post(func() { n = -1 })
	s.Step()
	if n == -1 {
		t.Error("work posted after Quit ran")
	}
	mustPanic(t, "positive", func() { s.SetScreen(sz(1, 1), 0) })
	mustPanic(t, "nil handler", func() { New().Start(nil) })
}

func TestThreadChecks(t *testing.T) {
	s := New()
	mustPanic(t, "off the UI thread", func() { s.Step() })
	s.Start(&app{})
	sf, _ := s.NewSurface(shell.SurfaceConfig{}, &win{})
	f := sf.(*Surface)
	clock := s.FakeClock()
	for name, fn := range map[string]func(){
		"Step":          func() { s.Step() },
		"Quit":          func() { s.Quit() },
		"NewSurface":    func() { s.NewSurface(shell.SurfaceConfig{}, &win{}) },
		"SetTitle":      func() { f.SetTitle("x") },
		"Size":          func() { f.Size() },
		"Invalidate":    func() { f.Invalidate() },
		"Click":         func() { f.Click(pt(1, 1)) },
		"Clock.Now":     func() { clock.Now() },
		"Clock.Advance": func() { clock.Advance(time.Second) },
		"SetAppearance": func() { s.SetAppearance(driver.Appearance{}) },
	} {
		done := make(chan any)
		go func() {
			defer func() { done <- recover() }()
			fn()
		}()
		if r := <-done; r == nil || !strings.Contains(r.(string), "off the UI thread") {
			t.Errorf("%s off the UI goroutine: %v", name, r)
		}
	}
	// Post is fine from anywhere.
	done := make(chan struct{})
	go func() { s.Post(func() {}); close(done) }()
	<-done
	if s.Step() != 1 {
		t.Error("Post from a goroutine")
	}
}

func TestBeforeReady(t *testing.T) {
	s := New()
	mustPanic(t, "off the UI thread", func() { s.TextSystem() }) // not started: no UI goroutine
	bound := New()
	bound.q.Bind() // on the UI goroutine, but Ready has not run
	mustPanic(t, "before Handler.Ready", func() { bound.Clock() })
	mustPanic(t, "before Handler.Ready", func() { bound.NewSurface(shell.SurfaceConfig{}, &win{}) })
	var got driver.Appearance
	s.Start(&app{onReady: func() { got = s.Appearance() }}) // allowed from Ready
	if got != driver.DefaultAppearance() {
		t.Errorf("Appearance in Ready = %v", got)
	}
	if want := (driver.CaretBlink{Period: time.Second, Timeout: 10 * time.Second}); got.CaretBlink != want {
		t.Errorf("default CaretBlink %+v, want %+v", got.CaretBlink, want)
	}
}

func TestSurfaceState(t *testing.T) {
	s := started(t)
	var w win
	sf, err := s.NewSurface(shell.SurfaceConfig{
		Title: "Notes", Size: sz(50, 900), MinSize: sz(100, 0), MaxSize: sz(0, 600),
		Style: driver.StyleResizable | driver.StyleTitled, Center: true,
	}, &w)
	if err != nil {
		t.Fatal(err)
	}
	f := sf.(*Surface)
	if f.Size() != sz(100, 600) || f.Title() != "Notes" || f.Centered() != 1 || f.Visible() || !f.Invalid() {
		t.Fatalf("new surface: %v %q centered %d visible %v", f.Size(), f.Title(), f.Centered(), f.Visible())
	}
	if f.Style() != driver.StyleResizable|driver.StyleTitled || f.MinSize() != sz(100, 0) || f.MaxSize() != sz(0, 600) {
		t.Fatal("config not recorded")
	}
	if len(w.log) != 0 {
		t.Fatalf("NewSurface called the handler: %v", w.log)
	}
	f.SetTitle("Other")
	f.Center()
	f.SetCursor(shell.CursorIBeam)
	if f.Title() != "Other" || f.Centered() != 2 || f.Cursor() != shell.CursorIBeam {
		t.Fatal("setters")
	}
	// SetSize clamps and calls Resized before returning; same size: nothing.
	f.SetSize(sz(300, 700))
	f.SetSize(sz(300, 600))
	f.SetSize(sz(-5, 200))
	if w.take() != "resized 300x600 @1; resized 100x200 @1" {
		t.Fatalf("log %v", w.log)
	}
	// SetSizeLimits sets both at once: one clamp, one Resized.
	f.SetSizeLimits(sz(150, 250), sz(160, 0))
	if f.Size() != sz(150, 250) || w.take() != "resized 150x250 @1" {
		t.Fatalf("limits: %v", f.Size())
	}
	f.SetSizeLimits(sz(150, 250), sz(140, 0)) // min wins
	if f.Size() != sz(150, 250) || f.MinSize() != sz(150, 250) || f.MaxSize() != sz(140, 0) {
		t.Fatalf("min must win over max: %v", f.Size())
	}
	f.SetScale(1.5)
	f.SetScale(1.5)
	if f.Scale() != 1.5 || w.take() != "resized 150x250 @1.5" {
		t.Fatal("SetScale")
	}
	mustPanic(t, "positive", func() { f.SetScale(0) })
}

func TestShowKeyMinimize(t *testing.T) {
	s := started(t)
	var w1, w2 win
	a, _ := s.NewSurface(shell.SurfaceConfig{Size: sz(10, 10)}, &w1)
	b, _ := s.NewSurface(shell.SurfaceConfig{Size: sz(10, 10)}, &w2)
	f1, f2 := a.(*Surface), b.(*Surface)
	f1.Show()
	if !f1.IsKey() || s.Key() != f1 || w1.take() != "key true" {
		t.Fatal("Show makes key")
	}
	f2.Show()
	if w1.take() != "key false" || w2.take() != "key true" {
		t.Fatal("key moves")
	}
	f1.Activate()
	if w1.take() != "key true" || w2.take() != "key false" {
		t.Fatal("Activate")
	}
	f1.Minimize()
	if !f1.Minimized() || s.Key() != nil || w1.take() != "key false" {
		t.Fatal("Minimize drops key")
	}
	f1.Minimize()
	f1.Invalidate()
	s.Flush()
	if f1.Paints() != 0 {
		t.Fatal("a minimized surface was painted")
	}
	f1.Activate() // restores; the pending paint runs at the next frame
	s.Flush()
	if f1.Minimized() || f1.Paints() != 1 || w1.take() != "key true; paint" {
		t.Fatalf("restore: %v", w1.log)
	}
	s.Deactivate()
	if s.Key() != nil || w1.take() != "key false" {
		t.Fatal("Deactivate")
	}
	hidden, _ := s.NewSurface(shell.SurfaceConfig{}, &win{})
	hidden.(*Surface).Activate()
	hidden.Minimize()
	if hidden.(*Surface).Minimized() || s.Key() != nil {
		t.Fatal("a hidden surface cannot be activated or minimized")
	}
}

func TestZoom(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(300, 200), MaxSize: sz(1000, 0), Style: driver.StyleResizable})
	f.Zoom()
	if !f.Zoomed() || f.Size() != sz(1000, 900) || w.take() != "resized 1000x900 @1" {
		t.Fatalf("zoom: %v", f.Size())
	}
	f.Zoom()
	if f.Zoomed() || f.Size() != sz(300, 200) || w.take() != "resized 300x200 @1" {
		t.Fatalf("unzoom: %v", f.Size())
	}
	f.Zoom()
	f.UserResize(sz(400, 400))
	if f.Zoomed() {
		t.Error("a user resize ends the zoom")
	}
	fixed := open(t, s, &w, shell.SurfaceConfig{Size: sz(300, 200)})
	fixed.Zoom()
	if fixed.Zoomed() || fixed.UserResize(sz(1, 1)) || fixed.Size() != sz(300, 200) {
		t.Error("Zoom and UserResize need StyleResizable")
	}
}

func TestCloseAndQuit(t *testing.T) {
	s := started(t)
	var w1, w2, w3 win
	f1 := open(t, s, &w1, shell.SurfaceConfig{})
	f2 := open(t, s, &w2, shell.SurfaceConfig{})
	f3 := open(t, s, &w3, shell.SurfaceConfig{})
	w1.take()
	w2.take()
	if f2.UserClose() || w2.take() != "closeRequested" || f2.Closed() {
		t.Fatal("vetoed close")
	}
	w2.allow = true
	if !f2.UserClose() || w2.take() != "closeRequested; closed" || !f2.Closed() {
		t.Fatal("allowed close")
	}
	f2.Close()
	f2.SetTitle("dead")
	f2.SetSize(sz(5, 5))
	f2.Invalidate()
	f2.Show()
	f2.Click(pt(1, 1))
	if len(w2.log) != 0 || f2.Title() != "" || f2.UserClose() {
		t.Fatalf("a dead surface called its handler: %v", w2.log)
	}
	if len(s.Surfaces()) != 2 || s.Key() != f3 {
		t.Fatal("surfaces after close")
	}
	f3.Close() // the key surface
	if s.Key() != nil || w3.take() != "closed" {
		t.Fatal("closing the key surface")
	}
	s.Quit()
	if w1.take() != "closed" || !f1.Closed() || len(s.Surfaces()) != 0 {
		t.Fatal("Quit closes without asking")
	}
}

func TestPaintOnFrame(t *testing.T) {
	s := started(t)
	blue := driver.RGB8(0, 0x7a, 0xff)
	var w win
	w.paint = func(c render.Canvas) {
		c.FillRoundedRect(render.Rect{W: 80, H: 22}, render.UniformRadii(5), render.Solid(blue))
	}
	sf, _ := s.NewSurface(shell.SurfaceConfig{Size: sz(200, 100)}, &w)
	f := sf.(*Surface)
	if s.Flush() || f.Paints() != 0 {
		t.Fatal("a hidden surface was painted or a frame ran without a request")
	}
	f.Show()
	f.Invalidate()
	f.Invalidate()
	if !s.Flush() || f.Paints() != 1 || f.Invalid() {
		t.Fatalf("one paint per frame: %d", f.Paints())
	}
	if got, want := f.Dump(), "fillRRect (0,0,80,22) r=5 solid #007aff"; got != want {
		t.Fatalf("dump %q, want %q", got, want)
	}
	if f.PaintErr() != nil || f.LastPaint().Depth() != 0 {
		t.Fatal(f.PaintErr())
	}
	if s.Flush() {
		t.Fatal("frame without a request")
	}
	// Advance paints at the frame boundary, after the frame function.
	clock := s.FakeClock()
	var order []string
	clock.SetFrameFunc(func(time.Duration) { order = append(order, "frame") })
	w.paint = func(render.Canvas) { order = append(order, "paint") }
	f.Invalidate()
	clock.Advance(time.Second)
	if strings.Join(order, ",") != "frame,paint" || f.Paints() != 2 {
		t.Fatalf("order %v", order)
	}
	// A paint that invalidates gets another frame, not a loop.
	w.paint = func(render.Canvas) { f.Invalidate() }
	f.Invalidate()
	if n := clock.Advance(3 * FrameInterval); n != 3 || f.Paints() != 5 {
		t.Fatalf("frames %d paints %d", n, f.Paints())
	}
	// An Invalidate inside the frame function joins that frame's paint
	// pass and requests no frame (S6.2a): the last step of an animation
	// ends without an empty frame after it.
	w.paint = func(render.Canvas) {}
	clock.SetFrameFunc(func(time.Duration) { f.Invalidate() })
	clock.RequestFrame()
	if n := clock.Advance(time.Second); n != 1 || f.Paints() != 6 || f.Invalid() || clock.Requested() {
		t.Fatalf("Invalidate in the frame function: %d frames, %d paints, invalid %v", n, f.Paints(), f.Invalid())
	}
	// Outside a frame it requests one again.
	clock.SetFrameFunc(nil)
	f.Invalidate()
	if !clock.Requested() {
		t.Fatal("Invalidate after a frame requested nothing")
	}
	// An unbalanced paint is reported, not panicked.
	w.paint = func(c render.Canvas) { c.Save() }
	s.Flush()
	if f.PaintErr() == nil || !strings.Contains(f.PaintErr().Error(), "not closed") {
		t.Fatalf("PaintErr = %v", f.PaintErr())
	}
	if (&Surface{shell: s}).Dump() != "" {
		t.Error("Dump before a paint")
	}
}

func TestPaintMisuse(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(10, 10)})
	for name, fn := range map[string]func(){
		"SetSize":    func() { f.SetSize(sz(1, 1)) },
		"Show":       func() { f.Show() },
		"Close":      func() { f.Close() },
		"NewSurface": func() { s.NewSurface(shell.SurfaceConfig{}, &win{}) },
		"Quit":       func() { s.Quit() },
		"Click":      func() { f.Click(pt(1, 1)) },
	} {
		w.paint = func(render.Canvas) { fn() }
		f.Invalidate()
		mustPanic(t, "during SurfaceHandler.Paint", func() { s.Flush() })
		_ = name
	}
	w.paint = func(render.Canvas) { f.SetTitle("ok"); f.SetCursor(shell.CursorArrow) }
	f.Invalidate()
	s.Flush()
	if f.Title() != "ok" {
		t.Error("SetTitle during Paint is allowed")
	}
}

func TestUserResizePaintsBeforeReturning(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100), MinSize: sz(50, 50), Style: driver.StyleResizable})
	if !f.UserResize(sz(10, 300)) || f.Size() != sz(50, 300) {
		t.Fatalf("size %v", f.Size())
	}
	if w.take() != "resized 50x300 @1; paint" || f.Invalid() || f.LastPaint() == nil {
		t.Fatal("live resize must paint before returning")
	}
	if f.UserResize(sz(50, 300)) {
		t.Error("no change")
	}
	s.Flush() // the frame requested by the resize finds nothing to paint
	if f.Paints() != 2 {
		t.Errorf("paints %d", f.Paints())
	}
}

func TestChromeRegions(t *testing.T) {
	s := started(t)
	f := open(t, s, &win{}, shell.SurfaceConfig{Size: sz(400, 300)})
	caption := rc(0, 0, 400, 28)
	caption.Kind = shell.RegionCaption
	menu := rc(80, 0, 40, 28) // a menu title: client again
	closeBtn := rc(8, 8, 12, 12)
	closeBtn.Kind = shell.RegionClose
	max := rc(48, 8, 12, 12)
	max.Kind = shell.RegionMaximize
	regions := []shell.ChromeRegion{caption, menu, closeBtn, max}
	f.SetChromeRegions(regions)
	regions[0].Kind = shell.RegionClient // copied
	for _, c := range []struct {
		p    driver.Point
		want shell.RegionKind
	}{
		{pt(200, 10), shell.RegionCaption},
		{pt(90, 10), shell.RegionClient},
		{pt(10, 10), shell.RegionClose},
		{pt(50, 10), shell.RegionMaximize},
		{pt(200, 100), shell.RegionClient},
		{pt(400, 10), shell.RegionClient}, // right edge is outside
	} {
		if got := f.HitTest(c.p); got != c.want {
			t.Errorf("HitTest(%v) = %d, want %d", c.p, got, c.want)
		}
	}
	got := f.ChromeRegions()
	got[0] = shell.ChromeRegion{}
	if f.ChromeRegions()[0].Kind != shell.RegionCaption || len(f.ChromeRegions()) != 4 {
		t.Error("ChromeRegions must return a copy")
	}
	f.SetChromeRegions(nil)
	if f.ChromeRegions() != nil || f.HitTest(pt(200, 10)) != shell.RegionClient {
		t.Error("nil: all client")
	}
}

// TestShellSetChecks: with the checks off a call off the UI goroutine
// does not panic (S6.2c's benchmarks); on again, it does.
func TestShellSetChecks(t *testing.T) {
	s := started(t)
	off := func() (r any) {
		done := make(chan any)
		go func() {
			defer func() { done <- recover() }()
			s.Clock().Now()
		}()
		return <-done
	}
	if off() == nil {
		t.Fatal("no panic off the UI goroutine with the checks on")
	}
	s.SetChecks(false)
	if r := off(); r != nil {
		t.Fatalf("panic with the checks off: %v", r)
	}
	s.SetChecks(true)
	if off() == nil {
		t.Fatal("no panic with the checks on again")
	}
}

// closedFunc is a win whose Closed also runs fn.
type closedFunc struct {
	*win
	fn func()
}

func (c closedFunc) Closed() { c.win.Closed(); c.fn() }

// TestQuitFirst (V2): Quit marks the shell quitting before it closes the
// surfaces, as the real shells do, so NewSurface in a Closed handler (and
// after Quit) returns ErrQuit; a second Quit does nothing.
func TestQuitFirst(t *testing.T) {
	s := started(t)
	var w win
	var inClosed error
	var quitting bool
	sf, err := s.NewSurface(shell.SurfaceConfig{}, closedFunc{&w, func() {
		quitting = s.Quitting()
		_, inClosed = s.NewSurface(shell.SurfaceConfig{Size: sz(10, 10)}, &win{})
	}})
	if err != nil {
		t.Fatal(err)
	}
	sf.Show()
	w.take()
	s.Quit()
	if !quitting || !errors.Is(inClosed, ErrQuit) {
		t.Fatalf("in Closed under Quit: Quitting %v, NewSurface error %v", quitting, inClosed)
	}
	if got := w.take(); got != "closed" || len(s.Surfaces()) != 0 {
		t.Fatalf("Quit: log %q, %d surfaces", got, len(s.Surfaces()))
	}
	if _, err := s.NewSurface(shell.SurfaceConfig{}, &win{}); !errors.Is(err, ErrQuit) {
		t.Fatalf("NewSurface after Quit = %v, want ErrQuit", err)
	}
	s.Quit()
	if len(w.log) != 0 || !s.Quitting() {
		t.Fatal("a second Quit did something")
	}
}
