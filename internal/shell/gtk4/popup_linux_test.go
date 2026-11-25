//go:build linux

package gtk4

import (
	"fmt"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// The popups over a real GtkPopover (S5.5f). popup_test.go checks the
// logic over a fake popover; these check what only GTK can say: that the
// popover is created, parented and shown around a GGSurface, that Placed
// comes with the child's allocation where GTK put it, that the popup
// paints, that Reposition moves it, and that Close and the parent's close
// tear it all down without a g_critical. Where it lands depends on the
// windowing system (and the screen, for the slide): that is logged as
// POPUP-RESULT, not failed.

// popRec is a recording shell.PopupHandler.
type popRec struct {
	placed                    []driver.Rect
	scale                     float64
	paints, dismissed, closed int
	leaves                    int // PointerLeave events
}

func (h *popRec) Paint(render.Canvas) { h.paints++ }
func (h *popRec) Placed(r driver.Rect, sc float64) {
	h.placed = append(h.placed, r)
	h.scale = sc
}
func (h *popRec) Event(ev shell.Event) bool {
	if pe, ok := ev.(shell.PointerEvent); ok && pe.Kind == shell.PointerLeave {
		h.leaves++
	}
	return false
}
func (h *popRec) Dismissed() { h.dismissed++ }
func (h *popRec) Closed()    { h.closed++ }

// TestPopupGTK: a grab menu and a tooltip over a shown surface.
func TestPopupGTK(t *testing.T) {
	needSurfaceGTK(t)
	base := dispatch.Len()
	runSurfaceTest(t, func(e *surfTest) error {
		sf, g, h, err := e.open("parent", shell.SurfaceConfig{Size: driver.Size{W: 400, H: 300}, Style: driver.StyleResizable})
		if err != nil {
			return err
		}
		if err := show(sf, g, h); err != nil {
			return err
		}
		menu := &popRec{}
		cfg := shell.PopupConfig{Anchor: driver.Rect{X: 40, Y: 60, W: 100, H: 20},
			Size: driver.Size{W: 120, H: 90}, Offset: driver.Point{X: -4, Y: -25}, Grab: true}
		pp, err := e.s.NewPopup(sf, cfg, menu)
		if err != nil {
			return fmt.Errorf("NewPopup: %w", err)
		}
		p := pp.(*popup)
		gp, ok := p.nw.(*gtkPopover)
		if !ok {
			return fmt.Errorf("the popover is a %T", p.nw)
		}
		if !iterateUntil(showWait, func() bool { return len(menu.placed) > 0 && menu.paints > 0 }) {
			return fmt.Errorf("within %v: %d Placed, %d paints, mapped %v", showWait, len(menu.placed), menu.paints, mapped(gp.surf))
		}
		t.Logf("POPUP-RESULT menu: placed %v at scale %v (asked 36,35 120×90)", menu.placed[len(menu.placed)-1], menu.scale)
		if !surfHasClass(gp.pop, popoverClass) {
			t.Errorf("the popover lacks the CSS class %s", popoverClass)
		}
		cfg.Offset.Y = -45
		p.Reposition(cfg)
		n := len(menu.placed)
		iterateUntil(shortWait, func() bool { return len(menu.placed) > n })
		t.Logf("POPUP-RESULT reposition: placed %v (asked 36,15 120×90)", menu.placed[len(menu.placed)-1])

		tip := &popRec{}
		tp, err := e.s.NewPopup(sf, shell.PopupConfig{Anchor: driver.Rect{X: 200, Y: 200}, Size: driver.Size{W: 60, H: 20},
			Offset: driver.Point{Y: 18}}, tip)
		if err != nil {
			return fmt.Errorf("NewPopup(tooltip): %w", err)
		}
		if !iterateUntil(showWait, func() bool { return len(tip.placed) > 0 && tip.paints > 0 }) {
			t.Errorf("tooltip: within %v: %d Placed, %d paints", showWait, len(tip.placed), tip.paints)
		} else {
			t.Logf("POPUP-RESULT tooltip: placed %v (asked 200,218 60×20)", tip.placed[0])
		}
		p.Close()
		if menu.closed != 1 || menu.dismissed != 0 {
			t.Errorf("menu after Close: %d Closed, %d Dismissed", menu.closed, menu.dismissed)
		}
		if err := trackGTK(t, e, sf); err != nil {
			return err
		}
		iterateFor(settle)
		sf.Close() // closes the tooltip first
		if tip.closed != 1 {
			t.Errorf("tooltip after the parent's Close: %d Closed", tip.closed)
		}
		_ = tp
		return nil
	})
	if n := dispatch.Len(); n != base {
		t.Errorf("%d handles after the run, want %d", n, base)
	}
}

func surfHasClass(w uintptr, class string) bool {
	return surfGtk.WidgetHasCSSClass(w, class) != 0
}

// trackGTK (S5.6e): a grab menu with a track opens and keeps it; the
// deferred PointerLeave runs through a real GLib idle source, and one
// still pending at Close is removed with its handle (the handle count of
// TestPopupGTK).
func trackGTK(t *testing.T, e *surfTest, sf *surface) error {
	menu := &popRec{}
	cfg := shell.PopupConfig{Anchor: driver.Rect{X: 40, Y: 0, W: 40, H: 28}, Size: driver.Size{W: 100, H: 80},
		Offset: driver.Point{Y: 28}, Grab: true, Track: driver.Rect{X: 10, Y: 0, W: 200, H: 28}}
	pp, err := e.s.NewPopup(sf, cfg, menu)
	if err != nil {
		return fmt.Errorf("NewPopup(track): %w", err)
	}
	p := pp.(*popup)
	if !iterateUntil(showWait, func() bool { return len(menu.placed) > 0 }) {
		return fmt.Errorf("track menu: no Placed within %v", showWait)
	}
	t.Logf("POPUP-RESULT track menu: placed %v (asked 40,28 100×80), track %v", menu.placed[len(menu.placed)-1], p.track)
	if p.track != cfg.Track {
		t.Errorf("track %+v, want %+v", p.track, cfg.Track)
	}
	if p.dismissed { // the compositor refused the grab (no press serial)
		t.Logf("POPUP-RESULT track menu: dismissed at once, the deferred leave not checked")
		p.Close()
		return nil
	}
	// The pointer on the track (as after a forwarded move), then the
	// parent's leave: deferred to the idle.
	c := p.c
	c.ptr.inside, c.ptr.lastPos = true, driver.Point{X: 10, Y: -10}
	p.parentLeft()
	if p.leaveID == 0 {
		t.Error("the parent's leave from the track was not deferred")
	}
	if !iterateUntil(shortWait, func() bool { return menu.leaves > 0 }) || p.leaveID != 0 {
		t.Errorf("the deferred leave: %d leaves, source %d", menu.leaves, p.leaveID)
	}
	c.ptr.inside = true
	p.parentLeft()
	if p.leaveID == 0 {
		t.Error("the second leave was not deferred")
	}
	p.Close() // removes the pending source
	if p.leaveID != 0 || menu.leaves != 1 {
		t.Errorf("after Close: source %d, %d leaves", p.leaveID, menu.leaves)
	}
	return nil
}
