package fake

import (
	"math"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render/record"
	"github.com/radix29/gogui/internal/shell"
)

// Surface is a fake shell.Surface. It records its state for tests and
// paints into a record.Canvas. Every method, the accessors included, must
// be called on the UI goroutine. After Close the state stays readable and
// the shell.Surface methods do nothing.
type Surface struct {
	shell *Shell
	h     shell.SurfaceHandler

	title    string
	size     driver.Size
	min, max driver.Size
	style    driver.WindowStyle
	scale    float64
	cursor   shell.Cursor
	regions  []shell.ChromeRegion

	visible, minimized, zoomed, dead bool
	unzoomed                         driver.Size // size before Zoom
	centers                          int         // Center calls, plus 1 for SurfaceConfig.Center

	invalid  bool
	paints   int
	last     *record.Canvas
	paintErr error

	ptr pointer // input state, see input.go
}

var _ shell.Surface = (*Surface)(nil)

func (sf *Surface) check(what string) { sf.shell.check("Surface." + what) }

// live checks the thread and reports whether the surface is still open.
func (sf *Surface) live(what string) bool {
	sf.check(what)
	return !sf.dead
}

// change checks like live and also rejects calls during a Paint.
func (sf *Surface) change(what string) bool {
	if !sf.live(what) {
		return false
	}
	sf.shell.notPainting("Surface." + what)
	return true
}

// clamp limits size to the min and max sizes (zero components: no limit;
// min wins over max) and to non-negative values.
func (sf *Surface) clamp(size driver.Size) driver.Size {
	lim := func(v, lo, hi float64) float64 {
		if math.IsNaN(v) || v < 0 {
			v = 0
		}
		if hi > 0 {
			v = math.Min(v, hi)
		}
		return math.Max(v, lo)
	}
	return driver.Size{W: lim(size.W, sf.min.W, sf.max.W), H: lim(size.H, sf.min.H, sf.max.H)}
}

// resize applies a new size and scale, calls Resized when either changed
// and invalidates. It reports whether anything changed.
func (sf *Surface) resize(size driver.Size, scale float64) bool {
	size = sf.clamp(size)
	if size == sf.size && scale == sf.scale {
		return false
	}
	sf.size, sf.scale = size, scale
	sf.h.Resized(size, scale)
	sf.Invalidate()
	return true
}

// SetTitle implements shell.Surface.
func (sf *Surface) SetTitle(title string) {
	if sf.live("SetTitle") {
		sf.title = title
	}
}

// Title returns the native title.
func (sf *Surface) Title() string {
	sf.check("Title")
	return sf.title
}

// Size implements shell.Surface.
func (sf *Surface) Size() driver.Size {
	sf.check("Size")
	return sf.size
}

// SetSize implements shell.Surface.
func (sf *Surface) SetSize(size driver.Size) {
	if sf.change("SetSize") {
		sf.resize(size, sf.scale)
	}
}

// SetMinSize implements shell.Surface.
func (sf *Surface) SetMinSize(size driver.Size) {
	if sf.change("SetMinSize") {
		sf.min = size
		sf.resize(sf.size, sf.scale)
	}
}

// SetMaxSize implements shell.Surface.
func (sf *Surface) SetMaxSize(size driver.Size) {
	if sf.change("SetMaxSize") {
		sf.max = size
		sf.resize(sf.size, sf.scale)
	}
}

// MinSize and MaxSize return the current limits.
func (sf *Surface) MinSize() driver.Size { sf.check("MinSize"); return sf.min }
func (sf *Surface) MaxSize() driver.Size { sf.check("MaxSize"); return sf.max }

// Style returns the style from SurfaceConfig.
func (sf *Surface) Style() driver.WindowStyle { sf.check("Style"); return sf.style }

// Scale implements shell.Surface.
func (sf *Surface) Scale() float64 {
	sf.check("Scale")
	return sf.scale
}

// SetScale simulates a move to a screen with another scale (or a scale
// setting change): Resized is called and the surface invalidated when the
// scale changes. It panics for a scale that is not positive.
func (sf *Surface) SetScale(scale float64) {
	if !(scale > 0) || math.IsInf(scale, 1) {
		panic("fake: SetScale with a scale that is not positive and finite")
	}
	if sf.change("SetScale") {
		sf.resize(sf.size, scale)
	}
}

// UserResize simulates the user dragging a resize border to size: the size
// is clamped, Resized is called, and a visible surface is painted before
// UserResize returns (the live-resize rule of SurfaceHandler.Resized). It
// does nothing without driver.StyleResizable, and reports whether the size
// changed.
func (sf *Surface) UserResize(size driver.Size) bool {
	if !sf.change("UserResize") || sf.style&driver.StyleResizable == 0 {
		return false
	}
	if !sf.resize(size, sf.scale) {
		return false
	}
	sf.zoomed = false
	if sf.visible && !sf.minimized {
		sf.paint()
	}
	return true
}

// Center implements shell.Surface.
func (sf *Surface) Center() {
	if sf.change("Center") {
		sf.centers++
	}
}

// Centered returns how often the surface was centered (SurfaceConfig.Center
// counts as one).
func (sf *Surface) Centered() int { sf.check("Centered"); return sf.centers }

// Show implements shell.Surface. It also restores a minimized surface. The
// surface becomes key, and is painted at the next frame if it is
// invalidated.
func (sf *Surface) Show() {
	if !sf.change("Show") {
		return
	}
	sf.visible, sf.minimized = true, false
	sf.shell.setKey(sf)
	if sf.invalid {
		sf.shell.clock.RequestFrame()
	}
}

// Visible reports whether the surface has been shown (it stays true while
// minimized).
func (sf *Surface) Visible() bool { sf.check("Visible"); return sf.visible }

// Minimize implements shell.Surface. A minimized surface stops being key
// and is not painted.
func (sf *Surface) Minimize() {
	if !sf.change("Minimize") || !sf.visible || sf.minimized {
		return
	}
	sf.minimized = true
	if sf.shell.key == sf {
		sf.shell.setKey(nil)
	}
}

// Minimized reports whether the surface is minimized.
func (sf *Surface) Minimized() bool { sf.check("Minimized"); return sf.minimized }

// Activate simulates the user making the surface key (clicking it, the
// taskbar, Alt-Tab); a minimized surface is restored. It does nothing for a
// hidden surface.
func (sf *Surface) Activate() {
	if sf.change("Activate") && sf.visible {
		sf.Show()
	}
}

// IsKey reports whether the surface is the key surface.
func (sf *Surface) IsKey() bool { sf.check("IsKey"); return sf.shell.key == sf }

// Zoom implements shell.Surface: the first call fills the shell's screen
// size (clamped to the limits), the next one restores the size before it.
// It does nothing without driver.StyleResizable.
func (sf *Surface) Zoom() {
	if !sf.change("Zoom") || sf.style&driver.StyleResizable == 0 {
		return
	}
	if sf.zoomed {
		sf.zoomed = false
		sf.resize(sf.unzoomed, sf.scale)
		return
	}
	sf.unzoomed = sf.size
	sf.zoomed = true
	sf.resize(sf.shell.screen, sf.scale)
}

// Zoomed implements shell.Surface.
func (sf *Surface) Zoomed() bool {
	sf.check("Zoomed")
	return sf.zoomed
}

// Close implements shell.Surface.
func (sf *Surface) Close() {
	if !sf.change("Close") {
		return
	}
	sf.dead = true
	s := sf.shell
	for i, o := range s.surfaces {
		if o == sf {
			s.surfaces = append(s.surfaces[:i], s.surfaces[i+1:]...)
			break
		}
	}
	if s.key == sf {
		s.key = nil
	}
	sf.h.Closed()
}

// UserClose simulates the user or the OS asking to close the surface
// (Alt+F4, the taskbar): it asks CloseRequested and closes the surface if
// allowed. It reports whether the surface closed.
func (sf *Surface) UserClose() bool {
	if !sf.change("UserClose") || !sf.h.CloseRequested() {
		return false
	}
	sf.Close()
	return true
}

// Closed reports whether the surface has closed.
func (sf *Surface) Closed() bool { sf.check("Closed"); return sf.dead }

// Invalidate implements shell.Surface.
func (sf *Surface) Invalidate() {
	if !sf.live("Invalidate") {
		return
	}
	sf.invalid = true
	sf.shell.clock.RequestFrame()
}

// Invalid reports whether the surface waits for a paint.
func (sf *Surface) Invalid() bool { sf.check("Invalid"); return sf.invalid }

// SetCursor implements shell.Surface.
func (sf *Surface) SetCursor(c shell.Cursor) {
	if sf.live("SetCursor") {
		sf.cursor = c
	}
}

// Cursor returns the cursor set last (CursorArrow initially).
func (sf *Surface) Cursor() shell.Cursor { sf.check("Cursor"); return sf.cursor }

// SetChromeRegions implements shell.Surface.
func (sf *Surface) SetChromeRegions(regions []shell.ChromeRegion) {
	if sf.live("SetChromeRegions") {
		sf.regions = append([]shell.ChromeRegion(nil), regions...)
	}
}

// ChromeRegions returns a copy of the regions set last (nil for none).
func (sf *Surface) ChromeRegions() []shell.ChromeRegion {
	sf.check("ChromeRegions")
	return append([]shell.ChromeRegion(nil), sf.regions...)
}

// HitTest answers the OS hit-test for p as the real shells do: the regions
// are tested from last to first, the first one containing p wins, and a
// point in none is RegionClient. (The fake has no resize borders.)
func (sf *Surface) HitTest(p driver.Point) shell.RegionKind {
	sf.check("HitTest")
	for i := len(sf.regions) - 1; i >= 0; i-- {
		if sf.regions[i].Rect.Contains(p) {
			return sf.regions[i].Kind
		}
	}
	return shell.RegionClient
}

// paint runs SurfaceHandler.Paint on a new record.Canvas of the current
// size and keeps it as the last paint.
func (sf *Surface) paint() {
	sf.invalid = false
	c := record.New(sf.size)
	s := sf.shell
	s.painting = sf
	func() {
		defer func() { s.painting = nil }()
		sf.h.Paint(c)
	}()
	sf.paints++
	sf.last = c
	sf.paintErr = c.End()
}

// Paints returns the number of paints so far.
func (sf *Surface) Paints() int { sf.check("Paints"); return sf.paints }

// LastPaint returns the canvas of the last paint (ended, so only its
// accessors are useful), or nil before the first paint.
func (sf *Surface) LastPaint() *record.Canvas { sf.check("LastPaint"); return sf.last }

// PaintErr returns the contract violations of the last paint (the result
// of record.Canvas.End), nil if there were none.
func (sf *Surface) PaintErr() error { sf.check("PaintErr"); return sf.paintErr }

// Dump returns the dump of the last paint ("" before the first one).
func (sf *Surface) Dump() string {
	sf.check("Dump")
	if sf.last == nil {
		return ""
	}
	return sf.last.Dump()
}
