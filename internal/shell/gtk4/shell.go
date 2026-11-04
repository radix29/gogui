package gtk4

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// ErrRunTwice is returned by Run when the shell has already been started.
var ErrRunTwice = errors.New("gtk4: Run called more than once")

// ErrQuit is returned by NewSurface after Quit.
var ErrQuit = errors.New("gtk4: NewSurface after Quit")

// loop is the native side of the shell's event loop: GTK and a GMainLoop
// on Linux (loop_linux.go), a fake in the host tests. Every method except
// idle runs on the GTK thread.
type loop interface {
	// init loads and initialises GTK, registers the trampolines and the
	// GGSurface class, and creates the main loop. Run calls it once,
	// before anything else; an error ends Run.
	init() error
	// run runs the main loop until quit is called from inside it.
	run()
	// quit makes a running run return. Outside run it does nothing (as
	// g_main_loop_quit before g_main_loop_run: the run would reset it).
	quit()
	// idle adds a one-shot idle source that calls p.source(k) through
	// sourceFunc (addIdle). It may be called from any goroutine. It returns
	// the source id and its handle, or 0, 0 if no source was added.
	idle(p sourcePeer, k kind) (uint32, ffi.Handle)
	// remove removes a source added by idle that has not run yet. The
	// caller frees its handle.
	remove(id uint32)
	// release frees the main loop. Run calls it on every way out after a
	// successful init.
	release()
	// newWindow creates a hidden, realized window for a surface
	// (window_linux.go), with p as the peer of its GGSurface and its
	// signals. On error nothing of it is left.
	newWindow(cfg windowConfig, p windowPeer) (window, error)
	// The frame clock's calls (clock.go).
	clockLoop
}

// openSurface is what the shell needs of an open surface to close it and
// to pace frames (the *surface, surface.go; probes in the tests).
type openSurface interface {
	// closeNow destroys the surface's window and removes it from
	// Shell.surfaces. Unless silent, its handler gets Closed.
	closeNow(silent bool)
	// tickWidget returns the widget whose frame clock can pace the shell's
	// frames (the surface is shown), or 0. A surface whose widget may pace
	// calls Shell.clock.surfacesChanged when that changes. A closing
	// surface first stops returning its widget (or leaves Shell.surfaces),
	// then calls Shell.clock.detach, then destroys the widget; detached in
	// the other order, the clock would put a new tick callback on it.
	tickWidget() uintptr
}

// Shell is the Linux shell.Shell. Create one with New.
type Shell struct {
	q       *mainthread.Queue
	started atomic.Bool
	h       shell.Handler
	loop    loop

	ready   bool // Handler.Ready has been called
	quit    bool
	running bool // inside loop.run

	// text is the text system (text.go); Pango opens with the first
	// layout and is released at the end of Run.
	text  *textSystem
	clock *clock
	// openSettings opens the appearance source in Run (portal.go;
	// openNoSettings unless New or a test sets it). settings is the open
	// source, nil outside Run; appearance is the cached reading of it.
	openSettings openSource
	settings     appearanceSource
	appearance   driver.Appearance

	// images is the shell's images (image.go); their textures go with the
	// last surface and at the end of Run.
	images imageSet

	surfaces []openSurface // the open surfaces, in creation order
	painting *surface      // the surface whose Paint runs, or nil
	// inLayout is above 0 while a Resized handler runs inside GTK's
	// size_allocate: native size changes, Close and Quit are posted.
	inLayout int

	// events keeps the input events' times from going backwards
	// (eventTime, input.go).
	events shellkit.Monotonic

	// The wake (wake.go): the idle sources posted and not yet run.
	wakeMu      sync.Mutex
	wakeClosed  bool
	wakePending map[*wakeSource]struct{}

	// wakes counts the idle sources the wake added, drains the ones that
	// ran (tests).
	wakes, drains atomic.Int64
}

var _ shell.Shell = (*Shell)(nil)

func newShell(l loop) *Shell {
	s := &Shell{
		q: mainthread.New(), loop: l, wakePending: map[*wakeSource]struct{}{},
		openSettings: openNoSettings, appearance: driver.DefaultAppearance(),
	}
	s.text = newTextSystem(s.q)
	s.clock = newClock(s, l)
	s.clock.paint = s.paintInvalid
	return s
}

// Queue returns the UI-thread queue behind Post. Its checks (SetChecks)
// also cover the shell's UI-thread methods.
func (s *Shell) Queue() *mainthread.Queue { return s.q }

func (s *Shell) checkReady(what string) {
	s.q.Check("gtk4.Shell." + what)
	if !s.ready {
		panic("gtk4: Shell." + what + " called before Handler.Ready")
	}
}

// Run implements shell.Shell. It locks the calling goroutine to its OS
// thread for the duration, which becomes the GTK thread: it loads GTK
// (a missing or too old GTK is reported to the user with gtk.Report and
// returned), calls gtk_init_check, registers the GGSurface class, creates
// a GMainLoop, opens the appearance source and reads the appearance
// (bounded, so a hung session bus cannot block start-up), starts the frame
// clock, installs the wake, calls h.Ready
// and runs the loop until Quit. Work posted before Quit still runs once on
// the way out. A panic in Ready, in posted work or in Go code called from
// GTK ends Run with that panic (a *PanicError for the callback cases)
// after the surfaces are closed, the frame clock's and the wake's pending
// sources removed and the main loop freed; the queue is closed, so
// goroutines in DoSync return false.
func (s *Shell) Run(h shell.Handler) error {
	if h == nil {
		panic("gtk4: Run with a nil handler")
	}
	if !s.started.CompareAndSwap(false, true) {
		return ErrRunTwice
	}
	called := time.Now() // Now's 0 (clock.begin)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s.h = h
	s.q.Bind()
	defer s.q.Close()

	if err := s.loop.init(); err != nil {
		return err
	}
	defer s.shutdown()
	onPanic = s.loop.quit // a trapped panic ends the loop, so Run re-raises it
	// The source reports its changes on its own goroutine: re-read on the
	// UI thread. A change posted before Ready runs after it (and dedupes
	// against the read here); one posted after Quit is dropped.
	s.settings = s.openSettings(func() { s.Post(s.appearanceChanged) })
	s.appearance = readAppearance(s.settings)
	s.clock.begin(time.Since(called))

	s.q.SetWake(s.wake)
	s.ready = true
	h.Ready()
	rethrow()    // a callback run by a GTK call made in Ready
	if !s.quit { // Quit in Ready: g_main_loop_quit before run would be lost
		s.running = true
		s.loop.run()
		s.running = false
		rethrow()
	}
	s.q.Drain() // work posted before Quit runs once
	return nil
}

// shutdown closes the surfaces a panic left open (without Closed), stops
// the appearance source, the frame clock and the wake, removes their
// pending sources and frees the main loop. It runs on every way out of Run
// after init, panics included.
func (s *Shell) shutdown() {
	s.quit = true
	if s.settings != nil {
		s.settings.close() // no change is reported after it
		s.settings = nil
	}
	if len(s.surfaces) > 0 { // left open by a panic
		s.closeAll(true)
	}
	s.images.release()
	s.text.release()
	s.clock.close()
	s.q.Close()
	s.q.SetWake(nil)
	s.stopWakes()
	s.loop.release()
	onPanic = nil
	pendingPanic = nil // a panic during the teardown has nowhere to go
}

// Quit implements shell.Shell. Every open surface is closed, last to first
// (Closed, no CloseRequested), and Run returns once the current callback
// has returned; later Post calls are dropped. Calling it again does
// nothing. During a GTK layout (a Resized handler) it is posted instead,
// so no window is destroyed while GTK allocates it.
func (s *Shell) Quit() {
	s.q.Check("gtk4.Shell.Quit")
	if s.quit {
		return
	}
	s.notPainting("Shell.Quit")
	if s.inLayout > 0 {
		s.Post(s.Quit)
		return
	}
	s.quit = true // first, so NewSurface in a Closed handler fails
	s.closeAll(false)
	s.q.Close()
	if s.running {
		s.loop.quit()
	}
}

// closeAll closes the open surfaces, last to first. A surface that does
// not remove itself is dropped all the same. Unless silent, a panic a
// Closed handler left pending is re-raised after its surface.
func (s *Shell) closeAll(silent bool) {
	for len(s.surfaces) > 0 {
		n := len(s.surfaces)
		sf := s.surfaces[n-1]
		sf.closeNow(silent)
		if n := len(s.surfaces); n > 0 && s.surfaces[n-1] == sf {
			s.surfaces = s.surfaces[:n-1]
		}
		if !silent {
			rethrow()
		}
	}
}

// Post implements shell.Shell. It is safe for concurrent use. Work posted
// before Run waits for it; after Quit it is dropped.
func (s *Shell) Post(fn func()) { s.q.Do(fn) }

// TextSystem implements shell.Shell: Pango (text.go), fixed-width metrics
// without it.
func (s *Shell) TextSystem() render.TextSystem {
	s.checkReady("TextSystem")
	return s.text
}

// Appearance implements shell.Shell: the cached reading of the settings
// portal and the GtkSettings (portal.go's readAppearance), read before
// Ready and kept current by appearanceChanged.
func (s *Shell) Appearance() driver.Appearance {
	s.checkReady("Appearance")
	return s.appearance
}

// appearanceChanged re-reads the appearance and reports a change to the
// handler (Win32's shape): a reading equal to the cache does nothing, and
// nothing is reported before Ready or after Quit. It runs on the UI
// thread; the source's change reports reach it through Post.
func (s *Shell) appearanceChanged() {
	if s.settings == nil { // after shutdown
		return
	}
	a := readAppearance(s.settings)
	if a == s.appearance {
		return
	}
	s.appearance = a
	if s.ready && !s.quit {
		s.h.AppearanceChanged(a)
	}
}

// Clock implements shell.Shell: the frame clock over GdkFrameClock
// (clock.go).
func (s *Shell) Clock() shell.FrameClock {
	s.checkReady("Clock")
	return s.clock
}

// pacingWidget returns the widget whose frame clock paces the frames: the
// first open surface's, in creation order, that has one; 0 for none.
func (s *Shell) pacingWidget() uintptr {
	for _, sf := range s.surfaces {
		if w := sf.tickWidget(); w != 0 {
			return w
		}
	}
	return 0
}
