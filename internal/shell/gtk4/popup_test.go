package gtk4

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The popups' host tests (S5.5f): the popup over a fakePopover, whose
// child is a fakeWindow driven through the real trampolines like a
// surface's, and whose parent is the parent surface's fakeWindow.

// fakePopover is a popover that records its calls (in the child's calls)
// and plays GTK's placement: unconstrained, the child lands where the
// placement asks; land overrides it (a slide).
type fakePopover struct {
	*fakeWindow
	parent *fakeWindow
	grab   bool
	pl     popoverPlacement
	shown  bool
	// land, if set, is where the child lands (parent pixels); noSurface
	// makes origin report no surface.
	land      *[2]float64
	noSurface bool
	// keySig are the handles of the popover's key controller, whose peer
	// is the parent surface (keys).
	keySig map[kind]ffi.Handle
}

var _ popover = (*fakePopover)(nil)

// newPopover creates a fakePopover as popover_linux.go creates a
// gtkPopover: a GGSurface child whose handle word holds a kindSurface
// entry of p, and one handle per connected signal.
func (f *winFactory) newPopover(parent window, keys windowPeer, grab bool, p windowPeer) (popover, error) {
	if !f.s.q.OnUIThread() {
		f.errorf("newPopover off the UI thread")
	}
	pw, ok := parent.(*fakeWindow)
	if !ok || pw.destroyed {
		f.errorf("newPopover on a parent that is not a live fake window")
	}
	fw := &fakeWindow{f: f, cfg: windowConfig{title: "popup"}, peer: p, sc: f.popScale(), sig: map[kind]ffi.Handle{}}
	fw.inst = new(fakeInstance)
	f.pins.Pin(fw.inst)
	fw.addr = uintptr(unsafe.Pointer(fw.inst))
	fw.surf = dispatch.New(entry{peer: p, kind: kindSurface})
	setSurfaceHandle(fw.addr, fw.surf)
	kinds := []kind{kindPointerEnter, kindPointerMotion, kindPointerLeave,
		kindPointerPressed, kindPointerReleased, kindPointerCancel,
		kindScroll, kindScrollBegin, kindScrollEnd, kindScrollDecelerate, kindPopoverClosed}
	if !grab {
		kinds = nil // a popover without grab gets no controllers and no "closed"
	}
	for _, k := range kinds {
		fw.sig[k] = dispatch.New(entry{peer: p, kind: k})
	}
	f.wins = append(f.wins, fw)
	fp := &fakePopover{fakeWindow: fw, parent: pw, grab: grab}
	if grab {
		fp.keySig = map[kind]ffi.Handle{
			kindKeyPressed:  dispatch.New(entry{peer: keys, kind: kindKeyPressed}),
			kindKeyReleased: dispatch.New(entry{peer: keys, kind: kindKeyReleased}),
		}
	}
	f.pops = append(f.pops, fp)
	if f.popErr != nil {
		fw.release()
		fp.releaseKeys()
		return nil, f.popErr
	}
	return fp, nil
}

func (f *winFactory) popScale() float64 {
	if f.popoverScale != 0 {
		return f.popoverScale
	}
	return f.scale
}

func (fp *fakePopover) place(pl popoverPlacement) {
	fp.call("place %d,%d %dx%d off %d,%d size %dx%d", pl.pointing.X, pl.pointing.Y,
		pl.pointing.Width, pl.pointing.Height, pl.dx, pl.dy, pl.w, pl.h)
	fp.pl = pl
}

func (fp *fakePopover) popup()   { fp.call("popup"); fp.shown = true }
func (fp *fakePopover) present() { fp.call("present") }

func (fp *fakePopover) origin() (float64, float64, bool) {
	fp.use("origin")
	if fp.noSurface {
		return 0, 0, false
	}
	if fp.land != nil {
		return fp.land[0], fp.land[1], true
	}
	r := fp.pl.pointing
	return float64(r.X + fp.pl.dx), float64(r.Y + r.Height + fp.pl.dy), true
}

// destroy checks that the popover goes before its parent's window.
func (fp *fakePopover) destroy() {
	if fp.parent.destroyed {
		fp.f.errorf("popover destroyed after its parent's window")
	}
	fp.releaseKeys()
	fp.fakeWindow.destroy()
}

func (fp *fakePopover) releaseKeys() {
	for _, h := range fp.keySig {
		dispatch.Free(h)
	}
	fp.keySig = nil
}

// keyPress and keyRelease are the popover's key controller: its events
// are the parent's (the parent window's eventTime and keyBase).
func (fp *fakePopover) keyPress(kv, keycode, state, t uint32) int32 {
	fp.parent.evTime, fp.parent.evKeyval, fp.parent.evDown = t, kv, true
	r := sigBoolUintUintUint(testKey, kv, keycode, state, uintptr(fp.keySig[kindKeyPressed]))
	rethrow()
	return r
}

func (fp *fakePopover) keyRelease(kv, keycode, state, t uint32) {
	fp.parent.evTime, fp.parent.evKeyval, fp.parent.evDown = t, kv, false
	sigUintUintUint(testKey, kv, keycode, state, uintptr(fp.keySig[kindKeyReleased]))
	rethrow()
}

// closed is GtkPopover::closed.
func (fp *fakePopover) closed() {
	sigVoid(fp.addr, uintptr(fp.sig[kindPopoverClosed]))
	rethrow()
}

// layoutNatural allocates the child its natural size, as GTK presents a
// popover (the peer's measure).
func (fp *fakePopover) layoutNatural() {
	sp, _ := surfacePeerOf(fp.addr)
	_, w := sp.measure(gtk.OrientationHorizontal, -1)
	_, h := sp.measure(gtk.OrientationVertical, -1)
	fp.allocate(w, h)
}

// popHandler is a shell.PopupHandler that logs its calls.
type popHandler struct {
	log       []string
	onPlaced  func()
	onDismiss func()
	onClosed  func()
	onKey     func(shell.KeyEvent) bool
	sf        sfHandler // formats the events
}

func (h *popHandler) Paint(render.Canvas) { h.log = append(h.log, "paint") }
func (h *popHandler) Placed(r driver.Rect, sc float64) {
	h.log = append(h.log, fmt.Sprintf("placed %v,%v %vx%v@%v", r.X, r.Y, r.W, r.H, sc))
	if h.onPlaced != nil {
		h.onPlaced()
	}
}
func (h *popHandler) Event(ev shell.Event) bool {
	h.sf.onKey = h.onKey
	r := h.sf.Event(ev)
	h.log = append(h.log, h.sf.take()...)
	return r
}
func (h *popHandler) Dismissed() {
	h.log = append(h.log, "dismissed")
	if h.onDismiss != nil {
		h.onDismiss()
	}
}
func (h *popHandler) Closed() {
	h.log = append(h.log, "closed")
	if h.onClosed != nil {
		h.onClosed()
	}
}

func (h *popHandler) take() []string {
	l := h.log
	h.log = nil
	return l
}

// openParent opens and shows a resizable 300×200 surface, allocated.
func openParent(t *testing.T, f *winFactory) (*surface, *fakeWindow, *sfHandler) {
	t.Helper()
	h := &sfHandler{}
	sf, fw := f.open(t, shell.SurfaceConfig{Size: driver.Size{W: 300, H: 200}, Style: driver.StyleResizable}, h)
	sf.Show()
	fw.layout()
	fw.isActive = true
	fw.notify(kindActive)
	fw.take()
	h.take()
	return sf, fw, h
}

// menuCfg is a grab popup over a pop-up button at 20,30 (100×20), the
// menu 120×80 with its selected item 25 above its top.
var menuCfg = shell.PopupConfig{
	Anchor: driver.Rect{X: 20, Y: 30, W: 100, H: 20},
	Size:   driver.Size{W: 120, H: 80},
	Offset: driver.Point{X: -4, Y: -25},
	Adjust: shell.AdjustSlideX | shell.AdjustSlideY | shell.AdjustResizeY,
	Grab:   true,
}

// openPopup opens a popup over sf, allocated at its natural size.
func openPopup(t *testing.T, f *winFactory, sf *surface, cfg shell.PopupConfig, h *popHandler) (*popup, *fakePopover) {
	t.Helper()
	p, err := f.s.NewPopup(sf, cfg, h)
	if err != nil {
		t.Fatalf("NewPopup = %v", err)
	}
	fp := f.pops[len(f.pops)-1]
	fp.layoutNatural()
	return p.(*popup), fp
}

func TestPopoverPlace(t *testing.T) {
	for _, c := range []struct {
		name   string
		cfg    shell.PopupConfig
		pw, ph int32
		want   popoverPlacement
		target [2]int32
	}{
		{"menu", menuCfg, 300, 200,
			popoverPlacement{gtk.Rectangle{X: 20, Y: 30, Width: 100, Height: 20}, -4, -45, 120, 80}, [2]int32{16, 5}},
		{"tooltip, zero-size anchor", shell.PopupConfig{Anchor: driver.Rect{X: 50.4, Y: 60.6}, Size: driver.Size{W: 40.4, H: 18},
			Offset: driver.Point{Y: 20}}, 300, 200,
			popoverPlacement{gtk.Rectangle{X: 50, Y: 61, Width: 1, Height: 1}, 0, 19, 40, 18}, [2]int32{50, 81}},
		{"anchor partly outside", shell.PopupConfig{Anchor: driver.Rect{X: -10, Y: 190, W: 30, H: 30}, Size: driver.Size{W: 10, H: 10}},
			300, 200,
			popoverPlacement{gtk.Rectangle{X: 0, Y: 190, Width: 20, Height: 10}, -10, -10, 10, 10}, [2]int32{-10, 190}},
		{"anchor wholly outside", shell.PopupConfig{Anchor: driver.Rect{X: 400, Y: -50, W: 10, H: 10}},
			300, 200,
			popoverPlacement{gtk.Rectangle{X: 299, Y: 0, Width: 1, Height: 1}, 101, -51, 0, 0}, [2]int32{400, -50}},
		{"NaN and negatives", shell.PopupConfig{Anchor: driver.Rect{X: nan(), Y: 5, W: -3, H: nan()},
			Size: driver.Size{W: -1, H: nan()}, Offset: driver.Point{X: nan(), Y: 2}}, 0, 0,
			popoverPlacement{gtk.Rectangle{X: 0, Y: 0, Width: 1, Height: 1}, 0, 6, 0, 0}, [2]int32{0, 7}},
	} {
		got, target := popoverPlace(c.cfg, c.pw, c.ph)
		if got != c.want || target != c.target {
			t.Errorf("%s: popoverPlace = %+v, %v, want %+v, %v", c.name, got, target, c.want, c.target)
		}
		// Unconstrained, GTK puts the top-left at the target.
		if r := got.pointing; r.X+got.dx != target[0] || r.Y+r.Height+got.dy != target[1] {
			t.Errorf("%s: GTK would put the popover at %d,%d, want %v", c.name, r.X+got.dx, r.Y+r.Height+got.dy, target)
		}
	}
}

// TestPopupNewGuards: the panics and the errors of NewPopup.
func TestPopupNewGuards(t *testing.T) {
	s, f := newSurfaceTest(t)
	mustPanic(t, "nil handler", func() { s.NewPopup(nil, menuCfg, nil) })
	h := &popHandler{}
	for _, c := range []struct {
		name   string
		parent shell.Surface
	}{{"nil", nil}, {"hidden", nil}, {"closed", nil}, {"minimized", nil}, {"popup", nil}} {
		var parent shell.Surface
		switch c.name {
		case "hidden":
			parent, _ = f.open(t, sfResizable, &sfHandler{})
		case "closed":
			sf, _, _ := openParent(t, f)
			sf.Close()
			parent = sf
		case "minimized":
			sf, fw, _ := openParent(t, f)
			fw.isMin = true
			fw.notify(kindState)
			parent = sf
		case "popup":
			sf, _, _ := openParent(t, f)
			p, _ := openPopup(t, f, sf, shell.PopupConfig{}, &popHandler{})
			parent = p.c
		}
		if p, err := s.NewPopup(parent, menuCfg, h); p != nil || !errors.Is(err, shell.ErrPopupParent) {
			t.Errorf("%s parent: NewPopup = %v, %v, want ErrPopupParent", c.name, p, err)
		}
	}
	sf, _, _ := openParent(t, f)
	g, _ := openPopup(t, f, sf, menuCfg, &popHandler{})
	if p, err := s.NewPopup(sf, menuCfg, h); p != nil || !errors.Is(err, shell.ErrPopupGrab) {
		t.Errorf("second grab: NewPopup = %v, %v, want ErrPopupGrab", p, err)
	}
	if _, err := s.NewPopup(sf, shell.PopupConfig{Size: driver.Size{W: 10, H: 10}}, &popHandler{}); err != nil {
		t.Errorf("a tooltip during a grab: %v", err)
	}
	g.dismiss()
	if _, err := s.NewPopup(sf, menuCfg, &popHandler{}); err != nil {
		t.Errorf("a grab after a dismissal: %v", err)
	}
	f.popErr = errors.New("no popover")
	if p, err := s.NewPopup(sf, shell.PopupConfig{}, h); p != nil || err != f.popErr {
		t.Errorf("failing popover: NewPopup = %v, %v", p, err)
	}
	f.popErr = nil
	wantLog(t, "handler", h.take())
}

// TestPopupOpenPlaced: NewPopup places and shows the popover; Placed comes
// with the child's allocation, exact where GTK put it where asked, then the
// first paint; a slide is reported in pixels; no surface, no Placed.
func TestPopupOpenPlaced(t *testing.T) {
	s, f := newSurfaceTest(t)
	sf, _, _ := openParent(t, f)
	h := &popHandler{}
	cfg := menuCfg
	cfg.Anchor.X, cfg.Offset.Y = 20.25, -25.5
	p, err := s.NewPopup(sf, cfg, h)
	if err != nil {
		t.Fatal(err)
	}
	fp := f.pops[0]
	wantLog(t, "popover", fp.take(), "place 20,30 100x20 off -4,-45 size 120x80", "popup")
	wantLog(t, "before the allocation", h.take())
	if p.Translucent() {
		t.Error("Translucent = true")
	}
	if s.clock.requested {
		f.frame(t)
	}
	if fp.draws != 0 {
		t.Errorf("drawn before Placed")
	}
	fp.layoutNatural()
	wantLog(t, "allocated", h.take(), "placed 16.25,4.5 120x80@1")
	f.frame(t)
	if fp.draws != 1 {
		t.Errorf("draws = %d after the frame, want 1", fp.draws)
	}
	fp.snap()
	wantLog(t, "snapshot", h.take(), "paint")
	fp.allocate(120, 80)
	wantLog(t, "same allocation", h.take())

	// The compositor resized it and slid it.
	fp.land = &[2]float64{10, 12}
	fp.allocate(120, 60)
	wantLog(t, "resized", h.take(), "placed 10,12 120x60@1")
	if !p.(*popup).c.invalid {
		t.Error("a resize did not invalidate")
	}

	// Without a surface (not mapped yet) nothing is reported.
	p2, err := s.NewPopup(sf, shell.PopupConfig{Size: driver.Size{W: 5, H: 5}}, h)
	if err != nil {
		t.Fatal(err)
	}
	f.pops[1].take()
	f.pops[1].noSurface = true
	f.pops[1].layoutNatural()
	wantLog(t, "no surface", h.take())
	f.pops[1].noSurface = false
	sf.w.(*fakeWindow).layout() // the parent's size_allocate presents it
	wantLog(t, "presented", f.pops[1].take(), "present")
	wantLog(t, "placed from the parent's allocation", h.take(), "placed 0,0 5x5@1")
	_ = p2
}

// TestPopupScale: the popover's scale is reported, and a change of it
// found after the parent's scale change.
func TestPopupScale(t *testing.T) {
	s, f := newSurfaceTest(t)
	f.popoverScale = 2
	sf, fw, _ := openParent(t, f)
	h := &popHandler{}
	_, fp := openPopup(t, f, sf, shell.PopupConfig{Size: driver.Size{W: 50, H: 40}}, h)
	wantLog(t, "Placed", h.take(), "placed 0,0 50x40@2")
	fp.sc = 1.5
	fw.sc = 1.5
	fw.notify(kindScale)
	wantLog(t, "Placed", h.take(), "placed 0,0 50x40@1.5")
	_ = s
}

// TestPopupReposition: a new placement, its result reported.
func TestPopupReposition(t *testing.T) {
	_, f := newSurfaceTest(t)
	sf, _, _ := openParent(t, f)
	h := &popHandler{}
	p, fp := openPopup(t, f, sf, menuCfg, h)
	h.take()
	fp.take()
	cfg := menuCfg
	cfg.Grab = false
	cfg.Offset.Y = -45
	cfg.Size.H = 100
	p.Reposition(cfg)
	wantLog(t, "popover", fp.take(), "place 20,30 100x20 off -4,-65 size 120x100")
	wantLog(t, "handler", h.take(), "placed 16,-15 120x80@1")
	fp.layoutNatural()
	wantLog(t, "handler", h.take(), "placed 16,-15 120x100@1")
	if !p.grab {
		t.Error("Reposition changed Grab")
	}
}

// TestPopupClosedDismissed: "closed" dismisses a grab popup once, the
// child's held buttons released first; Close then closes it.
func TestPopupClosedDismissed(t *testing.T) {
	s, f := newSurfaceTest(t)
	sf, _, ph := openParent(t, f)
	h := &popHandler{}
	p, fp := openPopup(t, f, sf, menuCfg, h)
	h.onDismiss = p.Close
	fp.ptrMotion(10, 10, fakeEv{t: 1})
	fp.ptrPressed(1, 10, 10, fakeEv{t: 2, button: 1})
	h.take()
	fp.closed()
	wantLog(t, "handler", h.take(), "up 10,10 b=1 held=0 n=0", "leave 10,10", "dismissed", "closed")
	fp.closed()
	wantLog(t, "twice", h.take())
	if s.grab != nil || len(s.popups) != 0 {
		t.Errorf("grab %v, popups %d after Close", s.grab, len(s.popups))
	}
	wantLog(t, "parent", ph.take())

	// A popup without grab is not dismissed (it has no "closed").
	h2 := &popHandler{}
	p2, _ := openPopup(t, f, sf, shell.PopupConfig{Size: driver.Size{W: 5, H: 5}}, h2)
	p2.closedSignal()
	wantLog(t, "tooltip", h2.take(), "placed 0,0 5x5@1")
}

// TestPopupGrabPointer: the parent's held button ends as an OS cancel and
// stays held for the popup, whose first events carry it; the parent's
// moves and the release reach the popup in its points; a later press
// outside is eaten with its release and dismisses.
func TestPopupGrabPointer(t *testing.T) {
	s, f := newSurfaceTest(t)
	sf, fw, ph := openParent(t, f)
	fw.ptrMotion(40, 40, fakeEv{t: 1})
	fw.ptrPressed(1, 40, 40, fakeEv{t: 2, button: 1})
	ph.take()
	h := &popHandler{}
	_, fp := openPopup(t, f, sf, menuCfg, h)
	wantLog(t, "parent", ph.take(), "up 40,40 b=1 held=0 n=0", "leave 40,40")
	h.take()
	// Over the popup (16,5 120×80), then outside, then released outside.
	fw.ptrMotion(30, 20, fakeEv{t: 3})
	fw.ptrMotion(200, 20, fakeEv{t: 4})
	fw.ptrReleased(1, 200, 20, fakeEv{t: 5, button: 1})
	wantLog(t, "popup", h.take(), "enter 14,15", "move 14,15", "move 184,15", "up 184,15 b=1 held=0 n=0", "leave 184,15")
	wantLog(t, "parent", ph.take())
	// The popup's own pointer: enter, move, a click.
	fp.ptrEnter(5, 6, fakeEv{t: 6})
	fp.ptrPressed(1, 5, 6, fakeEv{t: 7, button: 1})
	fp.ptrReleased(1, 5, 6, fakeEv{t: 8, button: 1})
	wantLog(t, "own", h.take(), "enter 5,6", "down 5,6 b=1 held=1 n=1", "up 5,6 b=1 held=0 n=1")
	// The parent's scroll, leave and cancel go nowhere.
	fw.scrollBy(0, 1, 0, fakeEv{t: 9})
	fw.ptrLeave(fakeEv{t: 10})
	fw.ptrCancel(fakeEv{t: 11})
	wantLog(t, "dropped", append(h.take(), ph.take()...))
	// A press outside: eaten with its release, dismissing.
	fw.ptrPressed(1, 250, 150, fakeEv{t: 12, button: 1})
	wantLog(t, "outside press", h.take(), "leave 5,6", "dismissed")
	fw.ptrReleased(1, 250, 150, fakeEv{t: 13, button: 1})
	wantLog(t, "after", append(h.take(), ph.take()...))
	if s.grab != nil {
		t.Error("grab left after the dismissal")
	}
	// The grab is gone: the parent's pointer is its own again.
	fw.ptrMotion(250, 150, fakeEv{t: 14})
	wantLog(t, "parent again", ph.take(), "enter 250,150", "move 250,150")
}

// TestPopupInheritedPress: a held button whose release never came is
// dropped by the child's next press of it.
func TestPopupInheritedPress(t *testing.T) {
	_, f := newSurfaceTest(t)
	sf, fw, _ := openParent(t, f)
	fw.ptrPressed(1, 40, 40, fakeEv{t: 2, button: 1})
	h := &popHandler{}
	_, fp := openPopup(t, f, sf, menuCfg, h)
	h.take()
	fp.ptrPressed(1, 5, 6, fakeEv{t: 7, button: 1})
	fp.ptrReleased(1, 5, 6, fakeEv{t: 8, button: 1})
	wantLog(t, "popup", h.take(), "enter 5,6", "down 5,6 b=1 held=1 n=1", "up 5,6 b=1 held=0 n=1")
}

// TestPopupGrabKeys: the parent's keys and text go to the grab popup's
// handler, back to the parent's after the dismissal; a tooltip takes none.
func TestPopupGrabKeys(t *testing.T) {
	_, f := newSurfaceTest(t)
	sf, fw, ph := openParent(t, f)
	tip := &popHandler{}
	openPopup(t, f, sf, shell.PopupConfig{Size: driver.Size{W: 5, H: 5}}, tip)
	h := &popHandler{onKey: func(shell.KeyEvent) bool { return true }}
	p, _ := openPopup(t, f, sf, menuCfg, h)
	h.take()
	if r := fw.keyPress(0xff1b, 9, 0, 1); r == 0 {
		t.Error("a key the popup handled was not consumed")
	}
	fw.keyRelease(0xff1b, 9, 0, 2)
	fw.imCommit("a")
	wantLog(t, "popup", h.take(), "key-down 0x1b", "key-up 0x1b", `text "a"`)
	wantLog(t, "parent", ph.take())
	p.dismiss()
	h.take()
	fw.keyPress(0x61, 38, 0, 3)
	wantLog(t, "parent after", ph.take(), "key-down 0x41")
	wantLog(t, "popup after", h.take())
	tip.take()
	wantLog(t, "tooltip", tip.take())
}

// TestPopupPopoverKeys: the popover's key controller feeds the parent's
// keyboard; a key both controllers see is delivered once, with the first
// one's result.
func TestPopupPopoverKeys(t *testing.T) {
	_, f := newSurfaceTest(t)
	sf, fw, ph := openParent(t, f)
	h := &popHandler{onKey: func(shell.KeyEvent) bool { return true }}
	_, fp := openPopup(t, f, sf, menuCfg, h)
	h.take()
	if r := fp.keyPress(0xff1b, 9, 0, 5); r == 0 {
		t.Error("the popover's key not consumed")
	}
	if r := fw.keyPress(0xff1b, 9, 0, 5); r == 0 {
		t.Error("the same key through the window not consumed")
	}
	fw.keyRelease(0xff1b, 9, 0, 6)
	fp.keyRelease(0xff1b, 9, 0, 6)
	fp.keyPress(0xff1b, 9, 0, 7)
	wantLog(t, "popup", h.take(), "key-down 0x1b", "key-up 0x1b", "key-down 0x1b")
	wantLog(t, "parent", ph.take())
}

// TestPopupDismissCauses: the parent's key loss, minimizing and resize
// dismiss the grab popup, once.
func TestPopupDismissCauses(t *testing.T) {
	for _, c := range []string{"key loss", "minimize", "resize"} {
		t.Run(c, func(t *testing.T) {
			_, f := newSurfaceTest(t)
			sf, fw, ph := openParent(t, f)
			h := &popHandler{}
			openPopup(t, f, sf, menuCfg, h)
			h.take()
			switch c {
			case "key loss":
				fw.isActive = false
				fw.notify(kindActive)
				wantLog(t, "parent", ph.take(), "key false")
			case "minimize":
				fw.isMin = true
				fw.notify(kindState)
			case "resize":
				fw.allocate(320, 200)
				fw.allocate(330, 200)
			}
			wantLog(t, "popup", h.take(), "dismissed")
		})
	}
}

// TestPopupClose: Close closes without Dismissed, frees the handles and
// makes every method a no-op; the parent's Close and Quit close the
// popups first, before the parent's window goes and its Closed.
func TestPopupClose(t *testing.T) {
	s, f := newSurfaceTest(t)
	sf, _, ph := openParent(t, f)
	base := dispatch.Len()
	h := &popHandler{}
	p, fp := openPopup(t, f, sf, menuCfg, h)
	h.take()
	p.Close()
	if n := dispatch.Len(); n != base {
		t.Errorf("after Close: %d handles, want %d", n, base)
	}
	wantLog(t, "Close", h.take(), "closed")
	if !fp.destroyed || s.grab != nil {
		t.Errorf("destroyed %v, grab %v", fp.destroyed, s.grab)
	}
	p.Close()
	p.Invalidate()
	p.Reposition(menuCfg)
	p.SetCursor(shell.CursorIBeam)
	wantLog(t, "dead", h.take())

	order := []string{}
	h1 := &popHandler{onClosed: func() { order = append(order, "popup 1") }}
	h2 := &popHandler{onClosed: func() { order = append(order, "popup 2") }}
	openPopup(t, f, sf, menuCfg, h1)
	openPopup(t, f, sf, shell.PopupConfig{}, h2)
	ph.onClosed = func() { order = append(order, "parent") }
	sf.Close()
	wantLog(t, "order", order, "popup 2", "popup 1", "parent")
	wantLog(t, "no dismissal", h1.take(), "placed 16,5 120x80@1", "closed")

	sf2, _, _ := openParent(t, f)
	h3 := &popHandler{}
	openPopup(t, f, sf2, menuCfg, h3)
	h3.take()
	s.Quit()
	wantLog(t, "Quit", h3.take(), "closed")
}

// TestPopupCloseInHandlers: Close from Placed and from Dismissed, and the
// parent closed from the popup's Closed.
func TestPopupCloseInHandlers(t *testing.T) {
	_, f := newSurfaceTest(t)
	sf, _, ph := openParent(t, f)
	h := &popHandler{}
	sp, err := f.s.NewPopup(sf, menuCfg, h)
	if err != nil {
		t.Fatal(err)
	}
	h.onPlaced = sp.Close
	f.pops[len(f.pops)-1].layoutNatural()
	wantLog(t, "Placed", h.take(), "placed 16,5 120x80@1", "closed")

	h2 := &popHandler{}
	h2.onClosed = sf.Close
	openPopup(t, f, sf, menuCfg, h2)
	sf.Close()
	wantLog(t, "popup", h2.take(), "placed 16,5 120x80@1", "closed")
	wantLog(t, "parent", ph.take(), "closed")
}

// TestPopupPaintPass: Invalidate draws the popup after the surfaces.
func TestPopupPaintPass(t *testing.T) {
	_, f := newSurfaceTest(t)
	sf, fw, _ := openParent(t, f)
	h := &popHandler{}
	p, fp := openPopup(t, f, sf, menuCfg, h)
	f.frame(t)
	fw.take()
	fp.take()
	p.Invalidate()
	sf.Invalidate()
	f.frame(t)
	if fw.draws == 0 || fp.draws == 0 {
		t.Errorf("draws: parent %d, popup %d", fw.draws, fp.draws)
	}
	fp.take()
	p.SetCursor(shell.CursorPointingHand)
	wantLog(t, "cursor", fp.take(), "setCursor pointer")
}

func nan() float64 { return math.NaN() }

// trackCfg is a grab menu below a strip title at 40,0 (40×28), its track
// the strip 10,0 200×28: the popup lands at 40,28, touching the track.
var trackCfg = shell.PopupConfig{
	Anchor: driver.Rect{X: 40, Y: 0, W: 40, H: 28},
	Size:   driver.Size{W: 100, H: 80},
	Offset: driver.Point{X: 0, Y: 28},
	Grab:   true,
	Track:  driver.Rect{X: 10, Y: 0, W: 200, H: 28},
}

// TestPopupGrabTrack: the grab's track area on the popover (S5.6e): the
// parent's moves over the track are the child's in its points; crossing
// between the parent's track and the popover (a leave of one surface, an
// enter of the other) gives no leave, the deferred leave cancelled before
// its idle runs; leaving the area from either surface leaves, from the
// parent's area off the track at once, out of both surfaces through the
// idle; the parent's wheel over the track is the child's; a press that
// reaches the parent's gesture on the track is delivered, a child's
// release over it keeps the pointer in; Reposition keeps the track; a
// dismissal drops a pending leave; an empty track is none.
func TestPopupGrabTrack(t *testing.T) {
	_, f := newSurfaceTest(t)
	l := f.l
	sf, fw, ph := openParent(t, f)
	h := &popHandler{}
	p, fp := openPopup(t, f, sf, trackCfg, h)
	wantLog(t, "open", h.take(), "placed 40,28 100x80@1")
	if p.track != trackCfg.Track {
		t.Fatalf("track %+v", p.track)
	}
	idles := func(what string, want int) {
		t.Helper()
		if n := l.runIdles(t); n != want {
			t.Errorf("%s: %d idle sources ran, want %d", what, n, want)
		}
	}

	fw.ptrMotion(50, 10, fakeEv{t: 1}) // the track
	fw.ptrLeave(fakeEv{t: 2})          // into the popover
	fp.ptrEnter(10, 2, fakeEv{t: 3})
	fp.ptrMotion(10, 5, fakeEv{t: 4})
	idles("into the popover", 0)
	fp.ptrLeave(fakeEv{t: 5}) // back to the track
	fw.ptrEnter(60, 20, fakeEv{t: 6})
	idles("back to the track", 0)
	fw.ptrMotion(250, 10, fakeEv{t: 7}) // off the track: at once
	idles("off the track", 0)
	wantLog(t, "crossing", h.take(), "enter 10,-18", "move 10,-18", "move 10,5", "move 20,-8", "leave 20,-8")

	fw.ptrMotion(60, 10, fakeEv{t: 8})
	fw.ptrLeave(fakeEv{t: 9}) // out of the window from the track
	wantLog(t, "before the idle", h.take(), "enter 20,-18", "move 20,-18")
	idles("out of the window", 1)
	wantLog(t, "out of the window", h.take(), "leave 20,-18")
	fp.ptrEnter(50, 50, fakeEv{t: 10})
	fp.ptrLeave(fakeEv{t: 11}) // out of the popover, not into the track
	idles("out of the popover", 1)
	wantLog(t, "out of the popover", h.take(), "enter 50,50", "leave 50,50")

	// The wheel over the track, and off it.
	fw.ptrMotion(60, 10, fakeEv{t: 12})
	fw.scrollBy(0, 1, 0, fakeEv{t: 13})
	fw.ptrMotion(250, 10, fakeEv{t: 14})
	fw.scrollBy(0, 1, 0, fakeEv{t: 15})
	got := h.take()
	if len(got) != 4 || got[0] != "enter 20,-18" || got[1] != "move 20,-18" ||
		got[2] != "scroll 0,-3 at 20,-18" || got[3] != "leave 20,-18" {
		t.Errorf("wheel: %q", got)
	}

	// A touchpad gesture that begins over the track follows to its end and
	// decelerate, also off the track; one that begins off it is dropped.
	// (The fake's scroll unit is per window: the child's is set too.)
	fw.ptrMotion(60, 10, fakeEv{t: 1500})
	fp.unit = gdkScrollUnitSurface
	fw.scrollBegin(fakeEv{t: 1501})
	fw.scrollBy(0, 2, gdkScrollUnitSurface, fakeEv{t: 1502})
	fw.ptrMotion(250, 10, fakeEv{t: 1503})
	fw.scrollBy(0, 2, gdkScrollUnitSurface, fakeEv{t: 1504})
	fw.scrollEnd(fakeEv{t: 1505})
	if !p.c.scr.ended || p.parentScroll != parentEnded {
		t.Errorf("gesture end: child ended %v, parentScroll %d", p.c.scr.ended, p.parentScroll)
	}
	fw.decelerate(0, 0)
	if p.c.scr.ended || p.parentScroll != parentNone {
		t.Errorf("decelerate: child ended %v, parentScroll %d", p.c.scr.ended, p.parentScroll)
	}
	fw.scrollBegin(fakeEv{t: 1506}) // off the track
	fw.scrollBy(0, 2, gdkScrollUnitSurface, fakeEv{t: 1507})
	fw.scrollEnd(fakeEv{t: 1508})
	fw.decelerate(0, 0)
	fp.unit = 0
	if got := h.take(); len(got) != 6 || got[0] != "enter 20,-18" || got[2] != "scroll 0,-2 precise began at 20,-18" ||
		got[3] != "leave 20,-18" || got[4] != "scroll 0,-2 precise changed at 20,-18" || got[5] != "scroll 0,0 precise ended at 20,-18" {
		t.Errorf("gesture: %q", got)
	}

	// A press on the track that reaches the parent's gesture, and a
	// child's press released over the track.
	fw.ptrMotion(60, 10, fakeEv{t: 16})
	fw.ptrPressed(1, 60, 10, fakeEv{t: 17, button: 1})
	fw.ptrReleased(1, 60, 10, fakeEv{t: 18, button: 1})
	fp.ptrMotion(10, 5, fakeEv{t: 19})
	fp.ptrPressed(1, 10, 5, fakeEv{t: 2000, button: 1})
	fp.ptrMotion(10, -10, fakeEv{t: 2001})
	fp.ptrReleased(1, 10, -10, fakeEv{t: 2002, button: 1})
	idles("press", 0)
	wantLog(t, "press", h.take(), "enter 20,-18", "move 20,-18", "down 20,-18 b=1 held=1 n=1", "up 20,-18 b=1 held=0 n=1",
		"move 10,5", "down 10,5 b=1 held=1 n=1", "move 10,-10", "up 10,-10 b=1 held=0 n=1")
	if p.dismissed || f.s.grab != p {
		t.Fatal("dismissed by a press on the track")
	}
	wantLog(t, "parent", ph.take())

	// Reposition keeps the track.
	moved := trackCfg
	moved.Anchor.X, moved.Track = 80, driver.Rect{}
	p.Reposition(moved)
	fp.layoutNatural()
	h.take()
	if p.track != trackCfg.Track || p.cfg.Track != trackCfg.Track {
		t.Errorf("after Reposition: track %+v, cfg %+v", p.track, p.cfg.Track)
	}

	// A dismissal drops a pending leave.
	fp.ptrMotion(10, 5, fakeEv{t: 2003})
	fp.ptrLeave(fakeEv{t: 2004})
	fp.closed()
	idles("dismissed", 0)
	wantLog(t, "dismissed", h.take(), "move 10,5", "leave 10,5", "dismissed")
	p.Close()
	h.take()

	// An empty track is none: the parent's moves over it go nowhere and a
	// press there dismisses.
	bad := trackCfg
	bad.Track.W = math.NaN()
	h2 := &popHandler{}
	p2, _ := openPopup(t, f, sf, bad, h2)
	h2.take()
	if p2.track != (driver.Rect{}) {
		t.Errorf("an empty track kept: %+v", p2.track)
	}
	fw.ptrMotion(50, 10, fakeEv{t: 2005})
	fw.ptrPressed(1, 50, 10, fakeEv{t: 2006, button: 1})
	wantLog(t, "empty track", h2.take(), "dismissed")
	p2.Close()
}

// TestPopupRepositionKeepsPointer: a grab popup moved by Reposition (a
// menu strip switching menus, S5.6f) keeps the child's last pointer
// position where it is on the screen, in the new place's points, so a
// wheel over the track that follows is at it.
func TestPopupRepositionKeepsPointer(t *testing.T) {
	_, f := newSurfaceTest(t)
	sf, fw, _ := openParent(t, f)
	h := &popHandler{}
	p, _ := openPopup(t, f, sf, trackCfg, h)
	fw.ptrMotion(50, 10, fakeEv{t: 1}) // the track
	wantLog(t, "open", h.take(), "placed 40,28 100x80@1", "enter 10,-18", "move 10,-18")
	cfg := trackCfg
	cfg.Anchor.X += 30
	p.Reposition(cfg)
	if got := h.take(); len(got) != 1 || got[0] != "placed 70,28 100x80@1" {
		t.Fatalf("Reposition: %q", got)
	}
	if lp := p.c.ptr.lastPos; lp != (driver.Point{X: -20, Y: -18}) {
		t.Errorf("last position %v, want -20,-18", lp)
	}
	fw.scrollBy(0, 1, 0, fakeEv{t: 2})
	wantLog(t, "wheel", h.take(), "scroll 0,-3 at -20,-18")
}
