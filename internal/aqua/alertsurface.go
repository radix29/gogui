package aqua

import (
	"errors"
	"math"
	"slices"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// Alerts in modal surfaces (PLAN.md S5.7g): an app-modal alert (no parent,
// or a closed one) and the sheet of a window too small for its panel
// (sheet.fits, decided in S5.7c) are shown in a modal surface of their own
// (shell.SurfaceConfig.Modal): an alertRoot as the whole content of a
// surface sized to the panel, not resizable, titled with the app name
// (AppConfig.Name), owned by the sheet's window (centred on it) or, app-
// modal, by none (centred on the key window's screen). The panel is
// square and borderless there (alert.setInSurface): the OS gives the
// surface its corners, border and shadow.
//
// The tree has no window: its key state is the surface's (KeyChanged sets
// the router's active state; inactive until then), its appearance the
// driver's (Driver.update repaints it and sends WindowStateEvent), its
// scale the surface's. Pointer events come in surface points, which are
// the root's. A key-down first goes to the menu bar's accelerators, where
// only Quit is enabled while any alert surface is open (Driver.menuEnabled,
// decided in S5.7g, as the app menu's Quit stays under runModal), then to
// the alert.
//
// Modality: while the surface is open the shell blocks every other
// surface (its input, activation and close; S5.7d), so no window's
// content nor menu strip takes input, and the driver's validation also
// disables every menu item but Quit (Driver.menuEnabled), the
// accelerators of the windows included.
//
// Closing: the answer closes the surface (alertSurface.close) before done
// runs. An OS close request (Alt+F4) presses the Cancel button when the
// alert has one, as Escape does, and is refused otherwise. A surface
// closed by anything else (Quit, its owner closing) answers its alert with
// the Cancel index (0 without one), as a sheet's closing window does.
//
// Queue: the app-modal alerts are shown one at a time in the order they
// came (Driver.appAlerts); the next one opens once the one before has been
// answered and its done has run. Alerts run before the shell is ready
// wait for it and open after AppHandler.Ready. A sheet in a surface keeps
// its place in its window's queue (sheet.go).
type alertSurface struct {
	d       *Driver
	root    *alertRoot
	surface shell.Surface
	closed  bool
}

// errAlertSurface is openAlertSurface's error outside the running shell.
var errAlertSurface = errors.New("aqua: no alert surface before the shell is ready or after Quit")

// openAlertSurface shows the alert of root, which must not be framed in a
// window yet, in a modal surface owned by owner (nil: app-modal), and
// makes it key. It fails with errAlertSurface before the shell is ready
// and after Quit, or with the shell's error; the root is then unchanged.
func (d *Driver) openAlertSurface(root *alertRoot, owner *Window) (*alertSurface, error) {
	if !d.ready || d.quit {
		return nil, errAlertSurface
	}
	a := root.alert
	a.setInSurface(true)
	size := root.FittingSize(driver.Size{W: math.Inf(1), H: math.Inf(1)})
	cfg := shell.SurfaceConfig{
		Title:   d.cfg.Name,
		Size:    size,
		MinSize: size,
		MaxSize: size,
		Style:   driver.StyleTitled,
		Modal:   true,
	}
	if owner != nil {
		cfg.Owner = owner.surface
	}
	m := &alertSurface{d: d, root: root}
	sf, err := d.sh.NewSurface(cfg, alertSurfaceHandler{m})
	if err != nil {
		a.setInSurface(false)
		return nil, err
	}
	m.surface = sf
	root.w, root.surf = nil, m
	root.scale = sf.Scale()
	root.SetFrame(driver.RectOf(driver.Point{}, sf.Size()))
	root.SetInvalidateFunc(m.invalidate)
	root.router.SetCursorFunc(sf.SetCursor)
	root.router.SetActive(false) // not key until the shell says so
	d.alertSurfaces = append(d.alertSurfaces, m)
	sf.Show()
	return m, nil
}

// invalidate is the root's invalidate callback.
func (m *alertSurface) invalidate() {
	if !m.closed {
		m.surface.Invalidate()
	}
}

// close closes the surface; Closed follows. Later calls do nothing.
func (m *alertSurface) close() {
	if m.closed {
		return
	}
	m.surface.Close() // calls alertSurfaceHandler.Closed
	m.teardown()
}

// teardown is the close on the aqua side, once: the surface leaves the
// driver, and an unanswered alert is answered with its Cancel index (0
// without one).
func (m *alertSurface) teardown() {
	if m.closed {
		return
	}
	m.closed = true
	m.d.alertSurfaces = slices.DeleteFunc(m.d.alertSurfaces, func(x *alertSurface) bool { return x == m })
	m.root.SetInvalidateFunc(nil)
	m.root.router.SetCursorFunc(nil)
	m.root.router.CancelPointer()
	m.root.router.SetActive(false)
	if a := m.root.alert; !a.answered {
		a.answer(max(0, a.cancelIndex()))
	}
}

// stateChanged repaints the tree and sends its elements a WindowStateEvent
// after an appearance change of the driver.
func (m *alertSurface) stateChanged() {
	if m.closed {
		return
	}
	m.root.stateChanged()
}

// alertModal reports whether an alert is open in a modal surface.
func (d *Driver) alertModal() bool { return len(d.alertSurfaces) > 0 }

// alertSurfaceHandler implements shell.SurfaceHandler for an alertSurface.
type alertSurfaceHandler struct{ m *alertSurface }

var _ shell.SurfaceHandler = alertSurfaceHandler{}

func (h alertSurfaceHandler) Paint(c render.Canvas) {
	if !h.m.closed {
		PaintTree(c, h.m.root)
	}
}

// Resized follows a scale change (the size is fixed): the tree is framed
// again and repainted.
func (h alertSurfaceHandler) Resized(size driver.Size, scale float64) {
	m := h.m
	if m.closed {
		return
	}
	m.root.scale = scale
	m.root.SetFrame(driver.RectOf(driver.Point{}, size))
	invalidateAll(m.root)
	m.root.router.RefreshHover()
}

// CloseRequested presses the Cancel button, if any, which closes the
// surface; the request itself is refused.
func (h alertSurfaceHandler) CloseRequested() bool {
	a := h.m.root.alert
	if c := a.cancelIndex(); c >= 0 && !h.m.closed && !a.answered {
		a.answer(c)
	}
	return false
}

func (h alertSurfaceHandler) Closed() { h.m.teardown() }

func (h alertSurfaceHandler) KeyChanged(isKey bool) {
	if !h.m.closed {
		h.m.root.SetActive(isKey)
	}
}

// Event sends a key-down to the accelerators first (only Quit is enabled),
// then every event to the alert's router.
func (h alertSurfaceHandler) Event(ev shell.Event) bool {
	m := h.m
	if m.closed {
		return false
	}
	if k, ok := ev.(shell.KeyEvent); ok && k.Down && m.d.accelerator(nil, k) {
		return true
	}
	if m.closed {
		return false
	}
	return m.root.Event(ev)
}

// showAppAlert opens the first app-modal alert of the queue when none is
// open, once the shell is ready and until Quit.
func (d *Driver) showAppAlert() {
	if !d.ready || d.quit || len(d.appAlerts) == 0 {
		return
	}
	rec := d.appAlerts[0]
	if rec.started {
		return
	}
	rec.started = true
	rec.el = rec.element()
	m, err := d.openAlertSurface(newAlertRoot(rec.el), nil)
	if err != nil {
		return // only recorded: Dismiss still answers it
	}
	rec.modal = m
}

// dismissAppAlerts answers the app-modal alerts still queued with their
// Cancel index (0 without one), in queue order, after Quit.
func (d *Driver) dismissAppAlerts() {
	for _, rec := range slices.Clone(d.appAlerts) {
		c := -1
		if rec.el != nil {
			c = rec.el.cancelIndex()
		} else if rec.Spec.Cancel < max(1, len(rec.Spec.Buttons)) {
			c = rec.Spec.Cancel
		}
		rec.Dismiss(max(0, c))
	}
}
