package gtk4

import "github.com/radix29/gogui/internal/shell"

// Modal surfaces (S5.7f, shell.SurfaceConfig.Modal), as the fake shell
// models them (S5.7d) and the Win32 shell builds them (S5.7e):
//
//   - The window is a surface's GtkWindow (the CSD shadow, the hidden title
//     bar: no GTK decorations), never resizable, made modal at creation
//     (gtk_window_set_modal: GTK's grab in the window group while it is
//     shown, the modal hint to the window manager). At its first Show,
//     before it is mapped, it becomes transient for Owner or, app-modal,
//     for the surface key then (decided in S5.7f: so the window manager
//     centres it over the app and gives it no taskbar entry; none without a
//     key surface). GTK 4 cannot place a toplevel, so the window manager
//     places it: GNOME's mutter attaches a transient modal to its parent
//     like a sheet. Minimize and Zoom do nothing; Center does nothing (as
//     for every gtk4 surface).
//   - The shell's shown modal surfaces are kept in first-Show order; the
//     last one is the active modal. applyBlocking makes every other open
//     surface insensitive (gtk_widget_set_sensitive on its window, GTK's
//     counterpart of EnableWindow: no controller of it runs, GTK's resize
//     borders included) and the rest sensitive again; it runs at a modal's
//     first Show, when a modal closes and for each new surface. Becoming
//     blocked ends the surface's grab popup, its pointer state (a PointerUp
//     with Clicks 0 for every held button, then PointerLeave), its scroll
//     momentum and its held keys.
//   - What still reaches a blocked surface is dropped in Go as well
//     (pointerLive, keyLive), so it holds whatever GTK delivers: pointer,
//     scroll, key, text and preedit input are dropped (a key press is
//     consumed); its activation (notify::is-active) is not reported as
//     KeyChanged and presents the active modal instead, posted; a
//     close-request is refused without CloseRequested and presents the
//     active modal. Show on a blocked surface presents it, then the active
//     modal.
//   - Closing: the surface leaves the modal list and the others become
//     sensitive again before its window is destroyed; after its Closed, if
//     it was key, the next modal, else Owner, else the surface key at its
//     first Show (open, shown, not minimized, not blocked) is presented, as
//     on the fake (the window manager may also activate the transient
//     parent on its own). Closing the owner closes its modal surfaces
//     first, newest first. Quit hands nothing on.
//
// Presenting a window is a request: on Wayland the compositor decides
// whether it gets the focus.

// modalOwner resolves SurfaceConfig.Owner for NewSurface.
func (s *Shell) modalOwner(cfg shell.SurfaceConfig) (*surface, error) {
	if cfg.Owner == nil {
		return nil, nil
	}
	o, ok := cfg.Owner.(*surface)
	if !cfg.Modal || !ok || o == nil || o.s != s || o.pop != nil || o.dead {
		return nil, shell.ErrSurfaceOwner
	}
	return o, nil
}

// activeModal returns the modal surface that blocks the others, or nil.
func (s *Shell) activeModal() *surface {
	if n := len(s.modals); n > 0 {
		return s.modals[n-1]
	}
	return nil
}

// keySurface returns the open surface reported key last, or nil.
func (s *Shell) keySurface() *surface {
	for _, o := range s.surfaces {
		if sf, ok := o.(*surface); ok && sf.key && !sf.dead {
			return sf
		}
	}
	return nil
}

// applyBlocking blocks every open surface but the active modal and
// unblocks the rest (every surface without an active modal).
func (s *Shell) applyBlocking() {
	m := s.activeModal()
	for _, o := range append([]openSurface(nil), s.surfaces...) {
		if sf, ok := o.(*surface); ok && !sf.dead {
			sf.setBlocked(m != nil && sf != m)
		}
	}
}

// inputBlocked reports whether a modal surface blocks sf's input: its own
// window's, or, for a popup's child, its parent's.
func (sf *surface) inputBlocked() bool {
	if sf.pop != nil {
		return sf.pop.parent.blocked
	}
	return sf.blocked
}

// setBlocked makes the surface's window insensitive or sensitive again.
// Becoming blocked ends its grab popup, pointer state, scroll momentum and
// held keys first.
func (sf *surface) setBlocked(b bool) {
	if b == sf.blocked || sf.dead {
		return
	}
	sf.blocked = b
	if b {
		sf.dismissGrabOf()
		if sf.dead {
			return
		}
		sf.endPointer()
		if sf.dead || !sf.stopMomentum() {
			return
		}
		sf.scr.gesture = false
		clear(sf.kbd.down)
	}
	sf.w.setSensitive(!b)
	rethrow()
}

// endPointer ends the surface's pointer state as a capture cancel does: a
// PointerUp with Clicks 0 for every held button at the last position, then
// PointerLeave if the pointer was inside. A caption press just ends.
func (sf *surface) endPointer() {
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
}

// firstModalShow runs at the first Show of a modal surface, before its
// window is presented: it remembers the key surface and makes the window
// transient for Owner, or for that key surface.
func (sf *surface) firstModalShow() {
	sf.modalShown = true
	sf.prevKey = sf.s.keySurface()
	var parent window
	switch {
	case sf.owner != nil && !sf.owner.dead:
		parent = sf.owner.w
	case sf.prevKey != nil && sf.prevKey != sf:
		parent = sf.prevKey.w
	}
	sf.w.setTransientFor(parent)
	rethrow()
}

// becomeModal runs after the first Show of a modal surface presented it:
// it becomes the active modal and blocks the other surfaces.
func (sf *surface) becomeModal() {
	sf.s.modals = append(sf.s.modals, sf)
	sf.s.applyBlocking()
}

// shownNormal reports whether sf is open, shown and not minimized.
func (sf *surface) shownNormal() bool {
	return sf != nil && !sf.dead && sf.shown && !sf.minimized
}

// leaveModality runs while a modal surface closes, before its window is
// destroyed: it leaves the modal list and the others are unblocked. Under
// Quit it only leaves the list.
func (sf *surface) leaveModality() {
	s := sf.s
	for i, m := range s.modals {
		if m == sf {
			s.modals = append(s.modals[:i], s.modals[i+1:]...)
			if !s.quit {
				s.applyBlocking()
			}
			return
		}
	}
}

// handKey runs after a closed modal surface's Closed, if it was key: the
// next modal, else the owner, else the surface key at its first Show is
// presented.
func (sf *surface) handKey() {
	s := sf.s
	if s.quit {
		return
	}
	var to *surface
	switch m := s.activeModal(); {
	case m != nil:
		to = m
	case sf.owner.shownNormal() && !sf.owner.blocked:
		to = sf.owner
	case sf.prevKey.shownNormal() && !sf.prevKey.blocked:
		to = sf.prevKey
	}
	if to != nil {
		to.w.present()
		rethrow()
	}
}

// closeOwned closes the modal surfaces sf owns, newest first.
func (sf *surface) closeOwned(silent bool) {
	for {
		var last *surface
		for _, o := range sf.s.surfaces {
			if m, ok := o.(*surface); ok && m.owner == sf && !m.dead {
				last = m
			}
		}
		if last == nil {
			return
		}
		last.closeNow(silent)
		if !silent {
			rethrow()
		}
		if !last.dead { // it did not leave: do not loop
			return
		}
	}
}

// modalKey presents the active modal surface (a blocked surface was
// activated or asked to close).
func (s *Shell) modalKey() {
	if m := s.activeModal(); m != nil && !m.dead {
		m.w.present()
		rethrow()
	}
}
