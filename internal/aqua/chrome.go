package aqua

import (
	"unicode/utf8"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/menubar"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// ellipsis ends a truncated title.
const ellipsis = "…"

// The title-bar metrics (PLAN.md §5.4) come from the theme's Chrome tokens
// (S3.1c): circles of LightSize, LightStep apart, the close light
// LightInset from the left edge, vertically centred in the bar.

// lightTop returns the y of the traffic lights in the title bar.
func lightTop() float64 {
	ch := &tokens.Chrome
	return (ch.TitleBarHeight - ch.LightSize) / 2
}

// lightsWidth returns the width of the traffic-light group.
func lightsWidth() float64 { return 2*tokens.Chrome.LightStep + tokens.Chrome.LightSize }

// titleMargin returns the least distance of the title from either edge of
// the bar: the lights plus TitleGap.
func titleMargin() float64 {
	return tokens.Chrome.LightInset + lightsWidth() + tokens.Chrome.TitleGap
}

// titleFont returns the font of the drawn window title: the regular system
// size in the WindowTitleWeight (13 pt semibold).
func titleFont() driver.Font {
	f := systemFont(driver.ControlSizeRegular)
	f.Weight = tokens.Typography.WindowTitleWeight
	return f
}

// lightKind is the kind of a traffic light.
type lightKind uint8

const (
	lightClose lightKind = iota
	lightMinimize
	lightZoom
)

var lightLabels = [...]string{"Close", "Minimize", "Zoom"}

// titleBar is the drawn title bar of a titled window: the background, the
// bottom separator, the traffic-light group, the menu strip when the
// driver has a menu bar (menustrip.go, S5.6c) and the centered title.
type titleBar struct {
	Base
	w      *Window
	group  *lightGroup
	lights [3]*trafficLight
	strip  *menuStrip // nil without a menu bar

	title      render.TextLayout // the (possibly truncated) title, nil when hidden
	titleFrame driver.Rect       // where it is drawn, in the bar's coordinates
}

func newTitleBar(w *Window) *titleBar {
	tb := &titleBar{w: w}
	tb.Init(tb)
	tb.SetAcceptsFirstMouse(true) // the traffic lights and the strip act at once (S6.1b)
	tb.group = &lightGroup{}
	tb.group.Init(tb.group)
	ch := &tokens.Chrome
	tb.group.SetFrame(driver.Rect{X: ch.LightInset, Y: lightTop(), W: lightsWidth(), H: ch.LightSize})
	flags := [3]driver.WindowStyle{driver.StyleClosable, driver.StyleMiniaturizable, driver.StyleResizable}
	for i := range tb.lights {
		l := &trafficLight{bar: tb, kind: lightKind(i)}
		l.Init(l)
		l.SetFrame(driver.Rect{X: float64(i) * ch.LightStep, W: ch.LightSize, H: ch.LightSize})
		l.SetEnabled(w.style&flags[i] != 0)
		tb.lights[i] = l
		tb.group.AddChild(l)
	}
	tb.AddChild(tb.group)
	return tb
}

// setMenuBar replaces the menu strip with one of bar (nil: none) and lays
// the bar out again.
func (tb *titleBar) setMenuBar(bar *menubar.Bar) {
	if tb.strip != nil {
		tb.strip.leaveKeyboard() // S5.6i
		tb.strip.CloseMenu()     // no report (S5.6f)

		tb.strip.stopFlash()
		tb.strip.RemoveFromParent()
		tb.strip = nil
	}
	if bar != nil {
		tb.strip = newMenuStrip(tb, bar)
		tb.InsertChild(tb.IndexOf(tb.group), tb.strip)
	}
	tb.layout()
}

// layout lays out the menu strip and the title for the current width: the
// title centered in the bar, truncated with an ellipsis so it stays
// titleMargin() away from both edges, or further when the strip reaches
// further (menubar.Strip.TitleRoom; symmetric, so it stays centered), and
// hidden when not even the ellipsis fits.
func (tb *titleBar) layout() {
	tb.title, tb.titleFrame = nil, driver.Rect{}
	defer tb.Invalidate()
	avail := tb.Frame().W - 2*titleMargin()
	if tb.strip != nil {
		avail = tb.strip.layout(tb.Frame().W)
	}
	if tb.w.title == "" || avail <= 0 {
		return
	}
	ts := tb.w.text
	l := truncate(ts, tb.w.title, titleFont(), avail)
	if l == nil {
		return
	}
	s := l.Size()
	tb.title = l
	tb.titleFrame = driver.Rect{
		X: (tb.Frame().W - s.W) / 2,
		Y: (tokens.Chrome.TitleBarHeight - s.H) / 2,
		W: s.W, H: s.H,
	}
}

// truncMemo keeps an element's last truncation (S6.2e): a title too wide
// for its control is truncated once, not laid out again (a binary search
// of layouts, each one shaped on Win32 and gtk4) on every paint. The zero
// value is empty; a change of text system, text, font or width truncates
// again. It is used on the UI thread, like the element that holds it.
type truncMemo struct {
	ts    render.TextSystem
	text  string
	font  driver.Font
	width float64
	l     render.TextLayout
	made  int // truncations made (tests)
}

// truncate is truncate(ts, text, font, width), from the memo when it holds
// that truncation.
func (m *truncMemo) truncate(ts render.TextSystem, text string, font driver.Font, width float64) render.TextLayout {
	if m.ts != nil && m.ts == ts && m.text == text && m.font == font && m.width == width {
		return m.l
	}
	m.ts, m.text, m.font, m.width = ts, text, font, width
	m.l = truncate(ts, text, font, width)
	m.made++
	return m.l
}

// truncate lays out text in font on one line and, when it is wider than
// width, cuts it at a rune boundary and appends an ellipsis: the longest
// prefix (trailing spaces dropped) that fits with it. It returns nil when not
// even the ellipsis alone fits. Newlines are shown as spaces.
func truncate(ts render.TextSystem, text string, font driver.Font, width float64) render.TextLayout {
	text = oneLine(text)
	if l := ts.NewLayout(text, font, 0); l.Size().W <= width {
		return l
	}
	// Binary search for the longest rune prefix that fits with the
	// ellipsis; widths grow with the prefix.
	bounds := []int{0}
	for i := range text {
		if i > 0 {
			bounds = append(bounds, i)
		}
	}
	bounds = append(bounds, len(text))
	var best render.TextLayout
	lo, hi := 0, len(bounds)-2 // candidate prefixes text[:bounds[k]], shorter than text
	for lo <= hi {
		mid := (lo + hi) / 2
		l := ts.NewLayout(trimRight(text[:bounds[mid]])+ellipsis, font, 0)
		if l.Size().W <= width {
			best, lo = l, mid+1
		} else {
			hi = mid - 1
		}
	}
	return best
}

// oneLine replaces line breaks with spaces.
func oneLine(s string) string {
	b := []byte(s)
	changed := false
	for i, c := range b {
		if c == '\n' || c == '\r' {
			b[i], changed = ' ', true
		}
	}
	if !changed {
		return s
	}
	return string(b)
}

// trimRight drops trailing spaces, so "Very long …" becomes "Very long…".
func trimRight(s string) string {
	for len(s) > 0 {
		r, n := utf8.DecodeLastRuneInString(s)
		if r != ' ' && r != '\t' {
			break
		}
		s = s[:len(s)-n]
	}
	return s
}

// regions returns the chrome regions of the title bar in surface
// coordinates: the bar as RegionCaption, then each shown menu title as
// RegionClient (S5.6c), then each enabled light as its button region. A
// disabled light stays part of the caption, so it cannot open Snap Layouts
// and dragging it moves the window.
func (tb *titleBar) regions() []shell.ChromeRegion {
	kinds := [3]shell.RegionKind{shell.RegionClose, shell.RegionMinimize, shell.RegionMaximize}
	rs := []shell.ChromeRegion{{Rect: tb.Frame(), Kind: shell.RegionCaption}}
	if tb.strip != nil && !tb.strip.Hidden() {
		rs = tb.strip.regions(rs)
	}
	for i, l := range tb.lights {
		if l.Enabled() {
			rs = append(rs, shell.ChromeRegion{Rect: driver.RectOf(ToRoot(l, driver.Point{}), l.Frame().Size()), Kind: kinds[i]})
		}
	}
	return rs
}

// TitleFrame returns where the title is drawn, in surface coordinates, and
// the drawn text ("" when the title is hidden).
func (w *Window) TitleFrame() (driver.Rect, string) {
	if w.bar == nil || w.bar.title == nil {
		return driver.Rect{}, ""
	}
	return w.bar.titleFrame.Offset(w.bar.Frame().Origin()), w.bar.title.Text()
}

// separator returns the hairline at the bar's bottom: whole device pixels
// (at least one) on the bar's pixel-aligned bottom edge, so it stays
// sharp at fractional scales like every other hairline (B17).
func (tb *titleBar) separator() driver.Rect {
	s := scaleIn(&tb.Base)
	t := pixelWidth(1, s)
	b := pixelAlign(&tb.Base, tb.Bounds(), s)
	return driver.Rect{X: b.X, Y: b.Y + b.H - t, W: b.W, H: t}
}

func (tb *titleBar) Paint(c render.Canvas) {
	a := tb.w.appearance
	b := tb.Bounds()
	c.FillRoundedRect(b, render.Radii{}, render.Solid(titleBarColor(a, tb.w.key)))
	sep := tokens.Chrome.TitleBarSeparator.Value(a)
	c.FillRoundedRect(tb.separator(), render.Radii{}, render.Solid(sep))
	if tb.title != nil {
		col := driver.ColorLabel
		if !tb.w.key {
			col = driver.ColorTertiaryLabel
		}
		c.DrawText(tb.title, tb.titleFrame.Origin(), semantic(col, a))
	}
}

func (tb *titleBar) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RoleTitleBar, Label: tb.w.title}
}

// lightGroup holds the three traffic lights. Hovering anywhere over it
// (gaps included) shows the glyphs on all of them, as on macOS.
type lightGroup struct {
	Base
	hovered bool
}

func (g *lightGroup) HandleEvent(ev Event) bool {
	if pe, ok := ev.(PointerEvent); ok && (pe.Kind == PointerEnter || pe.Kind == PointerLeave) {
		g.setHovered(pe.Kind == PointerEnter)
	}
	return false
}

func (g *lightGroup) setHovered(h bool) {
	if h == g.hovered {
		return
	}
	g.hovered = h
	for _, c := range g.children {
		c.base().Invalidate()
	}
}

// trafficLight is one of the three window buttons. Its clicks go through
// the clickControl core (control.go); it never takes the focus, so its
// ring stays hidden. A light the window's style disables does nothing.
type trafficLight struct {
	Base
	clickControl
	bar   *titleBar
	kind  lightKind
	glyph pathMemo[float64] // by the light's size
}

// HitSelf hits the circle only.
func (l *trafficLight) HitSelf(p driver.Point) bool {
	r := tokens.Chrome.LightSize / 2
	dx, dy := p.X-r, p.Y-r
	return dx*dx+dy*dy <= r*r
}

func (l *trafficLight) HandleEvent(ev Event) bool { return l.handleClick(l, ev, nil) }

// Activate performs the light's action, refused while it is disabled.
func (l *trafficLight) Activate() bool { return l.activate(l) }

// click runs the light's action.
func (l *trafficLight) click() {
	w := l.bar.w
	switch l.kind {
	case lightClose:
		w.performClose()
	case lightMinimize:
		w.surface.Minimize()
	case lightZoom:
		w.surface.Zoom()
	}
}

func (l *trafficLight) Paint(c render.Canvas) {
	w := l.bar.w
	a := w.appearance
	b := l.Bounds()
	ch := &tokens.Chrome
	radii := render.UniformRadii(ch.LightSize / 2)
	hovered := l.bar.group.hovered
	pressed := l.press.Pressed()
	var fill, border, glyph driver.Color
	switch {
	case !l.Enabled():
		fill, border = inactiveLightColors(a)
		fill.A, border.A = ch.LightDisabledAlpha, ch.LightDisabledAlpha
	case w.key || hovered || pressed:
		fill, border, glyph = lightColors(l.kind, a)
	default:
		fill, border = inactiveLightColors(a)
	}
	if pressed {
		shade := ch.LightPressedShade
		fill, border, glyph = darken(fill, shade), darken(border, shade), darken(glyph, shade)
	}
	c.FillRoundedRect(b, radii, render.Solid(fill))
	// The border is stroked just inside the circle.
	bw := ch.LightBorderWidth
	c.StrokeRoundedRect(b.Inset(driver.UniformInsets(bw/2)), render.UniformRadii((ch.LightSize-bw)/2), bw, render.Solid(border))
	if l.Enabled() && (hovered || pressed) {
		g := l.glyph.get(ch.LightSize, func(p *render.Path) { lightGlyph(p, l.kind, ch.LightSize) })
		c.StrokePath(g, ch.LightGlyphStroke, render.Solid(glyph), render.StrokeStyle{Cap: render.CapRound})
	}
}

// lightGlyph adds to p the ×, − or + of a light of diameter size, in its
// coordinates: drawn on a 12 pt design grid and scaled by size/12.
func lightGlyph(p *render.Path, k lightKind, size float64) {
	f := size / 12
	pt := func(x, y float64) driver.Point { return driver.Point{X: x * f, Y: y * f} }
	switch k {
	case lightClose:
		p.MoveTo(pt(3.5, 3.5)).LineTo(pt(8.5, 8.5))
		p.MoveTo(pt(8.5, 3.5)).LineTo(pt(3.5, 8.5))
	case lightMinimize:
		p.MoveTo(pt(3, 6)).LineTo(pt(9, 6))
	case lightZoom:
		p.MoveTo(pt(3, 6)).LineTo(pt(9, 6))
		p.MoveTo(pt(6, 3)).LineTo(pt(6, 9))
	}
}

func (l *trafficLight) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RoleButton, Label: lightLabels[l.kind], Actions: a11y.ActionPress}
}
