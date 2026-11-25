//go:build windows

package win32

import (
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// handler is a shell.Handler made of funcs; nil funcs do nothing.
type handler struct {
	services   shell.Services // what Ready got
	ready      func()
	appearance func(a driver.Appearance)
}

func (h *handler) Ready(sv shell.Services) {
	h.services = sv
	if h.ready != nil {
		h.ready()
	}
}

func (h *handler) AppearanceChanged(a driver.Appearance) {
	if h.appearance != nil {
		h.appearance(a)
	}
}

// run runs s with h on the calling goroutine (Run locks it to its thread)
// and fails the test if Run does not return within 10 s.
func run(t *testing.T, s *Shell, h shell.Handler) error {
	t.Helper()
	done := make(chan struct{})
	timer := time.AfterFunc(10*time.Second, func() {
		select {
		case <-done:
		default:
			buf := make([]byte, 1<<20)
			buf = buf[:runtime.Stack(buf, true)]
			s.Post(s.Quit)
			t.Errorf("Run did not return within 10 s; posted Quit. Goroutines:\n%s", buf)
		}
	})
	defer timer.Stop()
	defer close(done)
	return s.Run(h)
}

func newTestShell() *Shell {
	s := New()
	s.q.SetChecks(true)
	s.debug = true // the recorder checks every Paint
	return s
}

func TestRunReadyQuit(t *testing.T) {
	s := newTestShell()
	before := targets.Len()
	mustPanic(t, "Appearance before Run", func() { s.Appearance() })

	readies := 0
	h := &handler{}
	h.ready = func() {
		readies++
		if !s.q.OnUIThread() {
			t.Error("Ready off the UI thread")
		}
		if s.listener == 0 || !targets.Valid(handleOf(s.listener)) {
			t.Error("no listener window with a valid handle in Ready")
		}
		if targets.Len() != before+1 {
			t.Errorf("targets.Len() = %d in Ready, want %d", targets.Len(), before+1)
		}
		if s.TextSystem() == nil || s.TextSystem() != s.TextSystem() {
			t.Error("TextSystem nil or not the same value on every call")
		}
		mustPanic(t, "NewSurface with a nil handler", func() { _, _ = s.NewSurface(shell.SurfaceConfig{}, nil) })
		if s.Clock() != s.Clock() {
			t.Error("Clock not the same value on every call")
		}
		if h.services.Text != s.TextSystem() || h.services.Clock != s.Clock() || h.services.Clock == nil {
			t.Errorf("Ready got %+v, want the text system and the clock", h.services)
		}
		if !s.Caps().PopupShadow {
			t.Error("Caps without PopupShadow")
		}
		if a := s.Appearance(); a.Accent.A != 1 || a.Accent.Semantic != 0 {
			t.Errorf("Appearance().Accent = %+v, want an opaque concrete color", a.Accent)
		}
		if !w32.AreDpiAwarenessContextsEqual(w32.GetThreadDpiAwarenessContext(),
			w32.DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2) {
			t.Error("thread is not Per-Monitor-V2 DPI aware in Ready")
		}
		// The UI thread is an STA (S2.4a): asking for an MTA fails with
		// RPC_E_CHANGED_MODE and leaves nothing to undo.
		if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err == nil {
			windows.CoUninitialize()
			t.Error("UI thread accepted an MTA: no STA in Ready")
		} else if e, ok := err.(windows.Errno); !ok || uint32(e) != 0x80010106 {
			t.Errorf("CoInitializeEx(MTA) in Ready = %v, want RPC_E_CHANGED_MODE", err)
		}
		s.Post(func() {
			s.Quit()
			s.Quit() // a second Quit does nothing
		})
	}
	if err := run(t, s, h); err != nil {
		t.Fatalf("Run = %v", err)
	}
	if readies != 1 {
		t.Errorf("Ready called %d times", readies)
	}
	if targets.Len() != before {
		t.Errorf("targets.Len() = %d after Run, want %d (listener handle freed)", targets.Len(), before)
	}
	if s.listener != 0 {
		t.Error("listener not cleared after Run")
	}
	if err := s.Run(&handler{}); !errors.Is(err, ErrRunTwice) {
		t.Errorf("second Run = %v, want ErrRunTwice", err)
	}
	// Post after Quit is dropped.
	ran := false
	s.Post(func() { ran = true })
	if ran || s.q.Drain() != 0 {
		t.Error("work posted after Quit ran")
	}
}

// TestRunTwiceProcess runs a second shell in the same process: the class
// and the wndproc callback are registered once, and the DPI awareness that
// is already set is accepted.
func TestRunTwiceProcess(t *testing.T) {
	for i := range 2 {
		s := newTestShell()
		if err := run(t, s, &handler{ready: func() { s.Quit() }}); err != nil {
			t.Fatalf("Run %d = %v", i, err)
		}
	}
}

func TestQuitInReady(t *testing.T) {
	s := newTestShell()
	ran := 0
	err := run(t, s, &handler{ready: func() {
		s.Post(func() { ran++ })
		s.Quit()
		s.Post(func() { ran += 10 }) // dropped
	}})
	if err != nil || ran != 1 {
		t.Fatalf("Run = %v, ran = %d; want nil and the work posted before Quit run once", err, ran)
	}
}

func TestPostBeforeRun(t *testing.T) {
	s := newTestShell()
	var order []string
	s.Post(func() { order = append(order, "posted") })
	err := run(t, s, &handler{ready: func() {
		order = append(order, "ready")
		s.Post(s.Quit)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "ready" || order[1] != "posted" {
		t.Errorf("order = %v, want [ready posted]", order)
	}
}

// TestPostFIFO posts from 8 goroutines: every goroutine's functions run in
// order, all of them run, and the wakes are coalesced (one posted message
// per drain, never more wakes than drains plus one).
func TestPostFIFO(t *testing.T) {
	const goroutines, per = 8, 1000
	s := newTestShell()
	next := make([]int, goroutines)
	total := 0
	err := run(t, s, &handler{ready: func() {
		var wg sync.WaitGroup
		for g := range goroutines {
			wg.Go(func() {
				for i := range per {
					s.Post(func() {
						if !s.q.OnUIThread() {
							t.Error("posted work off the UI thread")
						}
						if next[g] != i {
							t.Errorf("goroutine %d: got %d, want %d", g, i, next[g])
						}
						next[g] = i + 1
						total++
					})
				}
			})
		}
		go func() {
			wg.Wait()
			s.Post(s.Quit)
		}()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if total != goroutines*per {
		t.Errorf("%d functions ran, want %d", total, goroutines*per)
	}
	wakes, drains := s.stats.wakes.Load(), s.stats.drains.Load()
	if wakes == 0 || wakes > drains+1 || wakes >= goroutines*per {
		t.Errorf("%d wakes posted for %d drains and %d functions; want coalesced wakes",
			wakes, drains, goroutines*per)
	}
	t.Logf("%d functions, %d wake messages, %d drains", total, wakes, drains)
}

// TestWakeCoalescedOnUIThread posts 1000 functions from one posted
// function: exactly one wake message is posted for all of them.
func TestWakeCoalescedOnUIThread(t *testing.T) {
	s := newTestShell()
	ran := 0
	var before, after int64
	err := run(t, s, &handler{ready: func() {
		s.Post(func() {
			before = s.stats.wakes.Load()
			for range 1000 {
				s.Post(func() { ran++ })
			}
			after = s.stats.wakes.Load()
			s.Post(s.Quit)
		})
	}})
	if err != nil {
		t.Fatal(err)
	}
	if after-before != 1 {
		t.Errorf("1000 posts from the UI thread posted %d wake messages, want 1", after-before)
	}
	if ran != 1000 {
		t.Errorf("%d of 1000 functions ran", ran)
	}
}

// TestPanicInPostedWork: a panic in posted work (inside the window
// procedure) ends Run with a *PanicError carrying the value, after the
// listener is gone and the queue is closed.
func TestPanicInPostedWork(t *testing.T) {
	s := newTestShell()
	before := targets.Len()
	boom := errors.New("boom")
	syncDone := make(chan bool, 1)
	var got any
	func() {
		defer func() { got = recover() }()
		_ = run(t, s, &handler{ready: func() {
			go func() {
				// Waits for a function that never runs: the queue closes.
				time.Sleep(20 * time.Millisecond)
				syncDone <- s.q.DoSync(func() {})
			}()
			s.Post(func() {
				time.Sleep(50 * time.Millisecond)
				panic(boom)
			})
		}})
	}()
	pe, ok := got.(*PanicError)
	if !ok {
		t.Fatalf("Run panicked with %T %v, want *PanicError", got, got)
	}
	if !errors.Is(pe, boom) || pe.Msg != msgWake || len(pe.Stack) == 0 {
		t.Errorf("PanicError = %v (msg %#x), want boom in msgWake with a stack", pe.Value, pe.Msg)
	}
	if targets.Len() != before {
		t.Errorf("targets.Len() = %d after the panic, want %d", targets.Len(), before)
	}
	if !s.q.Closed() {
		t.Error("queue not closed after the panic")
	}
	select {
	case ok := <-syncDone:
		if ok {
			t.Error("DoSync reported success after the panic")
		}
	case <-time.After(5 * time.Second):
		t.Error("DoSync still blocked after Run ended")
	}
	if pendingPanic != nil {
		t.Error("pendingPanic not cleared")
	}
}

// TestPanicInReady: a panic in Ready propagates out of Run as is (Ready is
// not called from the window procedure).
func TestPanicInReady(t *testing.T) {
	s := newTestShell()
	before := targets.Len()
	var got any
	func() {
		defer func() { got = recover() }()
		_ = run(t, s, &handler{ready: func() { panic("in ready") }})
	}()
	if got != "in ready" {
		t.Fatalf("Run panicked with %v, want \"in ready\"", got)
	}
	if targets.Len() != before || !s.q.Closed() {
		t.Errorf("after the panic: targets.Len() %d (want %d), queue closed %v", targets.Len(), before, s.q.Closed())
	}
}

// probe is a test window target that counts its messages.
type probe struct {
	msgs map[uint32]int
	fn   func(h windows.HWND, msg uint32, wp, lp uintptr) (uintptr, bool)
}

func (p *probe) handle(h windows.HWND, msg uint32, wp, lp uintptr) (uintptr, bool) {
	p.msgs[msg]++
	if p.fn != nil {
		return p.fn(h, msg, wp, lp)
	}
	return 0, false
}

// newProbeWindow creates a hidden window of the package's class for target
// handle hd (0: none) on the calling thread, which must be locked.
func newProbeWindow(t *testing.T, hd ffi.Handle) windows.HWND {
	t.Helper()
	inst, err := registerClass()
	if err != nil {
		t.Fatal(err)
	}
	h, err := w32.CreateWindowEx(w32.WS_EX_TOOLWINDOW|w32.WS_EX_NOACTIVATE, className, "probe",
		w32.WS_POPUP, 0, 0, 100, 50, 0, 0, inst, uintptr(hd))
	rethrow()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestWndprocDispatch: a window with a live handle gets its messages from
// WM_NCCREATE to WM_NCDESTROY, after which the handle is freed; a window
// whose handle is stale or 0 is left to DefWindowProcW.
func TestWndprocDispatch(t *testing.T) {
	s := newTestShell()
	err := run(t, s, &handler{ready: func() {
		defer s.Quit()
		p := &probe{msgs: map[uint32]int{}}
		hd := targets.New(p)
		h := newProbeWindow(t, hd)
		if handleOf(h) != hd {
			t.Errorf("GWLP_USERDATA = %v, want %v", handleOf(h), hd)
		}
		if p.msgs[w32.WM_NCCREATE] != 1 || p.msgs[w32.WM_CREATE] != 1 {
			t.Errorf("creation messages %v", p.msgs)
		}
		const custom = w32.WM_APP + 77
		p.fn = func(_ windows.HWND, msg uint32, wp, _ uintptr) (uintptr, bool) {
			if msg == custom {
				return wp * 2, true
			}
			return 0, false
		}
		if r := w32.SendMessage(h, custom, 21, 0); r != 42 {
			t.Errorf("SendMessage(custom) = %d, want 42", r)
		}
		if err := w32.DestroyWindow(h); err != nil {
			t.Fatal(err)
		}
		if p.msgs[w32.WM_DESTROY] != 1 || p.msgs[w32.WM_NCDESTROY] != 1 {
			t.Errorf("destruction messages %v", p.msgs)
		}
		if targets.Valid(hd) {
			t.Error("handle still valid after WM_NCDESTROY")
		}

		// A stale handle: the window is created (DefWindowProcW answers
		// WM_NCCREATE) and its messages never reach the old target.
		stale := &probe{msgs: map[uint32]int{}}
		sh := targets.New(stale)
		targets.Free(sh)
		for _, hd := range []ffi.Handle{sh, 0} {
			h := newProbeWindow(t, hd)
			if r := w32.SendMessage(h, custom, 21, 0); r != 0 {
				t.Errorf("handle %v: SendMessage(custom) = %d, want DefWindowProcW's 0", hd, r)
			}
			if err := w32.DestroyWindow(h); err != nil {
				t.Error(err)
			}
		}
		if len(stale.msgs) != 0 {
			t.Errorf("stale target got messages %v", stale.msgs)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWndprocPanic: a panic in a target during SendMessage is recovered in
// the window procedure (the message returns 0) and re-raised by rethrow.
func TestWndprocPanic(t *testing.T) {
	s := newTestShell()
	err := run(t, s, &handler{ready: func() {
		defer s.Quit()
		const custom = w32.WM_APP + 78
		p := &probe{msgs: map[uint32]int{}, fn: func(_ windows.HWND, msg uint32, _, _ uintptr) (uintptr, bool) {
			if msg == custom {
				panic("in target")
			}
			return 0, false
		}}
		h := newProbeWindow(t, targets.New(p))
		defer w32.DestroyWindow(h)
		if r := w32.SendMessage(h, custom, 0, 0); r != 0 {
			t.Errorf("SendMessage = %d after the panic, want 0", r)
		}
		mustPanicWith(t, "rethrow", func(v any) bool {
			pe, ok := v.(*PanicError)
			return ok && pe.Value == "in target" && pe.Msg == custom
		}, rethrow)
		rethrow() // nothing pending any more
	}})
	if err != nil {
		t.Fatal(err)
	}
}

// fakeSys is a sysReader with injected values.
type fakeSys struct {
	reg       map[string]uint32 // "path|name"
	color     uint32
	colorOK   bool
	hc        bool
	anim      bool
	animOK    bool
	blink     uint32 // GetCaretBlinkTime
	blinkOK   bool
	timeout   uint32 // SPI_GETCARETTIMEOUT
	timeoutOK bool
	readCount int
}

func (f *fakeSys) regDWORD(path, name string) (uint32, bool) {
	f.readCount++
	v, ok := f.reg[path+"|"+name]
	return v, ok
}
func (f *fakeSys) colorization() (uint32, bool)      { return f.color, f.colorOK }
func (f *fakeSys) highContrast() bool                { return f.hc }
func (f *fakeSys) clientAreaAnimation() (bool, bool) { return f.anim, f.animOK }
func (f *fakeSys) caretBlinkTime() (uint32, bool)    { return f.blink, f.blinkOK }
func (f *fakeSys) caretTimeout() (uint32, bool)      { return f.timeout, f.timeoutOK }

func TestReadAppearance(t *testing.T) {
	p, d, a := regPersonalize+"|", regDWM+"|", regAccessibility+"|"
	def, blink := driver.DefaultAppearance(), driver.DefaultCaretBlink
	tests := []struct {
		name string
		sys  fakeSys
		want driver.Appearance
	}{
		{"nothing readable", fakeSys{}, def},
		{"light, animations on", fakeSys{
			reg:  map[string]uint32{p + "AppsUseLightTheme": 1, p + "EnableTransparency": 1, a + "DynamicScrollbars": 1},
			anim: true, animOK: true,
		}, def},
		{"dark, everything reduced", fakeSys{
			reg: map[string]uint32{p + "AppsUseLightTheme": 0, p + "EnableTransparency": 0,
				a + "DynamicScrollbars": 0},
			hc: true, anim: false, animOK: true,
		}, driver.Appearance{Dark: true, Accent: driver.DefaultAccent, IncreaseContrast: true,
			ReduceMotion: true, ReduceTransparency: true, LegacyScrollers: true, CaretBlink: blink}},
		{"AccentColor is ABGR", fakeSys{
			reg:     map[string]uint32{d + "AccentColor": 0xFF_D7_78_00}, // the Windows default blue 0078D7
			color:   0xC4_11_22_33,
			colorOK: true,
		}, driver.Appearance{Accent: driver.RGB8(0x00, 0x78, 0xD7), CaretBlink: blink}},
		{"colorization is ARGB", fakeSys{color: 0x80_11_22_33, colorOK: true},
			driver.Appearance{Accent: driver.RGB8(0x11, 0x22, 0x33), CaretBlink: blink}},
		{"animations unreadable: motion on", fakeSys{anim: false, animOK: false},
			def},
	}
	for _, tt := range tests {
		if got := readAppearance(&tt.sys); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestReadAppearanceLive(t *testing.T) {
	a := readAppearance(liveSys{})
	if a.Accent.A != 1 || a.Accent.Semantic != 0 {
		t.Errorf("live accent %+v is not an opaque concrete color", a.Accent)
	}
	t.Logf("live appearance %+v", a)
}

func TestReadCaretBlink(t *testing.T) {
	const ms = time.Millisecond
	def := driver.DefaultCaretBlink
	tests := []struct {
		name string
		sys  fakeSys
		want driver.CaretBlink
	}{
		{"nothing readable", fakeSys{}, def},
		{"the Windows defaults", fakeSys{blink: 530, blinkOK: true, timeout: 5000, timeoutOK: true},
			driver.CaretBlink{Period: 1060 * ms, Timeout: 5 * time.Second}},
		{"INFINITE blink time: no blink", fakeSys{blink: w32.INFINITE, blinkOK: true, timeout: 5000, timeoutOK: true},
			driver.CaretBlink{Timeout: 5 * time.Second}},
		{"timeout 0: never stops", fakeSys{blink: 200, blinkOK: true, timeout: 0, timeoutOK: true},
			driver.CaretBlink{Period: 400 * ms}},
		{"timeout INFINITE: never stops", fakeSys{blink: 200, blinkOK: true, timeout: w32.INFINITE, timeoutOK: true},
			driver.CaretBlink{Period: 400 * ms}},
		{"timeout unreadable: the default", fakeSys{blink: 250, blinkOK: true},
			driver.CaretBlink{Period: 500 * ms, Timeout: def.Timeout}},
		{"blink time unreadable: the default", fakeSys{timeout: 3000, timeoutOK: true},
			driver.CaretBlink{Period: def.Period, Timeout: 3 * time.Second}},
	}
	for _, tt := range tests {
		if got := readCaretBlink(&tt.sys); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
		if got := readAppearance(&tt.sys).CaretBlink; got != tt.want {
			t.Errorf("%s: readAppearance's CaretBlink %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// TestReadCaretBlinkLive reads the host's caret settings through the real
// APIs and checks them against GetCaretBlinkTime and SPI_GETCARETTIMEOUT
// read directly.
func TestReadCaretBlinkLive(t *testing.T) {
	half, err := w32.GetCaretBlinkTime()
	if err != nil {
		t.Fatalf("GetCaretBlinkTime: %v", err)
	}
	timeout, err := w32.SystemParametersInfoUint32(w32.SPI_GETCARETTIMEOUT)
	if err != nil {
		t.Fatalf("SPI_GETCARETTIMEOUT: %v", err)
	}
	b := readCaretBlink(liveSys{})
	wantPeriod := 2 * time.Duration(half) * time.Millisecond
	if half == w32.INFINITE {
		wantPeriod = 0
	}
	if b.Period != wantPeriod {
		t.Errorf("Period %v, want %v (GetCaretBlinkTime %d)", b.Period, wantPeriod, half)
	}
	if timeout != 0 && timeout != w32.INFINITE && b.Timeout != time.Duration(timeout)*time.Millisecond {
		t.Errorf("Timeout %v, want %d ms", b.Timeout, timeout)
	}
	if b.Period < 0 || b.Timeout < 0 {
		t.Errorf("negative caret blink %+v", b)
	}
	t.Logf("live caret blink %+v (GetCaretBlinkTime %d ms, SPI_GETCARETTIMEOUT %d ms)", b, half, timeout)
}

// TestAppearanceChanged: the broadcasts re-read the appearance and call
// AppearanceChanged only when it changed.
func TestAppearanceChanged(t *testing.T) {
	sys := &fakeSys{reg: map[string]uint32{regPersonalize + "|AppsUseLightTheme": 1}}
	s := newShell(sys)
	s.q.SetChecks(true)
	s.debug = true // the recorder checks every Paint
	var got []driver.Appearance
	err := run(t, s, &handler{
		ready: func() {
			defer s.Quit()
			if s.Appearance().Dark {
				t.Error("initial appearance dark")
			}
			send := func(msg uint32) {
				w32.SendMessage(s.listener, msg, 0, 0)
				rethrow()
			}
			send(w32.WM_SETTINGCHANGE) // no change
			sys.reg[regPersonalize+"|AppsUseLightTheme"] = 0
			send(w32.WM_SETTINGCHANGE)
			send(w32.WM_THEMECHANGED) // no change
			sys.color, sys.colorOK = 0xFF_10_20_30, true
			send(w32.WM_DWMCOLORIZATIONCOLORCHANGED)
			sys.blink, sys.blinkOK = w32.INFINITE, true // the caret blink turned off
			send(w32.WM_SETTINGCHANGE)
			if !s.Appearance().Dark || s.Appearance().Accent != driver.RGB8(0x10, 0x20, 0x30) {
				t.Errorf("Appearance() = %+v after the changes", s.Appearance())
			}
		},
		appearance: func(a driver.Appearance) { got = append(got, a) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[0].Dark || got[0].Accent != driver.DefaultAccent ||
		got[1].Accent != driver.RGB8(0x10, 0x20, 0x30) || got[1].CaretBlink != driver.DefaultCaretBlink ||
		got[2].CaretBlink.Period != 0 || got[2].CaretBlink.Timeout != driver.DefaultCaretBlink.Timeout {
		t.Errorf("AppearanceChanged calls %+v, want dark, then the new accent, then no blink", got)
	}
}

// TestAppearanceChangedEveryField (S6.1h): each of the three broadcasts
// re-reads every field of the appearance, not only the one it is named
// for: for each message and each OS setting behind a field, a change of
// that setting alone and the message give exactly that field's new value
// and one AppearanceChanged.
func TestAppearanceChangedEveryField(t *testing.T) {
	p, d, a := regPersonalize+"|", regDWM+"|", regAccessibility+"|"
	changes := []struct {
		field string
		set   func(f *fakeSys)
		get   func(a driver.Appearance) any
	}{
		{"Dark", func(f *fakeSys) { f.reg[p+"AppsUseLightTheme"] = 0 }, func(a driver.Appearance) any { return a.Dark }},
		{"Accent (AccentColor)", func(f *fakeSys) { f.reg[d+"AccentColor"] = 0xFF_30_20_10 },
			func(a driver.Appearance) any { return a.Accent }},
		{"Accent (colorization)", func(f *fakeSys) { f.color, f.colorOK = 0xFF_40_50_60, true },
			func(a driver.Appearance) any { return a.Accent }},
		{"IncreaseContrast", func(f *fakeSys) { f.hc = true }, func(a driver.Appearance) any { return a.IncreaseContrast }},
		{"ReduceMotion", func(f *fakeSys) { f.anim = false }, func(a driver.Appearance) any { return a.ReduceMotion }},
		{"ReduceTransparency", func(f *fakeSys) { f.reg[p+"EnableTransparency"] = 0 },
			func(a driver.Appearance) any { return a.ReduceTransparency }},
		{"LegacyScrollers", func(f *fakeSys) { f.reg[a+"DynamicScrollbars"] = 0 },
			func(a driver.Appearance) any { return a.LegacyScrollers }},
		{"CaretBlink.Period", func(f *fakeSys) { f.blink = 250 }, func(a driver.Appearance) any { return a.CaretBlink.Period }},
		{"CaretBlink.Timeout", func(f *fakeSys) { f.timeout = 0 }, func(a driver.Appearance) any { return a.CaretBlink.Timeout }},
	}
	messages := []struct {
		name string
		msg  uint32
	}{
		{"WM_SETTINGCHANGE", w32.WM_SETTINGCHANGE},
		{"WM_DWMCOLORIZATIONCOLORCHANGED", w32.WM_DWMCOLORIZATIONCOLORCHANGED},
		{"WM_THEMECHANGED", w32.WM_THEMECHANGED},
	}
	for _, m := range messages {
		for _, c := range changes {
			sys := &fakeSys{reg: map[string]uint32{
				p + "AppsUseLightTheme": 1, p + "EnableTransparency": 1, a + "DynamicScrollbars": 1,
			}, anim: true, animOK: true, blink: 530, blinkOK: true, timeout: 5000, timeoutOK: true}
			s := newShell(sys)
			var got []driver.Appearance
			err := run(t, s, &handler{
				ready: func() {
					defer s.Quit()
					before := s.Appearance()
					c.set(sys)
					want := readAppearance(sys)
					if c.get(want) == c.get(before) {
						t.Fatalf("%s: the setting does not change the field", c.field)
					}
					w32.SendMessage(s.listener, m.msg, 0, 0)
					rethrow()
					if s.Appearance() != want {
						t.Errorf("%s after %s: Appearance() = %+v, want %+v", c.field, m.name, s.Appearance(), want)
					}
				},
				appearance: func(a driver.Appearance) { got = append(got, a) },
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 {
				t.Errorf("%s after %s: %d AppearanceChanged calls, want 1", c.field, m.name, len(got))
			}
		}
	}
}

func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	mustPanicWith(t, what, func(any) bool { return true }, fn)
}

func mustPanicWith(t *testing.T, what string, ok func(any) bool, fn func()) {
	t.Helper()
	defer func() {
		t.Helper()
		v := recover()
		if v == nil {
			t.Errorf("%s did not panic", what)
		} else if !ok(v) {
			t.Errorf("%s panicked with %v", what, v)
		}
	}()
	fn()
}
