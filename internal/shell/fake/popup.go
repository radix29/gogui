package fake

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render/record"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// Popup is a fake shell.Popup (S5.5a). The shell places it with
// shellkit.PlacePopup in its work area (Shell.SetWorkArea), with the parent
// at its screen position (Surface.UserMove), and paints it into a
// record.Canvas after the surfaces at each frame. A grab popup gets the
// parent's input as shell.PopupShell.NewPopup describes: drive it through the
// parent's input methods (Move, Press, Click, Wheel, KeyDown, Type …) in
// the parent's coordinates (ToParent converts), or send events to it
// directly with Send. A grab popup's track (PopupConfig.Track, S5.6d) is
// part of its area, kept in the parent's points from NewPopup on. Every
// method, the accessors included, must be called on the UI goroutine.
// After Close the state stays readable and the shell.Popup methods do
// nothing.
type Popup struct {
	shell  *Shell
	parent *Surface
	h      shell.PopupHandler

	cfg         shell.PopupConfig
	grab        bool
	track       driver.Rect // the grab's track area in the parent's points; zero: none
	rect        driver.Rect // placed, in the parent's points
	scale       float64
	placements  int  // placements (Placed calls unless SetDeferPlaced coalesced them)
	placedDue   bool // a deferred Placed is posted and has not run (SetDeferPlaced)
	cursor      shell.Cursor
	translucent bool

	dead, dismissed bool

	invalid  bool
	paints   int
	last     *record.Canvas
	paintErr error

	ptr pointer // the popup's own pointer state, in its coordinates
}

var _ shell.Popup = (*Popup)(nil)

// NewPopup implements shell.PopupShell. The popup is placed (Placed is called
// before NewPopup returns, or posted with SetDeferPlaced), shown and invalidated: its first paint happens
// at the next frame. Its scale is the parent's. Opening a grab popup sends
// the parent its capture cancel at once (see shell.PopupShell.NewPopup); the
// popup's own first pointer event comes with the next pointer move.
func (s *Shell) NewPopup(parent shell.Surface, cfg shell.PopupConfig, h shell.PopupHandler) (shell.Popup, error) {
	s.checkReady("Shell.NewPopup")
	s.notPainting("Shell.NewPopup")
	if h == nil {
		panic("fake: NewPopup with a nil handler")
	}
	sf, ok := parent.(*Surface)
	if !ok || sf == nil || sf.shell != s || sf.dead || !sf.visible || sf.minimized {
		return nil, shell.ErrPopupParent
	}
	if cfg.Grab && s.grab != nil {
		return nil, shell.ErrPopupGrab
	}
	p := &Popup{
		shell: s, parent: sf, h: h, grab: cfg.Grab,
		scale: sf.scale, translucent: s.translucent, invalid: true,
	}
	if cfg.Grab && shellkit.IsTrack(cfg.Track) {
		p.track = cfg.Track
	}
	s.popups = append(s.popups, p)
	p.place(cfg, true)
	s.clock.RequestFrame()
	if p.grab && !p.dead {
		s.grab = p
		sf.giveCapture(p)
	}
	return p, nil
}

// giveCapture ends the parent's pointer capture for the new grab popup g:
// a PointerUp for every held button and PointerLeave, as an OS capture
// cancel; the held buttons stay held for g.
func (sf *Surface) giveCapture(g *Popup) {
	held := sf.ptr.buttons
	for b := shell.ButtonPrimary; b <= shell.ButtonForward; b <<= 1 {
		if sf.ptr.buttons&b != 0 {
			sf.ptr.buttons &^= b
			sf.pointerEvent(shell.PointerUp, b, 0)
		}
	}
	sf.ptr.caption = false
	if sf.ptr.inside {
		sf.ptr.inside = false
		sf.pointerEvent(shell.PointerLeave, shell.ButtonNone, 0)
	}
	if g.dead || sf.shell.grab != g {
		return // a handler closed it
	}
	g.ptr.buttons = held
	g.ptr.pos = g.local(sf.ptr.pos)
}

// place places the popup from cfg and calls Placed when the rect or the
// scale changed (always the first time).
func (p *Popup) place(cfg shell.PopupConfig, first bool) {
	p.cfg = cfg
	o := p.parent.pos
	anchor := cfg.Anchor
	anchor.X += o.X
	anchor.Y += o.Y
	r := shellkit.PlacePopup(anchor, cfg.Size, cfg.Offset, cfg.Adjust, p.shell.work)
	r.X -= o.X
	r.Y -= o.Y
	if !first && r == p.rect && p.scale == p.parent.scale {
		return
	}
	resized := r.W != p.rect.W || r.H != p.rect.H
	if !first {
		// The pointer stays where it is on the screen: its position follows
		// the move into the new place's points (S5.6f).
		p.ptr.pos = p.ptr.pos.Add(p.rect.Origin()).Sub(r.Origin())
	}
	p.rect, p.scale = r, p.parent.scale
	p.placements++
	if resized && !first {
		p.Invalidate()
	}
	if p.shell.deferPlaced {
		if !p.placedDue {
			p.placedDue = true
			p.shell.q.Do(p.deliverPlaced)
		}
		return
	}
	p.h.Placed(r, p.scale)
}

// deliverPlaced is the posted Placed of SetDeferPlaced: the latest
// placement, unless the popup closed meanwhile.
func (p *Popup) deliverPlaced() {
	if p.dead || !p.placedDue {
		return
	}
	p.placedDue = false
	p.h.Placed(p.rect, p.scale)
	if p.invalid && !p.dead {
		p.shell.clock.invalidated() // it was skipped by the paint pass
	}
}

// PlacedPending reports whether a deferred Placed has not run yet
// (SetDeferPlaced).
func (p *Popup) PlacedPending() bool { p.check("PlacedPending"); return p.placedDue }

func (p *Popup) check(what string) { p.shell.check("Popup." + what) }

// change checks the thread, rejects calls during a Paint and reports
// whether the popup is still open.
func (p *Popup) change(what string) bool {
	p.check(what)
	if p.dead {
		return false
	}
	p.shell.notPainting("Popup." + what)
	return true
}

// Invalidate implements shell.Popup.
func (p *Popup) Invalidate() {
	p.check("Invalidate")
	if !p.dead {
		p.invalid = true
		p.shell.clock.invalidated()
	}
}

// SetCursor implements shell.Popup.
func (p *Popup) SetCursor(c shell.Cursor) {
	p.check("SetCursor")
	if !p.dead {
		p.cursor = c
	}
}

// Reposition implements shell.Popup. It panics during a Paint (Placed may
// not be called then).
func (p *Popup) Reposition(cfg shell.PopupConfig) {
	if p.change("Reposition") {
		cfg.Grab, cfg.Track = p.grab, p.cfg.Track
		p.place(cfg, false)
	}
}

// Translucent implements shell.Popup: the value of
// Shell.SetPopupTranslucent when the popup was created.
func (p *Popup) Translucent() bool { p.check("Translucent"); return p.translucent }

// Close implements shell.Popup. A grab ends without any event.
func (p *Popup) Close() {
	if !p.change("Close") {
		return
	}
	p.dead = true
	s := p.shell
	if s.grab == p {
		s.grab = nil
	}
	for i, o := range s.popups {
		if o == p {
			s.popups = append(s.popups[:i], s.popups[i+1:]...)
			break
		}
	}
	p.h.Closed()
}

// UserDismiss simulates the OS dismissing a grab popup without an outside
// press (WM_CANCELMODE, a lost capture, GTK hiding the popover): the grab
// ends (a button still held on the popup gets its PointerUp, then
// PointerLeave if the pointer was over it) and Dismissed is called. It does
// nothing for a popup without Grab, a dismissed or a closed one.
func (p *Popup) UserDismiss() {
	if p.change("UserDismiss") {
		p.dismiss()
	}
}

// dismiss ends the grab and calls Dismissed, once.
func (p *Popup) dismiss() {
	s := p.shell
	if p.dead || p.dismissed || s.grab != p {
		return
	}
	p.dismissed = true
	s.grab = nil
	p.cancelHeld()
	if !p.dead {
		p.h.Dismissed()
	}
}

// cancelHeld sends a PointerUp for every button held on the popup, then
// PointerLeave if the pointer is over it.
func (p *Popup) cancelHeld() {
	for b := shell.ButtonPrimary; b <= shell.ButtonForward; b <<= 1 {
		if p.ptr.buttons&b != 0 {
			p.ptr.buttons &^= b
			p.pointerEvent(shell.PointerUp, b, 0)
		}
	}
	if p.ptr.inside {
		p.ptr.inside = false
		p.pointerEvent(shell.PointerLeave, shell.ButtonNone, 0)
	}
}

// Accessors.

// Parent returns the popup's parent.
func (p *Popup) Parent() *Surface { p.check("Parent"); return p.parent }

// Config returns the configuration of the last NewPopup or Reposition.
func (p *Popup) Config() shell.PopupConfig { p.check("Config"); return p.cfg }

// Grab reports whether the popup is a grab popup.
func (p *Popup) Grab() bool { p.check("Grab"); return p.grab }

// Rect returns the placed rect in the parent's points, as passed to the
// last Placed.
func (p *Popup) Rect() driver.Rect { p.check("Rect"); return p.rect }

// Scale returns the scale passed to the last Placed.
func (p *Popup) Scale() float64 { p.check("Scale"); return p.scale }

// Placements returns how often Placed was called.
func (p *Popup) Placements() int { p.check("Placements"); return p.placements }

// Cursor returns the cursor set last (CursorArrow initially).
func (p *Popup) Cursor() shell.Cursor { p.check("Cursor"); return p.cursor }

// WasDismissed reports whether Dismissed has been called.
func (p *Popup) WasDismissed() bool { p.check("WasDismissed"); return p.dismissed }

// Closed reports whether the popup has closed.
func (p *Popup) Closed() bool { p.check("Closed"); return p.dead }

// Invalid reports whether the popup waits for a paint.
func (p *Popup) Invalid() bool { p.check("Invalid"); return p.invalid }

// Paints returns the number of paints so far.
func (p *Popup) Paints() int { p.check("Paints"); return p.paints }

// LastPaint returns the canvas of the last paint, or nil before the first.
func (p *Popup) LastPaint() *record.Canvas { p.check("LastPaint"); return p.last }

// PaintErr returns the contract violations of the last paint.
func (p *Popup) PaintErr() error { p.check("PaintErr"); return p.paintErr }

// Dump returns the dump of the last paint ("" before the first one).
func (p *Popup) Dump() string {
	p.check("Dump")
	if p.last == nil {
		return ""
	}
	return p.last.Dump()
}

// Pointer returns the popup's pointer state in its coordinates: the last
// position, whether the pointer is over it (between PointerEnter and
// PointerLeave) and the buttons held on it.
func (p *Popup) Pointer() (pos driver.Point, inside bool, buttons shell.PointerButton) {
	p.check("Pointer")
	return p.ptr.pos, p.ptr.inside, p.ptr.buttons
}

// ToParent converts a point in the popup's coordinates to the parent's,
// for the parent's input methods.
func (p *Popup) ToParent(q driver.Point) driver.Point {
	p.check("ToParent")
	return driver.Point{X: q.X + p.rect.X, Y: q.Y + p.rect.Y}
}

// Send delivers ev to the popup's handler as it is (a zero Time set to the
// clock's Now) and reports what it returned, for sequences the parent's
// input methods do not cover. It panics for ModPrimary.
func (p *Popup) Send(ev shell.Event) bool {
	if !p.change("Send") {
		return false
	}
	now := p.shell.clock.now
	switch e := ev.(type) {
	case shell.PointerEvent:
		noPrimary(e.Mods)
		if e.Time == 0 {
			e.Time = now
		}
		ev = e
	case shell.ScrollEvent:
		noPrimary(e.Mods)
		if e.Time == 0 {
			e.Time = now
		}
		ev = e
	case shell.KeyEvent:
		noPrimary(e.Mods)
		if e.Time == 0 {
			e.Time = now
		}
		ev = e
	case shell.TextEvent:
		if e.Time == 0 {
			e.Time = now
		}
		ev = e
	case shell.PreeditEvent:
		if e.Time == 0 {
			e.Time = now
		}
		ev = e
	}
	return p.h.Event(ev)
}

// paint runs PopupHandler.Paint on a new record.Canvas of the placed size.
func (p *Popup) paint() {
	p.invalid = false
	c := record.New(driver.Size{W: p.rect.W, H: p.rect.H})
	err := p.shell.timedPaint(func() { p.h.Paint(c) }, c)
	p.paints++
	p.last = c
	p.paintErr = err
}

// The grab's pointer routing (called from the parent's input methods).

// local converts a point in the parent's coordinates to the popup's.
func (p *Popup) local(q driver.Point) driver.Point {
	return driver.Point{X: q.X - p.rect.X, Y: q.Y - p.rect.Y}
}

// in reports whether q (popup coordinates) lies in the popup's area: its
// rect or its track (S5.6d).
func (p *Popup) in(q driver.Point) bool {
	if q.X >= 0 && q.Y >= 0 && q.X < p.rect.W && q.Y < p.rect.H {
		return true
	}
	return p.track.W > 0 && shellkit.InRect(p.track, driver.Point{X: q.X + p.rect.X, Y: q.Y + p.rect.Y})
}

func (p *Popup) pointerEvent(kind shell.PointerKind, button shell.PointerButton, clicks int) {
	p.h.Event(shell.PointerEvent{
		Kind: kind, Pos: p.ptr.pos, Button: button, Buttons: p.ptr.buttons,
		Clicks: clicks, Mods: p.parent.ptr.mods, Time: p.shell.clock.now,
	})
}

// moveTo moves the popup's pointer to q: PointerEnter and PointerMove when
// it comes over the popup (also while a button is held), moves while over
// it or while a button is held, and PointerLeave when it leaves with no
// button held.
func (p *Popup) moveTo(q driver.Point) {
	moved := q != p.ptr.pos
	p.ptr.pos = q
	switch {
	case p.in(q):
		if !p.ptr.inside {
			p.ptr.inside = true
			p.pointerEvent(shell.PointerEnter, shell.ButtonNone, 0)
			p.pointerEvent(shell.PointerMove, shell.ButtonNone, 0)
		} else if moved {
			p.pointerEvent(shell.PointerMove, shell.ButtonNone, 0)
		}
	case p.ptr.buttons != 0:
		if moved {
			p.pointerEvent(shell.PointerMove, shell.ButtonNone, 0)
		}
	case p.ptr.inside:
		p.ptr.inside = false
		p.pointerEvent(shell.PointerLeave, shell.ButtonNone, 0)
	}
}

// grabbed returns the grab popup of sf, or nil.
func (sf *Surface) grabbed() *Popup {
	if g := sf.shell.grab; g != nil && g.parent == sf {
		return g
	}
	return nil
}

// keyTo returns where sf's key, text and preedit events go: its grab
// popup's handler or its own.
func (sf *Surface) keyTo() interface{ Event(shell.Event) bool } {
	if g := sf.grabbed(); g != nil {
		return g.h
	}
	return sf.h
}

// pressGrab handles a press during a grab and reports whether there was
// one: over the popup (or with a button held on it) it is the popup's;
// elsewhere it is eaten and dismisses the popup.
func (sf *Surface) pressGrab(q driver.Point, b shell.PointerButton) bool {
	g := sf.grabbed()
	if g == nil {
		return false
	}
	sf.moveTo(q)
	if g = sf.grabbed(); g == nil {
		return true // a handler closed it during the move
	}
	lq := g.local(q)
	switch {
	case g.ptr.buttons&b != 0: // held already
	case g.ptr.buttons != 0 || g.in(lq):
		g.ptr.buttons |= b
		g.pointerEvent(shell.PointerDown, b, g.ptr.countClick(lq, b, sf.now()))
	default:
		sf.ptr.eaten |= b
		g.dismiss()
	}
	return true
}

// releaseGrab handles the release of an eaten press, and a release during
// a grab, and reports whether it did.
func (sf *Surface) releaseGrab(q driver.Point, b shell.PointerButton) bool {
	if sf.ptr.eaten&b != 0 {
		sf.ptr.eaten &^= b
		sf.moveTo(q)
		return true
	}
	g := sf.grabbed()
	if g == nil {
		return false
	}
	sf.moveTo(q)
	if g = sf.grabbed(); g == nil || g.ptr.buttons&b == 0 {
		return true
	}
	g.ptr.buttons &^= b
	clicks := 0
	if b == g.ptr.lastButton {
		clicks = g.ptr.clicks
	}
	g.pointerEvent(shell.PointerUp, b, clicks)
	if g.ptr.buttons == 0 && g.ptr.inside && !g.in(g.ptr.pos) && sf.grabbed() == g {
		g.ptr.inside = false
		g.pointerEvent(shell.PointerLeave, shell.ButtonNone, 0)
	}
	return true
}

// dismissGrab dismisses sf's grab popup, if it has one (the parent lost
// the key, moved, resized or was minimized).
func (sf *Surface) dismissGrab() {
	if g := sf.grabbed(); g != nil {
		g.dismiss()
	}
}

// closePopups closes sf's popups, last to first (before its Closed).
func (sf *Surface) closePopups() {
	ps := sf.Popups()
	for i := len(ps) - 1; i >= 0; i-- {
		ps[i].Close()
	}
}

// Popups returns the open popups of sf in creation order.
func (sf *Surface) Popups() []*Popup {
	sf.check("Popups")
	var ps []*Popup
	for _, p := range sf.shell.popups {
		if p.parent == sf {
			ps = append(ps, p)
		}
	}
	return ps
}

// Position returns the surface's top-left on the screen, in points
// ((0, 0) until UserMove).
func (sf *Surface) Position() driver.Point { sf.check("Position"); return sf.pos }

// UserMove simulates the user moving the surface so that its top-left is
// at pos on the screen (in points): the work area of the popups is in
// screen points, so this is where a popup's anchor lies. A move dismisses
// the surface's grab popup; popups without Grab stay where they are on the
// screen, and their Rect (in the surface's points) is not updated. A
// blocked surface cannot be moved by the user.
func (sf *Surface) UserMove(pos driver.Point) {
	if !sf.change("UserMove") || pos == sf.pos || sf.blocked() {
		return
	}
	sf.pos = pos
	sf.dismissGrab()
}

// Popups returns the open popups in creation order.
func (s *Shell) Popups() []*Popup {
	s.check("Shell.Popups")
	return append([]*Popup(nil), s.popups...)
}

// GrabPopup returns the open grab popup that holds the grab, or nil.
func (s *Shell) GrabPopup() *Popup { s.check("Shell.GrabPopup"); return s.grab }

// SetWorkArea sets the work area popups are placed in (in screen points;
// see Surface.UserMove). New shells have (0, 0) and DefaultScreenSize. It
// does not move open popups (Popup.Reposition does). Before Start it may be
// called from any goroutine; afterwards only on the UI goroutine.
func (s *Shell) SetWorkArea(r driver.Rect) {
	if s.started.Load() {
		s.check("Shell.SetWorkArea")
	}
	s.work = r
}

// WorkArea returns the work area set last.
func (s *Shell) WorkArea() driver.Rect { s.check("Shell.WorkArea"); return s.work }

// SetPopupTranslucent sets what Popup.Translucent reports for popups
// created afterwards (false initially: the fake has no blur).
func (s *Shell) SetPopupTranslucent(on bool) {
	if s.started.Load() {
		s.check("Shell.SetPopupTranslucent")
	}
	s.translucent = on
}

// SetDeferPlaced makes Placed asynchronous for popups, as gtk4 on Wayland
// (off initially, as Win32): NewPopup and Reposition still place the
// popup at once (Rect, Placements), but PopupHandler.Placed is posted and
// runs with the next Step (or the Run loop), with the rect and scale of
// the latest placement. Placements before that coalesce into one Placed.
// A popup whose Placed is pending is not painted. Before Start it may be
// called from any goroutine; afterwards only on the UI goroutine.
func (s *Shell) SetDeferPlaced(on bool) {
	if s.started.Load() {
		s.check("Shell.SetDeferPlaced")
	}
	s.deferPlaced = on
}

// popupInvalid reports whether one of sf's popups waits for a paint.
func (sf *Surface) popupInvalid() bool {
	for _, p := range sf.shell.popups {
		if p.parent == sf && p.invalid {
			return true
		}
	}
	return false
}

// Caps implements shell.Shell: PopupShadow is true (as Win32) unless
// SetCaps turned it off (as gtk4). It may be called from any goroutine.
func (s *Shell) Caps() shell.Caps { return shell.Caps{PopupShadow: !s.noShadow.Load()} }

// SetCaps sets what Caps reports (PopupShadow true initially). A real
// shell's capabilities never change; the fake's may, between tests'
// steps, and take effect for what reads them afterwards (the next popup
// a menu opens). It may be called from any goroutine.
func (s *Shell) SetCaps(c shell.Caps) { s.noShadow.Store(!c.PopupShadow) }
