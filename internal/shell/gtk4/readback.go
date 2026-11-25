package gtk4

import (
	"errors"
	"fmt"
	"math"

	"github.com/radix29/gogui/internal/shell/shellkit"
)

// The render seams and the readback (S4.3f): what a golden render of a
// surface on GTK needs (the Linux gallery test, S4.3g). With the fake
// appearance source of the tests (Shell.openSettings) they make the render
// independent of the machine:
//
//   - setFixedScale fixes the surface's scale (2× on a 1× display): the
//     canvas then snaps for that scale, and the readback rasterizes at it.
//   - setFixedKey fixes the key state, so the window draws as the key
//     window without a window manager activating it.
//   - readback renders the GGSurface's last drawn frame with the window's
//     own GSK renderer (what users see) and downloads its pixels:
//     gtk_widget_paintable_new → gdk_paintable_snapshot under a scale →
//     gsk_renderer_render_texture → gdk_texture_download.
//
// Only the tests call them; the linker drops them from apps.

// keyFixed is the key state a surface reports.
type keyFixed uint8

const (
	keyNative keyFixed = iota // gtk_window_is_active
	keyOn                     // always key
	keyOff                    // never key
)

// readbackFrame is a read-back render: w×h pixels in rows of stride bytes, each
// pixel premultiplied B, G, R, A (gdk_texture_download's
// GDK_MEMORY_DEFAULT), and the type name of the GSK renderer that drew it.
type readbackFrame struct {
	w, h, stride int
	pix          []byte
	renderer     string
}

// nativeScale is the scale the surface reports: the fixed one, else the
// GdkSurface's.
func (sf *surface) nativeScale() float64 {
	if sf.fixedScale > 0 {
		return sf.fixedScale
	}
	return sf.w.scale()
}

// scaleChanged follows notify::scale of the GdkSurface (and a new fixed
// scale): a changed, valid scale reports Resized with the size unchanged
// and invalidates. While NewSurface runs it only keeps the scale.
func (sf *surface) scaleChanged() {
	sc := sf.nativeScale()
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
}

// isActive is the key state the surface reports: the fixed one, else
// gtk_window_is_active.
func (sf *surface) isActive() bool {
	switch sf.fixedKey {
	case keyOn:
		return true
	case keyOff:
		return false
	}
	return sf.w.active()
}

// activeChanged follows notify::is-active of the window (and a new fixed
// key state): a change reports KeyChanged, after the input method's focus
// follows (keyFocus). While NewSurface runs nothing is reported. A blocked
// surface does not become key: its activation presents the active modal,
// posted (modal.go).
func (sf *surface) activeChanged() {
	if key := sf.isActive(); key != sf.key && !sf.creating {
		if key && sf.blocked {
			sf.s.Post(sf.s.modalKey)
			return
		}
		sf.key = key
		sf.keyMarks(key) // the first click (input.go)
		if !key {
			sf.dismissGrabOf() // before the parent's key loss
			if sf.dead {
				return
			}
		}
		sf.keyFocus(key)
		sf.h.KeyChanged(key)
	}
}

// setFixedScale fixes the surface's scale at sc whatever the GdkSurface's
// is (notify::scale then changes nothing), or returns it to the
// GdkSurface's with 0. A change is reported as for notify::scale. It
// panics on a scale that is not finite and at least 0.
func (sf *surface) setFixedScale(sc float64) {
	if !sf.change("setFixedScale") {
		return
	}
	if !(sc >= 0) || math.IsInf(sc, 1) {
		panic(fmt.Sprintf("gtk4: setFixedScale(%v)", sc))
	}
	sf.fixedScale = sc
	sf.scaleChanged()
	rethrow()
}

// setFixedKey fixes the surface's key state (keyOn, keyOff) whatever the
// window manager says, or returns it to gtk_window_is_active with
// keyNative. A change is reported as for notify::is-active.
func (sf *surface) setFixedKey(k keyFixed) {
	if !sf.change("setFixedKey") {
		return
	}
	if k > keyOff {
		panic(fmt.Sprintf("gtk4: setFixedKey(%d)", k))
	}
	sf.fixedKey = k
	sf.activeChanged()
	rethrow()
}

// The readback's errors.
var (
	errReadbackClosed  = errors.New("gtk4: readback of a closed surface")
	errReadbackHidden  = errors.New("gtk4: readback of a surface never shown (GTK draws only mapped widgets)")
	errReadbackPending = errors.New("gtk4: readback with a paint pending (the last drawn frame is stale)")
)

// readback returns the surface's last drawn frame at its scale (Scale,
// the fixed one included): the whole logical pixels of Size (px) × Scale,
// rounded as shellkit.ToPx. It fails on a closed surface, one never shown and one
// with a paint pending, so the caller lets a frame draw (iterating the
// main loop) after its last change; and when the frame's size or bytes
// do not match. It must not run during a Paint.
func (sf *surface) readback() (readbackFrame, error) {
	sf.s.q.Check("gtk4.Surface.readback")
	switch {
	case sf.dead:
		return readbackFrame{}, errReadbackClosed
	case !sf.shown:
		return readbackFrame{}, errReadbackHidden
	case sf.invalid:
		return readbackFrame{}, errReadbackPending
	}
	sf.s.notPainting("Surface.readback")
	f, err := sf.w.readback(sf.scale)
	rethrow()
	if err != nil {
		return readbackFrame{}, err
	}
	// GTK allocates whole logical pixels (px), which the frame scales.
	lw, lh := px(sf.size)
	pw, ph := int(shellkit.ToPx(float64(lw), sf.scale)), int(shellkit.ToPx(float64(lh), sf.scale))
	switch {
	case f.w != pw || f.h != ph:
		return readbackFrame{}, fmt.Errorf("gtk4: readback: a %d×%d frame, want %d×%d (%d×%d at %v)",
			f.w, f.h, pw, ph, lw, lh, sf.scale)
	case f.stride < 4*f.w || len(f.pix) < f.stride*f.h:
		return readbackFrame{}, fmt.Errorf("gtk4: readback: %d bytes in rows of %d for %d×%d pixels",
			len(f.pix), f.stride, f.w, f.h)
	}
	return f, nil
}
