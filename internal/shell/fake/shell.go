package fake

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/render/record"
	"github.com/radix29/gogui/internal/shell"
)

// ErrRunTwice is returned by Run and Start when the shell has already been
// started.
var ErrRunTwice = errors.New("fake: Run or Start called more than once")

// ErrQuit is returned by NewSurface after Quit, as on the real shells.
var ErrQuit = errors.New("fake: NewSurface after Quit")

// Default screen of a new Shell: the work area Zoom fills, and the scale of
// new surfaces.
var (
	DefaultScreenSize  = driver.Size{W: 1440, H: 900}
	DefaultScreenScale = 1.0
)

// Shell is a shell.Shell without an OS. Create one with New.
//
// It runs in one of two modes. Run binds the calling goroutine as the UI
// goroutine, calls Handler.Ready and then runs posted work until Quit, like
// a native loop. Start binds and calls Ready, then returns; the test drives
// the shell from the same goroutine with Step (drain posted work), Flush or
// Clock().Advance (run frames) and the Surface input methods. Neither mode
// runs frames by itself: time moves only with Clock().Advance.
type Shell struct {
	q    *mainthread.Queue
	wake chan struct{}

	started atomic.Bool
	ready   bool // Handler.Ready has been called
	quit    bool
	h       shell.Handler

	clock *Clock
	text  *TextSystem

	appearance  driver.Appearance
	screen      driver.Size
	screenScale float64

	surfaces []*Surface // open surfaces, in creation order
	key      *Surface   // the key surface, or nil
	modals   []*Surface // open, shown modal surfaces, in first-Show order (modal.go)
	quitting bool       // closeAll is closing every surface
	painting bool       // a SurfaceHandler.Paint or PopupHandler.Paint is running

	// The clipboard (clipboard.go): its text, whether it holds any, and
	// how many reads were asked for.
	clip      string
	clipOK    bool
	clipReads int

	// Popups (popup.go): the open ones in creation order, the grab popup
	// or nil, the work area they are placed in, whether new ones are
	// translucent and whether the shell draws their shadow.
	popups      []*Popup
	grab        *Popup
	work        driver.Rect
	translucent bool
	noShadow    atomic.Bool // Caps().PopupShadow is false (SetCaps)
	deferPlaced bool        // Placed is posted (SetDeferPlaced)

	// shiftLayout maps an unshifted character key to its character with
	// Shift (KeyEvent.Shifted, S5.6h); nil is the US layout.
	shiftLayout map[driver.Key]driver.Key

	// unchecked turns the UI-goroutine checks off (SetChecks).
	unchecked atomic.Bool
}

var _ shell.PopupShell = (*Shell)(nil)

// New returns a shell that has not been started, with the default
// appearance (driver.DefaultAppearance, so a 1 s caret blink that stops
// after 10 s), DefaultScreenSize and
// DefaultScreenScale.
func New() *Shell {
	s := &Shell{
		q:           mainthread.New(),
		wake:        make(chan struct{}, 1),
		text:        NewTextSystem(),
		appearance:  driver.DefaultAppearance(),
		screen:      DefaultScreenSize,
		screenScale: DefaultScreenScale,
		work:        driver.Rect{W: DefaultScreenSize.W, H: DefaultScreenSize.H},
	}
	s.clock = &Clock{check: s.check, afterFunc: s.paintPass, offScreen: s.offScreen}
	return s
}

// SetChecks turns the shell's UI-goroutine checks on (the default) or
// off. Each check parses the goroutine ID from a stack trace, about a
// microsecond, and the shell checks every call, so the frame benchmarks
// (PLAN.md S6.2c) turn them off to time the aqua driver rather than the
// checks. Any goroutine may call it.
func (s *Shell) SetChecks(on bool) { s.unchecked.Store(!on) }

// check panics unless the caller is on the UI goroutine.
func (s *Shell) check(what string) {
	if s.unchecked.Load() {
		return
	}
	if !s.q.OnUIThread() {
		panic("fake: " + what + " called off the UI thread (or before Run or Start)")
	}
}

// checkReady panics unless the caller is on the UI goroutine and Ready has
// been called.
func (s *Shell) checkReady(what string) {
	s.check(what)
	if !s.ready {
		panic("fake: " + what + " called before Handler.Ready")
	}
}

// notPainting panics while a Paint is running.
func (s *Shell) notPainting(what string) {
	if s.painting {
		panic("fake: " + what + " called during SurfaceHandler.Paint or PopupHandler.Paint")
	}
}

// Start binds the calling goroutine as the UI goroutine and calls h.Ready,
// then returns, so the test can drive the shell (see Shell). It returns
// ErrRunTwice if Run or Start was called before. h must be non-nil.
func (s *Shell) Start(h shell.Handler) error {
	if h == nil {
		panic("fake: Start with a nil handler")
	}
	if !s.started.CompareAndSwap(false, true) {
		return ErrRunTwice
	}
	s.h = h
	s.q.Bind()
	s.q.SetWake(func() {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	})
	s.ready = true
	h.Ready(shell.Services{Text: s.text, Clock: s.clock})
	return nil
}

// Run implements shell.Shell: Start, then posted work runs until Quit.
// Work posted before Quit still runs once, and surfaces it opens are closed
// too; then Run returns nil. A panic in Ready or in posted work propagates
// out of Run; the queue is closed on the way out, so goroutines blocked in
// Queue().DoSync return false.
func (s *Shell) Run(h shell.Handler) error {
	if err := s.Start(h); err != nil {
		return err
	}
	defer s.q.Close()
	for {
		s.q.Drain()
		if s.quit {
			s.q.Drain()
			s.closeAll()
			return nil
		}
		<-s.wake
	}
}

// Step runs the posted work that is queued, without blocking, and returns
// how many functions ran (see mainthread.Queue.Drain: work posted by that
// work waits for the next Step). It does not run frames. It may be called
// after Quit, to run work posted before it.
func (s *Shell) Step() int {
	s.check("Shell.Step")
	return s.q.Drain()
}

// Flush runs the requested frame now (Clock.Flush): the frame function,
// then the paints of the invalidated visible surfaces. It reports whether a
// frame ran.
func (s *Shell) Flush() bool {
	s.check("Shell.Flush")
	return s.clock.Flush()
}

// Quit implements shell.Shell. As on the real shells it marks the shell
// quitting first (so NewSurface in a Closed handler returns ErrQuit),
// then closes every open surface (each gets Closed), drops the pending
// timers (After), closes the queue so later Post calls are dropped, and
// makes Run return once the current work is done. A second Quit does
// nothing.
func (s *Shell) Quit() {
	s.check("Shell.Quit")
	if s.quit {
		return
	}
	s.notPainting("Shell.Quit")
	s.quit = true
	s.closeAll()
	s.clock.dropTimers()
	s.q.Close()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Quitting reports whether Quit has been called.
func (s *Shell) Quitting() bool {
	s.check("Shell.Quitting")
	return s.quit
}

func (s *Shell) closeAll() {
	s.quitting = true
	defer func() { s.quitting = false }()
	for len(s.surfaces) > 0 {
		s.surfaces[len(s.surfaces)-1].Close()
	}
}

// Post implements shell.Shell. It is safe for concurrent use. Functions
// posted before Run or Start wait for it; after Quit they are dropped.
func (s *Shell) Post(fn func()) { s.q.Do(fn) }

// Queue returns the UI-thread queue behind Post, for Do and DoSync.
func (s *Shell) Queue() *mainthread.Queue { return s.q }

// NewSurface implements shell.Shell. The surface starts hidden and
// invalidated: its first paint happens at the first frame after Show. The
// size is clamped to the limits; the scale is the screen scale. A modal
// surface loses StyleResizable and StyleMiniaturizable, and is centered at
// its first Show (Centered counts that, not SurfaceConfig.Center). After
// Quit it returns ErrQuit.
func (s *Shell) NewSurface(cfg shell.SurfaceConfig, h shell.SurfaceHandler) (shell.Surface, error) {
	s.checkReady("Shell.NewSurface")
	s.notPainting("Shell.NewSurface")
	if h == nil {
		panic("fake: NewSurface with a nil handler")
	}
	if s.quit {
		return nil, ErrQuit
	}
	owner, err := s.owner(cfg)
	if err != nil {
		return nil, err
	}
	sf := &Surface{
		shell:   s,
		modal:   cfg.Modal,
		owner:   owner,
		h:       h,
		title:   cfg.Title,
		min:     cfg.MinSize,
		max:     cfg.MaxSize,
		style:   cfg.Style,
		scale:   s.screenScale,
		invalid: true,
	}
	sf.size = sf.clamp(cfg.Size)
	if cfg.Modal {
		sf.style &^= driver.StyleResizable | driver.StyleMiniaturizable
	} else if cfg.Center {
		sf.centers = 1
	}
	s.surfaces = append(s.surfaces, sf)
	return sf, nil
}

// Surfaces returns the open surfaces in creation order.
func (s *Shell) Surfaces() []*Surface {
	s.check("Shell.Surfaces")
	return append([]*Surface(nil), s.surfaces...)
}

// Key returns the key surface, or nil when none is key.
func (s *Shell) Key() *Surface {
	s.check("Shell.Key")
	return s.key
}

// Deactivate simulates the app losing the foreground: the key surface gets
// KeyChanged(false), and no surface is key.
func (s *Shell) Deactivate() {
	s.check("Shell.Deactivate")
	s.setKey(nil)
}

func (s *Shell) setKey(sf *Surface) {
	if m := s.activeModal(); sf != nil && m != nil {
		sf = m // the OS refuses a blocked surface's activation
	}
	old := s.key
	if old == sf {
		return
	}
	if old != nil {
		old.dismissGrab() // the parent of a grab popup stops being key
	}
	s.key = sf
	if old != nil && !old.dead {
		old.h.KeyChanged(false)
	}
	if sf != nil {
		sf.h.KeyChanged(true)
	}
}

// TextSystem returns the text system handed to Handler.Ready (a
// test accessor; shell.Shell has none).
func (s *Shell) TextSystem() render.TextSystem {
	s.checkReady("Shell.TextSystem")
	return s.text
}

// Appearance implements shell.Shell.
func (s *Shell) Appearance() driver.Appearance {
	s.checkReady("Shell.Appearance")
	return s.appearance
}

// SetAppearance simulates a change of the system appearance. Before Start
// it only sets the initial value; afterwards it must be called on the UI
// goroutine and calls Handler.AppearanceChanged when the value changes.
func (s *Shell) SetAppearance(a driver.Appearance) {
	if !s.started.Load() {
		s.appearance = a
		return
	}
	s.check("Shell.SetAppearance")
	if a == s.appearance {
		return
	}
	s.appearance = a
	if s.ready {
		s.h.AppearanceChanged(a)
	}
}

// SetScreen sets the screen work area (the size Zoom gives) and the scale
// of surfaces created afterwards. Before Start it may be called from any
// goroutine; afterwards only on the UI goroutine. Existing surfaces keep
// their scale (see Surface.SetScale).
func (s *Shell) SetScreen(size driver.Size, scale float64) {
	if s.started.Load() {
		s.check("Shell.SetScreen")
	}
	if !(scale > 0) {
		panic("fake: SetScreen with a scale that is not positive")
	}
	s.screen, s.screenScale = size, scale
}

// Clock returns the manual clock handed to Handler.Ready (a test
// accessor; shell.Shell has none). Its frames also paint the surfaces.
func (s *Shell) Clock() shell.FrameClock {
	s.checkReady("Shell.Clock")
	return s.clock
}

// FakeClock returns the clock with its test methods (Advance, Flush,
// Requested, Frames).
func (s *Shell) FakeClock() *Clock {
	s.check("Shell.FakeClock")
	return s.clock
}

// paintPass runs after the frame function of each frame: every invalidated,
// visible, not minimized surface is painted once, in creation order, then
// every invalidated popup of such a surface whose Placed is not pending
// (SetDeferPlaced).
func (s *Shell) paintPass(time.Duration) {
	for _, sf := range append([]*Surface(nil), s.surfaces...) {
		if !sf.dead && sf.invalid && sf.visible && !sf.minimized {
			sf.paint()
		}
	}
	for _, p := range append([]*Popup(nil), s.popups...) {
		if !p.dead && p.invalid && !p.placedDue && p.parent.visible && !p.parent.minimized {
			p.paint()
		}
	}
}

// timedPaint runs paint (a handler's Paint on c) with painting set, ends
// c and returns its contract errors; with a frame observer the two parts
// are timed (S6.2c: the paint, and the recorder's checks as the submit).
func (s *Shell) timedPaint(paint func(), c *record.Canvas) error {
	ft := &s.clock.frameTimer
	var start, painted time.Time
	if ft.Active() {
		start = time.Now()
	}
	s.painting = true
	func() {
		defer func() { s.painting = false }()
		paint()
	}()
	if ft.Active() {
		painted = time.Now()
	}
	err := c.End()
	if ft.Active() {
		ft.Painted(painted.Sub(start), time.Since(painted), 0)
	}
	s.clock.presented()
	return err
}

// offScreen reports whether the shell has open surfaces and none on
// screen: each is hidden or minimized (its popups go with it). The clock
// then runs requested frames at the low rate (HiddenInterval, S6.2b).
func (s *Shell) offScreen() bool {
	open := false
	for _, sf := range s.surfaces {
		if sf.dead {
			continue
		}
		if sf.visible && !sf.minimized {
			return false
		}
		open = true
	}
	return open
}
