package gtk4

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// Popups (S5.5f): a GtkPopover without arrow, parented to the parent
// surface's GGSurface, whose only child is a second GGSurface. That child
// is a *surface of its own (c, with pop set): its snapshot paints through
// the canvas, its pointer and scroll controllers are the surfaces' (input.go,
// scroll.go). The popover's own padding, margins, border, background and
// shadow are cleared by the CSS class popoverClass from one display-wide
// GtkCssProvider (popover_linux.go), so the child fills the popover's
// surface.
//
// Placement. GTK places the popover (create_popup_layout in gtkpopover.c):
// GTK_POS_BOTTOM with halign START in LTR anchors the popover's top-left
// corner (GDK_GRAVITY_NORTH_WEST) at the bottom-left corner
// (GDK_GRAVITY_SOUTH_WEST) of pointing_to, moved by the popover's offset,
// with the anchor hints FLIP_Y | SLIDE_X | RESIZE. popoverPlace turns a
// PopupConfig into that: pointing_to is the anchor (whole logical pixels,
// at least 1×1 as GTK makes it, kept inside the parent's area as
// xdg_positioner requires), the offset what is left to reach the anchor's
// top-left plus Offset. Adjust is GTK's: the compositor may slide X, flip Y (above the
// anchor when the popover does not fit below it) and resize both ways;
// the bits of PopupConfig.Adjust are not passed on (GTK has no call for
// its anchor hints).
//
// GTK presents the popover's surface with its natural size: the child's
// measure reports the requested size as natural and 0 as minimum, so a
// RESIZE by the compositor can give it less. A shown popover is presented
// again by its parent's size_allocate (gtk_popover_present, which GTK
// requires of a widget with a popover child) and by Reposition
// (gtk_popover_set_pointing_to presents a visible popover).
//
// Placed. The size is the child's allocation; the position is where the
// popover's GdkPopup landed relative to the parent's GdkSurface, moved
// into the parent GGSurface's coordinates (popover.origin). It is checked
// after every allocation of the child, after the parent's size_allocate
// presented it, after Reposition and on a scale change of the parent;
// a change of the rect or the scale is reported. The first Paint waits for
// the first Placed. On Wayland the popup is configured asynchronously, so
// Placed comes from the loop, after NewPopup returned.
//
// The grab (Grab): autohide on, GTK's popup grab (Wayland's
// xdg_popup.grab). The parent's key controller sits on its window in the
// capture phase, so it sees every key first; while the grab lasts the
// parent's key, text and preedit events go to the popup's handler
// (surface.keySink), and a key the handler declines reaches the
// popover's own handling (Escape pops it down). The parent's pointer
// signals that still arrive (the implicit grab of a held button, GTK's
// grab redirection) are handed to the child in its coordinates
// (fromParent); a press outside the popup is eaten, its release too, and
// dismisses it. Dismissal, once (dismiss): "closed" (GTK popped it down:
// an outside press, Escape, the compositor's popup_done), an eaten press,
// the parent losing the key state, a resize of the parent or its
// minimizing. GTK 4 does not report moves of a toplevel, so a move ends
// the grab only through the press on the title bar that starts it.
//
// Without Grab (tooltips) autohide is off and the popover has can-target
// false: no pointer event picks it, they go to the parent below. GTK
// does not dismiss it.
//
// Translucent is false: GNOME offers no blur behind a popup (§13).
type popup struct {
	s      *Shell
	h      shell.PopupHandler
	parent *surface
	c      *surface // the child GGSurface's surface
	nw     popover  // the native popover (c.w)

	cfg    shell.PopupConfig
	grab   bool
	target [2]int32 // where popoverPlace asked the top-left to go, parent pixels

	placed     bool        // Placed has been called
	rect       driver.Rect // the rect last reported through Placed
	scale      float64     // the scale last reported through Placed
	placements int         // Placed calls (tests)

	// inherited are the parent's buttons held when the grab began, held
	// for the child until their release; eaten are the buttons pressed
	// outside, whose release is eaten too.
	inherited, eaten shell.PointerButton

	// track is the grab's track area in the parent's points (S5.6e; zero:
	// none); leaveID and leaveH are the idle source of a deferred
	// PointerLeave (0: none); parentScroll: whether a scroll gesture of the
	// parent is handed to the child (fromParentScroll).
	track        driver.Rect
	leaveID      uint32
	leaveH       ffi.Handle
	parentScroll uint8

	opening   bool // inside NewPopup: a failure closes it without Closed
	dismissed bool // Dismissed was called
	dead      bool // closed
}

var _ shell.Popup = (*popup)(nil)

// popover is the native side of a popup: a GtkPopover around a GGSurface
// (popover_linux.go, a fake in the host tests). Its window methods are
// the child GGSurface's (those a toplevel needs do nothing). Every method
// runs on the GTK thread and may run trampolines.
type popover interface {
	window
	// place sets the child's natural size (from its peer's measure: it
	// queues a resize), the offset and pointing_to, in that order; the
	// last presents a shown popover again.
	place(pl popoverPlacement)
	// popup shows the popover (gtk_popover_popup).
	popup()
	// present is gtk_popover_present, from the parent's size_allocate.
	present()
	// origin returns where the child's top-left corner landed, in the
	// parent GGSurface's logical pixels; ok is false while the popover
	// has no surface.
	origin() (x, y float64, ok bool)
}

// popoverPlacement is what GTK places a popover from (popoverPlace).
type popoverPlacement struct {
	pointing gtk.Rectangle // pointing_to, in the parent GGSurface's coordinates
	dx, dy   int32         // gtk_popover_set_offset
	w, h     int32         // the child's natural size
}

// popoverPlace converts cfg into a placement for a parent of parentW ×
// parentH logical pixels (see popup), and returns the top-left corner it
// asks for. The anchor is rounded to whole pixels, made at least 1×1 and
// kept within the parent's area; the offset then reaches the anchor's
// rounded top-left plus Offset whatever the anchor became.
func popoverPlace(cfg shell.PopupConfig, parentW, parentH int32) (popoverPlacement, [2]int32) {
	ax, ay := pxPos(cfg.Anchor.X), pxPos(cfg.Anchor.Y)
	aw, ah := shellkit.ToPx(cfg.Anchor.W, 1), shellkit.ToPx(cfg.Anchor.H, 1)
	x0, x1 := clampSpan(ax, ax+max(aw, 1), parentW)
	y0, y1 := clampSpan(ay, ay+max(ah, 1), parentH)
	tx, ty := pxPos(finite(cfg.Anchor.X)+finite(cfg.Offset.X)), pxPos(finite(cfg.Anchor.Y)+finite(cfg.Offset.Y))
	pl := popoverPlacement{
		pointing: gtk.Rectangle{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0},
		dx:       tx - x0,
		dy:       ty - y1,
		w:        shellkit.ToPx(length(cfg.Size.W), 1),
		h:        shellkit.ToPx(length(cfg.Size.H), 1),
	}
	return pl, [2]int32{tx, ty}
}

// clampSpan keeps [a, b) (b > a) within [0, n), at least one pixel wide;
// n below 1 counts as 1.
func clampSpan(a, b, n int32) (int32, int32) {
	n = max(n, 1)
	a = min(max(a, 0), n-1)
	b = min(max(b, a+1), n)
	return a, b
}

// pxPos rounds a position to whole logical pixels, NaN and infinities as
// 0, within ±shellkit.MaxPx.
func pxPos(v float64) int32 {
	return int32(math.Max(-shellkit.MaxPx, math.Min(finite(math.Round(v)), shellkit.MaxPx)))
}

func finite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// length is a size component: negative, NaN and infinite ones count as 0.
func length(v float64) float64 {
	if !(v > 0) || math.IsInf(v, 1) {
		return 0
	}
	return v
}

// popupSink is the child surface's SurfaceHandler: its paints and input
// events go to the PopupHandler. The child never reports Resized,
// KeyChanged, CloseRequested or Closed (its popup does).
type popupSink struct{ p *popup }

func (k popupSink) Paint(c render.Canvas)      { k.p.h.Paint(c) }
func (k popupSink) Event(ev shell.Event) bool  { return k.p.h.Event(ev) }
func (popupSink) Resized(driver.Size, float64) {}
func (popupSink) KeyChanged(bool)              {}
func (popupSink) CloseRequested() bool         { return false }
func (popupSink) Closed()                      {}

// NewPopup implements shell.PopupShell. The popover is created, placed and
// shown (gtk_popover_popup); Placed follows its first allocation (on X11
// usually inside NewPopup, on Wayland from the loop), the first paint the
// frame after it.
func (s *Shell) NewPopup(parent shell.Surface, cfg shell.PopupConfig, h shell.PopupHandler) (shell.Popup, error) {
	s.checkReady("NewPopup")
	if h == nil {
		panic("gtk4: NewPopup with a nil handler")
	}
	s.notPainting("NewPopup")
	ps, ok := parent.(*surface)
	if !ok || ps == nil || ps.s != s || ps.pop != nil || ps.dead || s.quit || !ps.shown || ps.minimized {
		return nil, shell.ErrPopupParent
	}
	if cfg.Grab && s.grab != nil {
		return nil, shell.ErrPopupGrab
	}
	cfg.Size = driver.Size{W: length(cfg.Size.W), H: length(cfg.Size.H)}
	p := &popup{s: s, h: h, parent: ps, cfg: cfg, grab: cfg.Grab, opening: true}
	if cfg.Grab && shellkit.IsTrack(cfg.Track) {
		p.track = cfg.Track
	}
	c := &surface{s: s, h: popupSink{p}, pop: p, creating: true, invalid: true, shown: true,
		want: cfg.Size, size: cfg.Size, scale: ps.scale}
	p.c = c
	nw, err := s.loop.newPopover(ps.w, ps, cfg.Grab, c)
	if err == nil && pendingPanic != nil {
		nw.destroy()
	}
	rethrow()
	if err != nil {
		return nil, err
	}
	c.w, p.nw = nw, nw
	c.creating = false
	s.popups = append(s.popups, p)
	pl, target := popoverPlace(cfg, ps.allocW, ps.allocH)
	p.target = target
	nw.place(pl)
	rethrow()
	if p.dead {
		return p, nil
	}
	p.opening = false
	if p.grab {
		s.grab = p
		p.startGrab()
		if p.dead {
			return p, nil
		}
	}
	nw.popup()
	rethrow()
	if !p.dead {
		p.checkPlaced()
	}
	return p, nil
}

// startGrab ends the parent's pointer state as an OS cancel does (a
// PointerUp with Clicks 0 for every held button, then PointerLeave); the
// held buttons stay held for the child.
func (p *popup) startGrab() {
	sf := p.parent
	held := sf.ptr.buttons
	sf.ptr.buttons, sf.ptr.caption = 0, false
	for b := shell.ButtonPrimary; b <= shell.ButtonForward && !sf.dead; b <<= 1 {
		if held&b != 0 {
			sf.pointer(shell.PointerUp, sf.ptr.lastPos, b, 0, 0)
		}
	}
	if sf.ptr.inside && !sf.dead {
		sf.ptr.inside, sf.ptr.left = false, false
		sf.pointer(shell.PointerLeave, sf.ptr.lastPos, shell.ButtonNone, 0, 0)
	}
	if p.grabbing() {
		p.inherited = held
		p.c.ptr.buttons |= held
	}
}

// grabbing reports whether the popup holds the shell's grab.
func (p *popup) grabbing() bool { return !p.dead && p.s.grab == p }

// grabbed returns the grab popup of sf, or nil.
func (sf *surface) grabbed() *popup {
	if g := sf.s.grab; g != nil && g.parent == sf && !g.dead {
		return g
	}
	return nil
}

// eventSink is what a surface's key, text and preedit events go to.
type eventSink interface{ Event(shell.Event) bool }

// keySink returns where sf's key, text and preedit events go: its grab
// popup's handler while it has one, else its own.
func (sf *surface) keySink() eventSink {
	if g := sf.grabbed(); g != nil {
		return g.h
	}
	return sf.h
}

// dismiss ends the grab and calls Dismissed, once: the buttons held on
// the child get their PointerUp (Clicks 0), then PointerLeave if the
// pointer was over it.
func (p *popup) dismiss() {
	if !p.grabbing() || p.dismissed {
		return
	}
	p.dismissed = true
	p.s.grab = nil
	p.inherited, p.eaten = 0, 0
	p.cancelLeave()
	p.parentScroll = parentNone
	c := p.c
	held := c.ptr.buttons
	c.ptr.buttons, c.ptr.left = 0, false
	for b := shell.ButtonPrimary; b <= shell.ButtonForward && !p.dead; b <<= 1 {
		if held&b != 0 {
			c.pointer(shell.PointerUp, c.ptr.lastPos, b, 0, 0)
		}
	}
	if c.ptr.inside && !p.dead {
		c.ptr.inside = false
		c.pointer(shell.PointerLeave, c.ptr.lastPos, shell.ButtonNone, 0, 0)
	}
	if !p.dead {
		p.h.Dismissed()
	}
}

// dismissGrabOf dismisses sf's grab popup, if it has one.
func (sf *surface) dismissGrabOf() {
	if g := sf.grabbed(); g != nil {
		g.dismiss()
	}
}

// toPopup converts a point of the parent into the popup's points.
func (p *popup) toPopup(pos driver.Point) driver.Point {
	return driver.Point{X: pos.X - p.rect.X, Y: pos.Y - p.rect.Y}
}

// fromParentButton handles the parent's pointer gesture's "pressed" and
// "released" during the grab: a press over the popup is the child's, one
// outside is eaten and dismisses; the release of an eaten press is eaten,
// that of a button the child holds is the child's.
func (p *popup) fromParentButton(k kind, c uintptr, pos driver.Point) {
	gb := p.parent.w.currentButton(c)
	b := pointerButton(gb)
	if b == shell.ButtonNone {
		return
	}
	q := p.toPopup(pos)
	switch k {
	case kindPointerPressed:
		if p.placed && !p.c.outside(q) {
			p.c.press(gb, q, c)
			return
		}
		p.eaten |= b
		p.dismiss()
	case kindPointerReleased:
		if p.eaten&b != 0 {
			p.eaten &^= b
			return
		}
		if p.c.ptr.buttons&b != 0 {
			p.c.release(gb, q, c)
		}
	}
}

// fromParentMotion handles the parent's "enter" and "motion" during the
// grab: a move over the popup, or anywhere while the child holds a
// button, is the child's; a move outside ends the child's hover.
func (p *popup) fromParentMotion(c uintptr, pos driver.Point) {
	if !p.placed {
		return
	}
	q, ch := p.toPopup(pos), p.c
	switch {
	case !ch.outside(q) || ch.ptr.buttons != 0:
		ch.motion(q, c)
		if ch.outside(q) && !ch.dead {
			ch.ptr.left = true // its PointerLeave follows the last release
		}
	case ch.ptr.inside:
		ch.leave(c)
	}
}

// The grab's track area (S5.6e). The child's area is its own rect plus
// the track (surface.outside asks onTrack), so the parent's moves over the
// track, forwarded by fromParentMotion, are the child's with enter and
// leave for the whole area, and a release there keeps the pointer in.
// GTK reports the popover and the parent as two surfaces: crossing between
// them is a "leave" of one and an "enter" of the other, in that order and
// in one batch of GDK's events. So a leave from the child's own controller,
// or from the parent's while the pointer was on the track, is deferred to
// an idle source (lower priority than GDK's events): a move into the area
// before it runs cancels it, otherwise it sends the PointerLeave. The
// parent's scrolls over the track are the child's (fromParentScroll).
//
// Presses: GDK's autohide check (check_autohide in gdksurface.c) hides
// the popover on a press on any other surface of the app and drops the
// press before GTK sees it, so a press on the track normally dismisses
// the popup (the documented degradation, PLAN.md S5.6e); one that reaches
// fromParentButton all the same is the child's.

// onTrack reports whether q (popup points) lies on the track while the
// popup holds the grab.
func (p *popup) onTrack(q driver.Point) bool {
	if p.track.W == 0 || !p.placed || !p.grabbing() {
		return false
	}
	return shellkit.InRect(p.track, driver.Point{X: q.X + p.rect.X, Y: q.Y + p.rect.Y})
}

// deferLeave defers the child's PointerLeave to an idle source and
// reports whether it did: only for a popup with a track whose child has
// the pointer and no button held.
func (p *popup) deferLeave() bool {
	c := p.c
	if p.track.W == 0 || !p.grabbing() || !c.ptr.inside || c.ptr.buttons != 0 || c.ptr.caption {
		return false
	}
	if p.leaveID == 0 {
		p.leaveID, p.leaveH = p.s.loop.idle(p, kindPopupLeave)
		if p.leaveID == 0 {
			return false // no source: leave at once
		}
	}
	return true
}

// cancelLeave drops a deferred PointerLeave.
func (p *popup) cancelLeave() {
	if p.leaveID == 0 {
		return
	}
	p.s.loop.remove(p.leaveID)
	dispatch.Free(p.leaveH)
	p.leaveID, p.leaveH = 0, 0
}

// source implements sourcePeer: the deferred PointerLeave, once.
func (p *popup) source(kind) bool {
	p.leaveID, p.leaveH = 0, 0 // sourceFunc frees the handle
	if !p.dead && p.c.ptr.buttons == 0 {
		p.c.leave(0)
	}
	return false
}

// parentLeft is the parent's "leave" during the grab: with the pointer on
// the track it is the child's leave (deferred; kept for the last release
// while a button is held).
func (p *popup) parentLeft() {
	c := p.c
	if !c.ptr.inside || c.inOwn(c.ptr.lastPos) {
		return
	}
	if c.ptr.buttons != 0 {
		c.ptr.left = true
		return
	}
	if !p.deferLeave() {
		c.leave(0)
	}
}

// fromParentScroll handles the parent's scroll controller during the
// grab: a gesture that begins, or a wheel step, while the pointer is on
// the track is the child's, at the child's last position; the end of a
// gesture handed over follows it, and the decelerate right after that end.
func (p *popup) fromParentScroll(k kind, c uintptr, dx, dy float64) {
	ch := p.c
	onTrack := ch.ptr.inside && !ch.inOwn(ch.ptr.lastPos)
	handed := p.parentScroll == parentGesture
	if k != kindScrollDecelerate {
		p.parentScroll = parentNone // a decelerate comes right after the end only
	}
	switch k {
	case kindScrollBegin:
		if onTrack {
			p.parentScroll = parentGesture
			ch.scrollBegin()
		}
	case kindScroll:
		if handed || (onTrack && !ch.scr.gesture) {
			if handed {
				p.parentScroll = parentGesture
			}
			ch.scroll(dx, dy, c)
		}
	case kindScrollEnd:
		if handed {
			p.parentScroll = parentEnded
			ch.scrollEnd(c)
		}
	case kindScrollDecelerate:
		if p.parentScroll == parentEnded {
			p.parentScroll = parentNone
			ch.decelerate(dx, dy)
		}
	}
}

// The states of popup.parentScroll.
const (
	parentNone    = iota // no gesture of the parent is the child's
	parentGesture        // one began over the track
	parentEnded          // it just ended: its decelerate is the child's
)

// measure is the child's measure: the requested size as natural, no
// minimum, so the compositor may give it less (RESIZE).
func (p *popup) measure(orientation int32) (int32, int32) {
	w, h := px(p.c.want)
	if orientation == gtk.OrientationHorizontal {
		return 0, w
	}
	return 0, h
}

// allocated is the child's size_allocate: the size it got, reported with
// the place through checkPlaced.
func (p *popup) allocated(width, height int32) {
	c := p.c
	c.allocW, c.allocH = width, height
	size := shellkit.FromPx(width, height, 1, p.cfg.Size)
	resized := size != c.size || !p.placed
	c.size = size
	p.checkPlaced()
	if resized && !p.dead {
		c.invalidate()
	}
}

// checkPlaced reports a new place or scale through Placed: once the child
// is allocated and the popover has a surface. A coordinate on the target
// popoverPlace asked for is reported exactly as configured.
func (p *popup) checkPlaced() {
	c := p.c
	if p.dead || (c.allocW == 0 && c.allocH == 0) {
		return
	}
	x, y, ok := p.nw.origin()
	rethrow()
	if !ok || p.dead {
		return
	}
	r := driver.Rect{X: x, Y: y, W: c.size.W, H: c.size.H}
	if pxPos(x) == p.target[0] && x == math.Round(x) {
		r.X = finite(p.cfg.Anchor.X) + finite(p.cfg.Offset.X)
	}
	if pxPos(y) == p.target[1] && y == math.Round(y) {
		r.Y = finite(p.cfg.Anchor.Y) + finite(p.cfg.Offset.Y)
	}
	sc := p.nw.scale()
	if !(sc > 0) {
		sc = p.parent.scale
	}
	if p.placed && r == p.rect && sc == p.scale {
		return
	}
	scaled := p.placed && sc != p.scale
	if p.placed {
		// The pointer stays where it is on the screen: its last position
		// follows the move into the new place's points (S5.6f).
		c.ptr.lastPos = c.ptr.lastPos.Add(p.rect.Origin()).Sub(r.Origin())
	}
	p.placed, p.rect, p.scale, c.scale = true, r, sc, sc
	p.placements++
	p.h.Placed(r, sc)
	if scaled && !p.dead {
		c.invalidate()
	}
}

// closedSignal is GtkPopover::closed: GTK popped the popover down. For a
// grab popup that is a dismissal; a popup without grab is not dismissed.
func (p *popup) closedSignal() {
	if p.grab && !p.dead {
		p.dismiss()
	}
}

// live checks the thread and reports whether the popup is open.
func (p *popup) live(what string) bool {
	p.s.q.Check("gtk4.Popup." + what)
	return !p.dead
}

// change is live for the methods that must not run during a Paint.
func (p *popup) change(what string) bool {
	if !p.live(what) {
		return false
	}
	p.s.notPainting("Popup." + what)
	return true
}

// Invalidate implements shell.Popup.
func (p *popup) Invalidate() {
	if p.live("Invalidate") {
		p.c.invalidate()
	}
}

// SetCursor implements shell.Popup: the child GGSurface's cursor.
func (p *popup) SetCursor(cur shell.Cursor) {
	if p.live("SetCursor") {
		p.c.SetCursor(cur)
	}
}

// Reposition implements shell.Popup: the popover is placed again (GTK
// presents it at once), and the result is checked.
func (p *popup) Reposition(cfg shell.PopupConfig) {
	if !p.change("Reposition") {
		return
	}
	cfg.Grab, cfg.Track = p.grab, p.cfg.Track
	cfg.Size = driver.Size{W: length(cfg.Size.W), H: length(cfg.Size.H)}
	p.cfg = cfg
	p.c.want = cfg.Size
	pl, target := popoverPlace(cfg, p.parent.allocW, p.parent.allocH)
	p.target = target
	p.nw.place(pl)
	rethrow()
	p.checkPlaced()
}

// Translucent implements shell.Popup: false, GNOME blurs nothing behind a
// popup.
func (p *popup) Translucent() bool {
	p.s.q.Check("gtk4.Popup.Translucent")
	return false
}

// Close implements shell.Popup.
func (p *popup) Close() {
	if p.change("Close") {
		p.close(false)
		rethrow()
	}
}

// close ends the grab without events, leaves the shell, destroys the
// popover (its signals disconnected first, so its popdown is not a
// "closed"; unparented from the parent's GGSurface) and, unless silent or
// still opening, calls Closed.
func (p *popup) close(silent bool) {
	if p.dead {
		return
	}
	p.dead, p.c.dead, p.c.invalid = true, true, false
	p.cancelLeave()
	s := p.s
	if s.grab == p {
		s.grab = nil
	}
	for i, o := range s.popups {
		if o == p {
			s.popups = append(s.popups[:i], s.popups[i+1:]...)
			break
		}
	}
	if p.nw != nil {
		p.nw.destroy()
	}
	if silent || p.opening {
		return
	}
	p.h.Closed()
}

// closePopupsOf closes the popups of parent, the last opened first.
func (s *Shell) closePopupsOf(parent *surface, silent bool) {
	for {
		var last *popup
		for _, p := range s.popups {
			if p.parent == parent {
				last = p
			}
		}
		if last == nil {
			return
		}
		last.close(silent)
		if !silent {
			rethrow()
		}
	}
}

// parentAllocated runs at the end of the parent's size_allocate: a resize
// dismisses its grab popup; every popup of it is presented (GTK requires
// it of a widget with popover children) and its place checked.
func (s *Shell) parentAllocated(sf *surface, resized bool) {
	if resized {
		sf.dismissGrabOf()
	}
	for _, p := range append([]*popup(nil), s.popups...) {
		if p.parent != sf || p.dead {
			continue
		}
		p.nw.present()
		rethrow()
		p.checkPlaced()
	}
}

// parentScaleChanged checks the place of sf's popups after a scale change
// of sf (the popover's surface follows its parent's monitor).
func (s *Shell) parentScaleChanged(sf *surface) {
	for _, p := range append([]*popup(nil), s.popups...) {
		if p.parent == sf {
			p.checkPlaced()
		}
	}
}

// Caps implements shell.Shell: no PopupShadow, the popover's own shadow
// is cleared by the CSS class popoverClass, so a menu draws its own.
func (s *Shell) Caps() shell.Caps { return shell.Caps{} }
