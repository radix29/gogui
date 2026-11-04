package layout

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

// notes and controls at other window sizes (PLAN.md §11.1, S0.4k). Both
// set a WindowConfig.MinSize: notes 480×320, which is far larger than its
// tree minimum, and controls 420×480, whose height is below the natural
// height of its tree.

func TestExampleNotesResized(t *testing.T) {
	// Tree minimum: the sidebar's 180 pt min width + the 1 pt vertical
	// separator + the editor (0); the status bar (161 + 2 × 6 = 173) is
	// narrower. Height: the split view's minimum is the vertical
	// separator's 1 pt, then the separator (1) and the status bar (28).
	root := layoutNotes(driver.Size{W: 720, H: 480}).root
	wantSizeNear(t, "notes tree min size", root.MinSize(), driver.Size{W: 181, H: 30})
	min := minWindow(root, driver.Size{W: 480, H: 320})
	wantSizeNear(t, "notes min size", min, driver.Size{W: 480, H: 320})

	for _, size := range []driver.Size{min, {W: 1024, H: 768}} {
		n := layoutNotes(size)
		// The split view takes the height change and the editor the width
		// change; the separator and the status bar keep their heights.
		splitH := size.H - 1 - 28
		wantRect(t, "vstack", n.vstack.Frame, driver.RectOf(driver.Point{}, size))
		wantRect(t, "split", n.split.Frame, driver.Rect{W: size.W, H: splitH})
		wantRect(t, "separator", n.hsep.Frame, driver.Rect{Y: splitH, W: size.W, H: 1})
		wantRect(t, "status bar", n.status.Frame, driver.Rect{Y: splitH + 1, W: size.W, H: 28})
		wantRect(t, "sidebar", n.sidebar.Frame, driver.Rect{W: 180, H: splitH})
		wantRect(t, "vertical separator", n.vsep.Frame, driver.Rect{X: 180, W: 1, H: splitH})
		wantRect(t, "editor", n.editor.Frame, driver.Rect{X: 181, W: size.W - 181, H: splitH})
		// The document doesn't depend on the window: still 180 × 796, so
		// the sidebar scrolls at both sizes.
		wantSize(t, "document size", n.sidebar.DocumentSize, driver.Size{W: 180, H: 796})
		wantRect(t, "option 30", n.options[29].Frame, driver.Rect{X: 12, Y: 12 + 26*29, W: 18 + 9*7, H: 18})
		wantRect(t, "stats", n.stats.Frame, driver.Rect{X: 6, Y: 6, W: 161, H: 16})
	}
}

func TestExampleControlsResized(t *testing.T) {
	// Tree minimum width: the button row (105 + 77 + 84 + 3 × 8 = 290,
	// the spacer at 0) is the widest box content; + 16 for the box and
	// 2 × 20 padding. Height: the natural 560 − 73 = 487, because nothing
	// can shrink vertically. The configured 420×480 is wider but shorter.
	root := layoutControls(driver.Size{W: 520, H: 560}).root
	wantSizeNear(t, "controls tree min size", root.MinSize(), driver.Size{W: 346, H: 487})
	min := minWindow(root, driver.Size{W: 420, H: 480})
	wantSizeNear(t, "controls min size", min, driver.Size{W: 420, H: 487})

	for _, c := range []struct {
		size driver.Size
		dw   float64 // inner width − the base 480
	}{
		{min, 380 - 480},
		{driver.Size{W: 800, H: 700}, 760 - 480},
	} {
		k := layoutControls(c.size)
		wantRect(t, "vstack", k.vstack.Frame, driver.RectOf(driver.Point{}, c.size))
		// The boxes take the width change and keep their heights and
		// positions; any extra height stays empty at the bottom.
		boxes := []driver.Rect{
			{X: 20, Y: 20, W: 480, H: 54}, {X: 20, Y: 88, W: 480, H: 98},
			{X: 20, Y: 200, W: 480, H: 84}, {X: 20, Y: 298, W: 480, H: 118},
		}
		for i, want := range boxes {
			wantRect(t, k.boxes[i].Title, k.boxes[i].Frame, widen(want, c.dw))
		}
		// Leading controls keep their frames; the rows and the expanding
		// controls take the width change.
		wantRect(t, "button row", k.buttonRow.Frame, widen(driver.Rect{W: 464, H: 22}, c.dw))
		wantRect(t, "disabled", k.disabled.Frame, driver.Rect{X: 198, W: 84, H: 22})
		wantRect(t, "toggle row", k.toggleRow.Frame, widen(driver.Rect{W: 464, H: 66}, c.dw))
		wantRect(t, "radio group", k.radioGroup.Frame, driver.Rect{X: 94, W: 60, H: 66})
		wantRect(t, "field", k.field.Frame, widen(driver.Rect{W: 464, H: 22}, c.dw))
		wantRect(t, "secure", k.secure.Frame, widen(driver.Rect{Y: 30, W: 464, H: 22}, c.dw))
		wantRect(t, "slider", k.slider.Frame, widen(driver.Rect{W: 464, H: 22}, c.dw))
		wantRect(t, "progress", k.progress.Frame, widen(driver.Rect{Y: 32, W: 464, H: 22}, c.dw))
		// The busy bar and the spacer share the change equally: at the
		// minimum the leftover is 185 (92.5 each), at 800 it is 565.
		wantRect(t, "busy", k.busy.Frame, driver.Rect{W: 96 + 142.5 + c.dw/2, H: 22})
		wantRect(t, "popup", k.popup.Frame, shift(driver.Rect{X: 397, W: 67, H: 22}, c.dw))
		wantRect(t, "separator", k.sep.Frame, widen(driver.Rect{X: 20, Y: 430, W: 480, H: 1}, c.dw))
		wantRect(t, "status row", k.statusRow.Frame, widen(driver.Rect{X: 20, Y: 445, W: 480, H: 22}, c.dw))
		wantRect(t, "status", k.status.Frame, driver.Rect{Y: 3, W: 35, H: 16})
		wantRect(t, "alert", k.alert.Frame, shift(driver.Rect{X: 375, W: 105, H: 22}, c.dw))
	}
	// At the minimum the status row ends exactly at the bottom padding.
	if k := layoutControls(min); !near(k.statusRow.Frame.MaxY()+20, min.H) {
		t.Errorf("status row bottom at min = %v, want %v", k.statusRow.Frame.MaxY()+20, min.H)
	}
}
