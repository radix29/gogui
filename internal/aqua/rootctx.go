package aqua

import "github.com/radix29/gogui/internal/driver"

// rootContext is the window context a tree's root gives its elements
// (review R1): implemented by windowRoot, popupRoot and alertRoot, the
// only roots that belong to a window or a surface. Base.Window,
// appearanceIn, scaleIn and the focus ring read it, so the rules for a
// tree without a window (a windowless alert, a closed window, a popup
// not yet placed) live in one place, next to each root.
type rootContext interface {
	Element
	// rootWindow returns the window the tree belongs to, or nil (an alert
	// in a modal surface).
	rootWindow() *Window
	// rootAppearance returns the appearance the tree paints with.
	rootAppearance() driver.Appearance
	// rootScale returns the device pixels per point of the surface the tree
	// is shown on, or 0 when there is none (closed, not yet placed).
	rootScale() float64
}

var (
	_ rootContext = (*windowRoot)(nil)
	_ rootContext = (*popupRoot)(nil)
	_ rootContext = (*alertRoot)(nil)
)

// rootContext returns the context of b's root, or nil when the root is
// none of the window roots (detached, or a root made with NewRouter
// directly).
func (b *Base) rootContext() rootContext {
	rc, _ := b.tree().root.self.(rootContext)
	return rc
}

func (r *windowRoot) rootWindow() *Window               { return r.w }
func (r *windowRoot) rootAppearance() driver.Appearance { return r.w.appearance }
func (r *windowRoot) rootScale() float64 {
	if r.w.closed {
		return 0
	}
	return r.w.surface.Scale()
}

// A popup paints in its window's appearance, at its own placed scale.
func (r *popupRoot) rootWindow() *Window               { return r.w }
func (r *popupRoot) rootAppearance() driver.Appearance { return r.w.appearance }
func (r *popupRoot) rootScale() float64 {
	if r.closed {
		return 0
	}
	return r.scale
}

// A sheet paints in its window's appearance and scale; an alert in a
// modal surface in the driver's appearance and the surface's scale.
func (r *alertRoot) rootWindow() *Window { return r.w }
func (r *alertRoot) rootAppearance() driver.Appearance {
	if r.w != nil {
		return r.w.appearance
	}
	return r.alert.d.current
}
func (r *alertRoot) rootScale() float64 {
	if r.w != nil && !r.w.closed {
		return r.w.surface.Scale()
	}
	return r.scale
}

// appearanceIn returns the appearance of the tree b is in (see
// rootContext), or d's outside a window root.
func appearanceIn(b *Base, d *Driver) driver.Appearance {
	if rc := b.rootContext(); rc != nil {
		return rc.rootAppearance()
	}
	return d.current
}

// scaleIn returns the device pixels per point of the surface b is shown
// on (see rootContext), or 1 when there is none.
func scaleIn(b *Base) float64 {
	if rc := b.rootContext(); rc != nil {
		if s := rc.rootScale(); s > 0 {
			return s
		}
	}
	return 1
}
