//go:build windows

package win32

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/mainthread"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/com"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// ErrRunTwice is returned by Run when the shell has already been started.
var ErrRunTwice = errors.New("win32: Run called more than once")

// ErrQuit is returned by NewSurface after Quit.
var ErrQuit = errors.New("win32: NewSurface after Quit")

// The shell's private messages to its listener window.
const (
	msgWake  = w32.WM_APP + 1 // drain the queue
	msgFrame = w32.WM_APP + 2 // run the requested frame (wParam frameModal: from a modal loop)
)

// frameModal is msgFrame's wParam when posted for a modal loop (clock.go).
const frameModal = 1

// Shell is the Windows shell.Shell. Create one with New.
type Shell struct {
	q       *mainthread.Queue
	started atomic.Bool
	h       shell.Handler

	listener windows.HWND // hidden window: wake, frames, broadcasts
	ready    bool         // Handler.Ready has been called
	quit     bool

	sys        sysReader
	appearance driver.Appearance
	clock      *clock
	text       *textSystem

	// Event times (input.go): the message-time and tick sources (tests
	// replace them) and the last time handed out.
	msgTime, tick func() uint32
	events        shellkit.Monotonic

	surfaces    []*surface // the open surfaces, in creation order
	painting    *surface   // the surface whose Paint runs, or nil
	silentClose bool       // closeAll without Closed (the teardown)

	// wakes counts the wake messages posted, drains the ones handled
	// (tests).
	wakes, drains atomic.Int64

	// The GPU (gpu.go): the device objects while a surface is open, how
	// often they were created and lost, the references left over at a
	// release, and the test hooks (an injected failure, a call between a
	// frame's EndDraw and its Present).
	gpu     *gpu
	warp    bool // tests: create the devices on WARP (newGPU; the golden renders)
	gpuGen  int
	losses  int
	leaks   []string
	fault   fault
	onDrawn func(sf *surface)

	// images are the shell's images (image.go); their bitmaps go with the
	// device objects.
	images imageSet

	// debug sends every Paint to a recorder too and keeps its contract
	// errors in the surface's paintErr (GOGUI_DEBUG=1; tests set it).
	debug bool
}

var _ shell.Shell = (*Shell)(nil)

// New returns a shell that has not been started. Its clock's timeline
// starts now.
func New() *Shell { return newShell(liveSys{}) }

func newShell(sys sysReader) *Shell {
	s := &Shell{q: mainthread.New(), sys: sys, debug: debugChecks,
		msgTime: liveMsgTime, tick: w32.GetTickCount}
	s.text = newTextSystem(s.q)
	s.clock = newClock(s)
	s.clock.paint = s.paintInvalid
	return s
}

// Queue returns the UI-thread queue behind Post, for Do and DoSync. Its
// checks (SetChecks) also cover the shell's UI-thread methods.
func (s *Shell) Queue() *mainthread.Queue { return s.q }

func (s *Shell) checkReady(what string) {
	s.q.Check("win32.Shell." + what)
	if !s.ready {
		panic("win32: Shell." + what + " called before Handler.Ready")
	}
}

// Run implements shell.Shell. It locks the calling goroutine to its OS
// thread for the duration, makes the thread a COM single-threaded
// apartment (com.InitUIThread, S2.4a), sets the process's DPI awareness to
// Per-Monitor-V2, creates the listener window, calls h.Ready and runs the
// message loop until Quit. Work posted before Quit still runs once on the
// way out. A panic in Ready, in posted work, in the frame function or in a
// handler called from the window procedure ends Run with that panic (a
// *PanicError for the window-procedure cases) after the listener is
// destroyed and the queue closed, so goroutines in DoSync return false.
func (s *Shell) Run(h shell.Handler) (err error) {
	if h == nil {
		panic("win32: Run with a nil handler")
	}
	if !s.started.CompareAndSwap(false, true) {
		return ErrRunTwice
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// The UI thread is a single-threaded COM apartment (S2.4a); CoUninitialize
	// runs before the thread is unlocked.
	uninit, err := com.InitUIThread()
	if err != nil {
		return fmt.Errorf("win32: initializing COM on the UI thread: %w", err)
	}
	defer uninit()
	s.h = h
	s.q.Bind()
	defer s.q.Close()

	if err := setDPIAwareness(); err != nil {
		return err
	}
	inst, err := registerClass()
	if err != nil {
		return fmt.Errorf("win32: registering the window class: %w", err)
	}
	handle := targets.New(listenerTarget{s})
	s.listener, err = w32.CreateWindowEx(w32.WS_EX_TOOLWINDOW|w32.WS_EX_NOACTIVATE,
		className, "gogui", w32.WS_POPUP, 0, 0, 0, 0, 0, 0, inst, uintptr(handle))
	rethrow()
	if err != nil {
		targets.Free(handle)
		return fmt.Errorf("win32: creating the listener window: %w", err)
	}
	defer s.shutdown()

	s.appearance = readAppearance(s.sys)
	listener := s.listener
	s.q.SetWake(func() {
		s.wakes.Add(1)
		// Fails only once the listener is gone, after Run.
		_ = w32.PostMessage(listener, msgWake, 0, 0)
	})
	s.clock.startTimer()
	s.ready = true
	h.Ready()

	// One message per round, after the clock's wait (clock.go): that runs
	// a due frame first, or waits for a frame, a message or both.
	var m w32.Msg
	for !s.quit {
		if err := s.wait(); err != nil {
			return err
		}
		if s.quit {
			break
		}
		ok := w32.PeekMessage(&m, 0, 0, 0, w32.PM_REMOVE) // also delivers sent messages
		rethrow()
		if !ok {
			continue
		}
		if m.Message == w32.WM_QUIT { // from someone else's PostQuitMessage
			s.Quit()
			break
		}
		w32.TranslateMessage(&m)
		w32.DispatchMessage(&m)
		rethrow()
	}
	s.q.Drain() // work posted before Quit runs once
	return nil
}

// shutdown destroys the surfaces a panic left open (without Closed) and the listener (its handle is freed in WM_NCDESTROY) and
// stops the wake. It runs on every way out of Run, panics included.
func (s *Shell) shutdown() {
	s.quit = true
	if len(s.surfaces) > 0 { // left open by a panic
		s.closeAll(true)
	}
	s.releaseGPU() // closeAll left nothing; this is the safety net
	s.text.release(s.leak)
	s.clock.close()
	s.q.Close()
	s.q.SetWake(nil)
	if s.listener != 0 {
		_ = w32.DestroyWindow(s.listener)
		s.listener = 0
	}
	pendingPanic = nil // a panic during the teardown has nowhere to go
}

// setDPIAwareness makes the process Per-Monitor-V2 DPI aware. A second
// call in the same process fails with ERROR_ACCESS_DENIED, which is fine as
// long as the awareness already is Per-Monitor-V2 (a second shell in a
// test binary, or a manifest that asks for it).
func setDPIAwareness() error {
	err := w32.SetProcessDpiAwarenessContext(w32.DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2)
	if err != nil && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return fmt.Errorf("win32: %w", err)
	}
	if !w32.AreDpiAwarenessContextsEqual(w32.GetThreadDpiAwarenessContext(),
		w32.DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2) {
		return errors.New("win32: the process DPI awareness is set to something other " +
			"than Per-Monitor-V2 (by the application manifest?)")
	}
	return nil
}

// Quit implements shell.Shell. Every open surface is closed (Closed, no
// CloseRequested), and Run returns once the current message has been
// handled; later Post calls are dropped. Calling it again does nothing.
func (s *Shell) Quit() {
	s.q.Check("win32.Shell.Quit")
	if s.quit {
		return
	}
	s.notPainting("Shell.Quit")
	s.quit = true // first, so NewSurface in a Closed handler fails
	s.closeAll(false)
	s.q.Close()
}

// Post implements shell.Shell. It is safe for concurrent use. Work posted
// before Run waits for it; after Quit it is dropped.
func (s *Shell) Post(fn func()) { s.q.Do(fn) }

// TextSystem implements shell.Shell: the DirectWrite text system (text.go),
// created with the first layout and released by Run.
func (s *Shell) TextSystem() render.TextSystem {
	s.checkReady("TextSystem")
	return s.text
}

// Appearance implements shell.Shell.
func (s *Shell) Appearance() driver.Appearance {
	s.checkReady("Appearance")
	return s.appearance
}

// Clock implements shell.Shell.
func (s *Shell) Clock() shell.FrameClock {
	s.checkReady("Clock")
	return s.clock
}

// appearanceChanged re-reads the appearance and reports a change.
func (s *Shell) appearanceChanged() {
	a := readAppearance(s.sys)
	if a == s.appearance {
		return
	}
	s.appearance = a
	for _, sf := range s.surfaces {
		_, _, _ = sf.applyFrame() // the dark frame and the corners follow
		sf.Invalidate()           // and the background the shell clears to
	}
	if s.ready && !s.quit {
		s.h.AppearanceChanged(a)
	}
}

// listenerTarget is the shell's hidden window: the wake, the frame
// messages and timer, and the broadcasts that change the appearance.
type listenerTarget struct{ s *Shell }

func (l listenerTarget) handle(_ windows.HWND, msg uint32, wp, _ uintptr) (uintptr, bool) {
	s := l.s
	switch msg {
	case msgWake:
		s.drains.Add(1)
		s.q.Drain()
		return 0, true
	case msgFrame:
		if wp == frameModal {
			s.clock.modalFrame()
		} else {
			s.clock.frame()
		}
		return 0, true
	case w32.WM_TIMER:
		if wp == timerFrame {
			s.clock.modalFrame()
			return 0, true
		}
	case w32.WM_SETTINGCHANGE, w32.WM_DWMCOLORIZATIONCOLORCHANGED, w32.WM_THEMECHANGED:
		s.appearanceChanged()
		return 0, true
	}
	return 0, false
}

var _ target = listenerTarget{}

// handleOf returns the handle stored on h (tests).
func handleOf(h windows.HWND) ffi.Handle {
	v, _ := w32.GetWindowLongPtr(h, w32.GWLP_USERDATA)
	return ffi.Handle(v)
}
