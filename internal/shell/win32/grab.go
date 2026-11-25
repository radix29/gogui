//go:build windows

package win32

import (
	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The grab of a popup and its dismissal (S5.5c), as shell.PopupShell.NewPopup
// describes:
//
//   - Opening a grab popup ends the parent's pointer state as an OS capture
//     cancel does (a PointerUp with Clicks 0 for every held button, then
//     PointerLeave); the held buttons stay held for the popup. Then the
//     popup takes the mouse capture (SetCapture). The capture of a window
//     of the UI thread gets every mouse message of the thread's windows,
//     buttons held or not, so the parent sees none of them while the grab
//     lasts; a press over another process's window activates that window,
//     which deactivates the parent and so dismisses the popup.
//   - The popup's client mouse messages (positions in its client pixels,
//     also outside it) are PointerEvents in its points: enter, move and
//     leave for its own area, moves anywhere while a button is held,
//     presses over it. The cursor is set on each move (WM_SETCURSOR is not
//     sent to a capture): the popup's own over it, the arrow elsewhere.
//   - The track (PopupConfig.Track, S5.6d) is part of the popup's area for
//     all of this: the capture already gets the moves over the parent, so
//     the track only widens the test (area). It is converted into the
//     popup's points at each placement (trackRect); the parent does not
//     move during the grab, a move dismisses it.
//   - A press outside the area is eaten (its release too) and dismisses
//     it. Mouse messages that reach the parent anyway (sent or posted to
//     it, the wheel) are handed to the popup as if they were its own.
//   - The wheel goes to the focus window (the parent) or, with the
//     "scroll inactive windows" setting, to the capture: both reach the
//     popup, which takes scrolls over it and drops the others.
//   - The parent's key, character and IME messages are delivered to the
//     popup's handler (surface.keySink); the parent stays key.
//   - Dismissal, once (popup.dismiss): an outside press, the parent's
//     WM_ACTIVATE to inactive, WM_ACTIVATEAPP to another application,
//     WM_WINDOWPOSCHANGING that moves, sizes or hides it, WM_CANCELMODE to
//     either window, WM_ENTERSIZEMOVE and WM_ENTERMENULOOP of the parent,
//     and a WM_CAPTURECHANGED that takes the capture from the popup. The
//     capture is released and every button held on the popup gets its
//     PointerUp (then PointerLeave if the pointer was over it) before
//     Dismissed.
//   - Close ends the grab without any event.
//
// A popup without Grab answers WM_NCHITTEST with HTTRANSPARENT, so the
// mouse goes to the window below it (of the same thread: the parent), and
// it is never dismissed.

// popupInput is the pointer state of a grab popup, in its points.
type popupInput struct {
	inside   bool // between the PointerEnter and PointerLeave sent
	seen     bool // pos is valid
	pos      driver.Point
	buttons  shell.PointerButton // held on the popup
	eaten    shell.PointerButton // pressed outside: the release is eaten too
	upClicks [5]int
	clicks   shellkit.ClickCounter
	scroll   bool // a precise scroll gesture was delivered (its PhaseEnded is due)
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

// keySink returns where sf's key, character and IME events go: its grab
// popup's handler while it has one, else its own.
func (sf *surface) keySink() eventSink {
	if g := sf.grabbed(); g != nil {
		return g.h
	}
	return sf.h
}

// startGrab takes the grab for p, just shown: the parent's held buttons
// end with an OS cancel and stay held for p, then p takes the capture.
func (p *popup) startGrab() {
	sf := p.parent
	held := sf.endPointer()
	if !p.grabbing() {
		return // a handler closed it, or the parent
	}
	p.in.buttons = held
	if pt, err := w32.GetCursorPos(); err == nil {
		p.in.pos, p.in.seen = p.screenPoint(pt), true
	}
	w32.SetCapture(p.hwnd)
	rethrow()
}

// dismiss ends the grab and calls Dismissed, once.
func (p *popup) dismiss() {
	if !p.grabbing() || p.dismissed {
		return
	}
	p.dismissed = true
	p.s.grab = nil
	if w32.GetCapture() == p.hwnd {
		_ = w32.ReleaseCapture() // the grab is gone: its WM_CAPTURECHANGED is ignored
		rethrow()
	}
	for b := shell.ButtonPrimary; b <= shell.ButtonForward && !p.dead; b <<= 1 {
		if p.in.buttons&b != 0 {
			p.in.buttons &^= b
			p.pointer(shell.PointerUp, b, 0)
		}
	}
	p.in.buttons = 0
	if p.in.inside && !p.dead {
		p.in.inside = false
		p.pointer(shell.PointerLeave, shell.ButtonNone, 0)
	}
	p.endScroll()
	if !p.dead {
		p.h.Dismissed()
	}
}

// endGrab ends the grab without events (Close, the window going).
func (p *popup) endGrab() {
	if p.s.grab != p {
		return
	}
	p.s.grab = nil
	if p.hwnd != 0 && w32.GetCapture() == p.hwnd {
		_ = w32.ReleaseCapture()
	}
}

// pointer delivers a pointer event at the popup's pointer position.
func (p *popup) pointer(kind shell.PointerKind, button shell.PointerButton, clicks int) {
	if p.dead {
		return
	}
	p.h.Event(shell.PointerEvent{Kind: kind, Pos: p.in.pos, Button: button, Buttons: p.in.buttons,
		Clicks: clicks, Mods: modifiers(), Time: p.s.eventTime()})
}

// over reports whether q (popup points) lies in the popup.
func (p *popup) over(q driver.Point) bool {
	return q.X >= 0 && q.Y >= 0 && q.X < p.rect.W && q.Y < p.rect.H
}

// area reports whether q (popup points) lies in the popup's area: the
// popup or its track.
func (p *popup) area(q driver.Point) bool {
	return p.over(q) || (p.track.W > 0 && shellkit.InRect(p.track, q))
}

// trackRect converts the track of a grab popup (PopupConfig.Track, in the
// parent's points) into the popup's points through whole pixels, as the
// mouse positions are: the parent's client pixels at its scale, the
// screen, the popup's client pixels at its own scale. It is zero without a
// track.
func (p *popup) trackRect() driver.Rect {
	t := p.cfg.Track
	if !p.grab || !shellkit.IsTrack(t) {
		return driver.Rect{}
	}
	var po, wo w32.Point
	if w32.ClientToScreen(p.parent.hwnd, &po) != nil || w32.ClientToScreen(p.hwnd, &wo) != nil {
		return driver.Rect{} // a destroyed window
	}
	ps, q := p.parent.px(), p.px()
	x := pxRound(t.X*ps) + float64(po.X-wo.X)
	y := pxRound(t.Y*ps) + float64(po.Y-wo.Y)
	w, h := float64(shellkit.ToPx(t.W, ps)), float64(shellkit.ToPx(t.H, ps))
	if w == 0 || h == 0 {
		return driver.Rect{}
	}
	return driver.Rect{X: x / q, Y: y / q, W: w / q, H: h / q}
}

// moveTo moves the popup's pointer to q: PointerEnter and PointerMove when
// it comes over the popup, moves over it or while a button is held, and
// PointerLeave when it leaves with no button held. It sets the cursor.
func (p *popup) moveTo(q driver.Point) {
	moved := !p.in.seen || q != p.in.pos
	p.in.pos, p.in.seen = q, true
	switch {
	case p.area(q):
		w32.SetCursor(cursorHandle(p.cursor))
		if !p.in.inside {
			p.in.inside = true
			p.pointer(shell.PointerEnter, shell.ButtonNone, 0)
			p.pointer(shell.PointerMove, shell.ButtonNone, 0)
		} else if moved {
			p.pointer(shell.PointerMove, shell.ButtonNone, 0)
		}
		return
	case p.in.buttons != 0:
		if moved {
			p.pointer(shell.PointerMove, shell.ButtonNone, 0)
		}
	case p.in.inside:
		p.in.inside = false
		p.pointer(shell.PointerLeave, shell.ButtonNone, 0)
	}
	w32.SetCursor(cursorHandle(shell.CursorArrow))
}

// press handles a press of b at the popup client pixel px: the popup's
// over it or while a button is held on it, else eaten and dismissing.
func (p *popup) press(b shell.PointerButton, px w32.Point, dbl bool) {
	q := pointAt(px, w32.Rect{}, p.px())
	p.moveTo(q)
	if !p.grabbing() || p.in.buttons&b != 0 {
		return
	}
	if p.in.buttons == 0 && !p.area(q) {
		p.in.eaten |= b
		p.dismiss()
		return
	}
	n := p.in.clicks.Press(b, shellkit.Point(px), p.s.msgTime(), p.clickRules(), dbl)
	p.in.buttons |= b
	p.in.upClicks[buttonIndex(b)] = n
	p.pointer(shell.PointerDown, b, n)
}

// release handles the release of b at q (popup points).
func (p *popup) release(b shell.PointerButton, q driver.Point) {
	if p.in.eaten&b != 0 {
		p.in.eaten &^= b
		return
	}
	if !p.grabbing() {
		return
	}
	p.moveTo(q)
	if !p.grabbing() || p.in.buttons&b == 0 {
		return
	}
	p.in.buttons &^= b
	p.pointer(shell.PointerUp, b, p.in.upClicks[buttonIndex(b)])
	if p.in.buttons == 0 && p.in.inside && !p.area(p.in.pos) && p.grabbing() {
		p.in.inside = false
		p.pointer(shell.PointerLeave, shell.ButtonNone, 0)
	}
}

// mouse handles the popup's client mouse messages (positions in its client
// pixels). A popup that does not hold the grab takes none of them.
func (p *popup) mouse(h windows.HWND, msg uint32, wp, lp uintptr) (uintptr, bool) {
	var r uintptr
	if msg >= w32.WM_XBUTTONDOWN && msg <= w32.WM_XBUTTONDBLCLK {
		r = 1 // the X-button messages return TRUE
	}
	switch msg {
	case w32.WM_CAPTURECHANGED:
		if windows.HWND(lp) != h {
			p.dismiss() // the OS or another window took the capture
		}
		return 0, true
	case w32.WM_MOUSEMOVE:
		if p.grabbing() {
			p.moveTo(pointAt(signedXY(lp), w32.Rect{}, p.px()))
		}
		return 0, true
	}
	b, down, dbl, ok := buttonOf(msg, wp)
	if !ok {
		return 0, false
	}
	if down {
		if p.grabbing() {
			p.press(b, signedXY(lp), dbl)
		}
	} else {
		p.release(b, pointAt(signedXY(lp), w32.Rect{}, p.px()))
	}
	return r, true
}

// packXY packs pixels into a mouse lParam.
func packXY(pt w32.Point) uintptr { return uintptr(uint16(pt.X)) | uintptr(uint16(pt.Y))<<16 }

// fromParent hands a mouse message that reached the parent during the
// grab to the popup, its position moved into the popup's client pixels
// (from the parent's client pixels, or from the screen for the non-client
// messages). The non-client buttons become client ones.
func (p *popup) fromParent(msg uint32, wp, lp uintptr) (uintptr, bool) {
	pt := signedXY(lp)
	switch msg {
	case w32.WM_NCMOUSEMOVE, w32.WM_NCLBUTTONDOWN, w32.WM_NCLBUTTONDBLCLK, w32.WM_NCLBUTTONUP:
		msg += w32.WM_MOUSEMOVE - w32.WM_NCMOUSEMOVE
		wp = 0
	case w32.WM_NCMOUSELEAVE, w32.WM_MOUSELEAVE, w32.WM_CAPTURECHANGED:
		return 0, true
	default:
		_ = w32.ClientToScreen(p.parent.hwnd, &pt) // fails only for a destroyed window
	}
	_ = w32.ScreenToClient(p.hwnd, &pt)
	r, _ := p.mouse(p.hwnd, msg, wp, packXY(pt))
	return r, true
}

// scroll delivers a scroll event of the popup's window: only while it
// holds the grab and the pointer is over it; the PhaseEnded of a gesture
// that was delivered always comes.
func (p *popup) scroll(ev shell.ScrollEvent) {
	if ev.Phase == shell.PhaseEnded {
		if !p.in.scroll || p.dead {
			return
		}
		p.in.scroll = false
		p.h.Event(ev)
		return
	}
	if !p.grabbing() || !p.area(ev.Pos) {
		return
	}
	if ev.Phase != shell.PhaseNone {
		p.in.scroll = true
	}
	p.h.Event(ev)
}

// endScroll ends a delivered gesture at once (the grab ended).
func (p *popup) endScroll() {
	if p.gesture.End() && p.in.scroll && !p.dead {
		p.in.scroll = false
		_ = w32.KillTimer(p.hwnd, timerGesture)
		p.h.Event(shell.ScrollEvent{Pos: p.scrollPos, Precise: true, Phase: shell.PhaseEnded,
			Mods: modifiers(), Time: p.s.eventTime()})
	}
	p.in.scroll = false
}

// parentGrabMsg handles the parent's messages that matter to its grab
// popup g: the dismissals, and the mouse messages that go to g. handled
// reports that the message was the grab's and is done.
func (sf *surface) parentGrabMsg(g *popup, msg uint32, wp, lp uintptr) (r uintptr, handled bool) {
	switch msg {
	case w32.WM_ACTIVATE:
		if loword(wp) == w32.WA_INACTIVE {
			g.dismiss()
		}
	case w32.WM_ACTIVATEAPP:
		if wp == 0 {
			g.dismiss()
		}
	case w32.WM_WINDOWPOSCHANGING:
		if sf.movesOrHides(lp) {
			g.dismiss()
		}
	case w32.WM_CANCELMODE, w32.WM_ENTERSIZEMOVE, w32.WM_ENTERMENULOOP:
		g.dismiss()
	case w32.WM_MOUSEMOVE, w32.WM_MOUSELEAVE, w32.WM_CAPTURECHANGED,
		w32.WM_LBUTTONDOWN, w32.WM_LBUTTONUP, w32.WM_LBUTTONDBLCLK,
		w32.WM_RBUTTONDOWN, w32.WM_RBUTTONUP, w32.WM_RBUTTONDBLCLK,
		w32.WM_MBUTTONDOWN, w32.WM_MBUTTONUP, w32.WM_MBUTTONDBLCLK,
		w32.WM_XBUTTONDOWN, w32.WM_XBUTTONUP, w32.WM_XBUTTONDBLCLK,
		w32.WM_NCMOUSEMOVE, w32.WM_NCMOUSELEAVE, w32.WM_NCLBUTTONDOWN, w32.WM_NCLBUTTONDBLCLK,
		w32.WM_NCLBUTTONUP:
		return g.fromParent(msg, wp, lp)
	case w32.WM_MOUSEWHEEL, w32.WM_MOUSEHWHEEL:
		g.wheel(msg, wp, lp) // the position is in screen pixels
		return 0, true
	}
	return 0, false
}

// movesOrHides reports whether a WM_WINDOWPOSCHANGING (its WINDOWPOS at
// lp) moves, sizes or hides the window.
func (sf *surface) movesOrHides(lp uintptr) bool {
	wp := ffi.At[w32.WindowPos](lp)
	if wp.Flags&w32.SWP_HIDEWINDOW != 0 {
		return true
	}
	wr, err := w32.GetWindowRect(sf.hwnd)
	if err != nil {
		return false
	}
	if wp.Flags&w32.SWP_NOMOVE == 0 && (wp.X != wr.Left || wp.Y != wr.Top) {
		return true
	}
	return wp.Flags&w32.SWP_NOSIZE == 0 && (wp.Cx != wr.Right-wr.Left || wp.Cy != wr.Bottom-wr.Top)
}

// htTransparent is HTTRANSPARENT as an LRESULT (-1, sign-extended).
var htTransparent = func() uintptr { v := int64(w32.HTTRANSPARENT); return uintptr(v) }()
