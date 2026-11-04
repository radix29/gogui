package stub

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
)

// Window implements driver.Window.
type Window struct {
	Config      driver.WindowConfig
	Title       string
	Content     driver.Widget
	Size        driver.Size
	MinSize     driver.Size
	MaxSize     driver.Size
	CenterCalls int
	Visible     bool
	IsKey       bool
	Closed      bool
	Handler     driver.WindowHandler

	drv *Driver
}

// NewWindow returns a hidden stub window that is not tracked by a Driver.
func NewWindow(cfg driver.WindowConfig, h driver.WindowHandler) *Window {
	w := &Window{
		Config:  cfg,
		Title:   cfg.Title,
		MinSize: cfg.MinSize,
		MaxSize: cfg.MaxSize,
		Handler: h,
	}
	w.Size = w.clampSize(cfg.Size)
	return w
}

func (w *Window) SetTitle(title string) { w.Title = title }

func (w *Window) SetContent(c driver.Widget) {
	if w.Content != nil && w.Content != c {
		release(w, w.Content)
	}
	w.Content = c
	if c != nil {
		adopt(w, c)
	}
}

func (w *Window) ContentSize() driver.Size { return w.Size }

// SetContentSize resizes the content (clamped) and calls ContentResized.
func (w *Window) SetContentSize(size driver.Size) { w.resize(size) }

func (w *Window) SetMinSize(size driver.Size) { w.MinSize = size }
func (w *Window) SetMaxSize(size driver.Size) { w.MaxSize = size }
func (w *Window) Center()                     { w.CenterCalls++ }

// Show makes the window visible and key.
func (w *Window) Show() {
	if w.Closed {
		return
	}
	w.Visible = true
	if w.drv != nil {
		for _, o := range w.drv.Windows {
			if o != w && o.IsKey {
				o.setKey(false)
			}
		}
	}
	w.setKey(true)
}

// Close closes the window and calls Closed, once.
func (w *Window) Close() {
	if w.Closed {
		return
	}
	w.Closed = true
	w.Visible = false
	w.IsKey = false
	if w.drv != nil {
		w.drv.forget(w)
	}
	if w.Handler != nil {
		w.Handler.Closed()
	}
}

// UserClose simulates the close button: it asks ShouldClose and closes the
// window if allowed. It reports whether the window was closed.
func (w *Window) UserClose() bool {
	if w.Closed {
		return false
	}
	if w.Handler != nil && !w.Handler.ShouldClose() {
		return false
	}
	w.Close()
	return true
}

// UserResize simulates the user resizing the window. The size is clamped
// to the min and max sizes.
func (w *Window) UserResize(size driver.Size) { w.resize(size) }

// UserSetKey simulates the window becoming or losing the key window.
func (w *Window) UserSetKey(isKey bool) { w.setKey(isKey) }

func (w *Window) setKey(isKey bool) {
	if w.IsKey == isKey {
		return
	}
	w.IsKey = isKey
	if w.Handler != nil {
		w.Handler.KeyChanged(isKey)
	}
}

func (w *Window) resize(size driver.Size) {
	if w.Closed {
		return
	}
	w.Size = w.clampSize(size)
	if w.Handler != nil {
		w.Handler.ContentResized(w.Size)
	}
}

func (w *Window) clampSize(s driver.Size) driver.Size {
	limit := func(v, lo, hi float64) float64 {
		if hi > 0 {
			v = math.Min(v, hi)
		}
		return math.Max(v, lo)
	}
	return driver.Size{
		W: limit(s.W, w.MinSize.W, w.MaxSize.W),
		H: limit(s.H, w.MinSize.H, w.MaxSize.H),
	}
}

func (w *Window) removeChild(child *Base) {
	if w.Content != nil && baseOf(w.Content) == child {
		w.Content = nil
	}
}
