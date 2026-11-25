package aqua

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// appLog is a driver.AppHandler whose Ready runs a callback and which logs
// appearance changes.
type appLog struct {
	ready       int
	onReady     func()
	appearances []driver.Appearance
}

func (a *appLog) Ready() {
	a.ready++
	if a.onReady != nil {
		a.onReady()
	}
}
func (a *appLog) AppearanceChanged(ap driver.Appearance) { a.appearances = append(a.appearances, ap) }
func (a *appLog) QuitRequested() bool                    { return true }
func (a *appLog) MenuItemSelected(int)                   {}

// runDriver calls d.Run on the test goroutine with a watchdog, so a Run that
// never returns fails the test instead of hanging it.
func runDriver(t *testing.T, d *Driver, h driver.AppHandler) error {
	t.Helper()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-stop:
		case <-time.After(10 * time.Second):
			panic("aqua: Driver.Run did not return")
		}
	}()
	return d.Run(driver.AppConfig{Name: "Test"}, h)
}

// fakeOf returns the fake shell under d.
func fakeOf(d *Driver) *fake.Shell { return d.Shell().(*fake.Shell) }

func TestDriverRunReadyQuit(t *testing.T) {
	d := New(fake.New())
	if d.Name() != "aqua" || d.Queue() != nil {
		t.Fatalf("before Run: name %q, queue %v", d.Name(), d.Queue())
	}
	h := &appLog{}
	h.onReady = func() {
		if q := d.Queue(); q == nil || !q.OnUIThread() || q.Closed() {
			t.Error("Ready: queue missing, unbound or closed")
		}
		if d.Config().Name != "Test" || d.Quitting() {
			t.Errorf("Ready: config %+v, quitting %v", d.Config(), d.Quitting())
		}
		d.Quit()
		d.Quit() // a second Quit does nothing
		if !d.Quitting() || !fakeOf(d).Quitting() {
			t.Error("Quit did not quit the shell")
		}
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	if h.ready != 1 || !d.Queue().Closed() {
		t.Errorf("ready = %d, queue closed = %v", h.ready, d.Queue().Closed())
	}
	if err := d.Run(driver.AppConfig{}, &appLog{}); !errors.Is(err, ErrRunTwice) {
		t.Errorf("second Run: %v", err)
	}
	mustPanic(t, "nil handler", func() { New(fake.New()).Run(driver.AppConfig{}, nil) })
	mustPanic(t, "nil shell", func() { New(nil) })
}

// errShell is a shell whose Run fails, like GTK 4 missing on Linux.
type errShell struct{ *fake.Shell }

var errNoDisplay = errors.New("no display")

func (errShell) Run(shell.Handler) error { return errNoDisplay }

func TestDriverRunError(t *testing.T) {
	d := New(errShell{fake.New()})
	h := &appLog{}
	if err := runDriver(t, d, h); err != errNoDisplay {
		t.Fatalf("Run = %v, want the shell's error", err)
	}
	if h.ready != 0 || !d.Queue().Closed() {
		t.Errorf("ready = %d, queue closed = %v", h.ready, d.Queue().Closed())
	}
}

func TestDriverQuitBeforeRun(t *testing.T) {
	d := New(fake.New())
	d.Quit()
	h := &appLog{}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	if h.ready != 0 || !d.Queue().Closed() {
		t.Errorf("ready = %d, queue closed = %v", h.ready, d.Queue().Closed())
	}
}

func TestDriverPostFIFOFromGoroutines(t *testing.T) {
	const producers, per = 8, 500
	d := New(fake.New())
	last := make([]int, producers)
	bad, total := 0, 0
	h := &appLog{onReady: func() {
		var wg sync.WaitGroup
		for p := range producers {
			wg.Go(func() {
				for i := 1; i <= per; i++ {
					d.Queue().Do(func() {
						if i != last[p]+1 || !d.Queue().OnUIThread() {
							bad++
						}
						last[p] = i
						total++
						if total == producers*per {
							d.Quit()
						}
					})
				}
			})
		}
		go wg.Wait()
	}}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	if bad > 0 || total != producers*per {
		t.Errorf("total = %d, %d ordering or thread errors", total, bad)
	}
}

func TestDriverDoSync(t *testing.T) {
	d := New(fake.New())
	var onUI, inline bool
	h := &appLog{onReady: func() {
		q := d.Queue()
		order := 0
		q.Do(func() { order = 2 })
		q.DoSync(func() { inline = order == 0; order = 1 })
		go func() {
			if !q.DoSync(func() { onUI = q.OnUIThread() }) {
				t.Error("DoSync from a goroutine returned false")
			}
			d.Queue().Do(d.Quit)
		}()
	}}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	if !inline || !onUI {
		t.Errorf("inline = %v, on the UI goroutine = %v", inline, onUI)
	}
	after := make(chan bool)
	go func() { after <- d.Queue().DoSync(func() { t.Error("DoSync ran after Quit") }) }()
	if <-after {
		t.Error("DoSync after Quit returned true")
	}
}

func TestDriverQueueBeforeRunAndAfterQuit(t *testing.T) {
	d := New(fake.New())
	if d.Queue() != nil {
		t.Fatal("Queue before Run is not nil")
	}
	early := make(chan struct{})
	var ran []string
	h := &appLog{onReady: func() {
		// Work queued from a goroutine before the wake is installed would
		// wait; here it is queued after Ready and must run.
		go func() {
			d.Queue().Do(func() { ran = append(ran, "a") })
			close(early)
		}()
		<-early
		d.Queue().Do(func() {
			ran = append(ran, "b")
			d.Quit()
			d.Queue().Do(func() { t.Error("work posted after Quit ran") })
		})
		d.Queue().Do(func() { ran = append(ran, "c") }) // queued before Quit: runs once
	}}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	if strings.Join(ran, "") != "abc" {
		t.Errorf("ran %q, want abc", ran)
	}
	if d.Queue().Do(func() { t.Error("work queued after Run ran") }) {
		t.Error("Do after Run returned true")
	}
}

func TestDriverQuitClosesWindows(t *testing.T) {
	d := New(fake.New())
	h1, h2 := &winLog{}, &winLog{}
	h := &appLog{onReady: func() {
		for _, h := range []*winLog{h1, h2} {
			w, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 100, H: 50}, Style: allStyles}, h)
			if err != nil {
				t.Fatal(err)
			}
			w.Show()
		}
		if n := len(d.Windows()); n != 2 {
			t.Fatalf("%d windows", n)
		}
		d.Windows()[0].Close()
		if n := len(d.Windows()); n != 1 || d.Windows()[0].Title() != "W" {
			t.Fatalf("after Close: %d windows", n)
		}
		d.Queue().Do(d.Quit)
	}}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	if h1.take() != "key unkey closed" || !strings.HasSuffix(h2.take(), "closed") || len(d.Windows()) != 0 {
		t.Errorf("windows not closed: %v %v %d", h1.log, h2.log, len(d.Windows()))
	}
}

func TestDriverAppearance(t *testing.T) {
	sh := fake.New()
	osDark := driver.Appearance{Dark: true, Accent: driver.RGB8(1, 2, 3)}
	sh.SetAppearance(osDark)
	d := New(sh)
	d.SetAppearance(driver.AppearanceDark) // before Run: the default (light) becomes dark, unreported
	if !d.Appearance().Dark {
		t.Fatal("forced dark before Run")
	}
	d.SetAppearance(driver.AppearanceLight) // before Run: recorded, not reported
	if d.Mode() != driver.AppearanceLight {
		t.Fatalf("mode %v", d.Mode())
	}
	h := &appLog{}
	light := osDark
	light.Dark = false
	h.onReady = func() {
		if d.Appearance() != light || len(h.appearances) != 0 {
			t.Fatalf("at Ready: %+v, %d changes", d.Appearance(), len(h.appearances))
		}
		w, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 100, H: 50}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		aw := w.(*Window)
		if aw.Appearance() != light {
			t.Fatalf("window appearance %+v", aw.Appearance())
		}
		w.Show()
		sf := aw.Surface().(*fake.Surface)
		lightDump := paint(t, sh, sf)

		d.SetAppearance(driver.AppearanceLight) // no change
		d.SetAppearance(driver.AppearanceSystem)
		if d.Appearance() != osDark || aw.Appearance() != osDark || !sf.Invalid() {
			t.Errorf("system: %+v, window %+v, invalid %v", d.Appearance(), aw.Appearance(), sf.Invalid())
		}
		darkDump := paint(t, sh, sf)
		if darkDump == lightDump || !strings.Contains(darkDump, "fillRRect (0,28,100,50) solid #323232") {
			t.Errorf("dark dump:\n%s", darkDump)
		}

		// Forced dark: the OS switching dark/light changes nothing; another
		// accent does.
		d.SetAppearance(driver.AppearanceDark)
		sh.SetAppearance(light)
		sh.SetAppearance(osDark)
		accent := osDark
		accent.Accent = driver.RGB8(9, 9, 9)
		sh.SetAppearance(driver.Appearance{Accent: accent.Accent})
		if d.Appearance() != accent || aw.Appearance() != accent {
			t.Errorf("forced dark: %+v", d.Appearance())
		}
		d.Quit()
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	accent := osDark
	accent.Accent = driver.RGB8(9, 9, 9)
	if len(h.appearances) != 2 || h.appearances[0] != osDark || h.appearances[1] != accent {
		t.Errorf("AppearanceChanged calls: %+v", h.appearances)
	}
}

func TestDriverMenuBarAndAlerts(t *testing.T) {
	d := New(fake.New())
	if d.MenuBar() != nil {
		t.Fatal("menu bar before SetMenuBar")
	}
	d.SetMenuBar(nil)
	if d.MenuBar() == nil || len(d.MenuBar().Menus) == 0 {
		t.Fatal("nil menu bar is not the default one")
	}
	bar := &driver.MenuBarSpec{Menus: []driver.MenuSpec{{Title: "File"}}}
	d.SetMenuBar(bar)
	bar.Menus[0].Title = "changed"
	if d.MenuBar().Menus[0].Title != "File" {
		t.Error("SetMenuBar did not copy the spec")
	}

	var results []int
	buttons := []string{"OK", "Cancel"}
	d.RunAlert(nil, driver.AlertSpec{Message: "A", Buttons: buttons}, func(r int) { results = append(results, r) })
	d.RunAlert(nil, driver.AlertSpec{Message: "B", Buttons: buttons}, nil)
	buttons[0] = "changed"
	as := d.Alerts()
	if len(as) != 2 || as[0].Spec.Buttons[0] != "OK" {
		t.Fatalf("alerts %+v", as)
	}
	as[0].Dismiss(1)
	as[0].Dismiss(0) // dismissed already
	as[1].Dismiss(0) // no done function
	if len(d.Alerts()) != 0 || len(results) != 1 || results[0] != 1 {
		t.Errorf("after Dismiss: %d alerts, results %v", len(d.Alerts()), results)
	}
}

func TestDriverFactories(t *testing.T) {
	d := New(fake.New())
	if _, ok := d.NewContainer().(*Container); !ok {
		t.Error("NewContainer is not an aqua Container")
	}
	// Every other factory returns a peer around its stub peer (a placeholder,
	// or a real control: Label, Separator, Box).
	peers := []driver.Widget{
		d.NewLabel(), d.NewButton(nil), d.NewCheckbox(nil), d.NewRadio(nil),
		d.NewTextField(false, nil), d.NewTextField(true, nil), d.NewSlider(nil),
		d.NewProgress(), d.NewPopUpButton(nil), d.NewSeparator(), d.NewBox(),
		d.NewScrollView(nil), d.NewTextView(nil),
	}
	for i, p := range peers {
		if _, isElement := p.(Element); isElement {
			t.Errorf("factory %d returned an element", i)
		}
		if ElementOf(p) == nil {
			t.Errorf("factory %d: no placeholder element", i)
		}
	}
	if tf, ok := peers[5].(*TextField); !ok || !tf.Secure {
		t.Errorf("secure text field: %#v", peers[5])
	}
}

func TestDriverNewWindowError(t *testing.T) {
	sh := fake.New()
	d := New(failShell{sh})
	h := &appLog{onReady: func() {
		if _, err := d.NewWindow(driver.WindowConfig{}, &winLog{}); err != errNoSurface {
			t.Errorf("NewWindow error: %v", err)
		}
		if len(d.Windows()) != 0 {
			t.Error("failed window tracked")
		}
		mustPanic(t, "nil window handler", func() { d.NewWindow(driver.WindowConfig{}, nil) })
		d.Quit()
	}}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}

// widgetBox is a test box that also is a driver.Widget.
type widgetBox struct{ *box }

func (widgetBox) SetEnabled(bool)                   {}
func (widgetBox) SetHidden(bool)                    {}
func (widgetBox) SetControlSize(driver.ControlSize) {}
func (widgetBox) SetTooltip(string)                 {}
func (widgetBox) Focus()                            {}
func (widgetBox) Destroy()                          {}

func TestContainer(t *testing.T) {
	c := NewContainer()
	a := newBox("a", 0, 0, 20, 10)
	b := newBox("b", 10, 5, 20, 10)
	b.color = driver.RGB8(0, 0, 255)
	label := stub.NewLabel()
	c.SetFrame(driver.Rect{X: 5, Y: 5, W: 100, H: 50})
	c.SetChildren([]driver.Widget{widgetBox{a}, label, widgetBox{b}})
	if names(c.Children()) != "a b" || len(c.Widgets()) != 3 || c.Widgets()[1] != driver.Widget(label) {
		t.Fatalf("children %q, widgets %d", names(c.Children()), len(c.Widgets()))
	}
	checkDump(t, dump(t, c, driver.Size{W: 200, H: 100}),
		"fillRRect (5,5,20,10) solid #ff0000",
		"fillRRect (15,10,20,10) solid #0000ff",
	)
	// Reordered: b paints first; a moved to another container leaves c.
	c.SetChildren([]driver.Widget{widgetBox{b}, widgetBox{a}})
	if names(c.Children()) != "b a" {
		t.Fatalf("reordered %q", names(c.Children()))
	}
	other := NewContainer()
	other.SetChildren([]driver.Widget{widgetBox{a}})
	if names(c.Children()) != "b" || a.Parent() != Element(other) {
		t.Fatalf("after move: %q", names(c.Children()))
	}
	// A nested container; an invalid list panics without changing anything.
	c.SetChildren([]driver.Widget{other, widgetBox{b}})
	mustPanic(t, "repeated child", func() { c.SetChildren([]driver.Widget{widgetBox{b}, widgetBox{b}}) })
	mustPanic(t, "self as child", func() { other.SetChildren([]driver.Widget{c}) })
	if len(c.Children()) != 2 || c.Child(0) != Element(other) || c.Child(1) != Element(b) {
		t.Fatalf("changed by a panicking SetChildren: %d children", len(c.Children()))
	}
	if len(c.Widgets()) != 2 || len(other.Widgets()) != 1 {
		t.Fatalf("widgets changed by a panicking SetChildren")
	}
	checkDump(t, dump(t, c, driver.Size{W: 200, H: 100}),
		"fillRRect (5,5,20,10) solid #ff0000",
		"fillRRect (15,10,20,10) solid #0000ff",
	)

	// The driver.Widget methods.
	c.SetControlSize(driver.ControlSizeSmall)
	c.SetTooltip("tip")
	c.Focus()
	if c.ControlSize() != driver.ControlSizeSmall || c.Tooltip() != "tip" || c.Focusable() {
		t.Errorf("widget state: %v %q %v", c.ControlSize(), c.Tooltip(), c.Focusable())
	}
	c.SetHidden(true)
	if dump(t, c, driver.Size{W: 200, H: 100}) != "" {
		t.Error("hidden container painted")
	}
	c.SetHidden(false)
	other.Destroy()
	if !other.Destroyed() || other.Parent() != nil || c.ChildCount() != 1 || names(other.Children()) != "a" {
		t.Errorf("Destroy: parent %v, children %q", other.Parent(), names(c.Children()))
	}
	c.SetChildren(nil)
	if c.ChildCount() != 0 || b.Parent() != nil {
		t.Error("SetChildren(nil) kept children")
	}
}

func TestDriverWindowWithContainers(t *testing.T) {
	d := New(fake.New())
	h := &appLog{onReady: func() {
		sh := fakeOf(d)
		w, err := d.NewWindow(driver.WindowConfig{Title: "Hi", Size: driver.Size{W: 200, H: 100}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		aw := w.(*Window)
		sf := aw.Surface().(*fake.Surface)
		root := d.NewContainer()
		inner := d.NewContainer()
		var log []string
		p := newGreenProbe(&log)
		inner.SetFrame(driver.Rect{X: 20, Y: 10, W: 100, H: 50})
		inner.SetChildren([]driver.Widget{p})
		root.SetFrame(driver.Rect{W: 200, H: 100})
		root.SetChildren([]driver.Widget{stub.NewLabel(), inner}) // a non-element child is ignored
		w.SetContent(root)
		w.Show()
		got := paint(t, sh, sf)
		const want = "fillRRect (30,48,30,20) solid #00ff00 clip=(0,28,200,100)"
		if !strings.Contains(got, "fillRRect (0,28,200,100) solid #ececec\n"+want+"\n") {
			t.Errorf("dump lacks the probe after the content background:\n%s", got)
		}
		// Input reaches the nested element in its coordinates (after the
		// state event of Show making the window key).
		sf.Click(pt(55, 50))
		checkLog(t, &log, "p state key=true dark=false", "p enter 25,2", "p move 25,2", "p down 25,2 x1", "p up 25,2 x1")
		// Moving the inner container repaints.
		inner.SetFrame(driver.Rect{X: 0, Y: 0, W: 100, H: 50})
		if !sf.Invalid() || !strings.Contains(paint(t, sh, sf), "fillRRect (10,38,30,20) solid #00ff00 clip=") {
			t.Errorf("moved container:\n%s", sf.Dump())
		}
		d.Quit()
	}}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}

// greenProbe is a probe that fills its bounds green and is a driver.Widget.
type greenProbe struct {
	probe
	widgetMethods
}

func newGreenProbe(log *[]string) *greenProbe {
	p := &greenProbe{probe: probe{name: "p", log: log, handles: map[string]bool{"down": true}}}
	p.Init(p)
	p.SetFrame(driver.Rect{X: 10, Y: 10, W: 30, H: 20})
	return p
}

func (p *greenProbe) Paint(c render.Canvas) {
	c.FillRoundedRect(p.Bounds(), render.Radii{}, render.Solid(driver.RGB8(0, 255, 0)))
}

// widgetMethods are the driver.Widget methods an element lacks.
type widgetMethods struct{}

func (widgetMethods) SetEnabled(bool)                   {}
func (widgetMethods) SetHidden(bool)                    {}
func (widgetMethods) SetControlSize(driver.ControlSize) {}
func (widgetMethods) SetTooltip(string)                 {}
func (widgetMethods) Focus()                            {}
func (widgetMethods) Destroy()                          {}

// TestDriverTimeline: the driver's timeline runs on the shell's frame
// clock, is shared by its windows, requests no frames while idle and
// follows Reduce Motion.
func TestDriverTimeline(t *testing.T) {
	sh := fake.New()
	reduced := driver.DefaultAppearance()
	reduced.ReduceMotion = true
	sh.SetAppearance(reduced) // the OS starts with Reduce Motion on
	d := New(sh)
	if d.Timeline() != nil {
		t.Fatal("a timeline before Run")
	}
	h := &appLog{}
	h.onReady = func() {
		defer d.Quit()
		tl := d.Timeline()
		if tl == nil || !tl.ReduceMotion() {
			t.Fatalf("at Ready: timeline %v", tl)
		}
		w, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 100, H: 50}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		aw := w.(*Window)
		if aw.Timeline() != tl || aw.Root().base().Window().Timeline() != tl {
			t.Fatal("the window does not share the driver's timeline")
		}
		alone, err := NewWindow(sh, services(sh), driver.WindowConfig{Size: driver.Size{W: 10, H: 10}}, &winLog{})
		if err != nil || alone.Timeline() != nil {
			t.Fatalf("a window outside the driver: %v, timeline %v", err, alone.Timeline())
		}
		w.Show()
		sf := aw.Surface().(*fake.Surface)
		paint(t, sh, sf)
		clk := sh.FakeClock()

		changes := 0
		v := tl.NewValue(0, func(float64) { changes++; aw.Root().base().Invalidate() })
		v.AnimateTo(1, 100*time.Millisecond, anim.Linear) // Reduce Motion: a jump
		if v.Value() != 1 || v.Running() || changes != 1 || !sf.Invalid() {
			t.Fatalf("with Reduce Motion: %v running %v", v.Value(), v.Running())
		}
		paint(t, sh, sf)
		if clk.Requested() || clk.Advance(time.Second) != 0 {
			t.Fatal("an idle app requested a frame")
		}

		sh.SetAppearance(driver.DefaultAppearance())
		if tl.ReduceMotion() {
			t.Fatal("Reduce Motion did not follow the appearance")
		}
		paint(t, sh, sf)
		frames := clk.Frames()
		// Frames at 1/60 s: the 7th ends the animation. Its change func
		// invalidates the root inside the frame function, which requests no
		// frame (S6.2a): the 7th frame's paint pass paints it, and the
		// clock stops with no empty frame after it.
		v.AnimateTo(0, 100*time.Millisecond, anim.Linear)
		if n := clk.Advance(time.Second); n != 7 || clk.Frames()-frames != 7 {
			t.Fatalf("the animation ran %d frames", n)
		}
		if v.Value() != 0 || v.Running() || changes != 8 || clk.Requested() || sf.Invalid() {
			t.Fatalf("after the animation: %v running %v, %d changes", v.Value(), v.Running(), changes)
		}

		v.AnimateTo(1, time.Second, anim.Linear)
		clk.Advance(100 * time.Millisecond)
		sh.SetAppearance(reduced) // turning it on finishes the animation
		if v.Value() != 1 || v.Running() || !tl.ReduceMotion() {
			t.Fatalf("Reduce Motion while running: %v running %v", v.Value(), v.Running())
		}
		clk.Advance(time.Second)
		if clk.Requested() {
			t.Fatal("frames after the animation finished")
		}
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}
