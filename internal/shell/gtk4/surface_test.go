package gtk4

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// The surface's host tests (S4.1f): the surface over a fakeWindow, which
// records the window calls and plays GTK's part through the real
// trampolines (surfaceSizeAllocate and surfaceSnapshot on a fake GGSurface
// instance, sigBool and sigPointer with the signal handles), with the
// handles created and freed as window_linux.go does.

// sfHandler is a shell.SurfaceHandler that logs its calls.
type sfHandler struct {
	log       []string
	paints    int
	allow     bool // CloseRequested's answer
	onPaint   func(c render.Canvas)
	onResized func(size driver.Size, scale float64)
	onClosed  func()
	onRequest func()
	onEvent   func(shell.PointerEvent)
	onScroll  func(shell.ScrollEvent)
	onKey     func(shell.KeyEvent) bool // the key events' result
	times     []time.Duration           // of the pointer events
}

func (h *sfHandler) Paint(c render.Canvas) {
	h.paints++
	h.log = append(h.log, "paint")
	if h.onPaint != nil {
		h.onPaint(c)
	}
}

func (h *sfHandler) Resized(size driver.Size, scale float64) {
	h.log = append(h.log, fmt.Sprintf("resized %vx%v@%v", size.W, size.H, scale))
	if h.onResized != nil {
		h.onResized(size, scale)
	}
}

func (h *sfHandler) CloseRequested() bool {
	h.log = append(h.log, "closeRequested")
	if h.onRequest != nil {
		h.onRequest()
	}
	return h.allow
}

func (h *sfHandler) Closed() {
	h.log = append(h.log, "closed")
	if h.onClosed != nil {
		h.onClosed()
	}
}

func (h *sfHandler) KeyChanged(isKey bool) { h.log = append(h.log, fmt.Sprintf("key %v", isKey)) }

// Event logs a PointerEvent as "kind x,y", plus " b=button held=buttons
// n=clicks" for presses and releases and " m=mods" when modifiers are
// held; other events as "event". The times go to times.
func (h *sfHandler) Event(ev shell.Event) bool {
	switch ke := ev.(type) {
	case shell.KeyEvent:
		h.times = append(h.times, ke.Time)
		s := fmt.Sprintf("key-up %#x", ke.Key)
		if ke.Down {
			s = fmt.Sprintf("key-down %#x", ke.Key)
		}
		if ke.Repeat {
			s += " repeat"
		}
		if ke.Mods != 0 {
			s += fmt.Sprintf(" m=%d", ke.Mods)
		}
		h.log = append(h.log, s)
		if h.onKey != nil {
			return h.onKey(ke)
		}
		return false
	case shell.TextEvent:
		h.times = append(h.times, ke.Time)
		h.log = append(h.log, fmt.Sprintf("text %q", ke.Text))
		return false
	}
	if se, ok := ev.(shell.ScrollEvent); ok {
		h.times = append(h.times, se.Time)
		h.log = append(h.log, scrollLog(se))
		if h.onScroll != nil {
			h.onScroll(se)
		}
		return false
	}
	pe, ok := ev.(shell.PointerEvent)
	if !ok {
		h.log = append(h.log, "event")
		return false
	}
	h.times = append(h.times, pe.Time)
	s := fmt.Sprintf("%s %v,%v", [...]string{"move", "down", "up", "enter", "leave"}[pe.Kind], pe.Pos.X, pe.Pos.Y)
	if pe.Kind == shell.PointerDown || pe.Kind == shell.PointerUp {
		s += fmt.Sprintf(" b=%d held=%d n=%d", pe.Button, pe.Buttons, pe.Clicks)
	} else if pe.Button != 0 || pe.Clicks != 0 {
		s += fmt.Sprintf(" b=%d n=%d!", pe.Button, pe.Clicks)
	}
	if pe.Mods != 0 {
		s += fmt.Sprintf(" m=%d", pe.Mods)
	}
	h.log = append(h.log, s)
	if h.onEvent != nil {
		h.onEvent(pe)
	}
	return false
}

// take returns the log and clears it.
func (h *sfHandler) take() []string {
	l := h.log
	h.log = nil
	return l
}

func wantLog(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s: %q, want %q", what, got, want)
	}
}

// winFactory is the newWindow of the surface tests: it creates
// fakeWindows and checks the rules of the window interface.
type winFactory struct {
	t *testing.T
	s *Shell
	l *fakeClockLoop // nil over a fakeLoop (no tick callbacks to check)

	scale  float64              // the scale of new windows
	during func(fw *fakeWindow) // runs inside newWindow, signals connected
	err    error                // newWindow fails with it (after during)

	wins []*fakeWindow
	errs []string
	pins runtime.Pinner
}

func (f *winFactory) errorf(format string, args ...any) {
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}

// newWindow creates a fakeWindow as window_linux.go creates a gtkWindow:
// a GGSurface instance whose handle word holds a kindSurface entry of p,
// and one handle per connected signal.
func (f *winFactory) newWindow(cfg windowConfig, p windowPeer) (window, error) {
	if !f.s.q.OnUIThread() {
		f.errorf("newWindow off the UI thread")
	}
	fw := &fakeWindow{f: f, cfg: cfg, peer: p, title: cfg.title,
		defW: cfg.width, defH: cfg.height, reqW: cfg.minW, reqH: cfg.minH,
		sc: f.scale, sig: map[kind]ffi.Handle{}}
	fw.inst = new(fakeInstance)
	f.pins.Pin(fw.inst)
	fw.addr = uintptr(unsafe.Pointer(fw.inst))
	fw.surf = dispatch.New(entry{peer: p, kind: kindSurface})
	setSurfaceHandle(fw.addr, fw.surf)
	for _, k := range []kind{kindCloseRequest, kindActive, kindScale, kindState,
		kindCaptionPress, kindCaptionDragBegin, kindCaptionDragUpdate,
		kindPointerEnter, kindPointerMotion, kindPointerLeave,
		kindPointerPressed, kindPointerReleased, kindPointerCancel,
		kindScroll, kindScrollBegin, kindScrollEnd, kindScrollDecelerate,
		kindKeyPressed, kindKeyReleased, kindIMCommit} {
		fw.sig[k] = dispatch.New(entry{peer: p, kind: k})
	}
	f.wins = append(f.wins, fw)
	if f.during != nil {
		f.during(fw)
	}
	if f.err != nil {
		fw.release() // on error nothing of it is left
		return nil, f.err
	}
	return fw, nil
}

// finish closes what a test left open (silently, as a panic teardown) and
// reports the misuse found.
func (f *winFactory) finish() {
	f.s.closeAll(true)
	for i, fw := range f.wins {
		if !fw.released {
			f.errorf("window %d (%q) never released", i, fw.cfg.title)
		}
	}
	for _, e := range f.errs {
		f.t.Errorf("fake window: %s", e)
	}
	f.pins.Unpin()
}

// last returns the window created last.
func (f *winFactory) last(t *testing.T) *fakeWindow {
	t.Helper()
	if len(f.wins) == 0 {
		t.Fatal("no window created")
	}
	return f.wins[len(f.wins)-1]
}

// open creates a surface and returns it with its window.
func (f *winFactory) open(t *testing.T, cfg shell.SurfaceConfig, h shell.SurfaceHandler) (*surface, *fakeWindow) {
	t.Helper()
	sf, err := f.s.NewSurface(cfg, h)
	if err != nil {
		t.Fatalf("NewSurface(%+v) = %v", cfg, err)
	}
	return sf.(*surface), f.last(t)
}

// frame runs the requested frame: from the tick callback if the clock has
// one, else from the fallback timeout.
func (f *winFactory) frame(t *testing.T) {
	t.Helper()
	if !f.s.clock.requested {
		t.Fatal("frame: no frame requested")
	}
	if w := f.s.clock.tickWidget; w != 0 {
		f.l.now += 16_667
		f.l.fireTick(t, w, f.l.now, 0, 0)
	} else {
		f.l.fireTimeout(t)
	}
}

// fakeWindow is a window that records its calls and the state GTK would
// keep.
type fakeWindow struct {
	f    *winFactory
	cfg  windowConfig
	peer windowPeer
	inst *fakeInstance
	addr uintptr // the GGSurface
	surf ffi.Handle
	sig  map[kind]ffi.Handle

	calls []string // the window calls but the getters

	title                  string
	defW, defH, reqW, reqH int32
	allocW, allocH         int32 // the last allocation
	isMax, isActive, isMin bool
	sc                     float64
	draws                  int
	snaps                  []*fakeSnapshot // one per snapshotter call
	destroyed, released    bool

	// ignoreDefault: layout keeps the allocated size (a GTK that ignores
	// the default size of a mapped window).
	ignoreDefault bool

	// The input controllers: the time, the GdkModifierType and the gesture's
	// GDK button of the event they handle now (eventTime, eventState,
	// currentButton).
	evTime   uint32
	evState  uint32
	evButton uint32
	// unit is the scroll controller's GdkScrollUnit (scrollUnit).
	unit uint32
	// The keyboard: base maps keycodes to their level-0 keyval (keyBase);
	// imCommits maps the keyval of a key-down the input method takes to
	// the text it commits (imFilter; "" takes it without a commit, a dead
	// key); evKeyval is the keyval of the event handled now; ims logs the
	// input-method calls.
	base      map[uint32]uint32
	imCommits map[uint32]string
	evKeyval  uint32
	evDown    bool
	ims       []string
}

var _ window = (*fakeWindow)(nil)

// use checks that a window method may run: before destroy, on the UI
// thread.
func (fw *fakeWindow) use(what string) {
	if fw.destroyed {
		fw.f.errorf("%s after destroy", what)
	}
	if !fw.f.s.q.OnUIThread() {
		fw.f.errorf("%s off the UI thread", what)
	}
}

func (fw *fakeWindow) call(format string, args ...any) {
	c := fmt.Sprintf(format, args...)
	fw.use(c)
	fw.calls = append(fw.calls, c)
}

func (fw *fakeWindow) take() []string {
	c := fw.calls
	fw.calls = nil
	return c
}

func (fw *fakeWindow) widget() uintptr       { fw.use("widget"); return fw.addr }
func (fw *fakeWindow) setTitle(title string) { fw.call("setTitle %s", title); fw.title = title }
func (fw *fakeWindow) queueResize()          { fw.call("queueResize") }
func (fw *fakeWindow) present()              { fw.call("present") }
func (fw *fakeWindow) minimize()             { fw.call("minimize") }
func (fw *fakeWindow) maximized() bool       { fw.use("maximized"); return fw.isMax }
func (fw *fakeWindow) active() bool          { fw.use("active"); return fw.isActive }
func (fw *fakeWindow) minimized() bool       { fw.use("minimized"); return fw.isMin }
func (fw *fakeWindow) scale() float64        { fw.use("scale"); return fw.sc }
func (fw *fakeWindow) setCursor(name string) { fw.call("setCursor %s", name) }
func (fw *fakeWindow) queueDraw()            { fw.call("queueDraw"); fw.draws++ }

// snapshotter returns a new fakeSnapshot for the snapshot s, kept in snaps.
func (fw *fakeWindow) snapshotter(s uintptr) snapshotter {
	fw.use("snapshotter")
	if s != testSnapshot {
		fw.f.errorf("snapshotter(%#x), want the test snapshot", s)
	}
	sn := &fakeSnapshot{}
	fw.snaps = append(fw.snaps, sn)
	return sn
}

func (fw *fakeWindow) setMaximized(on bool) { fw.call("setMaximized %v", on); fw.isMax = on }
func (fw *fakeWindow) setDefaultSize(w, h int32) {
	fw.call("setDefaultSize %dx%d", w, h)
	fw.defW, fw.defH = w, h
}
func (fw *fakeWindow) setSizeRequest(w, h int32) {
	fw.call("setSizeRequest %dx%d", w, h)
	fw.reqW, fw.reqH = w, h
}

// The caption gestures' instances (beginMove and eventTime check them).
const (
	testClick uintptr = 0xc11c00
	testDrag  uintptr = 0xd4a900
	// The pointer controllers' instances.
	testMotion  uintptr = 0x307100
	testPointer uintptr = 0x901400
	// The scroll controller's instance.
	testScroll uintptr = 0x5c1100
	// The key controller's and the input method's instances.
	testKey uintptr = 0x4e7000
	testIM  uintptr = 0x13c000
)

// testDragThreshold is gtk-dnd-drag-threshold's default.
const testDragThreshold = 8

func (fw *fakeWindow) eventTime(c uintptr) uint32 {
	fw.use("eventTime")
	if c != testClick && c != testDrag && c != testMotion && c != testPointer && c != testScroll && c != testKey {
		fw.f.errorf("eventTime(%#x), want an input controller", c)
	}
	return fw.evTime
}

func (fw *fakeWindow) eventState(c uintptr) uint32 {
	fw.use("eventState")
	if c != testMotion && c != testPointer && c != testScroll {
		fw.f.errorf("eventState(%#x), want a pointer controller", c)
	}
	return fw.evState
}

func (fw *fakeWindow) currentButton(g uintptr) uint32 {
	fw.use("currentButton")
	if g != testPointer {
		fw.f.errorf("currentButton(%#x), want the pointer gesture", g)
	}
	return fw.evButton
}

// testClickRules are GTK's defaults: 400 ms, 5 px each way.
var testClickRules = shellkit.ClickRules{Time: 400, W: 10, H: 10}

func (fw *fakeWindow) clickRules() shellkit.ClickRules {
	fw.use("clickRules")
	return testClickRules
}

// pastDragThreshold is gtk_drag_check_threshold from 0, 0: either axis of
// the offset beyond the threshold, in whole pixels.
func (fw *fakeWindow) pastDragThreshold(dx, dy float64) bool {
	fw.use("pastDragThreshold")
	return math.Abs(math.Round(dx)) > testDragThreshold || math.Abs(math.Round(dy)) > testDragThreshold
}

func (fw *fakeWindow) beginMove(drag uintptr, x, y float64) {
	fw.call("beginMove %v,%v", x, y)
	if drag != testDrag {
		fw.f.errorf("beginMove(%#x), want the drag gesture", drag)
	}
}

// destroy checks the closing order (the surface has left the shell and
// stopped pacing, and the clock has removed its tick callback), drops the
// widget's tick callbacks as GTK does, and frees the handles.
func (fw *fakeWindow) destroy() {
	fw.call("destroy")
	for _, o := range fw.f.s.surfaces {
		if any(o) == any(fw.peer) {
			fw.f.errorf("window destroyed while its surface is in Shell.surfaces")
		}
	}
	if fw.peer.(openSurface).tickWidget() != 0 {
		fw.f.errorf("window destroyed while its surface still paces")
	}
	if l := fw.f.l; l != nil {
		if slices.ContainsFunc(l.ticks, func(tk *fakeTick) bool { return tk.widget == fw.addr }) {
			fw.f.errorf("window destroyed with the clock's tick callback on it (detach after destroy)")
		}
		l.destroyWidget(fw.addr)
	}
	fw.destroyed = true
	fw.release()
}

// release frees the window's handles, as closureNotify and
// releaseSurfaceWidget do.
func (fw *fakeWindow) release() {
	if fw.released {
		fw.f.errorf("window released twice")
	}
	fw.released = true
	dispatch.Free(fw.surf)
	for _, h := range fw.sig {
		dispatch.Free(h)
	}
}

// handles returns every handle the window holds.
func (fw *fakeWindow) handles() []ffi.Handle {
	hs := []ffi.Handle{fw.surf}
	for _, h := range fw.sig {
		hs = append(hs, h)
	}
	return hs
}

// GTK's side, through the trampolines; a trapped panic is re-raised.

const testSnapshot uintptr = 0x5a5a00

func (fw *fakeWindow) allocate(w, h int32) {
	fw.allocW, fw.allocH = w, h
	surfaceSizeAllocate(fw.addr, w, h, -1)
	rethrow()
}

// layout allocates what GTK would at the next layout: the default size
// (or, with ignoreDefault, the allocated size once there is one), at least
// the size request.
func (fw *fakeWindow) layout() {
	w, h := fw.defW, fw.defH
	if fw.ignoreDefault && fw.allocW > 0 {
		w, h = fw.allocW, fw.allocH
	}
	fw.allocate(max(w, fw.reqW), max(h, fw.reqH))
}

func (fw *fakeWindow) snap() {
	surfaceSnapshot(fw.addr, testSnapshot)
	rethrow()
}

func (fw *fakeWindow) closeRequest() int32 {
	r := sigBool(fw.addr, uintptr(fw.sig[kindCloseRequest]))
	rethrow()
	return r
}

func (fw *fakeWindow) notify(k kind) {
	sigPointer(fw.addr, 0, uintptr(fw.sig[k]))
	rethrow()
}

// pressAt is the click gesture's "pressed": the n-th press at x, y, at
// event time t.
func (fw *fakeWindow) pressAt(n int32, x, y float64, t uint32) {
	fw.evTime = t
	sigIntDoubleDouble(testClick, n, x, y, uintptr(fw.sig[kindCaptionPress]))
	rethrow()
}

// dragBegin and dragUpdate are the drag gesture's signals: the start at
// event time t, and the offset from it.
func (fw *fakeWindow) dragBegin(x, y float64, t uint32) {
	fw.evTime = t
	sigDoubleDouble(testDrag, x, y, uintptr(fw.sig[kindCaptionDragBegin]))
	rethrow()
}

func (fw *fakeWindow) dragUpdate(dx, dy float64) {
	sigDoubleDouble(testDrag, dx, dy, uintptr(fw.sig[kindCaptionDragUpdate]))
	rethrow()
}

// newSurfaceTest returns a shell over a fakeClockLoop whose clock has
// begun, with Ready passed, the fake GGSurface class set and a window
// factory. At the end every surface left open is closed silently and
// every window must have been released.
func newSurfaceTest(t *testing.T) (*Shell, *winFactory) {
	t.Helper()
	s, c, l := newClockTest(t)
	withFakeClass(t, nil)
	f := &winFactory{t: t, s: s, l: l, scale: 1}
	l.newWin = f.newWindow
	c.begin(0)
	t.Cleanup(f.finish)
	return s, f
}

var (
	sfResizable = shell.SurfaceConfig{Title: "r", Size: driver.Size{W: 400, H: 300},
		Style: driver.StyleTitled | driver.StyleClosable | driver.StyleMiniaturizable | driver.StyleResizable}
	sfFixed = shell.SurfaceConfig{Title: "f", Size: driver.Size{W: 400, H: 300},
		Style: driver.StyleTitled | driver.StyleClosable}
)

func panicsWith(sub string) func(any) bool {
	return func(v any) bool {
		msg, _ := v.(string)
		return strings.Contains(msg, sub)
	}
}

// TestSurfaceNewGuards: NewSurface panics before Ready and with a nil
// handler; a failing newWindow leaves no surface and no handle behind;
// after Quit it fails with ErrQuit without creating a window.
func TestSurfaceNewGuards(t *testing.T) {
	s, f := newSurfaceTest(t)
	base := dispatch.Len()
	s.ready = false
	mustPanicWith(t, "NewSurface before Ready", panicsWith("before Handler.Ready"),
		func() { _, _ = s.NewSurface(sfResizable, &sfHandler{}) })
	s.ready = true
	mustPanicWith(t, "NewSurface(nil handler)", panicsWith("nil handler"),
		func() { _, _ = s.NewSurface(sfResizable, nil) })
	if len(f.wins) != 0 {
		t.Fatalf("the guards created %d windows", len(f.wins))
	}

	cause := errors.New("no window")
	f.err = cause
	h := &sfHandler{}
	sf, err := s.NewSurface(sfResizable, h)
	if sf != nil || !errors.Is(err, cause) {
		t.Errorf("NewSurface with a failing newWindow = %v, %v; want nil and its error", sf, err)
	}
	if len(s.surfaces) != 0 {
		t.Errorf("%d surfaces after a failed NewSurface", len(s.surfaces))
	}
	checkLen(t, base)
	wantLog(t, "handler after a failed NewSurface", h.take())
	f.err = nil

	s.Quit()
	sf, err = s.NewSurface(sfResizable, h)
	if sf != nil || !errors.Is(err, ErrQuit) {
		t.Errorf("NewSurface after Quit = %v, %v; want nil, ErrQuit", sf, err)
	}
	if len(f.wins) != 1 {
		t.Errorf("NewSurface after Quit created a window (%d windows)", len(f.wins))
	}
	mustPanic(t, "NewSurface(nil handler) after Quit", func() { _, _ = s.NewSurface(sfResizable, nil) })
}

// TestSurfaceNewConfig: the windowConfig of a SurfaceConfig (title without
// NULs, size clamped and rounded to whole pixels, the minimum as size
// request, a fixed surface requesting its size); Size and Scale are valid
// from NewSurface, nothing is reported and no window method is called.
func TestSurfaceNewConfig(t *testing.T) {
	s, f := newSurfaceTest(t)
	nan := math.NaN()
	for _, c := range []struct {
		name  string
		cfg   shell.SurfaceConfig
		scale float64
		want  windowConfig
		size  driver.Size
	}{
		{"NULs stripped, half pixels rounded up",
			shell.SurfaceConfig{Title: "a\x00b\x00", Size: driver.Size{W: 400.4, H: 300.5}, Style: driver.StyleResizable}, 1,
			windowConfig{title: "ab", width: 400, height: 301, resizable: true}, driver.Size{W: 400.4, H: 300.5}},
		{"clamped to the limits, the minimum requested",
			shell.SurfaceConfig{Size: driver.Size{W: 100, H: 1000}, MinSize: driver.Size{W: 200, H: 50.6},
				MaxSize: driver.Size{H: 500}, Style: driver.StyleResizable}, 2,
			windowConfig{width: 200, height: 500, minW: 200, minH: 51, resizable: true}, driver.Size{W: 200, H: 500}},
		{"min wins over max",
			shell.SurfaceConfig{Size: driver.Size{W: 10, H: 10}, MinSize: driver.Size{W: 300, H: 300},
				MaxSize: driver.Size{W: 100, H: 100}, Style: driver.StyleResizable}, 1.5,
			windowConfig{width: 300, height: 300, minW: 300, minH: 300, resizable: true}, driver.Size{W: 300, H: 300}},
		{"fixed: requests its size",
			shell.SurfaceConfig{Title: "fixed", Size: driver.Size{W: 320.6, H: 240}, MinSize: driver.Size{W: 100, H: 100},
				Style: driver.StyleTitled}, 1,
			windowConfig{title: "fixed", width: 321, height: 240, minW: 321, minH: 240}, driver.Size{W: 320.6, H: 240}},
		{"negative and NaN sizes are 0",
			shell.SurfaceConfig{Size: driver.Size{W: -5, H: nan}, Style: driver.StyleResizable}, 1,
			windowConfig{resizable: true}, driver.Size{}},
	} {
		f.scale = c.scale
		h := &sfHandler{}
		sf, fw := f.open(t, c.cfg, h)
		if fw.cfg != c.want {
			t.Errorf("%s: windowConfig %+v, want %+v", c.name, fw.cfg, c.want)
		}
		if got := sf.Size(); got != c.size {
			t.Errorf("%s: Size() = %v, want %v", c.name, got, c.size)
		}
		if got := sf.Scale(); got != c.scale {
			t.Errorf("%s: Scale() = %v, want %v", c.name, got, c.scale)
		}
		if fw.peer != windowPeer(sf) {
			t.Errorf("%s: the window's peer is not the surface", c.name)
		}
		if sf.creating || sf.shown || !sf.invalid || sf.dead {
			t.Errorf("%s: creating %v, shown %v, invalid %v, dead %v; want false, false, true, false",
				c.name, sf.creating, sf.shown, sf.invalid, sf.dead)
		}
		if s.surfaces[len(s.surfaces)-1] != openSurface(sf) {
			t.Errorf("%s: the surface is not last in Shell.surfaces", c.name)
		}
		wantLog(t, c.name+": window calls in NewSurface", fw.take())
		wantLog(t, c.name+": handler calls in NewSurface", h.take())
	}
	if s.clock.requested {
		t.Error("NewSurface requested a frame")
	}
}

// TestSurfaceNewSignalsDuringCreate: GTK's signals while the window is
// being created reach the surface, which only keeps the allocated size and
// calls no handler method.
func TestSurfaceNewSignalsDuringCreate(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{allow: true}
	f.during = func(fw *fakeWindow) {
		fw.allocate(640, 480)
		fw.sc = 2
		fw.notify(kindScale)
		fw.isActive = true
		fw.notify(kindActive)
		fw.isMin = true
		fw.notify(kindState)
		if r := fw.closeRequest(); r != gtk.True {
			t.Errorf("close-request during creation returned %d, want TRUE", r)
		}
		fw.snap()
	}
	sf, fw := f.open(t, sfResizable, h)
	f.during = nil
	wantLog(t, "handler calls during creation", h.take())
	if got := sf.Size(); got != (driver.Size{W: 640, H: 480}) {
		t.Errorf("Size() = %v, want the allocation during creation 640x480", got)
	}
	if sf.Scale() != 2 {
		t.Errorf("Scale() = %v, want the window's 2", sf.Scale())
	}
	if sf.paints != 0 || sf.dead || len(s.surfaces) != 1 {
		t.Errorf("after creation: %d paints, dead %v, %d surfaces", sf.paints, sf.dead, len(s.surfaces))
	}
	if s.clock.requested {
		t.Error("the signals during creation requested a frame")
	}
	wantLog(t, "window calls", fw.take())
	// The same allocation again reports nothing.
	fw.allocate(640, 480)
	wantLog(t, "the creation's allocation again", h.take())
}

// TestSurfaceSetSize: SetSize clamps, reports Resized once before it
// returns (after the window calls), invalidates; an allocation of the same
// pixels keeps a fractional size, one of other pixels reports it; a
// maximized window is restored first.
func TestSurfaceSetSize(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	var during []string
	h.onResized = func(driver.Size, float64) { during = slices.Clone(fw.calls) }

	sf.SetSize(driver.Size{W: 500.5, H: 400.4})
	calls := []string{"setDefaultSize 501x400", "setSizeRequest 0x0", "queueResize"}
	wantLog(t, "SetSize window calls", fw.take(), calls...)
	wantLog(t, "window calls before Resized", during, calls...)
	wantLog(t, "SetSize", h.take(), "resized 500.5x400.4@1")
	if sf.Size() != (driver.Size{W: 500.5, H: 400.4}) || !sf.invalid || !s.clock.requested {
		t.Errorf("after SetSize: Size %v, invalid %v, frame requested %v", sf.Size(), sf.invalid, s.clock.requested)
	}
	fw.layout()
	wantLog(t, "allocation of the same pixels", h.take())
	if sf.Size() != (driver.Size{W: 500.5, H: 400.4}) {
		t.Errorf("Size() = %v after the allocation of its pixels, want 500.5x400.4", sf.Size())
	}
	f.frame(t)
	fw.take()
	sf.invalid = false
	fw.allocate(600, 450)
	wantLog(t, "allocation of another size", h.take(), "resized 600x450@1")
	if sf.Size() != (driver.Size{W: 600, H: 450}) || !sf.invalid || !s.clock.requested {
		t.Errorf("after the allocation: Size %v, invalid %v, frame requested %v", sf.Size(), sf.invalid, s.clock.requested)
	}
	sf.SetSize(driver.Size{W: 600, H: 450})
	wantLog(t, "SetSize to the size", fw.take(), "setDefaultSize 600x450", "setSizeRequest 0x0", "queueResize")
	wantLog(t, "SetSize to the size", h.take())

	// Clamped to the limits.
	sf.SetMaxSize(driver.Size{W: 800, H: 700})
	sf.SetMinSize(driver.Size{W: 100, H: 100})
	fw.take()
	sf.SetSize(driver.Size{W: 2000, H: 50})
	wantLog(t, "clamped SetSize", fw.take(), "setDefaultSize 800x100", "setSizeRequest 100x100", "queueResize")
	wantLog(t, "clamped SetSize", h.take(), "resized 800x100@1")

	// A maximized window is restored first.
	fw.isMax = true
	sf.SetSize(driver.Size{W: 500, H: 400})
	wantLog(t, "SetSize maximized", fw.take(),
		"setMaximized false", "setDefaultSize 500x400", "setSizeRequest 100x100", "queueResize")
	wantLog(t, "SetSize maximized", h.take(), "resized 500x400@1")

	// Shown, with the default-size path: no forced size request.
	if mappedResize != resizeDefaultSize {
		t.Fatalf("mappedResize = %d, want resizeDefaultSize", mappedResize)
	}
	sf.Show()
	fw.take()
	sf.SetSize(driver.Size{W: 700, H: 600})
	wantLog(t, "SetSize shown", fw.take(), "setDefaultSize 700x600", "setSizeRequest 100x100", "queueResize")
	if sf.forced {
		t.Error("the default-size path forced the size request")
	}
	h.take()

	// A Resized handler that closes the surface: nothing after it.
	h.onResized = func(driver.Size, float64) { sf.Close() }
	sf.SetSize(driver.Size{W: 300, H: 300})
	wantLog(t, "SetSize closing in Resized", h.take(), "resized 300x300@1", "closed")
	if !fw.destroyed {
		t.Error("the window was not destroyed")
	}
}

// TestSurfaceMinMax: SetMinSize and SetMaxSize update the size request
// and resize a surface that violates them; a fixed surface requests its
// size.
func TestSurfaceMinMax(t *testing.T) {
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	sf.SetMinSize(driver.Size{W: 500, H: 350})
	wantLog(t, "SetMinSize above", fw.take(), "setDefaultSize 500x350", "setSizeRequest 500x350", "queueResize")
	wantLog(t, "SetMinSize above", h.take(), "resized 500x350@1")
	sf.SetMinSize(driver.Size{W: 100, H: 100.5})
	wantLog(t, "SetMinSize below", fw.take(), "setSizeRequest 100x101")
	wantLog(t, "SetMinSize below", h.take())
	sf.SetMaxSize(driver.Size{W: 300, H: 200})
	wantLog(t, "SetMaxSize below", fw.take(), "setDefaultSize 300x200", "setSizeRequest 100x101", "queueResize")
	wantLog(t, "SetMaxSize below", h.take(), "resized 300x200@1")
	sf.SetMaxSize(driver.Size{})
	wantLog(t, "SetMaxSize none", fw.take(), "setSizeRequest 100x101")
	wantLog(t, "SetMaxSize none", h.take())
	if sf.Size() != (driver.Size{W: 300, H: 200}) {
		t.Errorf("Size() = %v, want 300x200", sf.Size())
	}

	hf := &sfHandler{}
	ff, fwf := f.open(t, sfFixed, hf)
	if fwf.cfg.minW != 400 || fwf.cfg.minH != 300 {
		t.Errorf("fixed surface requests %dx%d, want its size 400x300", fwf.cfg.minW, fwf.cfg.minH)
	}
	ff.SetSize(driver.Size{W: 500, H: 250})
	wantLog(t, "fixed SetSize", fwf.take(), "setDefaultSize 500x250", "setSizeRequest 500x250", "queueResize")
	ff.SetMinSize(driver.Size{W: 600})
	wantLog(t, "fixed SetMinSize", fwf.take(), "setDefaultSize 600x250", "setSizeRequest 600x250", "queueResize")
	wantLog(t, "fixed", hf.take(), "resized 500x250@1", "resized 600x250@1")
	fwf.isMax = true
	ff.Zoom()
	ff.Show()
	ff.Zoom()
	wantLog(t, "Zoom of a fixed surface", fwf.take(), "present")
}

// TestSurfaceResizeSizeRequest: on the size-request path SetSize raises
// the request only when it grows a shown window, until an allocation
// reaches it; the request is then lowered from posted work.
func TestSurfaceResizeSizeRequest(t *testing.T) {
	saved := mappedResize
	mappedResize = resizeSizeRequest
	t.Cleanup(func() { mappedResize = saved })
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	cfg := sfResizable
	cfg.MinSize = driver.Size{W: 100, H: 100}
	sf, fw := f.open(t, cfg, h)
	fw.ignoreDefault = true

	sf.SetSize(driver.Size{W: 600, H: 400}) // hidden
	wantLog(t, "SetSize hidden", fw.take(), "setDefaultSize 600x400", "setSizeRequest 100x100", "queueResize")
	if sf.forced {
		t.Error("SetSize of a hidden surface forced the request")
	}
	fw.layout() // the first layout uses the default size
	sf.Show()
	fw.take()
	wantLog(t, "before", h.take(), "resized 600x400@1")

	sf.SetSize(driver.Size{W: 800, H: 300}) // wider
	wantLog(t, "SetSize growing", fw.take(), "setDefaultSize 800x300", "setSizeRequest 800x300", "queueResize")
	wantLog(t, "SetSize growing", h.take(), "resized 800x300@1")
	if !sf.forced {
		t.Fatal("growing a shown surface did not force the request")
	}
	sf.SetMinSize(driver.Size{W: 50, H: 50})
	wantLog(t, "SetMinSize while forced", fw.take(), "setSizeRequest 800x300")
	if n := s.q.Drain(); n != 0 {
		t.Errorf("%d functions posted before the allocation", n)
	}
	fw.layout() // GTK keeps 600x400, at least the request: 800x400
	wantLog(t, "allocation reaching the request", h.take(), "resized 800x400@1")
	if sf.forced {
		t.Error("still forced after an allocation reached the request")
	}
	wantLog(t, "lowering inside the allocation", fw.take())
	if n := s.q.Drain(); n != 1 {
		t.Errorf("%d functions posted by the allocation, want 1", n)
	}
	wantLog(t, "posted lowering", fw.take(), "setSizeRequest 50x50")

	sf.SetSize(driver.Size{W: 500, H: 300}) // smaller
	wantLog(t, "SetSize shrinking", fw.take(), "setDefaultSize 500x300", "setSizeRequest 50x50", "queueResize")
	if sf.forced {
		t.Error("shrinking forced the request")
	}
	h.take()

	sf.SetSize(driver.Size{W: 400, H: 700}) // taller
	wantLog(t, "SetSize taller", fw.take(), "setDefaultSize 400x700", "setSizeRequest 400x700", "queueResize")
	fw.allocate(500, 650) // short of it
	if !sf.forced || s.q.Drain() != 0 {
		t.Errorf("an allocation short of the request: forced %v", sf.forced)
	}
	fw.allocate(500, 700)
	if sf.forced {
		t.Error("still forced")
	}
	// Closed before the posted lowering runs: it does nothing.
	sf.Close()
	if n := s.q.Drain(); n != 1 {
		t.Errorf("%d functions posted, want 1", n)
	}
	wantLog(t, "after Close", fw.take(), "destroy")
}

// TestSurfaceResizeSizeRequestStale: every SetSize decides the forced
// request again, against the last allocation (not the size reported):
// growing then shrinking before a layout leaves nothing forced, so the
// window does not grow to the abandoned size; growing less than an earlier
// SetSize but past the allocation forces the new size.
func TestSurfaceResizeSizeRequestStale(t *testing.T) {
	saved := mappedResize
	mappedResize = resizeSizeRequest
	t.Cleanup(func() { mappedResize = saved })
	_, f := newSurfaceTest(t)
	sf, fw := f.open(t, sfResizable, &sfHandler{})
	fw.ignoreDefault = true
	fw.layout()
	sf.Show()
	sf.SetSize(driver.Size{W: 800, H: 600})
	sf.SetSize(driver.Size{W: 300, H: 200}) // before any layout
	if sf.forced {
		t.Error("still forced after a SetSize within the allocation")
	}
	fw.take()
	fw.layout()
	if fw.allocW > 400 || fw.allocH > 300 {
		t.Errorf("after SetSize(800x600) then SetSize(300x200) the window grew to %dx%d (request %dx%d)",
			fw.allocW, fw.allocH, fw.reqW, fw.reqH)
	}
	sf.SetSize(driver.Size{W: 800, H: 600})
	sf.SetSize(driver.Size{W: 600, H: 250}) // smaller than 800x600, wider than the allocation
	wantLog(t, "SetSize past the allocation", fw.take(),
		"setDefaultSize 800x600", "setSizeRequest 800x600", "queueResize",
		"setDefaultSize 600x250", "setSizeRequest 600x250", "queueResize")
	fw.layout()
	if fw.allocW != 600 || fw.allocH != 300 || sf.forced {
		t.Errorf("allocated %dx%d, forced %v; want 600x300 and the force lifted", fw.allocW, fw.allocH, sf.forced)
	}
}

// TestSurfaceNewPanicDuringCreate: a callback that panicked while
// newWindow ran is re-raised by NewSurface after the window is destroyed:
// no surface, no window and no handle are left.
func TestSurfaceNewPanicDuringCreate(t *testing.T) {
	s, f := newSurfaceTest(t)
	base := dispatch.Len()
	h := &sfHandler{}
	f.during = func(fw *fakeWindow) {
		fw.allocate(500, 400)
		trapped("gtk4.test.create", func() { panic("while creating") })
	}
	var got any
	func() {
		defer func() { got = recover() }()
		_, _ = s.NewSurface(sfResizable, h)
	}()
	pe, ok := got.(*PanicError)
	if !ok || pe.Value != "while creating" || pe.Callback != "gtk4.test.create" {
		t.Fatalf("NewSurface panicked with %#v, want the trapped *PanicError", got)
	}
	fw := f.last(t)
	if !fw.destroyed || !fw.released {
		t.Errorf("the window was not destroyed (destroyed %v, released %v)", fw.destroyed, fw.released)
	}
	if len(s.surfaces) != 0 || pendingPanic != nil {
		t.Errorf("%d surfaces, pending panic %v", len(s.surfaces), pendingPanic)
	}
	checkLen(t, base)
	wantLog(t, "handler", h.take())
	wantLog(t, "window calls", fw.take(), "destroy")
	// newWindow failing with a pending panic: nothing to destroy, the
	// panic wins.
	f.err = errors.New("failed")
	func() {
		defer func() { got = recover() }()
		_, _ = s.NewSurface(sfResizable, h)
	}()
	if pe, ok := got.(*PanicError); !ok || pe.Value != "while creating" {
		t.Errorf("NewSurface with a failing newWindow and a panic: %#v", got)
	}
	if fw := f.last(t); fw.destroyed || !fw.released {
		t.Errorf("failed window: destroyed %v (want false: newWindow cleaned up), released %v", fw.destroyed, fw.released)
	}
	checkLen(t, base)
}

// TestSurfaceCloseRequestPanic: a panicking CloseRequested still stops
// GTK's own handling (TRUE, so GTK does not destroy the window behind the
// surface's back); the panic is kept for rethrow and the surface stays.
func TestSurfaceCloseRequestPanic(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{allow: true, onRequest: func() { panic("in CloseRequested") }}
	sf, fw := f.open(t, sfResizable, h)
	if r := sigBool(fw.addr, uintptr(fw.sig[kindCloseRequest])); r != gtk.True {
		t.Errorf("close-request with a panicking handler = %d, want TRUE", r)
	}
	pe, ok := rethrown().(*PanicError)
	if !ok || pe.Value != "in CloseRequested" || pe.Callback != nameSigBool {
		t.Fatalf("rethrow raised %#v, want the *PanicError of sigBool", pe)
	}
	if sf.dead || fw.destroyed || len(s.surfaces) != 1 {
		t.Error("the surface closed after a panicking CloseRequested")
	}
	// Another kind on sigBool still answers FALSE when its peer panics.
	hd := newEntry(t, &recPeer{panics: "other"}, kindActive)
	if r := sigBool(0, uintptr(hd)); r != gtk.False {
		t.Errorf("a panicking sigBool of another kind = %d, want FALSE", r)
	}
	if _, ok := rethrown().(*PanicError); !ok {
		t.Error("no panic kept for the other kind")
	}
}

// TestSurfaceQuitGuards: Quit panics during Paint; in a Resized handler
// that runs inside GTK's allocation it is posted, and closes the surfaces
// when the posted work runs.
func TestSurfaceQuitGuards(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	fw.layout() // the first allocation, at the asked size
	h.onPaint = func(render.Canvas) {
		mustPanicWith(t, "Quit during Paint", panicsWith("during SurfaceHandler.Paint"), s.Quit)
	}
	fw.snap()
	if s.quit || sf.dead {
		t.Fatal("Quit during Paint quit")
	}
	h.onPaint = nil
	h.onResized = func(driver.Size, float64) {
		s.Quit()
		if s.quit || sf.dead || fw.destroyed {
			t.Error("Quit inside the allocation ran at once")
		}
	}
	fw.allocate(500, 400)
	wantLog(t, "allocation", h.take(), "paint", "resized 500x400@1")
	if s.quit || sf.dead {
		t.Fatal("Quit inside the allocation ran before the posted work")
	}
	if n := s.q.Drain(); n != 1 {
		t.Errorf("%d functions posted, want 1 (Quit)", n)
	}
	if !s.quit || !sf.dead || !fw.destroyed {
		t.Errorf("after the posted Quit: quit %v, dead %v, destroyed %v", s.quit, sf.dead, fw.destroyed)
	}
	wantLog(t, "posted Quit", h.take(), "closed")
}

// TestSurfaceInLayout: in a Resized handler that runs inside GTK's
// allocation (Shell.inLayout), SetSize, SetMinSize and SetMaxSize keep and
// report their sizes at once but post their window calls, and Close is
// posted; posted window calls of a surface closed meanwhile are dropped.
// A panic in such a handler resets inLayout.
func TestSurfaceInLayout(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	fw.layout() // the first allocation, at the asked size
	fw.take()
	fw.isMax = true
	nested := 0
	h.onResized = func(size driver.Size, _ float64) {
		if s.inLayout != 1 {
			t.Errorf("Resized of an allocation with inLayout %d, want 1", s.inLayout)
		}
		if nested++; nested > 1 {
			return
		}
		sf.SetSize(driver.Size{W: 700, H: 500})
		if sf.Size() != (driver.Size{W: 700, H: 500}) {
			t.Errorf("Size() = %v right after SetSize in the allocation", sf.Size())
		}
		sf.SetMinSize(driver.Size{W: 100, H: 100})
		sf.SetMaxSize(driver.Size{W: 650})
		if calls := fw.take(); len(calls) != 0 {
			t.Errorf("window calls inside the allocation: %q", calls)
		}
	}
	fw.allocate(600, 450)
	wantLog(t, "allocation", h.take(), "resized 600x450@1", "resized 700x500@1", "resized 650x500@1")
	if s.inLayout != 0 {
		t.Errorf("inLayout %d after the allocation", s.inLayout)
	}
	if n := s.q.Drain(); n != 3 {
		t.Errorf("%d functions posted, want 3", n)
	}
	wantLog(t, "posted window calls", fw.take(),
		"setMaximized false", "setDefaultSize 700x500", "setSizeRequest 100x100", "queueResize", // the request when it runs
		"setSizeRequest 100x100",
		"setDefaultSize 650x500", "setSizeRequest 100x100", "queueResize")

	// Close is posted; a SetSize posted after it is dropped.
	h.onResized = func(driver.Size, float64) {
		h.onResized = nil
		sf.Close()
		if sf.dead {
			t.Error("Close inside the allocation ran at once")
		}
		sf.SetSize(driver.Size{W: 300, H: 300})
	}
	fw.allocate(640, 480)
	wantLog(t, "allocation closing", h.take(), "resized 640x480@1", "resized 300x300@1")
	if n := s.q.Drain(); n != 2 {
		t.Errorf("%d functions posted, want 2", n)
	}
	wantLog(t, "posted Close", h.take(), "closed")
	wantLog(t, "posted Close window calls", fw.take(), "destroy")

	// A panic in Resized inside the allocation.
	h2 := &sfHandler{onResized: func(driver.Size, float64) { panic("in Resized") }}
	sf2, fw2 := f.open(t, sfResizable, h2)
	surfaceSizeAllocate(fw2.addr, 800, 600, -1)
	pe, ok := rethrown().(*PanicError)
	if !ok || pe.Value != "in Resized" || pe.Callback != nameSizeAllocate {
		t.Errorf("rethrow raised %#v, want the *PanicError of size_allocate", pe)
	}
	if s.inLayout != 0 {
		t.Errorf("inLayout %d after a panic in Resized", s.inLayout)
	}
	// Outside an allocation (SetSize's own Resized) Close runs at once.
	h2.onResized = func(driver.Size, float64) {
		h2.onResized = nil
		sf2.Close()
	}
	sf2.SetSize(driver.Size{W: 10, H: 10})
	if !fw2.destroyed {
		t.Error("Close in SetSize's Resized did not run at once")
	}
}

// TestSurfaceShowPacing: Show presents the window; a shown surface paces
// the frame clock from its widget, and a pending paint requests a frame.
func TestSurfaceShowPacing(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	if sf.tickWidget() != 0 || s.pacingWidget() != 0 {
		t.Error("a hidden surface paces")
	}
	sf.Show()
	wantLog(t, "Show", fw.take(), "present")
	if !sf.shown || sf.tickWidget() != fw.addr || s.pacingWidget() != fw.addr {
		t.Errorf("after Show: shown %v, tickWidget %#x, pacingWidget %#x; want the widget %#x",
			sf.shown, sf.tickWidget(), s.pacingWidget(), fw.addr)
	}
	if !s.clock.requested || s.clock.tickWidget != fw.addr {
		t.Errorf("Show of an invalid surface: frame requested %v, tick on %#x", s.clock.requested, s.clock.tickWidget)
	}
	f.frame(t)
	wantLog(t, "the frame's paint pass", fw.take(), "queueDraw")
	fw.snap()
	wantLog(t, "the snapshot", h.take(), "paint")
	if sf.invalid {
		t.Error("still invalid after the snapshot")
	}
	sf.Show()
	wantLog(t, "Show again", fw.take(), "present")
	if s.clock.requested {
		t.Error("Show of a valid surface requested a frame")
	}

	// A valid surface shown while a frame is pending on the fallback
	// timeout: the request moves to its widget.
	b, fb := f.open(t, sfResizable, &sfHandler{})
	fb.snap() // painted: valid
	sf.Close()
	s.clock.RequestFrame()
	if s.clock.tickWidget != 0 || s.clock.timeoutID == 0 {
		t.Fatalf("no fallback timeout pending (tick on %#x)", s.clock.tickWidget)
	}
	if b.invalid {
		t.Fatal("the second surface is still invalid")
	}
	b.Show()
	if s.clock.tickWidget != fb.addr || !s.clock.stall {
		t.Errorf("Show of a valid surface with a frame pending: tick on %#x (want %#x), stall %v",
			s.clock.tickWidget, fb.addr, s.clock.stall)
	}
}

// TestSurfaceMinimizeZoom: Minimize and Zoom do nothing for a hidden
// surface (Zoom also without StyleResizable, Minimize for a minimized
// one); Zoom toggles maximized, which Zoomed reports.
func TestSurfaceMinimizeZoom(t *testing.T) {
	_, f := newSurfaceTest(t)
	sf, fw := f.open(t, sfResizable, &sfHandler{})
	sf.Minimize()
	sf.Zoom()
	wantLog(t, "hidden", fw.take())
	if sf.Zoomed() {
		t.Error("Zoomed() before any Zoom")
	}
	sf.Show()
	fw.take()
	sf.Zoom()
	if !sf.Zoomed() {
		t.Error("Zoomed() false after Zoom")
	}
	sf.Zoom()
	if sf.Zoomed() {
		t.Error("Zoomed() true after a second Zoom")
	}
	sf.Minimize()
	wantLog(t, "shown", fw.take(), "setMaximized true", "setMaximized false", "minimize")
	fw.isMin = true
	fw.notify(kindState)
	sf.Minimize()
	wantLog(t, "Minimize when minimized (GTK decides)", fw.take(), "minimize")
	sf.Center()
	wantLog(t, "Center", fw.take())
	sf.Close()
	if sf.Zoomed() {
		t.Error("Zoomed() true after Close")
	}
}

// TestSurfaceClose: Close reports Closed once, removes the surface from
// the shell, moves the clock off its widget before the window is
// destroyed, frees the window's handles, and leaves every method a no-op.
func TestSurfaceClose(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	other, _ := f.open(t, sfResizable, &sfHandler{})
	sf.Show()
	if s.clock.tickWidget != fw.addr {
		t.Fatalf("the clock ticks on %#x, want the shown surface's widget", s.clock.tickWidget)
	}
	h.onClosed = func() {
		sf.Close() // re-entrant: nothing
		sf.SetSize(driver.Size{W: 1, H: 1})
	}
	fw.take()
	sf.Close()
	wantLog(t, "Close", h.take(), "closed")
	wantLog(t, "Close window calls", fw.take(), "destroy")
	if len(s.surfaces) != 1 || s.surfaces[0] != openSurface(other) {
		t.Errorf("Shell.surfaces = %v, want only the other surface", s.surfaces)
	}
	for _, hd := range fw.handles() {
		if _, ok := dispatch.Get(hd); ok {
			t.Errorf("handle %v still live after Close", hd)
		}
	}
	if s.clock.tickWidget != 0 || len(f.l.ticks) != 0 || !s.clock.requested || s.clock.timeoutID == 0 || s.clock.stall {
		t.Errorf("clock after Close: tick on %#x, %d ticks, requested %v, timeout %d, stall %v; want the fallback timeout",
			s.clock.tickWidget, len(f.l.ticks), s.clock.requested, s.clock.timeoutID, s.clock.stall)
	}

	size := sf.Size()
	sf.SetTitle("x")
	sf.SetSize(driver.Size{W: 10, H: 10})
	sf.SetMinSize(driver.Size{W: 10, H: 10})
	sf.SetMaxSize(driver.Size{W: 10, H: 10})
	sf.Center()
	sf.Show()
	sf.Minimize()
	sf.Zoom()
	sf.Close()
	sf.Invalidate()
	sf.SetCursor(shell.CursorIBeam)
	sf.SetChromeRegions([]shell.ChromeRegion{{Kind: shell.RegionCaption}})
	if sf.Zoomed() || sf.Size() != size || sf.tickWidget() != 0 {
		t.Errorf("after Close: Zoomed %v, Size %v (was %v), tickWidget %#x", sf.Zoomed(), sf.Size(), size, sf.tickWidget())
	}
	// GTK's side after Close: stale handles, and the peer methods
	// themselves.
	if r := sigBool(fw.addr, uintptr(fw.sig[kindCloseRequest])); r != gtk.False {
		t.Errorf("close-request with a stale handle = %d, want FALSE", r)
	}
	fw.allocate(10, 10)
	fw.snap()
	fw.notify(kindScale)
	sf.sizeAllocate(20, 20, -1)
	sf.snapshot(testSnapshot)
	if !sf.boolSignal(kindCloseRequest, 0) {
		t.Error("boolSignal after Close returned false")
	}
	for _, k := range []kind{kindActive, kindScale, kindState} {
		sf.pointerSignal(k, 0, 0)
	}
	wantLog(t, "after Close", h.take())
	wantLog(t, "window calls after Close", fw.take())
}

// TestSurfaceCloseRequest: close-request asks CloseRequested; false keeps
// the surface, true closes it (Closed once, also if the handler closes it
// itself); GTK's own handling is always stopped.
func TestSurfaceCloseRequest(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	if r := fw.closeRequest(); r != gtk.True {
		t.Errorf("close-request refused returned %d, want TRUE", r)
	}
	wantLog(t, "refused", h.take(), "closeRequested")
	if sf.dead || fw.destroyed || len(s.surfaces) != 1 {
		t.Fatal("a refused close-request closed the surface")
	}
	h.allow = true
	if r := fw.closeRequest(); r != gtk.True {
		t.Errorf("close-request agreed returned %d, want TRUE", r)
	}
	wantLog(t, "agreed", h.take(), "closeRequested", "closed")
	if !sf.dead || !fw.destroyed || len(s.surfaces) != 0 {
		t.Error("an agreed close-request did not close the surface")
	}

	h2 := &sfHandler{allow: true}
	sf2, fw2 := f.open(t, sfResizable, h2)
	h2.onRequest = func() { sf2.Close() }
	if r := fw2.closeRequest(); r != gtk.True {
		t.Errorf("close-request returned %d, want TRUE", r)
	}
	wantLog(t, "closed in CloseRequested", h2.take(), "closeRequested", "closed")

	h3 := &sfHandler{allow: true}
	sf3, _ := f.open(t, sfResizable, h3)
	if !sf3.boolSignal(kindActive, 0) {
		t.Error("boolSignal of another kind returned false")
	}
	wantLog(t, "boolSignal of another kind", h3.take())
}

// TestSurfaceQuitClosesAll: Quit closes the surfaces last to first with
// Closed and destroys their windows; NewSurface in Closed fails.
func TestSurfaceQuitClosesAll(t *testing.T) {
	s, f := newSurfaceTest(t)
	var order []string
	var newErr error
	ha := &sfHandler{onClosed: func() { order = append(order, "a") }}
	a, fa := f.open(t, sfResizable, ha)
	hb := &sfHandler{}
	_, fb := f.open(t, sfResizable, hb)
	hb.onClosed = func() {
		order = append(order, "b")
		_, newErr = s.NewSurface(sfResizable, &sfHandler{})
		a.SetTitle("still open")
	}
	s.Quit()
	if !slices.Equal(order, []string{"b", "a"}) {
		t.Errorf("closed %v, want [b a]", order)
	}
	if !errors.Is(newErr, ErrQuit) {
		t.Errorf("NewSurface in Closed = %v, want ErrQuit", newErr)
	}
	if !fa.destroyed || !fb.destroyed || len(s.surfaces) != 0 {
		t.Errorf("after Quit: destroyed %v %v, %d surfaces", fa.destroyed, fb.destroyed, len(s.surfaces))
	}
	wantLog(t, "a", fa.take(), "setTitle still open", "destroy")
}

// TestSurfaceRunQuitAndPanic: in Run, Quit closes the surfaces with
// Closed; a panic closes them silently in the teardown. No handle is left.
func TestSurfaceRunQuitAndPanic(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(fmt.Sprint("panic=", panics), func(t *testing.T) {
			s, l := newTestShell(t)
			withFakeClass(t, nil)
			base := dispatch.Len()
			f := &winFactory{t: t, s: s, scale: 1}
			l.newWin = f.newWindow
			t.Cleanup(f.finish)
			h := &sfHandler{}
			ready := func() {
				for range 2 {
					if _, err := s.NewSurface(sfResizable, h); err != nil {
						t.Error(err)
					}
				}
				if panics {
					panic("in ready")
				}
				s.Post(s.Quit)
			}
			if panics {
				if got := runPanics(t, s, &handler{ready: ready}); got != "in ready" {
					t.Errorf("Run panicked with %v", got)
				}
				wantLog(t, "panic teardown", h.take())
			} else {
				if err := run(t, s, &handler{ready: ready}); err != nil {
					t.Fatal(err)
				}
				wantLog(t, "Quit", h.take(), "closed", "closed")
			}
			for i, fw := range f.wins {
				if !fw.destroyed {
					t.Errorf("window %d not destroyed", i)
				}
			}
			checkTornDown(t, s, l, base)
		})
	}
}

// TestSurfaceKeyChanged: notify::is-active reports changes of the key
// state only.
func TestSurfaceKeyChanged(t *testing.T) {
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	_, fw := f.open(t, sfResizable, h)
	fw.notify(kindActive)
	fw.isActive = true
	fw.notify(kindActive)
	fw.notify(kindActive)
	fw.isActive = false
	fw.notify(kindActive)
	wantLog(t, "KeyChanged", h.take(), "key true", "key false")
}

// TestSurfaceScaleChanged: a scale change reports Resized with the new
// scale and invalidates; an unchanged or invalid scale is ignored.
func TestSurfaceScaleChanged(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	sf.invalid = false
	fw.sc = 1.25
	fw.notify(kindScale)
	wantLog(t, "scale change", h.take(), "resized 400x300@1.25")
	if sf.Scale() != 1.25 || !sf.invalid || !s.clock.requested {
		t.Errorf("after the scale change: Scale %v, invalid %v, requested %v", sf.Scale(), sf.invalid, s.clock.requested)
	}
	fw.notify(kindScale)
	for _, bad := range []float64{0, -1, math.NaN()} {
		fw.sc = bad
		fw.notify(kindScale)
	}
	wantLog(t, "unchanged or invalid scales", h.take())
	if sf.Scale() != 1.25 {
		t.Errorf("Scale() = %v, want 1.25", sf.Scale())
	}
	fw.sc = 2
	h.onResized = func(driver.Size, float64) { sf.Close() }
	fw.notify(kindScale)
	wantLog(t, "scale change closing", h.take(), "resized 400x300@2", "closed")
}

// TestSurfaceMinimizedPacing: a minimized surface stops pacing (the clock
// falls back to its timeout); restored, it paces again, and a pending
// paint requests a frame.
func TestSurfaceMinimizedPacing(t *testing.T) {
	s, f := newSurfaceTest(t)
	sf, fw := f.open(t, sfResizable, &sfHandler{})
	sf.Show()
	if s.clock.tickWidget != fw.addr {
		t.Fatal("the shown surface does not pace")
	}
	fw.isMin = true
	fw.notify(kindState)
	if !sf.minimized || sf.tickWidget() != 0 || s.pacingWidget() != 0 {
		t.Errorf("minimized: minimized %v, tickWidget %#x, pacingWidget %#x", sf.minimized, sf.tickWidget(), s.pacingWidget())
	}
	if s.clock.tickWidget != 0 || len(f.l.ticks) != 0 || s.clock.timeoutID == 0 || s.clock.stall {
		t.Errorf("minimized: the clock did not fall back to its timeout (tick on %#x, timeout %d, stall %v)",
			s.clock.tickWidget, s.clock.timeoutID, s.clock.stall)
	}
	f.frame(t)
	fw.take()
	if s.clock.requested || !sf.invalid {
		t.Fatalf("after the frame: requested %v, invalid %v; want false, true", s.clock.requested, sf.invalid)
	}
	fw.isMin = false
	fw.notify(kindState)
	if sf.minimized || s.pacingWidget() != fw.addr {
		t.Errorf("restored: minimized %v, pacingWidget %#x", sf.minimized, s.pacingWidget())
	}
	if !s.clock.requested || s.clock.tickWidget != fw.addr {
		t.Errorf("restored with a pending paint: requested %v, tick on %#x", s.clock.requested, s.clock.tickWidget)
	}
	f.frame(t)
	wantLog(t, "frame after restore", fw.take(), "queueDraw")
	fw.snap()
	// Restored without a pending paint: no frame.
	fw.isMin = true
	fw.notify(kindState)
	fw.isMin = false
	fw.notify(kindState)
	if s.clock.requested {
		t.Error("restoring a valid surface requested a frame")
	}
}

// TestSurfacePaintPass: the frame's paint pass queues a draw of the
// invalid, shown surfaces only.
func TestSurfacePaintPass(t *testing.T) {
	s, f := newSurfaceTest(t)
	a, fa := f.open(t, sfResizable, &sfHandler{})
	b, fb := f.open(t, sfResizable, &sfHandler{})
	c, fc := f.open(t, sfResizable, &sfHandler{})
	a.Show()
	c.Show()
	f.frame(t)
	if fa.draws != 1 || fb.draws != 0 || fc.draws != 1 {
		t.Errorf("draws %d %d %d, want 1 0 1", fa.draws, fb.draws, fc.draws)
	}
	fa.snap()
	fc.snap()
	a.Invalidate()
	b.Invalidate()
	if !s.clock.requested {
		t.Fatal("Invalidate requested no frame")
	}
	f.frame(t)
	if fa.draws != 2 || fb.draws != 0 || fc.draws != 1 {
		t.Errorf("draws %d %d %d, want 2 0 1", fa.draws, fb.draws, fc.draws)
	}
	if !b.invalid {
		t.Error("a hidden surface lost its pending paint")
	}
	// Invalidating a closed surface does nothing.
	c.Close()
	c.Invalidate()
	if c.invalid {
		t.Error("Invalidate after Close marked the surface")
	}
}

// TestSurfaceSnapshotPaint: every snapshot calls Paint with the
// canvas over the snapshot GTK passed, clipped to the surface and at its
// scale, and clears the pending paint; the canvas draws the shell's text
// system's layouts (S4.2i), releasing their native layouts at the end of
// the paint; the snapshot is balanced after any paint; with debugChecks
// the recorder's errors are kept; Invalidate in Paint keeps it pending.
func TestSurfaceSnapshotPaint(t *testing.T) {
	old := debugChecks
	debugChecks = true
	t.Cleanup(func() { debugChecks = old })
	s, f := newSurfaceTest(t)
	pl := &txLayouter{cw: 7, lh: 16}
	s.text.open = func() (pangoLayouter, error) { return pl, nil }
	h := &sfHandler{}
	cfg := sfResizable
	cfg.Size = driver.Size{W: 400.5, H: 300}
	sf, fw := f.open(t, cfg, h)
	sf.scale = 2
	var painting *surface
	h.onPaint = func(c render.Canvas) {
		painting = s.painting
		c.FillRoundedRect(render.Rect{W: 1, H: 1}, render.Radii{}, render.Solid(driver.RGBA(1, 0, 0, 1)))
		c.DrawText(s.TextSystem().NewLayout("ab", render.Font{Size: 13}, 0), render.Point{X: 2, Y: 3}, driver.RGBA(0, 0, 0, 1))
		if _, ok := c.(*canvas); !ok {
			t.Errorf("Paint got a %T", c)
		}
	}
	fw.snap()
	if h.paints != 1 || sf.paints != 1 || sf.invalid || sf.paintErr != nil || s.painting != nil || painting != sf {
		t.Errorf("after a snapshot: %d/%d paints, invalid %v, error %v, painting %v (in Paint %v)",
			h.paints, sf.paints, sf.invalid, sf.paintErr, s.painting, painting)
	}
	if len(fw.snaps) != 1 {
		t.Fatalf("%d snapshotters, want 1", len(fw.snaps))
	}
	sn := fw.snaps[0]
	want := []string{"clip (0,0,400.5,300)", "color (0,0,1,1) rgba(1,0,0,1)",
		"save", "translate 2,3", "text #1 rgba(0,0,0,1)", "restore", "pop"}
	if !slices.Equal(sn.ops, want) {
		t.Errorf("snapshot ops %q, want the surface clip, the fill, the text at (2,3), pop: %q", sn.ops, want)
	}
	if len(pl.layouts) != 1 || pl.layouts[0].released != 1 || pl.layouts[0].args.text != "ab" {
		t.Errorf("native layouts %+v, want one of \"ab\", released once by the paint", pl.layouts)
	}
	if err := sn.balanced(); err != nil {
		t.Error(err)
	}
	h.onPaint = func(c render.Canvas) {
		c.Save()
		c.PushOpacity(0.5)
		c.ClipRoundedRect(render.Rect{W: 5, H: 5}, render.Radii{})
	}
	fw.snap()
	if sf.paintErr == nil || !strings.Contains(sf.paintErr.Error(), "not closed") {
		t.Errorf("paint error of an unbalanced Save = %v", sf.paintErr)
	}
	if err := fw.snaps[len(fw.snaps)-1].balanced(); err != nil {
		t.Errorf("after an unbalanced paint: %v", err)
	}
	h.onPaint = func(render.Canvas) { sf.Invalidate() }
	fw.snap()
	if !sf.invalid || !s.clock.requested || sf.paintErr != nil {
		t.Errorf("Invalidate in Paint: invalid %v, requested %v, error %v", sf.invalid, s.clock.requested, sf.paintErr)
	}
	if h.paints != 3 {
		t.Errorf("%d paints, want 3", h.paints)
	}
}

// TestSurfacePaintGuard: the methods that change a surface panic during
// Paint (of any surface), the others work.
func TestSurfacePaintGuard(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	other, fo := f.open(t, sfResizable, &sfHandler{})
	sf.Show()
	fw.take()
	sz := driver.Size{W: 10, H: 10}
	h.onPaint = func(render.Canvas) {
		for name, fn := range map[string]func(){
			"SetSize":       func() { sf.SetSize(sz) },
			"SetMinSize":    func() { sf.SetMinSize(sz) },
			"SetMaxSize":    func() { sf.SetMaxSize(sz) },
			"Center":        func() { sf.Center() },
			"Show":          func() { sf.Show() },
			"Minimize":      func() { sf.Minimize() },
			"Zoom":          func() { sf.Zoom() },
			"Close":         func() { sf.Close() },
			"NewSurface":    func() { _, _ = s.NewSurface(sfResizable, &sfHandler{}) },
			"other.SetSize": func() { other.SetSize(sz) },
			"other.Close":   func() { other.Close() },
		} {
			mustPanicWith(t, name+" during Paint", panicsWith("during SurfaceHandler.Paint"), fn)
		}
		sf.SetTitle("in paint")
		sf.Invalidate()
		sf.SetCursor(shell.CursorIBeam)
		sf.SetChromeRegions([]shell.ChromeRegion{{Kind: shell.RegionCaption}})
		_, _, _ = sf.Size(), sf.Scale(), sf.Zoomed()
		other.Invalidate()
		other.SetTitle("other")
	}
	nwins := len(f.wins)
	fw.snap()
	if s.painting != nil {
		t.Error("s.painting set after Paint")
	}
	if len(f.wins) != nwins || fw.destroyed || fo.destroyed || sf.dead || other.dead {
		t.Error("a guarded method ran during Paint")
	}
	wantLog(t, "window calls in Paint", fw.take(), "setTitle in paint", "setCursor text")
	wantLog(t, "other window calls in Paint", fo.take(), "setTitle other")
	if !sf.invalid || len(sf.regions) != 1 || sf.Size() != (driver.Size{W: 400, H: 300}) {
		t.Errorf("after Paint: invalid %v, %d regions, Size %v", sf.invalid, len(sf.regions), sf.Size())
	}
	h.onPaint = nil
	sf.SetSize(sz) // the guard is off again
	wantLog(t, "after Paint", h.take(), "paint", "resized 10x10@1")
}

// TestSurfacePaintPanic: a panic in Paint is trapped by the snapshot
// trampoline and re-raised by rethrow, with the paint state reset.
func TestSurfacePaintPanic(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	h.onPaint = func(c render.Canvas) {
		c.Save()
		panic("boom")
	}
	surfaceSnapshot(fw.addr, testSnapshot)
	if s.painting != nil {
		t.Error("s.painting still set after a panic in Paint")
	}
	pe, ok := rethrown().(*PanicError)
	if !ok || pe.Value != "boom" || pe.Callback != nameSnapshot {
		t.Fatalf("rethrow raised %#v, want the *PanicError of the snapshot", pe)
	}
	if sf.paints != 0 {
		t.Errorf("%d paints counted for a panicking Paint", sf.paints)
	}
	h.onPaint = nil
	sf.SetSize(driver.Size{W: 500, H: 400}) // not guarded any more
	fw.snap()
	if sf.paints != 1 {
		t.Errorf("%d paints after a good snapshot", sf.paints)
	}
}

// TestSurfaceCursorTitleRegions: the cursors' CSS names, SetCursor skips
// an unchanged cursor, SetTitle drops NULs, SetChromeRegions copies.
func TestSurfaceCursorTitleRegions(t *testing.T) {
	for c, want := range map[shell.Cursor]string{
		shell.CursorArrow: "default", shell.CursorIBeam: "text", shell.CursorPointingHand: "pointer",
		shell.CursorResizeLeftRight: "ew-resize", shell.CursorResizeUpDown: "ns-resize",
		shell.CursorCrosshair: "crosshair", shell.CursorNotAllowed: "not-allowed", shell.CursorHidden: "none",
		shell.CursorHidden + 1: "default", 255: "default",
	} {
		if got := cursorName(c); got != want {
			t.Errorf("cursorName(%d) = %q, want %q", c, got, want)
		}
	}
	_, f := newSurfaceTest(t)
	sf, fw := f.open(t, sfResizable, &sfHandler{})
	sf.SetCursor(shell.CursorArrow) // the initial cursor
	sf.SetCursor(shell.CursorIBeam)
	sf.SetCursor(shell.CursorIBeam)
	sf.SetCursor(shell.CursorHidden)
	sf.SetCursor(shell.CursorArrow)
	sf.SetCursor(42)
	sf.SetTitle("a\x00b")
	wantLog(t, "window calls", fw.take(),
		"setCursor text", "setCursor none", "setCursor default", "setCursor default", "setTitle ab")
	regions := []shell.ChromeRegion{{Rect: driver.Rect{W: 100, H: 28}, Kind: shell.RegionCaption}, {Kind: shell.RegionClose}}
	sf.SetChromeRegions(regions)
	regions[0].Kind = shell.RegionClient
	if len(sf.regions) != 2 || sf.regions[0].Kind != shell.RegionCaption || sf.regions[0].Rect.W != 100 {
		t.Errorf("regions = %v, want a copy", sf.regions)
	}
	sf.SetChromeRegions(nil)
	if len(sf.regions) != 0 {
		t.Errorf("regions = %v after SetChromeRegions(nil)", sf.regions)
	}
}

// TestSurfaceUIThread: every method panics off the UI thread, before it
// touches the window.
func TestSurfaceUIThread(t *testing.T) {
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	sz := driver.Size{W: 10, H: 10}
	methods := map[string]func(){
		"SetTitle":         func() { sf.SetTitle("x") },
		"Size":             func() { sf.Size() },
		"SetSize":          func() { sf.SetSize(sz) },
		"SetMinSize":       func() { sf.SetMinSize(sz) },
		"SetMaxSize":       func() { sf.SetMaxSize(sz) },
		"Scale":            func() { sf.Scale() },
		"Center":           func() { sf.Center() },
		"Show":             func() { sf.Show() },
		"Minimize":         func() { sf.Minimize() },
		"Zoom":             func() { sf.Zoom() },
		"Zoomed":           func() { sf.Zoomed() },
		"Close":            func() { sf.Close() },
		"Invalidate":       func() { sf.Invalidate() },
		"SetCursor":        func() { sf.SetCursor(shell.CursorIBeam) },
		"SetChromeRegions": func() { sf.SetChromeRegions(nil) },
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for name, fn := range methods {
			mustPanicWith(t, name+" off the UI thread", func(v any) bool {
				msg, _ := v.(string)
				return strings.Contains(msg, "gtk4.Surface."+name) && strings.Contains(msg, "off the UI thread")
			}, fn)
		}
	}()
	<-done
	wantLog(t, "window calls", fw.take())
	wantLog(t, "handler calls", h.take())
	if sf.dead {
		t.Error("closed off the UI thread")
	}
}

// TestSurfaceFirstAllocation: a first allocation that misses the asked
// size (the window's own CSS padding, solid-csd) is reported, and the
// default size is set again once, posted; the next layout reports the
// asked size. A resize or a maximize in between cancels the correction,
// and a first allocation at the asked size posts nothing.
func TestSurfaceFirstAllocation(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	fw.take()
	fw.allocate(390, 290)
	wantLog(t, "short first allocation", h.take(), "resized 390x290@1")
	if n := s.q.Drain(); n != 1 {
		t.Fatalf("%d functions posted, want 1 (the correction)", n)
	}
	wantLog(t, "correction", fw.take(), "setDefaultSize 400x300", "queueResize")
	fw.layout()
	wantLog(t, "corrected layout", h.take(), "resized 400x300@1")
	fw.allocate(380, 280) // a later allocation is the user's: no correction
	if n := s.q.Drain(); n != 0 {
		t.Errorf("%d functions posted after a later allocation, want 0", n)
	}
	h.take()

	sf2, fw2 := f.open(t, sfResizable, h)
	fw2.allocate(390, 290)
	sf2.SetSize(driver.Size{W: 500, H: 400})
	fw2.take()
	s.q.Drain()
	if calls := fw2.take(); slices.Contains(calls, "setDefaultSize 400x300") {
		t.Errorf("correction ran after a SetSize: %q", calls)
	}

	_, fw3 := f.open(t, sfResizable, h)
	fw3.allocate(390, 290)
	fw3.isMax = true
	fw3.take()
	s.q.Drain()
	if calls := fw3.take(); slices.Contains(calls, "setDefaultSize 400x300") {
		t.Errorf("correction ran on a maximized window: %q", calls)
	}

	_, fw4 := f.open(t, sfResizable, h)
	fw4.allocate(400, 300)
	if n := s.q.Drain(); n != 0 {
		t.Errorf("%d functions posted after an exact first allocation, want 0", n)
	}
	_ = sf
}

// The pointer controllers' signals, through the real trampolines; ev sets
// the event's time, modifier state and (for the gesture) GDK button.
type fakeEv struct {
	t      uint32
	state  uint32
	button uint32
}

func (fw *fakeWindow) setEv(ev fakeEv) {
	fw.evTime, fw.evState, fw.evButton = ev.t, ev.state, ev.button
}

func (fw *fakeWindow) ptrEnter(x, y float64, ev fakeEv) {
	fw.setEv(ev)
	sigDoubleDouble(testMotion, x, y, uintptr(fw.sig[kindPointerEnter]))
	rethrow()
}

func (fw *fakeWindow) ptrMotion(x, y float64, ev fakeEv) {
	fw.setEv(ev)
	sigDoubleDouble(testMotion, x, y, uintptr(fw.sig[kindPointerMotion]))
	rethrow()
}

func (fw *fakeWindow) ptrLeave(ev fakeEv) {
	fw.setEv(ev)
	sigVoid(testMotion, uintptr(fw.sig[kindPointerLeave]))
	rethrow()
}

func (fw *fakeWindow) ptrPressed(n int32, x, y float64, ev fakeEv) {
	fw.setEv(ev)
	sigIntDoubleDouble(testPointer, n, x, y, uintptr(fw.sig[kindPointerPressed]))
	rethrow()
}

func (fw *fakeWindow) ptrReleased(n int32, x, y float64, ev fakeEv) {
	fw.setEv(ev)
	sigIntDoubleDouble(testPointer, n, x, y, uintptr(fw.sig[kindPointerReleased]))
	rethrow()
}

func (fw *fakeWindow) ptrCancel(ev fakeEv) {
	fw.setEv(ev)
	sigPointer(testPointer, 0x5e9, uintptr(fw.sig[kindPointerCancel]))
	rethrow()
}

// scrollLog formats a ScrollEvent as "scroll dx,dy" with " precise", the
// phase (" began", " changed", " ended"), the momentum phase (" mom-…"),
// " at x,y" for a position other than 0,0 and " m=mods".
func scrollLog(se shell.ScrollEvent) string {
	phases := [...]string{"", "began", "changed", "ended"}
	s := fmt.Sprintf("scroll %v,%v", se.Delta.X, se.Delta.Y)
	if se.Precise {
		s += " precise"
	}
	if se.Phase != shell.PhaseNone {
		s += " " + phases[se.Phase]
	}
	if se.Momentum != shell.PhaseNone {
		s += " mom-" + phases[se.Momentum]
	}
	if se.Pos != (driver.Point{}) {
		s += fmt.Sprintf(" at %v,%v", se.Pos.X, se.Pos.Y)
	}
	if se.Mods != 0 {
		s += fmt.Sprintf(" m=%d", se.Mods)
	}
	return s
}

func (fw *fakeWindow) scrollUnit(c uintptr) uint32 {
	fw.use("scrollUnit")
	if c != testScroll {
		fw.f.errorf("scrollUnit(%#x), want the scroll controller", c)
	}
	return fw.unit
}

// The scroll controller's signals, through the real trampolines. scrollBy
// returns the "scroll" handler's gboolean.
func (fw *fakeWindow) scrollBy(dx, dy float64, unit uint32, ev fakeEv) int32 {
	fw.setEv(ev)
	fw.unit = unit
	r := sigBoolDoubleDouble(testScroll, dx, dy, uintptr(fw.sig[kindScroll]))
	rethrow()
	return r
}

func (fw *fakeWindow) scrollBegin(ev fakeEv) {
	fw.setEv(ev)
	sigVoid(testScroll, uintptr(fw.sig[kindScrollBegin]))
	rethrow()
}

func (fw *fakeWindow) scrollEnd(ev fakeEv) {
	fw.setEv(ev)
	sigVoid(testScroll, uintptr(fw.sig[kindScrollEnd]))
	rethrow()
}

func (fw *fakeWindow) decelerate(vx, vy float64) {
	sigDoubleDouble(testScroll, vx, vy, uintptr(fw.sig[kindScrollDecelerate]))
	rethrow()
}

func (fw *fakeWindow) keyBase(c uintptr, keycode uint32) uint32 {
	fw.use("keyBase")
	if c != testKey {
		fw.f.errorf("keyBase(%#x), want the key controller", c)
	}
	return fw.base[keycode]
}

// keyvalToUnicode is gdk_keyval_to_unicode for Latin-1 and the Unicode
// keyvals (0x01000000 + code point).
func (fw *fakeWindow) keyvalToUnicode(kv uint32) rune { return testKeyvalToUnicode(kv) }

func testKeyvalToUnicode(kv uint32) rune {
	switch {
	case kv >= 0x20 && kv <= 0x7e, kv >= 0xa0 && kv <= 0xff:
		return rune(kv)
	case kv >= 0x01000100 && kv <= 0x0110ffff:
		return rune(kv - 0x01000000)
	}
	return 0
}

// imFilter takes a key-down whose keyval is in imCommits and commits its
// text (if any) through the real "commit" trampoline; key-ups are logged.
func (fw *fakeWindow) imFilter(c uintptr) bool {
	fw.use("imFilter")
	if c != testKey {
		fw.f.errorf("imFilter(%#x), want the key controller", c)
	}
	if !fw.evDown {
		fw.ims = append(fw.ims, fmt.Sprintf("filter-up %#x", fw.evKeyval))
		return false
	}
	text, ok := fw.imCommits[fw.evKeyval]
	fw.ims = append(fw.ims, fmt.Sprintf("filter %#x %v", fw.evKeyval, ok))
	if ok && text != "" {
		testCommits[0x7e57] = text
		sigPointer(testIM, 0x7e57, uintptr(fw.sig[kindIMCommit]))
	}
	return ok
}

// testCommits are the C strings of the fake commits, by address.
var testCommits = map[uintptr]string{}

func (fw *fakeWindow) commitString(p uintptr) string { fw.use("commitString"); return testCommits[p] }
func (fw *fakeWindow) imReset()                      { fw.use("imReset"); fw.ims = append(fw.ims, "reset") }
func (fw *fakeWindow) imFocus(in bool) {
	fw.use("imFocus")
	fw.ims = append(fw.ims, fmt.Sprintf("focus %v", in))
}

// keyPress and keyRelease are the key controller's signals, through the
// real trampolines; keyPress returns the "key-pressed" gboolean.
func (fw *fakeWindow) keyPress(kv, keycode, state, t uint32) int32 {
	fw.evTime, fw.evKeyval, fw.evDown = t, kv, true
	r := sigBoolUintUintUint(testKey, kv, keycode, state, uintptr(fw.sig[kindKeyPressed]))
	rethrow()
	return r
}

func (fw *fakeWindow) keyRelease(kv, keycode, state, t uint32) {
	fw.evTime, fw.evKeyval, fw.evDown = t, kv, false
	sigUintUintUint(testKey, kv, keycode, state, uintptr(fw.sig[kindKeyReleased]))
	rethrow()
}

// imCommit is a commit outside a key event (a sequence that ends on a
// release, or the input method on its own).
func (fw *fakeWindow) imCommit(text string) {
	testCommits[0x7e58] = text
	sigPointer(testIM, 0x7e58, uintptr(fw.sig[kindIMCommit]))
	rethrow()
}
