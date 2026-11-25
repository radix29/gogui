//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"math"
	"unsafe"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// gtkWindow is the window of a surface over GTK (surface.go): a GtkWindow
// (GTK's toplevel list owns it), whose child is a GGSurface, and the
// window's GdkSurface (realized at creation). The gtkWindow holds one
// reference to each of the three, so its pointers stay valid until destroy
// whatever GTK did to the window before.
type gtkWindow struct {
	win, surf, gdk uintptr
	// click and drag are the caption gestures on the GGSurface (chrome.go;
	// 0 if GTK returned none). The GGSurface owns them, so they live as
	// long as the reference to it.
	click, drag uintptr
	// motion and pointer are the pointer controllers (input.go; 0 if GTK
	// returned none), owned by the GGSurface too.
	motion, pointer uintptr
	// scroll is the scroll controller (scroll.go; 0 if GTK returned none),
	// owned by the GGSurface too.
	scroll uintptr
	// key is the key controller on the window and im the input method
	// (keyboard.go; 0 if GTK returned none). The window owns key; the
	// gtkWindow holds the reference to im until releaseIM, which the
	// surface runs after destroy, once the event being handled is over
	// (ime.go). imAttached: key has im as its IM context (imAttach).
	key, im    uintptr
	imAttached bool
	// conns are the signal handlers connected, disconnected by destroy so
	// no signal of a dying window reaches the surface and every handle is
	// freed at once (closureNotify).
	conns []conn
}

type conn struct{ inst, id uintptr }

var _ window = (*gtkWindow)(nil)

func gbool(b bool) int32 {
	if b {
		return gtk.True
	}
	return gtk.False
}

// newWindow implements loop: the GtkWindow with client-side decorations
// and a hidden, empty title bar (gtk_window_set_titlebar enables CSD, so
// GTK keeps its shadow and resize borders; an invisible title bar takes no
// space, so the GGSurface gets the whole content), the GGSurface child with
// p as its peer, the size request and the default size, the signals, and
// realize, which creates the GdkSurface.
func (l *gtkLoop) newWindow(cfg windowConfig, p windowPeer) (window, error) {
	g := &gtkWindow{win: gtk.Gtk.WindowNew()}
	if g.win == 0 {
		return nil, errors.New("gtk4: gtk_window_new returned NULL")
	}
	gtk.GObject.ObjectRef(g.win)
	fail := func(err error) (window, error) {
		g.destroy()
		g.releaseIM() // no event of it ran yet
		return nil, err
	}
	g.surf = newSurfaceWidget(p)
	if g.surf == 0 {
		return fail(errors.New("gtk4: could not create the GGSurface"))
	}
	if tb := newSurfaceWidget(nil); tb != 0 {
		gtk.Gtk.WidgetSetVisible(tb, gtk.False)
		gtk.Gtk.WindowSetTitlebar(g.win, tb) // the window takes its own reference
		releaseSurfaceWidget(tb)
	}
	gtk.Gtk.WindowSetTitle(g.win, cfg.title)
	gtk.Gtk.WindowSetResizable(g.win, gbool(cfg.resizable))
	if cfg.modal { // modal.go
		gtk.Gtk.WindowSetModal(g.win, gtk.True)
	}
	gtk.Gtk.WidgetSetSizeRequest(g.surf, cfg.minW, cfg.minH)
	gtk.Gtk.WindowSetDefaultSize(g.win, cfg.width, cfg.height)
	gtk.Gtk.WindowSetChild(g.win, g.surf)
	if !g.connect(g.win, "close-request", tramp.sigBool, p, kindCloseRequest) {
		return fail(errors.New("gtk4: could not connect GtkWindow::close-request"))
	}
	g.connect(g.win, "notify::is-active", tramp.sigPointer, p, kindActive)
	g.addCaptionGestures(p)
	g.addPointerControllers(g.surf, p)
	g.addScrollController(g.surf, p)
	g.addKeyboard(p)
	gtk.Gtk.WidgetRealize(g.win)
	if g.gdk = gtk.Gtk.NativeGetSurface(g.win); g.gdk != 0 {
		gtk.GObject.ObjectRef(g.gdk)
		g.connect(g.gdk, "notify::state", tramp.sigPointer, p, kindState)
		g.connect(g.gdk, "notify::scale", tramp.sigPointer, p, kindScale)
	}
	return g, nil
}

// connect connects one of the window's signals and keeps the handler.
func (g *gtkWindow) connect(inst uintptr, signal string, tr uintptr, p any, k kind) bool {
	id := connect(inst, signal, tr, p, k)
	if id != 0 {
		g.conns = append(g.conns, conn{inst, id})
	}
	return id != 0
}

// addCaptionGestures adds the caption gestures (chrome.go) to the
// GGSurface, both for the primary button only: a GtkGestureClick whose
// "pressed" counts the presses, and a GtkGestureDrag whose "drag-begin"
// and "drag-update" track a press. Without them the caption does not
// move the window, but the surface works.
func (g *gtkWindow) addCaptionGestures(p windowPeer) {
	if g.click = gtk.Gtk.GestureClickNew(); g.click != 0 {
		gtk.Gtk.GestureSingleSetButton(g.click, gtk.ButtonPrimary)
		gtk.Gtk.WidgetAddController(g.surf, g.click) // transfer full
		g.connect(g.click, "pressed", tramp.sigIntDoubleDouble, p, kindCaptionPress)
	}
	if g.drag = gtk.Gtk.GestureDragNew(); g.drag != 0 {
		gtk.Gtk.GestureSingleSetButton(g.drag, gtk.ButtonPrimary)
		gtk.Gtk.WidgetAddController(g.surf, g.drag)
		g.connect(g.drag, "drag-begin", tramp.sigDoubleDouble, p, kindCaptionDragBegin)
		g.connect(g.drag, "drag-update", tramp.sigDoubleDouble, p, kindCaptionDragUpdate)
	}
}

func (g *gtkWindow) eventTime(c uintptr) uint32 {
	return gtk.Gtk.EventControllerGetCurrentEventTime(c)
}

// pastDragThreshold checks the offset from 0, 0, as GtkWindowHandle does,
// rounded to whole logical pixels (GTK's _double variant is private).
func (g *gtkWindow) pastDragThreshold(dx, dy float64) bool {
	r := func(v float64) int32 { return int32(max(min(math.Round(v), math.MaxInt32), math.MinInt32)) }
	return gtk.Gtk.DragCheckThreshold(g.surf, 0, 0, r(dx), r(dy)) != gtk.False
}

// beginMove follows GtkWindowHandle: it claims the press (other gestures
// in the widget tree drop it), converts the start from the GGSurface's
// coordinates to the GdkSurface's (the window's widget coordinates plus
// the CSD shadow), starts the move with the press's device, button and
// time, and resets both caption gestures and the pointer gesture, whose
// press the compositor now
// owns.
func (g *gtkWindow) beginMove(drag uintptr, x, y float64) {
	gtk.Gtk.GestureSetState(drag, gtk.EventSequenceClaimed)
	if dev := gtk.Gtk.GestureGetDevice(drag); dev != 0 && g.gdk != 0 {
		in := gtk.GraphenePoint{X: float32(x), Y: float32(y)}
		out := in
		if gtk.Gtk.WidgetComputePoint(g.surf, g.win, &in, &out) == gtk.False {
			out = in
		}
		var dx, dy float64
		gtk.Gtk.NativeGetSurfaceTransform(g.win, &dx, &dy)
		button := int32(gtk.Gtk.GestureSingleGetCurrentButton(drag))
		gtk.Gdk.ToplevelBeginMove(g.gdk, dev, button, float64(out.X)+dx, float64(out.Y)+dy,
			gtk.Gtk.EventControllerGetCurrentEventTime(drag))
	}
	gtk.Gtk.EventControllerReset(drag)
	if g.click != 0 {
		gtk.Gtk.EventControllerReset(g.click)
	}
	// The claim denied the press to the pointer gesture, which GTK ends
	// without "cancel" or "released"; the reset cancels it (input.go).
	if g.pointer != 0 {
		gtk.Gtk.EventControllerReset(g.pointer)
	}
}

func (g *gtkWindow) widget() uintptr { return g.surf }

func (g *gtkWindow) setTitle(title string) { gtk.Gtk.WindowSetTitle(g.win, title) }

// setDefaultSize sets the window's default size for a drawable area of
// w × h. The default size is the window widget's border box, which also
// holds the window's own CSS padding and border (none with a compositor; a
// few pixels of solid-csd without one), so once both are allocated the
// difference between the window's border box (gtk_widget_compute_bounds;
// gtk_widget_get_width is the content box, which the child fills) and the
// GGSurface's size is added.
func (g *gtkWindow) setDefaultSize(w, h int32) {
	var b gtk.GrapheneRect
	sw, sh := gtk.Gtk.WidgetGetWidth(g.surf), gtk.Gtk.WidgetGetHeight(g.surf)
	if sw > 0 && sh > 0 && gtk.Gtk.WidgetComputeBounds(g.win, g.win, &b) != gtk.False {
		w += max(int32(math.Round(float64(b.Size.Width)))-sw, 0)
		h += max(int32(math.Round(float64(b.Size.Height)))-sh, 0)
	}
	gtk.Gtk.WindowSetDefaultSize(g.win, w, h)
}

func (g *gtkWindow) setSizeRequest(w, h int32) { gtk.Gtk.WidgetSetSizeRequest(g.surf, w, h) }

func (g *gtkWindow) queueResize() { gtk.Gtk.WidgetQueueResize(g.surf) }

func (g *gtkWindow) present() { gtk.Gtk.WindowPresent(g.win) }

// setTransientFor sets the window's transient parent (none for nil); the
// window does not hold a reference to it, GTK drops it when it goes.
func (g *gtkWindow) setTransientFor(parent window) {
	var pw uintptr
	if p, ok := parent.(*gtkWindow); ok && p != nil {
		pw = p.win
	}
	if g.win != 0 {
		gtk.Gtk.WindowSetTransientFor(g.win, pw)
	}
}

// setSensitive makes the whole window, its own resize borders included,
// take input or not.
func (g *gtkWindow) setSensitive(on bool) {
	if g.win != 0 {
		gtk.Gtk.WidgetSetSensitive(g.win, gbool(on))
	}
}

func (g *gtkWindow) minimize() { gtk.Gtk.WindowMinimize(g.win) }

func (g *gtkWindow) setMaximized(on bool) {
	if on {
		gtk.Gtk.WindowMaximize(g.win)
	} else {
		gtk.Gtk.WindowUnmaximize(g.win)
	}
}

func (g *gtkWindow) maximized() bool { return gtk.Gtk.WindowIsMaximized(g.win) != gtk.False }

func (g *gtkWindow) active() bool { return gtk.Gtk.WindowIsActive(g.win) != gtk.False }

func (g *gtkWindow) minimized() bool {
	const hidden = gtk.ToplevelStateMinimized | gtk.ToplevelStateSuspended
	return g.gdk != 0 && gtk.Gdk.ToplevelGetState(g.gdk)&hidden != 0
}

// scale returns the GdkSurface's fractional scale (gdk_surface_get_scale,
// required since the GTK 4.14 floor), or 1 without a GdkSurface.
func (g *gtkWindow) scale() float64 {
	if g.gdk != 0 {
		if s := gtk.Gdk.SurfaceGetScale(g.gdk); s > 0 {
			return s
		}
	}
	return 1
}

func (g *gtkWindow) queueDraw() { gtk.Gtk.WidgetQueueDraw(g.surf) }

func (g *gtkWindow) snapshotter(s uintptr) snapshotter { return newGtkSnapshot(s) }

func (g *gtkWindow) setCursor(name string) { gtk.Gtk.WidgetSetCursorFromName(g.surf, name) }

// destroy disconnects the handlers still connected (their handles are
// freed by closureNotify; a disposed object has dropped its own, and
// disconnecting those again would be a g_critical), destroys the window
// (GTK drops its reference; a second destroy does nothing), releases the
// GGSurface (its handle, then the reference) and drops the references to
// the window and the GdkSurface. It may run on a half-built window.
func (g *gtkWindow) destroy() {
	for _, c := range g.conns {
		if gtk.GObject.SignalHandlerIsConnected(c.inst, c.id) != gtk.False {
			gtk.GObject.SignalHandlerDisconnect(c.inst, c.id)
		}
	}
	g.conns = nil
	if g.im != 0 { // both before the GGSurface goes
		g.imAttach(false)
		gtk.Gtk.IMContextSetClientWidget(g.im, 0)
	}
	if g.win != 0 {
		gtk.Gtk.WindowDestroy(g.win)
	}
	releaseSurfaceWidget(g.surf)
	if g.gdk != 0 {
		gtk.GObject.ObjectUnref(g.gdk)
	}
	if g.win != 0 {
		gtk.GObject.ObjectUnref(g.win)
	}
	g.win, g.surf, g.gdk, g.click, g.drag, g.motion, g.pointer, g.scroll = 0, 0, 0, 0, 0, 0, 0, 0
	g.key = 0
}

// releaseIM drops the gtkWindow's reference to the input method, the last
// one once destroy detached it from the key controller. The surface runs
// it after destroy, outside the event that closed it: a close in a commit
// or preedit handler runs inside the input method (in the key controller's
// filter, or in IBus's D-Bus callback), which must outlive it.
func (g *gtkWindow) releaseIM() {
	if g.im != 0 {
		gtk.GObject.ObjectUnref(g.im)
		g.im = 0
	}
}

// addPointerControllers adds the pointer controllers (input.go) to w,
// the GGSurface (a popup's popover, popover_linux.go): a GtkEventControllerMotion ("enter", "motion", "leave") and
// a GtkGestureClick for every button (button 0: "pressed", "released",
// "cancel"). The widget owns them. Without them the surface gets no
// pointer events, but works.
func (g *gtkWindow) addPointerControllers(w uintptr, p windowPeer) {
	if g.motion = gtk.Gtk.EventControllerMotionNew(); g.motion != 0 {
		m := g.motion
		gtk.Gtk.WidgetAddController(w, m) // transfer full
		g.connect(m, "enter", tramp.sigDoubleDouble, p, kindPointerEnter)
		g.connect(m, "motion", tramp.sigDoubleDouble, p, kindPointerMotion)
		g.connect(m, "leave", tramp.sigVoid, p, kindPointerLeave)
	}
	if g.pointer = gtk.Gtk.GestureClickNew(); g.pointer != 0 {
		c := g.pointer
		gtk.Gtk.GestureSingleSetButton(c, 0)
		gtk.Gtk.WidgetAddController(w, c)
		g.connect(c, "pressed", tramp.sigIntDoubleDouble, p, kindPointerPressed)
		g.connect(c, "released", tramp.sigIntDoubleDouble, p, kindPointerReleased)
		g.connect(c, "cancel", tramp.sigPointer, p, kindPointerCancel)
	}
}

func (g *gtkWindow) eventState(c uintptr) uint32 {
	return gtk.Gtk.EventControllerGetCurrentEventState(c)
}

func (g *gtkWindow) currentButton(c uintptr) uint32 {
	return gtk.Gtk.GestureSingleGetCurrentButton(c)
}

// clickRules reads gtk-double-click-time (ms) and gtk-double-click-distance
// (logical pixels from the first press) from the GtkSettings, GTK's
// defaults (400 ms, 5 px) where a setting reads as 0 or less.
func (g *gtkWindow) clickRules() shellkit.ClickRules {
	t, d := settingInt("gtk-double-click-time", 400), settingInt("gtk-double-click-distance", 5)
	return shellkit.ClickRules{Time: uint32(max(t, 0)), W: 2 * max(d, 0), H: 2 * max(d, 0)}
}

// settingInt reads an int property of the default GtkSettings through a
// GValue in g_malloc memory (GLib writes it).
func settingInt(name string, def int32) int32 {
	st := gtk.Gtk.SettingsGetDefault()
	if st == 0 {
		return def
	}
	v := gtk.GLib.Malloc0(unsafe.Sizeof(gtk.GValue{}))
	defer gtk.GLib.Free(v)
	gtk.GObject.ValueInit(v, gtk.TypeInt)
	defer gtk.GObject.ValueUnset(v)
	gtk.GObject.ObjectGetProperty(st, name, v)
	if r := gtk.GObject.ValueGetInt(v); r > 0 {
		return r
	}
	return def
}

// addScrollController adds the scroll controller (scroll.go) to w,
// the GGSurface or a popup's popover: a GtkEventControllerScroll for both axes, KINETIC, so GTK
// reports the release velocity ("decelerate"). The widget owns it.
// Without it the surface gets no scroll events, but works.
func (g *gtkWindow) addScrollController(w uintptr, p windowPeer) {
	if g.scroll = gtk.Gtk.EventControllerScrollNew(gtk.ScrollBothAxes | gtk.ScrollKinetic); g.scroll == 0 {
		return
	}
	gtk.Gtk.WidgetAddController(w, g.scroll) // transfer full
	g.connect(g.scroll, "scroll", tramp.sigBoolDoubleDouble, p, kindScroll)
	g.connect(g.scroll, "scroll-begin", tramp.sigVoid, p, kindScrollBegin)
	g.connect(g.scroll, "scroll-end", tramp.sigVoid, p, kindScrollEnd)
	g.connect(g.scroll, "decelerate", tramp.sigDoubleDouble, p, kindScrollDecelerate)
}

func (g *gtkWindow) scrollUnit(c uintptr) uint32 { return gtk.Gtk.EventControllerScrollGetUnit(c) }

// addKeyboard adds the keyboard (keyboard.go): a GtkEventControllerKey on
// the window in the CAPTURE phase ("key-pressed", "key-released"), so it
// sees every key before GTK's own handling, and a GtkIMMulticontext with
// use-preedit (S5.3i; we draw the preedit) and the GGSurface as its client
// widget ("commit", and the preedit signals of ime.go). The key controller
// gets the input method only while the text input is on (imAttach, S5.3j).
// Without them the surface gets no keys, but works.
func (g *gtkWindow) addKeyboard(p windowPeer) {
	if g.key = gtk.Gtk.EventControllerKeyNew(); g.key != 0 {
		gtk.Gtk.EventControllerSetPropagationPhase(g.key, gtk.PhaseCapture)
		gtk.Gtk.WidgetAddController(g.win, g.key) // transfer full
		g.connect(g.key, "key-pressed", tramp.sigBoolUintUintUint, p, kindKeyPressed)
		g.connect(g.key, "key-released", tramp.sigUintUintUint, p, kindKeyReleased)
	}
	if g.im = gtk.Gtk.IMMulticontextNew(); g.im != 0 {
		gtk.Gtk.IMContextSetUsePreedit(g.im, gtk.True)
		gtk.Gtk.IMContextSetClientWidget(g.im, g.surf)
		g.connect(g.im, "commit", tramp.sigPointer, p, kindIMCommit)
		g.connect(g.im, "preedit-start", tramp.sigVoid, p, kindIMPreeditStart)
		g.connect(g.im, "preedit-changed", tramp.sigVoid, p, kindIMPreeditChanged)
		g.connect(g.im, "preedit-end", tramp.sigVoid, p, kindIMPreeditEnd)
	}
}

// preedit implements window: gtk_im_context_get_preedit_string, whose
// string (g_free) and PangoAttrList (unref) are freed here once read.
func (g *gtkWindow) preedit() (string, int, []preeditAttr) {
	if g.im == 0 {
		return "", 0, nil
	}
	var str, list uintptr
	var cursor int32
	gtk.Gtk.IMContextGetPreeditString(g.im, &str, &list, &cursor)
	text := ""
	if str != 0 {
		text = ffi.GoString(str)
		gtk.GLib.Free(str)
	}
	var attrs []preeditAttr
	if list != 0 {
		attrs = pangoAttrs(list)
		gtk.Pango.AttrListUnref(list)
	}
	return text, int(cursor), attrs
}

// pangoAttrs reads the attributes of the PangoAttrList list from the
// GSList of copies pango_attr_list_get_attributes returns, and frees the
// copies and the list's cells.
func pangoAttrs(list uintptr) []preeditAttr {
	head := gtk.Pango.AttrListGetAttributes(list)
	var out []preeditAttr
	for l := head; l != 0; l = ffi.ReadUintptr(l, unsafe.Offsetof(gtk.GSList{}.Next)) {
		a := ffi.ReadUintptr(l, unsafe.Offsetof(gtk.GSList{}.Data))
		if a == 0 {
			continue
		}
		pa := preeditAttr{typ: gtk.AttrType(a)}
		pa.start, pa.end = gtk.AttrRange(a)
		if pa.typ == gtk.AttrUnderline {
			pa.value = gtk.AttrInt(a)
		}
		out = append(out, pa)
		gtk.Pango.AttributeDestroy(a)
	}
	if head != 0 {
		gtk.GLib.SlistFree(head)
	}
	return out
}

// keyBase translates keycode at level 0 (no modifiers) in the layout group
// of the current key event.
func (g *gtkWindow) keyBase(c uintptr, keycode uint32) uint32 { return g.translateKey(c, keycode, 0) }

// keyShifted translates keycode with Shift alone in the layout group of
// the current key event (S5.6h).
func (g *gtkWindow) keyShifted(c uintptr, keycode uint32) uint32 {
	return g.translateKey(c, keycode, gdkShiftMask)
}

// translateKey is gdk_display_translate_key of keycode with state in the
// layout group of the current key event, 0 if unknown.
func (g *gtkWindow) translateKey(c uintptr, keycode, state uint32) uint32 {
	ev := gtk.Gtk.EventControllerGetCurrentEvent(c)
	disp := gtk.Gtk.WidgetGetDisplay(g.win)
	if ev == 0 || disp == 0 {
		return 0
	}
	var kv uint32
	if gtk.Gdk.DisplayTranslateKey(disp, keycode, state, int32(gtk.Gdk.KeyEventGetLayout(ev)), &kv, nil, nil, nil) == gtk.False {
		return 0
	}
	return kv
}

func (g *gtkWindow) keyvalToUnicode(kv uint32) rune { return rune(gtk.Gdk.KeyvalToUnicode(kv)) }

// imFilter holds a reference to the input method while it filters: a
// commit handler may close the surface, and destroy drops the gtkWindow's
// reference while the input method still runs.
func (g *gtkWindow) imFilter(c uintptr) bool {
	ev := gtk.Gtk.EventControllerGetCurrentEvent(c)
	if g.im == 0 || ev == 0 {
		return false
	}
	im := gtk.GObject.ObjectRef(g.im)
	defer gtk.GObject.ObjectUnref(im)
	return gtk.Gtk.IMContextFilterKeypress(im, ev) != gtk.False
}

func (g *gtkWindow) imReset() {
	if g.im != 0 {
		gtk.Gtk.IMContextReset(g.im)
	}
}

// imAttach gives the key controller the input method (on) or takes it
// away. While it has one, GTK filters every key through it before
// "key-pressed" and "key-released", and a key it takes reaches neither
// ("im-update" instead); the controller also focuses it in and out on its
// own focus crossings. gtk_event_controller_key_set_im_context resets the
// context it replaces and holds a reference to the new one until replaced
// or finalized.
func (g *gtkWindow) imAttach(on bool) {
	if g.key == 0 || g.im == 0 || on == g.imAttached {
		return
	}
	g.imAttached = on
	if on {
		gtk.Gtk.EventControllerKeySetIMContext(g.key, g.im)
	} else {
		gtk.Gtk.EventControllerKeySetIMContext(g.key, 0)
	}
}

// imCursor sets the input method's cursor location (the GGSurface's
// coordinates, points).
func (g *gtkWindow) imCursor(a gtk.Allocation) {
	if g.im != 0 {
		gtk.Gtk.IMContextSetCursorLocation(g.im, &a)
	}
}

// imEventTime is the time of the key event the key controller handles now
// (0 outside one): a commit or preedit from its filter is that key's.
func (g *gtkWindow) imEventTime() uint32 {
	if g.key == 0 {
		return 0
	}
	return gtk.Gtk.EventControllerGetCurrentEventTime(g.key)
}

func (g *gtkWindow) imFocus(in bool) {
	switch {
	case g.im == 0:
	case in:
		gtk.Gtk.IMContextFocusIn(g.im)
	default:
		gtk.Gtk.IMContextFocusOut(g.im)
	}
}

func (g *gtkWindow) commitString(p uintptr) string {
	if p == 0 {
		return ""
	}
	return ffi.GoString(p)
}

// readback implements window (readback.go): a new widget paintable of the
// GGSurface (it holds the widget's last render node, so nothing is
// painted here), snapshotted at its allocation under a scale transform,
// rendered by the window's own GSK renderer into a texture of the
// allocation × scale pixels, and downloaded into g_malloc memory (no Go
// pointer reaches GDK) in GDK_MEMORY_DEFAULT, premultiplied BGRA.
func (g *gtkWindow) readback(scale float64) (readbackFrame, error) {
	w, h := gtk.Gtk.WidgetGetWidth(g.surf), gtk.Gtk.WidgetGetHeight(g.surf)
	if w <= 0 || h <= 0 {
		return readbackFrame{}, fmt.Errorf("gtk4: readback: the GGSurface is allocated %d×%d", w, h)
	}
	r := gtk.Gtk.NativeGetRenderer(g.win)
	if r == 0 {
		return readbackFrame{}, errors.New("gtk4: readback: the window has no GSK renderer")
	}
	p := gtk.Gtk.WidgetPaintableNew(g.surf)
	if p == 0 {
		return readbackFrame{}, errors.New("gtk4: readback: gtk_widget_paintable_new returned NULL")
	}
	defer gtk.GObject.ObjectUnref(p)
	s := gtk.Gtk.SnapshotNew()
	if s == 0 {
		return readbackFrame{}, errors.New("gtk4: readback: gtk_snapshot_new returned NULL")
	}
	if scale != 1 {
		gtk.Gtk.SnapshotScale(s, float32(scale), float32(scale))
	}
	gtk.Gdk.PaintableSnapshot(p, s, float64(w), float64(h))
	node := gtk.Gtk.SnapshotFreeToNode(s)
	if node == 0 {
		return readbackFrame{}, errors.New("gtk4: readback: the GGSurface has drawn nothing yet")
	}
	defer gtk.Gsk.RenderNodeUnref(node)
	pw, ph := shellkit.ToPx(float64(w), scale), shellkit.ToPx(float64(h), scale)
	vp := gtk.Rect(0, 0, float32(pw), float32(ph))
	tex := gtk.Gsk.RendererRenderTexture(r, node, &vp)
	if tex == 0 {
		return readbackFrame{}, errors.New("gtk4: readback: gsk_renderer_render_texture returned NULL")
	}
	defer gtk.GObject.ObjectUnref(tex)
	tw, th := int(gtk.Gdk.TextureGetWidth(tex)), int(gtk.Gdk.TextureGetHeight(tex))
	f := readbackFrame{w: tw, h: th, stride: 4 * tw, renderer: gtypeName(r)}
	if n := f.stride * th; n > 0 {
		buf := gtk.GLib.Malloc0(uintptr(n))
		if buf == 0 {
			return readbackFrame{}, errors.New("gtk4: readback: g_malloc0 returned NULL")
		}
		defer gtk.GLib.Free(buf)
		gtk.Gdk.TextureDownload(tex, buf, uintptr(f.stride))
		f.pix = ffi.CopyBytes(buf, n)
	}
	return f, nil
}

// gtypeName returns the type name of a GObject instance
// (G_OBJECT_TYPE_NAME: the GType is the first word of its class, the class
// the first word of the instance).
func gtypeName(instance uintptr) string {
	return ffi.GoString(gtk.GObject.TypeName(ffi.ReadUintptr(ffi.ReadUintptr(instance, 0), 0)))
}
