package aqua

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// TestNestedScrollViews: an inner scroll view at its edge declines a
// gesture pushing past it when the outer one can move, which then takes
// and keeps the whole gesture; at the outer's edge too, the inner one
// rubber-bands. Wheel steps the inner one cannot take bubble out.
func TestNestedScrollViews(t *testing.T) {
	d := startDriver(t)
	outer := d.NewScrollView(nil).(*ScrollView)
	inner := d.NewScrollView(nil).(*ScrollView)
	doc := d.NewContainer()
	innerDoc := d.NewContainer()
	doc.SetChildren([]driver.Widget{inner})
	outer.SetDocument(doc)
	inner.SetDocument(innerDoc)
	outer.SetHasVerticalScroller(true)
	inner.SetHasVerticalScroller(true)
	outer.SetFrame(driver.Rect{W: 100, H: 100})
	outer.SetDocumentSize(driver.Size{W: 100, H: 300})
	doc.SetFrame(driver.Rect{W: 100, H: 300})
	inner.SetFrame(driver.Rect{Y: 10, W: 100, H: 50})
	inner.SetDocumentSize(driver.Size{W: 100, H: 100})
	r := NewRouter(ElementOf(outer))
	at := driver.Point{X: 50, Y: 30} // over the inner view

	send := func(dy float64, ph shell.ScrollPhase, precise bool) bool {
		return r.Event(shell.ScrollEvent{Pos: at, Delta: driver.Point{Y: dy}, Precise: precise, Phase: ph})
	}
	// Inner at its top, pushing up (content down): declined, the outer
	// is at its top too, so the inner keeps it (it would rubber-band; a driver that is not
	// running jumps, as with Reduce Motion).
	send(10, shell.PhaseBegan, true)
	if r.Latched() != ElementOf(inner) {
		t.Fatalf("both at the top: latched %T", r.Latched())
	}
	inner.SetOffset(driver.Point{})
	// Inner at its end, pushing down: the outer takes the gesture.
	inner.SetOffset(driver.Point{Y: 50})
	send(-10, shell.PhaseBegan, true)
	send(-10, shell.PhaseChanged, true)
	if r.Latched() != ElementOf(outer) || outer.Offset().Y != 20 || inner.Offset().Y != 50 {
		t.Errorf("outer takes: latched %v, offsets %v / %v", r.Latched(), outer.Offset(), inner.Offset())
	}
	// Back up within the same gesture: still the outer's.
	send(5, shell.PhaseChanged, true)
	if outer.Offset().Y != 15 || inner.Offset().Y != 50 {
		t.Errorf("latched back: offsets %v / %v", outer.Offset(), inner.Offset())
	}
	send(0, shell.PhaseEnded, true)
	outer.SetOffset(driver.Point{})
	// A wheel step the inner cannot take bubbles to the outer.
	if !send(-1, shell.PhaseNone, false) || outer.y.Target() <= 0 {
		t.Errorf("wheel bubble: outer target %v", outer.y.Target())
	}
}
