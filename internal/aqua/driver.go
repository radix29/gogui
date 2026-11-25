package aqua

import (
	"errors"
	"slices"
	"sync/atomic"
	"time"

	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/aqua/menubar"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// ErrRunTwice is returned by Driver.Run when it is called a second time.
var ErrRunTwice = errors.New("aqua: Run called more than once")

// Driver is the aqua driver (PLAN.md S2.2d): driver.Driver on top of a
// shell.Shell. It owns the UI-thread queue (Queue), whose wake posts one
// Drain through Shell.Post; merges the forced light or dark mode over the
// shell's OS appearance; and creates aqua windows and containers. Every
// other factory returns a control's peer (peer.go): its driver/stub
// peer (§11.1 rule 2) plus the control's drawn element.
//
// At Ready the driver installs its Timeline's Tick as the frame function
// of the shell's frame clock (shell.Services): the one Timeline every window's elements animate on
// (Window.Timeline), with Reduce Motion following the effective
// appearance.
//
// SetMenuBar builds the menu model (menubar.Build, titled with
// AppConfig.Name) and shows it as the menu strip of every titled window,
// present and future (D11, S5.6c), whose picks report through the handler
// (S5.6f). RunAlert shows an alert with a parent as a sheet of it (S5.7c)
// and an app-modal one in a modal surface (S5.7g, alertsurface.go).
//
// A Driver runs once. Every method except Queue must be called on
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
	accent  driver.Color      // the app's accent override, zero for none (S6.1g)
	current driver.Appearance // the effective appearance

	text     render.TextSystem // from Ready (shell.Services)
	clock    shell.FrameClock  // from Ready
	timeline *anim.Timeline    // from Ready

	windows []*Window
	menuBar *driver.MenuBarSpec
	bar     *menubar.Bar // built from menuBar, nil before SetMenuBar
	alerts  []*Alert

	appAlerts     []*Alert        // the app-modal alerts, first shown (S5.7g)
	alertSurfaces []*alertSurface // the open alert surfaces (S5.7g)
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

// Clock returns the frame clock the shell handed to Ready (shell.Services),
// or nil before it.
func (d *Driver) Clock() shell.FrameClock { return d.clock }

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
	if d.menuBar != nil {
		d.rebuildMenuBar() // SetMenuBar before Run: the app name is known now
	}
	return d.sh.Run(shellHandler{d})
}

// Quit implements driver.Driver. It closes the queue, so later Do and
// DoSync calls from other goroutines are dropped, and quits the shell, which
// closes every window (each gets WindowHandler.Closed) and makes Run return
// once the current work is done. Work already queued still runs once: the
// shell drains its own queue on the way out, and the drain posted for the
// driver's queue runs it. The app-modal alerts left in the queue are then
// answered with their Cancel index (0 without one), after the shown one,
// whose surface the shell closed (S5.7g).
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
	d.dismissAppAlerts()
}

// Quitting reports whether Quit has been called.
func (d *Driver) Quitting() bool { return d.quit }

// ObserveFrames implements driver.Driver: fn becomes the frame observer of
// the shell's frame clock, whose records are driver.FrameTiming values, and the
// clock's Now is returned.
func (d *Driver) ObserveFrames(fn func(driver.FrameTiming)) func() time.Duration {
	d.clock.SetFrameObserver(fn)
	return d.clock.Now
}

// drain runs the queued work; the queue's wake posts it to the shell.
func (d *Driver) drain() { d.q.Load().Drain() }

// Appearance implements driver.Driver.
func (d *Driver) Appearance() driver.Appearance { return d.current }

// Mode returns the mode last passed to SetAppearance.
func (d *Driver) Mode() driver.AppearanceMode { return d.mode }

// SetAppearance implements driver.Driver. The forced mode is merged over the
// shell's OS appearance (only Dark is overridden; SetAccentColor overrides the accent); on a change every window
// repaints and AppHandler.AppearanceChanged is called.
func (d *Driver) SetAppearance(mode driver.AppearanceMode) {
	d.mode = mode
	d.update()
}

// SetAccentColor implements driver.Driver: the app's accent replaces the
// shell's OS accent in the effective appearance before it reaches the
// theme, so every accent-derived color follows (S6.1g). The zero Color
// clears the override. A semantic c is resolved against the merged
// appearance each time it changes, with driver.DefaultAccent for the
// accent colors; the result is opaque. On a change every window repaints
// and AppHandler.AppearanceChanged is called.
func (d *Driver) SetAccentColor(c driver.Color) {
	d.accent = c
	d.update()
}

// AccentOverride returns the color last passed to SetAccentColor (the
// zero Color when there is no override).
func (d *Driver) AccentOverride() driver.Color { return d.accent }

// merged returns the shell's OS appearance with the forced mode and the
// app's accent applied.
func (d *Driver) merged() driver.Appearance {
	a := d.system
	switch d.mode {
	case driver.AppearanceLight:
		a.Dark = false
	case driver.AppearanceDark:
		a.Dark = true
	}
	if d.accent != (driver.Color{}) {
		a.Accent = resolveAccent(d.accent, a)
	}
	return a
}

// resolveAccent returns the app's accent c as a concrete, opaque color for
// the appearance a: a semantic c resolves with the default accent, so the
// colors the theme derives from the accent (ColorSystemBlue,
// ColorKeyboardFocusIndicator …) do not resolve to the OS one.
func resolveAccent(c driver.Color, a driver.Appearance) driver.Color {
	a.Accent = driver.DefaultAccent
	c = resolveColor(c, a)
	c.A = 1
	return c
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
	for _, m := range slices.Clone(d.alertSurfaces) {
		m.stateChanged()
	}
	d.h.AppearanceChanged(a)
}

// SetMenuBar implements driver.Driver. It records a copy of bar (nil
// becomes driver.DefaultMenuBar), which MenuBar returns, builds its model
// and rebuilds the menu strip of every titled window (S5.6c).
func (d *Driver) SetMenuBar(bar *driver.MenuBarSpec) {
	if bar == nil {
		d.menuBar = driver.DefaultMenuBar()
	} else {
		d.menuBar = bar.Clone()
	}
	d.rebuildMenuBar()
}

// rebuildMenuBar builds the model of the recorded spec with the app name
// and gives it to every window.
func (d *Driver) rebuildMenuBar() {
	d.bar = menubar.Build(d.menuBar, d.cfg.Name)
	for _, w := range slices.Clone(d.windows) {
		w.setMenuBar(d.bar)
	}
}

// MenuBarModel returns the menu model the strips show, or nil.
func (d *Driver) MenuBarModel() *menubar.Bar { return d.bar }

// menuPicked runs a pick of item r in w's menu strip (S5.6f, S5.6g),
// validated again first (menuEnabled): an item with an ID is reported to
// AppHandler.MenuItemSelected, a role item without one runs its action
// (menuaction.go), any other item does nothing.
func (d *Driver) menuPicked(w *Window, r menubar.Ref) {
	if d.h == nil || !d.menuEnabled(w, r) {
		return
	}
	d.runItem(w, r)
}

// runItem runs item r, validated, in w: an item with an ID is reported to
// AppHandler.MenuItemSelected, a role item runs its action.
func (d *Driver) runItem(w *Window, r menubar.Ref) {
	it := d.bar.Item(r)
	if it.ID != 0 {
		d.h.MenuItemSelected(it.ID)
		return
	}
	d.runRole(w, it.Role)
}

// accelerator handles a key-down in the key window w before its router
// (S5.6h): the bar's item for the key (menubar.Bar.Lookup with the
// shell's shifted character), validated as at a menu opening
// (menuEnabled), runs as a pick would, after its menu's title in w's
// strip starts its flash (menuStrip.flash; untitled windows have no
// strip but take the accelerators all the same, D11), and the key is
// consumed. A disabled item, or none, leaves the key to the focus.
// Auto-repeat runs the item again. A nil w is an alert's modal surface
// (S5.7g), where only Quit is enabled.
func (d *Driver) accelerator(w *Window, k shell.KeyEvent) bool {
	if d.h == nil || !k.Down || (w != nil && w.closed) {
		return false
	}
	r, ok := d.bar.Lookup(k.Key, k.Shifted, k.Mods)
	if !ok || !d.menuEnabled(w, r) {
		return false
	}
	if w != nil {
		if s := w.menuStrip(); s != nil {
			s.flash(r.Menu)
		}
	}
	d.runItem(w, r)
	return true
}

// MenuBar returns the spec recorded by SetMenuBar, or nil.
func (d *Driver) MenuBar() *driver.MenuBarSpec { return d.menuBar }

// Alert is an alert of Driver.RunAlert, until it is answered: shown or
// queued as a sheet of its parent (S5.7c; in a modal surface owned by the
// parent when the window is too small for it, S5.7g), or app-modal, shown
// or queued in a modal surface of its own (S5.7g, alertsurface.go).
type Alert struct {
	Parent driver.Window
	Spec   driver.AlertSpec
	done   func(int)
	drv    *Driver
	el     *alert
	sheet  *sheet        // nil for an app-modal alert
	modal  *alertSurface // an app-modal alert's surface, once shown

	started bool // an app-modal alert: its turn in the queue came
}

// RunAlert implements driver.Driver. With an open aqua Window as parent
// the alert is queued as a sheet of it (sheet.go); any other alert (a nil
// parent, or a window that has closed or is closing) is app-modal, queued
// in the driver and shown in a modal surface (alertsurface.go). Either is
// answered by its buttons, and stays in Alerts until it is answered
// (Dismiss answers it as a button would).
func (d *Driver) RunAlert(parent driver.Window, a driver.AlertSpec, done func(result int)) {
	a.Buttons = slices.Clone(a.Buttons)
	rec := &Alert{Parent: parent, Spec: a, done: done, drv: d}
	d.alerts = append(d.alerts, rec)
	w, _ := parent.(*Window)
	if w == nil || w.closed || w.closing {
		d.appAlerts = append(d.appAlerts, rec) // its element comes with its turn
		d.showAppAlert()
		return
	}
	rec.el = rec.element()
	rec.sheet = w.addSheet(rec.el)
}

// element builds the alert's element, answering through the record.
func (a *Alert) element() *alert {
	el := newAlert(a.drv, a.Spec)
	el.SetPressFunc(func(i int) { a.answered(i) })
	return el
}

// answered ends the alert with button i: it leaves Alerts, its sheet
// slides out (and the window's modality may end) or its modal surface
// closes, then done runs; after an app-modal one the next of the queue
// opens.
func (a *Alert) answered(i int) {
	d := a.drv
	if d == nil {
		return
	}
	d.alerts = slices.DeleteFunc(d.alerts, func(x *Alert) bool { return x == a })
	a.drv = nil
	if a.sheet != nil {
		a.sheet.answeredBy()
	} else {
		d.appAlerts = slices.DeleteFunc(d.appAlerts, func(x *Alert) bool { return x == a })
		if a.modal != nil {
			a.modal.close()
		}
	}
	if a.done != nil {
		a.done(i)
	}
	if a.sheet == nil {
		d.showAppAlert()
	}
}

// Alerts returns the recorded alerts that have not been dismissed.
func (d *Driver) Alerts() []*Alert { return slices.Clone(d.alerts) }

// Dismiss answers the alert as if the user pressed the button at index
// result, through its element (alert.answer), so a sheet slides out and a
// modal surface closes as after a click; a queued app-modal alert, which
// has no element yet, is answered directly. The alert leaves Alerts and
// its done function is called. Later calls do nothing.
func (a *Alert) Dismiss(result int) {
	switch {
	case a.drv == nil:
	case a.el != nil:
		a.el.answer(result)
	default:
		a.answered(result)
	}
}

// Window returns the window the alert is a sheet of, or nil (app-modal).
func (a *Alert) Window() *Window {
	if a.sheet == nil {
		return nil
	}
	return a.sheet.w
}

// Surface returns the modal surface the alert is shown in (app-modal, or
// a sheet of a window too small for it), or nil: queued, shown as a
// sheet, or answered.
func (a *Alert) Surface() shell.Surface {
	m := a.modal
	if a.sheet != nil {
		m = a.sheet.surf
	}
	if m == nil || m.closed {
		return nil
	}
	return m.surface
}

// Root returns the root element of the alert's own tree (the alert is
// its only child), in the points of its sheet or modal surface: for tests
// above the driver.
func (a *Alert) Root() Element {
	if a.sheet != nil {
		return a.sheet.root
	}
	if a.modal != nil {
		return a.modal.root
	}
	return nil
}

// NewWindow implements driver.Driver: an aqua Window (S2.2c) painted with
// the driver's effective appearance.
func (d *Driver) NewWindow(cfg driver.WindowConfig, h driver.WindowHandler) (driver.Window, error) {
	if h == nil {
		panic("aqua: NewWindow with a nil handler")
	}
	th := &trackedHandler{WindowHandler: h, d: d}
	w, err := NewWindow(d.sh, d.services(), cfg, th)
	if err != nil {
		return nil, err
	}
	th.w = w
	w.timeline = d.timeline
	w.driver = d
	w.menuPicked = func(r menubar.Ref) { d.menuPicked(w, r) }
	w.menuEnabled = func(r menubar.Ref) bool { return d.menuEnabled(w, r) }
	w.accelerator = func(k shell.KeyEvent) bool { return d.accelerator(w, k) }
	w.SetAppearance(d.current)
	w.setMenuBar(d.bar)
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

func (s shellHandler) Ready(sv shell.Services) {
	d := s.d
	d.text, d.clock = sv.Text, sv.Clock
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
	clk := d.clock
	d.timeline = anim.NewTimeline(clk)
	d.timeline.SetReduceMotion(d.current.ReduceMotion)
	clk.SetFrameFunc(d.timeline.Tick)
	d.q.Load().SetWake(func() { d.sh.Post(d.drain) })
	d.h.Ready()
	d.showAppAlert() // one run before Ready (S5.7g)
}

func (s shellHandler) AppearanceChanged(a driver.Appearance) {
	s.d.system = a
	s.d.update()
}

// NewContainer implements driver.Driver: an aqua Container. The other
// factories return the controls' peers (peer.go).
func (d *Driver) NewContainer() driver.Container { return NewContainer() }

// services returns what the shell handed to Ready.
func (d *Driver) services() shell.Services {
	return shell.Services{Text: d.text, Clock: d.clock}
}

// NewWithServices returns a driver on sh, which is running already, with
// the services its Handler.Ready handed over: for tests and tools that
// build windows and controls on a shell they started themselves, without
// Run (the driver then has no timeline and no queue). Run would replace
// the services with its own Ready's.
func NewWithServices(sh shell.Shell, sv shell.Services) *Driver {
	d := New(sh)
	d.text, d.clock = sv.Text, sv.Clock
	return d
}
