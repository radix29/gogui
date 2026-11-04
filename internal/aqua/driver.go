package aqua

import (
	"errors"
	"slices"
	"sync/atomic"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/shell"
)

// ErrRunTwice is returned by Driver.Run when it is called a second time.
var ErrRunTwice = errors.New("aqua: Run called more than once")

// Driver is the aqua driver (PLAN.md S2.2d): driver.Driver on top of a
// shell.Shell. It owns the UI-thread queue behind Post, whose wake posts one
// Drain through Shell.Post; merges the forced light or dark mode over the
// shell's OS appearance; and creates aqua windows and containers. Every
// other factory returns a placeholder peer (S2.2e, controls.go): its
// driver/stub peer (§11.1 rule 2) plus an element that draws a labelled
// rounded rectangle.
//
// At Ready the driver installs its Timeline's Tick as the frame function
// of Shell.Clock(): the one Timeline every window's elements animate on
// (Window.Timeline), with Reduce Motion following the effective
// appearance.
//
// SetMenuBar only records the spec (the menu strip comes in S5.6), and
// RunAlert only records the alert (S5.7); neither draws anything yet.
//
// A Driver runs once. Every method except Post and Queue must be called on
// the UI goroutine (the one that called Run).
type Driver struct {
	sh shell.Shell

	q       atomic.Pointer[mainthread.Queue]
	started atomic.Bool
	ready   bool // shell.Handler.Ready has been called
	quit    bool

	cfg driver.AppConfig
	h   driver.AppHandler

	mode    driver.AppearanceMode
	system  driver.Appearance // the shell's OS appearance
	current driver.Appearance // the effective appearance

	timeline *anim.Timeline // from Ready

	windows []*Window
	menuBar *driver.MenuBarSpec
	alerts  []*Alert
}

var _ driver.Driver = (*Driver)(nil)

// New returns an aqua driver on sh, which must not have been started.
func New(sh shell.Shell) *Driver {
	if sh == nil {
		panic("aqua: New with a nil shell")
	}
	return &Driver{
		sh:      sh,
		system:  driver.DefaultAppearance(),
		current: driver.DefaultAppearance(),
	}
}

// Name implements driver.Driver.
func (d *Driver) Name() string { return "aqua" }

// Shell returns the driver's shell.
func (d *Driver) Shell() shell.Shell { return d.sh }

// Queue returns the UI-thread queue, or nil before Run. After Quit it
// returns the closed queue, whose Do and DoSync return false.
func (d *Driver) Queue() *mainthread.Queue { return d.q.Load() }

// Config returns the AppConfig passed to Run.
func (d *Driver) Config() driver.AppConfig { return d.cfg }

// Run implements driver.Driver. It binds the calling goroutine as the UI
// goroutine and runs the shell; once the shell is ready it reads the OS
// appearance, installs the queue's wake and calls h.Ready. It returns the
// shell's result: nil after Quit, or the error that kept the shell from
// starting. The queue is closed on the way out, so goroutines waiting in
// DoSync return false. A second call returns ErrRunTwice.
func (d *Driver) Run(cfg driver.AppConfig, h driver.AppHandler) error {
	if h == nil {
		panic("aqua: Run with a nil handler")
	}
	if !d.started.CompareAndSwap(false, true) {
		return ErrRunTwice
	}
	q := mainthread.New()
	q.Bind()
	d.q.Store(q)
	defer q.Close()
	d.cfg, d.h = cfg, h
	return d.sh.Run(shellHandler{d})
}

// Quit implements driver.Driver. It closes the queue, so later Post, Do and
// DoSync calls from other goroutines are dropped, and quits the shell, which
// closes every window (each gets WindowHandler.Closed) and makes Run return
// once the current work is done. Work already queued still runs once: the
// shell drains its own queue on the way out, and the drain posted for the
// driver's queue runs it.
func (d *Driver) Quit() {
	if d.quit {
		return
	}
	d.quit = true
	if q := d.q.Load(); q != nil {
		q.Close()
	}
	if d.ready {
		d.sh.Quit()
	}
}

// Quitting reports whether Quit has been called.
func (d *Driver) Quitting() bool { return d.quit }

// Post implements driver.Driver. It is safe for concurrent use. Before Run
// and after Quit it drops fn.
func (d *Driver) Post(fn func()) {
	if q := d.q.Load(); q != nil {
		q.Do(fn)
	}
}

// drain runs the queued work; the queue's wake posts it to the shell.
func (d *Driver) drain() { d.q.Load().Drain() }

// Appearance implements driver.Driver.
func (d *Driver) Appearance() driver.Appearance { return d.current }

// Mode returns the mode last passed to SetAppearance.
func (d *Driver) Mode() driver.AppearanceMode { return d.mode }

// SetAppearance implements driver.Driver. The forced mode is merged over the
// shell's OS appearance (only Dark is overridden); on a change every window
// repaints and AppHandler.AppearanceChanged is called.
func (d *Driver) SetAppearance(mode driver.AppearanceMode) {
	d.mode = mode
	d.update()
}

// merged returns the shell's OS appearance with the forced mode applied.
func (d *Driver) merged() driver.Appearance {
	a := d.system
	switch d.mode {
	case driver.AppearanceLight:
		a.Dark = false
	case driver.AppearanceDark:
		a.Dark = true
	}
	return a
}

// update recomputes the effective appearance and reports a change: the
// timeline's Reduce Motion follows it (finishing the running animations
// when it turns on), every window repaints and AppearanceChanged is
// called. Before the shell is ready it only records the value.
func (d *Driver) update() {
	a := d.merged()
	if a == d.current {
		return
	}
	d.current = a
	if !d.ready {
		return
	}
	d.timeline.SetReduceMotion(a.ReduceMotion)
	for _, w := range slices.Clone(d.windows) {
		w.SetAppearance(a)
	}
	d.h.AppearanceChanged(a)
}

// SetMenuBar implements driver.Driver. Until S5.6 it only records a copy of
// bar (nil becomes driver.DefaultMenuBar), which MenuBar returns.
func (d *Driver) SetMenuBar(bar *driver.MenuBarSpec) {
	if bar == nil {
		d.menuBar = driver.DefaultMenuBar()
		return
	}
	d.menuBar = bar.Clone()
}

// MenuBar returns the spec recorded by SetMenuBar, or nil.
func (d *Driver) MenuBar() *driver.MenuBarSpec { return d.menuBar }

// Alert is an alert recorded by Driver.RunAlert. Nothing is shown until
// S5.7.
type Alert struct {
	Parent driver.Window
	Spec   driver.AlertSpec
	done   func(int)
	drv    *Driver
}

// RunAlert implements driver.Driver. Until S5.7 it only records the alert,
// which stays in Alerts until Dismiss is called on it.
func (d *Driver) RunAlert(parent driver.Window, a driver.AlertSpec, done func(result int)) {
	a.Buttons = slices.Clone(a.Buttons)
	d.alerts = append(d.alerts, &Alert{Parent: parent, Spec: a, done: done, drv: d})
}

// Alerts returns the recorded alerts that have not been dismissed.
func (d *Driver) Alerts() []*Alert { return slices.Clone(d.alerts) }

// Dismiss simulates the user pressing the button at index result: the alert
// is forgotten and its done function called. Later calls do nothing.
func (a *Alert) Dismiss(result int) {
	if a.drv == nil {
		return
	}
	a.drv.alerts = slices.DeleteFunc(a.drv.alerts, func(x *Alert) bool { return x == a })
	a.drv = nil
	if a.done != nil {
		a.done(result)
	}
}

// NewWindow implements driver.Driver: an aqua Window (S2.2c) painted with
// the driver's effective appearance.
func (d *Driver) NewWindow(cfg driver.WindowConfig, h driver.WindowHandler) (driver.Window, error) {
	if h == nil {
		panic("aqua: NewWindow with a nil handler")
	}
	th := &trackedHandler{WindowHandler: h, d: d}
	w, err := NewWindow(d.sh, cfg, th)
	if err != nil {
		return nil, err
	}
	th.w = w
	w.timeline = d.timeline
	w.SetAppearance(d.current)
	d.windows = append(d.windows, w)
	return w, nil
}

// Timeline returns the timeline the driver's animations run on (S3.2b),
// or nil before the shell is ready.
func (d *Driver) Timeline() *anim.Timeline { return d.timeline }

// Windows returns the open windows in creation order.
func (d *Driver) Windows() []*Window { return slices.Clone(d.windows) }

// trackedHandler forgets the window when it closes.
type trackedHandler struct {
	driver.WindowHandler
	d *Driver
	w *Window
}

func (t *trackedHandler) Closed() {
	t.d.windows = slices.DeleteFunc(t.d.windows, func(x *Window) bool { return x == t.w })
	t.WindowHandler.Closed()
}

// shellHandler implements shell.Handler for a Driver, so the handler
// methods stay out of Driver's method set.
type shellHandler struct{ d *Driver }

func (s shellHandler) Ready() {
	d := s.d
	d.ready = true
	if d.quit { // Quit was called before Run
		d.q.Load().Close()
		d.sh.Quit()
		return
	}
	// The first appearance is no change: AppearanceChanged never comes
	// before Ready.
	d.system = d.sh.Appearance()
	d.current = d.merged()
	clk := d.sh.Clock()
	d.timeline = anim.NewTimeline(clk)
	d.timeline.SetReduceMotion(d.current.ReduceMotion)
	clk.SetFrameFunc(d.timeline.Tick)
	d.q.Load().SetWake(func() { d.sh.Post(d.drain) })
	d.h.Ready()
}

func (s shellHandler) AppearanceChanged(a driver.Appearance) {
	s.d.system = a
	s.d.update()
}

// NewContainer implements driver.Driver: an aqua Container. The other
// factories return the placeholder peers of controls.go.
func (d *Driver) NewContainer() driver.Container { return NewContainer() }
