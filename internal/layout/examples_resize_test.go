package layout

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

// Example layouts at other window sizes (PLAN.md §11.1, S0.4j): each
// example at a bigger size, and at its minimum window size, which S0.5
// computes as max(Root.MinSize, WindowConfig.MinSize). hello, form and
// converter set no WindowConfig.MinSize, so the minimum is Root.MinSize.

// minWindow is the window minimum S0.5 sets: the larger of the tree's
// minimum and the configured one.
func minWindow(r *Root, cfg driver.Size) driver.Size { return r.MinSize().Max(cfg) }

// wantSizeNear checks a size with the frame tolerance.
func wantSizeNear(t *testing.T, name string, got, want driver.Size) {
	t.Helper()
	if !sizeNear(got, want) {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

// widen returns r widened by dw: the delta for children that fill an
// expanding parent when the window gets wider.
func widen(r driver.Rect, dw float64) driver.Rect { r.W += dw; return r }

// shift returns r moved right by dx: the delta for trailing children.
func shift(r driver.Rect, dx float64) driver.Rect { r.X += dx; return r }

func TestExampleHelloResized(t *testing.T) {
	gw, gh := 630.0/13, 240.0/13
	nameY := 20 + gh + 12
	// Minimum: the widest child minimum is the 96 pt text field, and the
	// height is the natural one (nothing can shrink vertically).
	min := minWindow(layoutHello(driver.Size{W: 360, H: 180}).root, driver.Size{})
	wantSizeNear(t, "hello min size", min, driver.Size{W: 136, H: 108 + gh})

	for _, c := range []struct {
		size driver.Size
		dw   float64 // inner width − the base 320
	}{
		{min, 96 - 320},
		{driver.Size{W: 600, H: 400}, 560 - 320},
	} {
		h := layoutHello(c.size)
		wantRect(t, "vstack", h.vstack.Frame, driver.RectOf(driver.Point{}, c.size))
		wantRect(t, "greeting", h.greeting.Frame, driver.Rect{X: 20, Y: 20, W: gw, H: gh})
		wantRect(t, "name", h.name.Frame, widen(driver.Rect{X: 20, Y: nameY, W: 320, H: 22}, c.dw))
		wantRect(t, "hstack", h.hstack.Frame, widen(driver.Rect{X: 20, Y: nameY + 34, W: 320, H: 22}, c.dw))
		wantRect(t, "greet", h.greet.Frame, shift(driver.Rect{X: 257, W: 63, H: 22}, c.dw))
	}
}

func TestExampleFormResized(t *testing.T) {
	// Minimum width: the 63 pt label column + 8 + the widest control
	// minimum ("I agree to the terms of service", 18 + 31 × 7 = 235) is 306,
	// wider than the button row (70 + 8 + 126 = 212), plus the padding.
	// Height: 20 + 238 + 16 + 0 (spacer) + 16 + 22 + 20.
	min := minWindow(layoutForm(driver.Size{W: 460, H: 380}).root, driver.Size{})
	wantSizeNear(t, "form min size", min, driver.Size{W: 346, H: 332})

	for _, c := range []struct {
		size   driver.Size
		dw     float64 // inner width − the base 420
		spacer float64 // the spacer's share of the height (48 at the base)
	}{
		{min, 306 - 420, 0},
		{driver.Size{W: 640, H: 500}, 600 - 420, 460 - (238 + 22 + 2*16)},
	} {
		f := layoutForm(c.size)
		wantRect(t, "vstack", f.vstack.Frame, driver.RectOf(driver.Point{}, c.size))
		wantRect(t, "form", f.form.Frame, widen(driver.Rect{X: 20, Y: 20, W: 420, H: 238}, c.dw))
		// The labels and the non-expanding controls don't move; the text
		// fields take the width change.
		wantRect(t, "Password:", f.labels[2].Frame, driver.Rect{X: 0, Y: 63, W: 63, H: 16})
		wantRect(t, "name", f.name.Frame, widen(driver.Rect{X: 71, Y: 0, W: 349, H: 22}, c.dw))
		wantRect(t, "email", f.email.Frame, widen(driver.Rect{X: 71, Y: 30, W: 349, H: 22}, c.dw))
		wantRect(t, "password", f.password.Frame, widen(driver.Rect{X: 71, Y: 60, W: 349, H: 22}, c.dw))
		wantRect(t, "role", f.role.Frame, driver.Rect{X: 71, Y: 90, W: 123, H: 22})
		wantRect(t, "plan", f.plan.Frame, driver.Rect{X: 71, Y: 120, W: 46, H: 66})
		wantRect(t, "terms", f.terms.Frame, driver.Rect{X: 71, Y: 220, W: 235, H: 18})
		// The spacer's share moves the button row; the buttons stay trailing.
		rowY := 20 + 238 + 16 + c.spacer + 16
		wantRect(t, "button row", f.row.Frame, widen(driver.Rect{X: 20, Y: rowY, W: 420, H: 22}, c.dw))
		wantRect(t, "cancel", f.cancel.Frame, shift(driver.Rect{X: 216, W: 70, H: 22}, c.dw))
		wantRect(t, "create", f.create.Frame, shift(driver.Rect{X: 294, W: 126, H: 22}, c.dw))
	}
	// At the minimum the button row has room to spare: Cancel at 102.
	if f := layoutForm(min); !near(f.cancel.Frame.X, 102) {
		t.Errorf("cancel at min: x = %v, want 102", f.cancel.Frame.X)
	}
}

func TestExampleConverterResized(t *testing.T) {
	// Minimum width: the form (77 + 8 + 96 = 181) is wider than the sync
	// row (96 + 8 + an empty label) and the slider (96), plus the padding.
	// Height: 20 + 52 + 12 + 22 + 12 + 1 + 12 + 22 + 20.
	min := minWindow(layoutConverter(driver.Size{W: 380, H: 220}, "").root, driver.Size{})
	wantSizeNear(t, "converter min size", min, driver.Size{W: 221, H: 173})
	// After a sync the row (96 + 8 + 126 = 230) sets the minimum width.
	synced := "Synced at 15:04:05"
	minSynced := minWindow(layoutConverter(driver.Size{W: 380, H: 220}, synced).root, driver.Size{})
	wantSizeNear(t, "converter min size after sync", minSynced, driver.Size{W: 270, H: 173})

	big := driver.Size{W: 600, H: 300}
	for _, c := range []struct {
		size    driver.Size
		updated string
		dw      float64 // inner width − the base 340
		barW    float64 // the sync bar's width
	}{
		{min, "", 181 - 340, 173},
		{big, "", 560 - 340, 552},
		{minSynced, synced, 230 - 340, 96},
		{big, synced, 560 - 340, 426},
	} {
		v := layoutConverter(c.size, c.updated)
		wantRect(t, "vstack", v.vstack.Frame, driver.RectOf(driver.Point{}, c.size))
		wantRect(t, "form", v.form.Frame, widen(driver.Rect{X: 20, Y: 20, W: 340, H: 52}, c.dw))
		wantRect(t, "celsius label", v.cLabel.Frame, driver.Rect{X: 21, Y: 3, W: 56, H: 16})
		wantRect(t, "celsius", v.celsius.Frame, widen(driver.Rect{X: 85, Y: 0, W: 255, H: 22}, c.dw))
		wantRect(t, "fahrenheit", v.fahrenheit.Frame, widen(driver.Rect{X: 85, Y: 30, W: 255, H: 22}, c.dw))
		wantRect(t, "slider", v.slider.Frame, widen(driver.Rect{X: 20, Y: 84, W: 340, H: 22}, c.dw))
		wantRect(t, "separator", v.sep.Frame, widen(driver.Rect{X: 20, Y: 118, W: 340, H: 1}, c.dw))
		wantRect(t, "row", v.row.Frame, widen(driver.Rect{X: 20, Y: 131, W: 340, H: 22}, c.dw))
		// The bar takes the row minus the gap and the label; the label is
		// trailing.
		lw := v.updated.Frame.W
		wantRect(t, "sync bar", v.syncBar.Frame, driver.Rect{W: c.barW, H: 22})
		wantRect(t, "updated", v.updated.Frame, driver.Rect{X: c.barW + 8, Y: 3, W: lw, H: 16})
		if want := 340 + c.dw; !near(c.barW+8+lw, want) {
			t.Errorf("%v %q: bar + gap + label = %v, want %v", c.size, c.updated, c.barW+8+lw, want)
		}
	}
}
