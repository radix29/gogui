package fake

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// Modal surfaces (S5.7d, shell.SurfaceConfig.Modal). The shell keeps the
// modal surfaces that have been shown and are still open in the order they
// were first shown; the last one is the active modal and blocks every
// other surface. Blocking is computed, not stored: a surface is blocked
// while an active modal exists and it is not that modal. The side effects
// of becoming blocked (grab, capture, leave) run when a modal is first
// shown.

// owner resolves SurfaceConfig.Owner for NewSurface.
func (s *Shell) owner(cfg shell.SurfaceConfig) (*Surface, error) {
	if cfg.Owner == nil {
		return nil, nil
	}
	o, ok := cfg.Owner.(*Surface)
	if !cfg.Modal || !ok || o == nil || o.shell != s || o.dead {
		return nil, shell.ErrSurfaceOwner
	}
	return o, nil
}

// activeModal returns the modal surface that blocks the others, or nil.
func (s *Shell) activeModal() *Surface {
	if n := len(s.modals); n > 0 {
		return s.modals[n-1]
	}
	return nil
}

// Modal returns the active modal surface (the one shown last of the open,
// shown modal surfaces), or nil when no surface is blocked.
func (s *Shell) Modal() *Surface {
	s.check("Shell.Modal")
	return s.activeModal()
}

// blocked reports whether a modal surface blocks sf.
func (sf *Surface) blocked() bool {
	m := sf.shell.activeModal()
	return m != nil && m != sf
}

// Blocked reports whether a modal surface blocks the surface (its input,
// activation and user moves, resizes and closes are refused).
func (sf *Surface) Blocked() bool { sf.check("Blocked"); return sf.blocked() }

// Modal reports whether the surface was created with SurfaceConfig.Modal.
func (sf *Surface) Modal() bool { sf.check("Modal"); return sf.modal }

// Owner returns the owner from SurfaceConfig (nil for an app-modal or a
// non-modal surface).
func (sf *Surface) Owner() *Surface { sf.check("Owner"); return sf.owner }

// showModal runs at the first Show of a modal surface: it places it,
// blocks the other surfaces and becomes the active modal.
func (sf *Surface) showModal() {
	s := sf.shell
	sf.modalShown = true
	sf.prevKey = s.key
	sf.place()
	s.modals = append(s.modals, sf)
	for _, o := range append([]*Surface(nil), s.surfaces...) {
		if o != sf && !o.dead {
			o.block()
		}
	}
}

// block ends what a surface that becomes blocked holds: its grab popup,
// its pointer capture (PointerUp for every held button) and the pointer's
// presence (PointerLeave), as an OS capture cancel does.
func (sf *Surface) block() {
	sf.ptr.eaten = 0
	sf.ptr.caption = false
	sf.dismissGrab()
	if sf.dead {
		return
	}
	for b := shell.ButtonPrimary; b <= shell.ButtonForward; b <<= 1 {
		if sf.ptr.buttons&b != 0 {
			sf.ptr.buttons &^= b
			sf.pointerEvent(shell.PointerUp, b, 0)
		}
	}
	if sf.ptr.inside && !sf.dead {
		sf.ptr.inside = false
		sf.pointerEvent(shell.PointerLeave, shell.ButtonNone, 0)
	}
}

// place centers a modal surface on its owner, when that is shown and not
// minimized, or else on the work area, and keeps it inside the work area
// (its top-left wins when it is larger).
func (sf *Surface) place() {
	s := sf.shell
	area := s.work
	if o := sf.owner; o != nil && !o.dead && o.visible && !o.minimized {
		area = driver.Rect{X: o.pos.X, Y: o.pos.Y, W: o.size.W, H: o.size.H}
	}
	x := area.X + (area.W-sf.size.W)/2
	y := area.Y + (area.H-sf.size.H)/2
	w := s.work
	x = math.Max(math.Min(x, w.X+w.W-sf.size.W), w.X)
	y = math.Max(math.Min(y, w.Y+w.H-sf.size.H), w.Y)
	sf.pos = driver.Point{X: x, Y: y}
	sf.centers++
}

// leaveModality runs when a modal surface closes, before its Closed (as on
// the real shells): it leaves the modal list, so it stops blocking the
// others.
func (sf *Surface) leaveModality() {
	s := sf.shell
	for i, m := range s.modals {
		if m == sf {
			s.modals = append(s.modals[:i], s.modals[i+1:]...)
			break
		}
	}
}

// handOnKey runs after a closed modal surface's Closed, as on gtk4 (Win32
// hands the key on before it): if the modal was key, the key goes to the
// next modal, else to Owner, else to the surface key at its first Show.
// Quit hands nothing on.
func (sf *Surface) handOnKey(wasKey bool) {
	s := sf.shell
	if !wasKey || s.quitting {
		return
	}
	shown := func(o *Surface) bool {
		return o != nil && !o.dead && !o.closing && o.visible && !o.minimized
	}
	switch m := s.activeModal(); {
	case m != nil:
		s.setKey(m)
	case shown(sf.owner):
		s.setKey(sf.owner)
	case shown(sf.prevKey):
		s.setKey(sf.prevKey)
	}
}

// closeOwned closes the modal surfaces sf owns, newest first, before sf
// itself closes.
func (sf *Surface) closeOwned() {
	for {
		var last *Surface
		for _, o := range sf.shell.surfaces {
			if o.owner == sf {
				last = o
			}
		}
		if last == nil {
			return
		}
		last.Close()
	}
}
