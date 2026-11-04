package gtk4

import (
	"log"
	"strings"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// The surface (S4.1f): a GtkWindow with client-side decorations and no
// visible GTK title bar, whose only child is a GGSurface. GTK keeps its
// shadow and resize borders; the aqua driver draws everything inside, the
// title bar included. GTK owns the window (its toplevel list holds it);
// the surface holds one reference to the GGSurface and destroys the window
// itself (gtk_window_destroy), also when the user closes it, so it always
// knows when the window goes.
//
// Like the Shell, the surface is untagged: it drives the native side
// through the window interface (window_linux.go over GTK, a fake in the
// host tests), and GTK reaches it as the windowPeer of the GGSurface's
// vfuncs and of the window's signals.
//
// Sizes. GTK's unit is the logical pixel, which is the point: whole
// numbers, at any scale. The drawable size is the GGSurface's allocation;
// want remembers the size asked for last, so a fractional size reads back
// unchanged (shellkit.FromPx at scale 1). The minimum is the GGSurface's
// size request (a non-resizable surface requests its size). GTK 4 has no
// maximum-size hint: the maximum clamps SetSize and SetMaxSize, but the
// user can still make a resizable window larger.
//
// Position. GTK 4 cannot place a toplevel (the compositor places windows
// on Wayland, and GTK 4 dropped gtk_window_move on X11 too), so Center and
// SurfaceConfig.Center do nothing.
//
// Scale. The fractional gdk_surface_get_scale (GTK 4.12+, required since
// the 4.14 floor); a change (notify::scale of the GdkSurface) reports
// Resized. The window is realized at creation, so Scale is valid from
// NewSurface on; on Wayland it may be 1 until the window is mapped on a
// screen (an output is entered only then), and a Resized with the real
// scale follows Show.
//
// Layout. Resized from an allocation runs inside GTK's size_allocate of
// that window (Shell.inLayout), so what a handler does to the native
// window then (SetSize, SetMinSize, SetMaxSize, Close, Shell.Quit) is
// posted and runs right after; the reported size changes at once.
//
// Painting. Invalidate marks the surface and requests a frame; the frame's
// paint pass (Shell.paintInvalid) queues a draw of every invalidated,
// shown surface, which GTK snapshots in the same frame (the clock runs in
// GDK's UPDATE phase). Every snapshot calls SurfaceHandler.Paint on the
// canvas (canvas.go, S4.1g; real drawing since S4.2b–S4.2i).

// windowConfig is what a native window is created with, in logical
// pixels.
type windowConfig struct {
	title         string
	width, height int32 // the default size: the drawable area
	minW, minH    int32 // the GGSurface's size request
	resizable     bool
}

// windowPeer is the Go side of a native window: the GGSurface's vfuncs and
// the window's signals (kindCloseRequest, kindActive, kindScale,
// kindState), the caption gestures (chrome.go), the pointer controllers
// (input.go), the scroll controller (scroll.go) and the keyboard
// (keyboard.go).
type windowPeer interface {
	surfacePeer
	boolSignalPeer
	pointerSignalPeer
	intDoubleDoublePeer
	doubleDoublePeer
	voidSignalPeer
	boolDoubleDoublePeer
	boolUintUintUintPeer
	uintUintUintPeer
}

// window is the native side of one surface: a GtkWindow and its GGSurface
// child (window_linux.go). Every method runs on the GTK thread; any of
// them may run trampolines, so callers rethrow after them. After destroy
// no method is called.
type window interface {
	// widget returns the GGSurface (nonzero).
	widget() uintptr
	setTitle(title string)
	// setDefaultSize sets the window's default size
	// (gtk_window_set_default_size), the size of the drawable area.
	setDefaultSize(w, h int32)
	// setSizeRequest sets the GGSurface's size request, the minimum.
	setSizeRequest(w, h int32)
	// queueResize makes GTK allocate the GGSurface again at the next
	// layout, changed or not (gtk_widget_queue_resize).
	queueResize()
	present()
	minimize()
	setMaximized(on bool)
	maximized() bool
	// active reports gtk_window_is_active.
	active() bool
	// minimized reports whether the toplevel is minimized or suspended
	// (GDK_TOPLEVEL_STATE_SUSPENDED, 4.12+: the compositor sends it no
	// frames); false where the windowing system reports neither.
	minimized() bool
	scale() float64
	queueDraw()
	// snapshotter returns the canvas's calls into the GtkSnapshot s
	// of a snapshot vfunc (S4.1g).
	snapshotter(s uintptr) snapshotter
	// setCursor sets the GGSurface's cursor by CSS name.
	setCursor(name string)
	// eventTime returns the time (ms) of the event the event controller c
	// handles now.
	eventTime(c uintptr) uint32
	// eventState returns the GdkModifierType of the event the event
	// controller c handles now.
	eventState(c uintptr) uint32
	// currentButton returns the GDK button the gesture g handles now (0:
	// none).
	currentButton(g uintptr) uint32
	// clickRules returns GTK's double-click time and distance, the
	// distance as a rectangle in logical pixels.
	clickRules() shellkit.ClickRules
	// scrollUnit returns the GdkScrollUnit of the scroll controller c's
	// deltas.
	scrollUnit(c uintptr) uint32
	// keyBase returns the keyval at level 0 of keycode in the layout group
	// of the key event the key controller c handles now (0 if unknown).
	keyBase(c uintptr, keycode uint32) uint32
	// keyvalToUnicode is gdk_keyval_to_unicode (0 for none).
	keyvalToUnicode(kv uint32) rune
	// imFilter passes the key event the key controller c handles now to the
	// input method and reports whether it took it; a commit it makes
	// arrives (kindIMCommit) before it returns.
	imFilter(c uintptr) bool
	// imReset drops the input method's pending state (a dead key, a
	// compose sequence).
	imReset()
	// imFocus tells the input method that the surface gained or lost the
	// keyboard focus.
	imFocus(in bool)
	// commitString returns the C string of a commit (NULL gives "").
	commitString(p uintptr) string
	// pastDragThreshold reports whether a press that moved by dx, dy
	// (points) has passed GTK's drag threshold.
	pastDragThreshold(dx, dy float64) bool
	// beginMove hands the press of the caption drag gesture drag to the
	// compositor's interactive move, from x, y (points of the drawable
	// area), and resets the caption gestures.
	beginMove(drag uintptr, x, y float64)
	// destroy disconnects the signal handlers, destroys the window and
	// releases the GGSurface.
	destroy()
}

// resizeMode is how SetSize resizes a mapped window.
type resizeMode uint8

const (
	// resizeDefaultSize sets the default size only; GTK 4 keeps the
	// default size in step with the window's size and applies a new one
	// at the next layout.
	resizeDefaultSize resizeMode = iota
	// resizeSizeRequest also raises the GGSurface's size request to a
	// larger size until an allocation reaches it (it can only grow the
	// window), for a GTK that ignores the default size of a mapped window.
	resizeSizeRequest
)

// mappedResize is the resize path for mapped windows. Which one works is
// open until the Linux log (spike/gtk's resize check, S0.9c): the linux
// tests try both.
var mappedResize = resizeDefaultSize

// cursorNames maps the cursors to CSS cursor names.
var cursorNames = [...]string{
	shell.CursorArrow:           "default",
	shell.CursorIBeam:           "text",
	shell.CursorPointingHand:    "pointer",
	shell.CursorResizeLeftRight: "ew-resize",
	shell.CursorResizeUpDown:    "ns-resize",
	shell.CursorCrosshair:       "crosshair",
	shell.CursorNotAllowed:      "not-allowed",
	shell.CursorHidden:          "none",
}

// cursorName returns the CSS name of c ("default" for an unknown c).
func cursorName(c shell.Cursor) string {
	if int(c) >= len(cursorNames) {
		return cursorNames[shell.CursorArrow]
	}
	return cursorNames[c]
}

// px converts a logical size to GTK's whole logical pixels.
func px(sz driver.Size) (w, h int32) { return shellkit.ToPx(sz.W, 1), shellkit.ToPx(sz.H, 1) }

// noNUL drops the NUL characters a C string cannot hold.
func noNUL(s string) string { return strings.ReplaceAll(s, "\x00", "") }

// surface is the Linux shell.Surface.
type surface struct {
	s *Shell
	h shell.SurfaceHandler
	w window

	resizable bool
	size      driver.Size // the size last reported (or the initial one)
	want      driver.Size // the size asked for last, see shellkit.FromPx
	scale     float64     // the scale last reported (or the initial one)
	min, max  driver.Size

	creating  bool // inside NewSurface: no handler calls
	dead      bool // closed
	shown     bool // Show has run
	minimized bool // the toplevel state says minimized or suspended
	key       bool // the key state last reported
	invalid   bool // waits for a paint

	// forced: SetSize raised the size request to forceW × forceH
	// (resizeSizeRequest) until an allocation reaches it.
	forced         bool
	forceW, forceH int32
	allocW, allocH int32 // the last allocation (0 before the first)

	cursor   shell.Cursor
	regions  []shell.ChromeRegion // hit-tested by the caption gestures (chrome.go)
	paints   int                  // paints so far (tests)
	paintErr error                // the recorder errors of the last paint (debugChecks)

	drag     captionDrag // the caption drag gesture's press
	zoomTime uint32      // the event time of the last caption double-click (0: none)
	zooms    int         // caption double-clicks so far (tests)

	ptr pointerState // the pointer (input.go)
	scr scrollState  // scrolling (scroll.go)
	kbd keyState     // the keyboard (keyboard.go)
}

var (
	_ shell.Surface = (*surface)(nil)
	_ windowPeer    = (*surface)(nil)
	_ openSurface   = (*surface)(nil)
)

// NewSurface implements shell.Shell: a hidden, realized window of the
// configured size, clamped to the limits.
func (s *Shell) NewSurface(cfg shell.SurfaceConfig, h shell.SurfaceHandler) (shell.Surface, error) {
	s.checkReady("NewSurface")
	if h == nil {
		panic("gtk4: NewSurface with a nil handler")
	}
	s.notPainting("NewSurface")
	if s.quit {
		return nil, ErrQuit
	}
	sf := &surface{s: s, h: h, min: cfg.MinSize, max: cfg.MaxSize,
		resizable: cfg.Style&driver.StyleResizable != 0, creating: true, invalid: true}
	sf.want = shellkit.ClampSize(cfg.Size, sf.min, sf.max)
	sf.size = sf.want
	wc := windowConfig{title: noNUL(cfg.Title), resizable: sf.resizable}
	wc.width, wc.height = px(sf.want)
	wc.minW, wc.minH = sf.request()
	w, err := s.loop.newWindow(wc, sf)
	if err == nil && pendingPanic != nil { // a callback panicked while creating it
		w.destroy()
	}
	rethrow()
	if err != nil {
		return nil, err
	}
	sf.w = w
	sf.scale = w.scale()
	s.surfaces = append(s.surfaces, sf)
	sf.creating = false
	return sf, nil
}

// request returns the GGSurface's size request: the minimum, the size for
// a non-resizable surface, raised to a forced size.
func (sf *surface) request() (w, h int32) {
	if sf.resizable {
		w, h = px(sf.min)
	} else {
		w, h = px(sf.want)
	}
	if sf.forced {
		w, h = max(w, sf.forceW), max(h, sf.forceH)
	}
	return w, h
}

func (sf *surface) applyRequest() { sf.w.setSizeRequest(sf.request()) }

// native runs fn, a change of the native window's size, at once, or
// posted while a Resized handler runs inside GTK's size_allocate
// (Shell.inLayout): GTK is then allocating that very window.
func (sf *surface) native(fn func()) {
	if sf.s.inLayout == 0 {
		fn()
		rethrow()
		return
	}
	sf.s.Post(func() {
		if !sf.dead {
			fn()
			rethrow()
		}
	})
}

// live checks the thread and reports whether the surface is open.
func (sf *surface) live(what string) bool {
	sf.s.q.Check("gtk4.Surface." + what)
	return !sf.dead
}

// change is live for the methods that must not run during a Paint.
func (sf *surface) change(what string) bool {
	if !sf.live(what) {
		return false
	}
	sf.s.notPainting("Surface." + what)
	return true
}

// SetTitle implements shell.Surface.
func (sf *surface) SetTitle(title string) {
	if sf.live("SetTitle") {
		sf.w.setTitle(noNUL(title))
		rethrow()
	}
}

// Size implements shell.Surface.
func (sf *surface) Size() driver.Size {
	sf.s.q.Check("gtk4.Surface.Size")
	return sf.size
}

// SetSize implements shell.Surface. A maximized surface is restored first.
// The clamped size is reported at once; if the window manager gives the
// window another size, the allocation that follows reports that one. In a
// Resized handler the native resize is posted (native).
func (sf *surface) SetSize(size driver.Size) {
	if sf.change("SetSize") {
		sf.resizeTo(size)
	}
}

func (sf *surface) resizeTo(size driver.Size) {
	sf.want = shellkit.ClampSize(size, sf.min, sf.max)
	w, h := px(sf.want)
	// resizeSizeRequest: force the request while growing past the last
	// allocation; any later SetSize decides again.
	sf.forced = sf.shown && mappedResize == resizeSizeRequest && (w > sf.allocW || h > sf.allocH)
	sf.forceW, sf.forceH = w, h
	sf.native(func() {
		if sf.w.maximized() {
			sf.w.setMaximized(false)
			if sf.dead {
				return
			}
		}
		sf.w.setDefaultSize(w, h)
		sf.applyRequest()
		sf.w.queueResize() // an allocation follows even if GTK keeps the size
	})
	if sf.dead || sf.want == sf.size {
		return
	}
	sf.size = sf.want
	sf.h.Resized(sf.size, sf.scale)
	if !sf.dead {
		sf.invalidate()
	}
}

// SetMinSize implements shell.Surface.
func (sf *surface) SetMinSize(size driver.Size) {
	if sf.change("SetMinSize") {
		sf.min = size
		sf.applyLimits()
	}
}

// SetMaxSize implements shell.Surface.
func (sf *surface) SetMaxSize(size driver.Size) {
	if sf.change("SetMaxSize") {
		sf.max = size
		sf.applyLimits()
	}
}

func (sf *surface) applyLimits() {
	if c := shellkit.ClampSize(sf.size, sf.min, sf.max); c != sf.size {
		sf.resizeTo(c)
		return
	}
	sf.native(sf.applyRequest)
}

// Scale implements shell.Surface.
func (sf *surface) Scale() float64 {
	sf.s.q.Check("gtk4.Surface.Scale")
	return sf.scale
}

// Center implements shell.Surface. It does nothing: GTK 4 cannot place a
// toplevel (see above).
func (sf *surface) Center() { sf.change("Center") }

// Show implements shell.Surface (gtk_window_present, which also restores
// a minimized surface where the windowing system allows it).
func (sf *surface) Show() {
	if !sf.change("Show") {
		return
	}
	sf.w.present()
	rethrow()
	if sf.dead {
		return
	}
	if !sf.shown {
		sf.shown = true
		sf.s.clock.surfacesChanged() // it can pace now
	}
	if sf.invalid {
		sf.s.clock.RequestFrame()
	}
}

// Minimize implements shell.Surface. It does nothing for a hidden surface.
func (sf *surface) Minimize() {
	if !sf.change("Minimize") || !sf.shown {
		return
	}
	sf.w.minimize()
	rethrow()
}

// Zoom implements shell.Surface. It does nothing without StyleResizable or
// for a hidden surface. Resized comes with the new allocation.
func (sf *surface) Zoom() {
	if !sf.change("Zoom") || !sf.resizable || !sf.shown {
		return
	}
	sf.w.setMaximized(!sf.w.maximized())
	rethrow()
}

// Zoomed implements shell.Surface.
func (sf *surface) Zoomed() bool {
	if !sf.live("Zoomed") {
		return false
	}
	return sf.w.maximized()
}

// Close implements shell.Surface. In a Resized handler it is posted
// instead, so no window is destroyed while GTK allocates it.
func (sf *surface) Close() {
	if !sf.change("Close") {
		return
	}
	if sf.s.inLayout > 0 {
		sf.s.Post(sf.Close)
		return
	}
	sf.closeNow(false)
	rethrow()
}

// closeNow implements openSurface: the surface leaves the shell and the
// frame clock, the window is destroyed and, unless silent, Closed is
// called.
func (sf *surface) closeNow(silent bool) {
	if sf.dead {
		return
	}
	sf.dead = true // tickWidget returns 0 from here on
	sf.s.removeSurface(sf)
	sf.s.clock.detach(sf.w.widget())
	sf.w.destroy()
	if !silent {
		sf.h.Closed()
	}
}

// tickWidget implements openSurface: the GGSurface of a shown surface that
// is not known to be minimized.
func (sf *surface) tickWidget() uintptr {
	if sf.dead || !sf.shown || sf.minimized {
		return 0
	}
	return sf.w.widget()
}

// Invalidate implements shell.Surface.
func (sf *surface) Invalidate() {
	if sf.live("Invalidate") {
		sf.invalidate()
	}
}

func (sf *surface) invalidate() {
	sf.invalid = true
	sf.s.clock.RequestFrame()
}

// SetCursor implements shell.Surface: the GGSurface's cursor, so GTK's
// resize borders keep theirs.
func (sf *surface) SetCursor(c shell.Cursor) {
	if !sf.live("SetCursor") || c == sf.cursor {
		return
	}
	sf.cursor = c
	sf.w.setCursor(cursorName(c))
	rethrow()
}

// SetChromeRegions implements shell.Surface. The regions are kept for the
// caption gestures (chrome.go); a press already moving the window goes on.
func (sf *surface) SetChromeRegions(regions []shell.ChromeRegion) {
	if sf.live("SetChromeRegions") {
		sf.regions = append([]shell.ChromeRegion(nil), regions...)
	}
}

// The windowPeer side: GTK calls these through the trampolines. While
// NewSurface runs they only keep state; after Close they do nothing.

// measure implements surfacePeer: no size of its own; the size request
// is the minimum and the default size the size.
func (sf *surface) measure(int32, int32) (int32, int32) { return 0, 0 }

// sizeAllocate implements surfacePeer: the drawable area's new size. The
// first allocation can miss the asked size by the window's own CSS padding
// (solid-csd without a compositor), which is only known once the window
// is allocated; then the default size is set again, once, posted, and the
// allocation that follows reports the asked size.
func (sf *surface) sizeAllocate(width, height, _ int32) {
	if sf.dead {
		return
	}
	if sf.allocW == 0 && sf.allocH == 0 && !sf.creating {
		asked := sf.want
		if aw, ah := px(asked); aw != width || ah != height {
			got := shellkit.FromPx(width, height, 1, asked)
			sf.s.Post(func() {
				if sf.dead || sf.want != got || sf.w.maximized() { // resized since, or maximized
					return
				}
				sf.want = asked
				sf.w.setDefaultSize(aw, ah)
				sf.w.queueResize()
				rethrow()
			})
		}
	}
	sf.allocW, sf.allocH = width, height
	if sf.forced && width >= sf.forceW && height >= sf.forceH {
		sf.forced = false
		// Not from inside GTK's allocation: lowering the request queues
		// another resize.
		sf.s.Post(func() {
			if !sf.dead && !sf.forced {
				sf.applyRequest()
				rethrow()
			}
		})
	}
	size := shellkit.FromPx(width, height, 1, sf.want)
	sf.want = size
	if size == sf.size {
		return
	}
	sf.size = size
	if sf.creating {
		return
	}
	func() {
		sf.s.inLayout++ // GTK is allocating this window: see native
		defer func() { sf.s.inLayout-- }()
		sf.h.Resized(size, sf.scale)
	}()
	if !sf.dead {
		sf.invalidate()
	}
}

// snapshot implements surfacePeer: every snapshot paints the whole
// surface into the GtkSnapshot s, through the canvas at the
// surface's scale. The canvas is finished even when Paint panics, so GTK
// always gets a balanced snapshot; only a paint that returned counts.
func (sf *surface) snapshot(s uintptr) {
	if sf.dead || sf.creating {
		return
	}
	sf.invalid = false
	c := newCanvas(sf.w.snapshotter(s), sf.size, sf.scale, debugChecks)
	c.images = &sf.s.images
	c.text = sf.s.text
	sf.s.painting = sf
	done := false
	defer func() {
		sf.s.painting = nil
		if done {
			sf.paints++
		}
		prev := sf.paintErr
		sf.paintErr = c.finish()
		// Logged once per new error text, not at the frame rate.
		if err := sf.paintErr; err != nil && debugChecks && (prev == nil || prev.Error() != err.Error()) {
			log.Printf("gtk4: paint: %v", err)
		}
	}()
	sf.h.Paint(c)
	done = true
}

// boolSignal implements boolSignalPeer: GtkWindow::close-request, the
// user or the compositor asking to close. The surface closes itself when
// CloseRequested agrees; GTK's own handling is always stopped (TRUE).
func (sf *surface) boolSignal(k kind, _ uintptr) bool {
	if k != kindCloseRequest || sf.dead || sf.creating {
		return true
	}
	if sf.h.CloseRequested() && !sf.dead {
		sf.closeNow(false)
	}
	return true
}

// pointerSignal implements pointerSignalPeer: the notify signals and the
// pointer gesture's "cancel" (input.go) and the input method's "commit"
// (keyboard.go).
func (sf *surface) pointerSignal(k kind, instance, arg uintptr) {
	if sf.dead || sf.w == nil {
		return
	}
	switch k {
	case kindActive:
		if key := sf.w.active(); key != sf.key && !sf.creating {
			sf.key = key
			sf.keyFocus(key)
			sf.h.KeyChanged(key)
		}
	case kindScale:
		sc := sf.w.scale()
		if !(sc > 0) || sc == sf.scale {
			return
		}
		sf.scale = sc
		if sf.creating {
			return
		}
		sf.h.Resized(sf.size, sc)
		if !sf.dead {
			sf.invalidate()
		}
	case kindIMCommit:
		if sf.keyLive() {
			sf.commit(sf.w.commitString(arg))
		}
	case kindPointerCancel:
		if sf.pointerLive() {
			sf.cancel(instance)
		}
	case kindState:
		if m := sf.w.minimized(); m != sf.minimized {
			sf.minimized = m
			sf.s.clock.surfacesChanged()
			if !m && sf.invalid {
				sf.s.clock.RequestFrame()
			}
		}
	}
}

// removeSurface drops sf from the open surfaces.
func (s *Shell) removeSurface(sf *surface) {
	for i, o := range s.surfaces {
		if o == sf {
			s.surfaces = append(s.surfaces[:i], s.surfaces[i+1:]...)
			if len(s.surfaces) == 0 {
				s.images.release() // no renderer is left to draw them
			}
			return
		}
	}
}

// paintInvalid is the clock's paint pass: the scroll momentum moves
// (stepMomentum), then a draw is queued for every
// invalidated, shown surface, and GTK snapshots it in the same frame.
// Hidden surfaces stay invalid until shown.
func (s *Shell) paintInvalid(t time.Duration) {
	s.stepMomentum(t)
	for _, o := range s.surfaces {
		if sf, ok := o.(*surface); ok && sf.invalid && sf.shown && !sf.dead {
			sf.w.queueDraw()
		}
	}
	rethrow()
}

// notPainting panics while a SurfaceHandler.Paint runs.
func (s *Shell) notPainting(what string) {
	if s.painting != nil {
		panic("gtk4: " + what + " called during SurfaceHandler.Paint")
	}
}

// voidSignal implements voidSignalPeer: the motion controller's "leave"
// (input.go) and the scroll controller's "scroll-begin" and "scroll-end"
// (scroll.go).
func (sf *surface) voidSignal(k kind, instance uintptr) {
	if !sf.pointerLive() {
		return
	}
	switch k {
	case kindPointerLeave:
		sf.leave(instance)
	case kindScrollBegin:
		sf.scrollBegin()
	case kindScrollEnd:
		sf.scrollEnd(instance)
	}
}

// boolDoubleDouble implements boolDoubleDoublePeer: the scroll
// controller's "scroll" (scroll.go). It always reports the scroll
// handled, so GTK does not pass it on.
func (sf *surface) boolDoubleDouble(k kind, instance uintptr, dx, dy float64) bool {
	if k == kindScroll && sf.pointerLive() {
		sf.scroll(dx, dy, instance)
	}
	return true
}

// boolUintUintUint implements boolUintUintUintPeer: the key controller's
// "key-pressed" (keyboard.go), keyval, keycode and state.
func (sf *surface) boolUintUintUint(k kind, instance uintptr, kv, keycode, state uint32) bool {
	if k != kindKeyPressed || !sf.keyLive() {
		return false
	}
	return sf.keyPressed(instance, kv, keycode, state)
}

// uintUintUint implements uintUintUintPeer: the key controller's
// "key-released" (keyboard.go).
func (sf *surface) uintUintUint(k kind, instance uintptr, kv, keycode, state uint32) {
	if k == kindKeyReleased && sf.keyLive() {
		sf.keyReleased(instance, kv, keycode, state)
	}
}
