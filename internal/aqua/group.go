package aqua

import (
	"math"
	"strings"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/render"
)

// The separator and the group box (PLAN.md S3.3d): real elements drawn
// from the Controls tokens. Both keep their state in their stub peer, as
// the Label does; neither is focusable.

// snapPixel rounds v (in points) to the nearest device pixel at scale.
func snapPixel(v, scale float64) float64 { return math.Round(v*scale) / scale }

// pixelWidth returns the width w (in points) rounded to whole device
// pixels at scale, at least one pixel.
func pixelWidth(w, scale float64) float64 { return max(1, math.Round(w*scale)) / scale }

// pixelAlign returns r (in e's own coordinates) with its edges moved to the
// nearest device pixels of the surface: the snapping is done in root
// coordinates, so it holds wherever e sits.
func pixelAlign(e *Base, r driver.Rect, scale float64) driver.Rect {
	o := ToRoot(e.Self(), driver.Point{})
	x0, y0 := snapPixel(o.X+r.X, scale)-o.X, snapPixel(o.Y+r.Y, scale)-o.Y
	x1, y1 := snapPixel(o.X+r.X+r.W, scale)-o.X, snapPixel(o.Y+r.Y+r.H, scale)-o.Y
	return driver.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Separator is the aqua driver.Separator: a hairline in ColorSeparator,
// SeparatorThickness thick (rounded to whole device pixels, at least one)
// and pixel-aligned at every scale. It runs along the longer side of its
// frame, centred across it; a square frame follows SetVertical. Its
// FittingSize is the thickness on both axes (the public layer expands it
// along its length). Accessibility: a separator.
type Separator struct {
	*stub.Separator
	peerBase
	view *separator
}

// NewSeparator returns a horizontal separator.
func (d *Driver) NewSeparator() driver.Separator {
	s := &Separator{Separator: stub.NewSeparator()}
	s.view = &separator{d: d, p: s}
	s.view.Init(s.view)
	s.initElement(s.Separator, &s.Separator.Base, s.view)
	return s
}

// SetVertical implements driver.Separator; it decides the direction of a
// square frame only.
func (s *Separator) SetVertical(vertical bool) { s.Separator.SetVertical(vertical); s.redraw() }

// FittingSize implements driver.Widget: SeparatorThickness on both axes,
// clamped to max; a stub Fixed size wins.
func (s *Separator) FittingSize(max driver.Size) driver.Size {
	if !s.Fixed.IsZero() {
		return s.Separator.FittingSize(max)
	}
	t := tokens.Controls.SeparatorThickness
	return driver.Size{W: t, H: t}.Min(max)
}

// separator is the element of a Separator.
type separator struct {
	Base
	d *Driver
	p *Separator
}

func (e *separator) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// vertical reports whether the line runs down the frame.
func (e *separator) vertical() bool {
	b := e.Bounds()
	return b.H > b.W || b.H == b.W && e.p.Vertical
}

// line returns the rectangle of the hairline in the element's coordinates.
func (e *separator) line() driver.Rect {
	return hairline(&e.Base, e.Bounds(), tokens.Controls.SeparatorThickness, scaleIn(&e.Base), e.vertical())
}

// Paint draws the hairline (nothing for an empty frame).
func (e *separator) Paint(c render.Canvas) {
	if e.Bounds().Empty() {
		return
	}
	col := semantic(driver.ColorSeparator, appearanceIn(&e.Base, e.d))
	c.FillRoundedRect(e.line(), render.Radii{}, render.Solid(col))
}

func (e *separator) AccessibleNode() a11y.Node { return a11y.Node{Role: a11y.RoleSeparator} }

// Box is the aqua driver.Box, an NSBox of the primary type: its title in
// the small system font (BoxTitleSize, BoxTitleWeight) at the top left of
// the frame, truncated with an ellipsis to the frame width, and below it,
// BoxTitleGap down, the group: a rounded rect (BoxCornerRadius) filled
// with BoxFill and outlined with BoxBorder, BoxBorderWidth wide (whole
// device pixels), its edges pixel-aligned. A blank title hides the title
// line and the group fills the frame.
//
// ContentInsets are the group's (Controls.BoxContentInsets) plus, with a
// title, the title band: the title's line height rounded up to whole
// points, plus BoxTitleGap. The stub's insets stay for the headless
// driver. The content element sits in a host element at these insets, so
// the content's frame (set by the public layer relative to the content
// area) lands inside the group. The tokens do not depend on the control
// size. Accessibility: a group with the title.
type Box struct {
	*stub.Box
	peerBase
	view    *group
	content *host

	title layoutMemo // the title layout
}

// NewBox returns an untitled box.
func (d *Driver) NewBox() driver.Box {
	b := &Box{Box: stub.NewBox(), content: newHost()}
	b.view = &group{d: d, p: b}
	b.view.Init(b.view)
	b.view.AddChild(b.content)
	b.initElement(b.Box, &b.Box.Base, b.view)
	return b
}

// boxTitleFont returns the font of the title.
func boxTitleFont() driver.Font {
	c := &tokens.Controls
	f := driver.Font{Size: c.BoxTitleSize, Weight: c.BoxTitleWeight}
	if f.Weight == driver.FontWeightRegular {
		f.Weight = 0 // the same font; keeps the dumps short
	}
	return f
}

// titleLayout returns the title on one line, untruncated, or nil for a
// blank title.
func (b *Box) titleLayout() render.TextLayout {
	if strings.TrimSpace(b.Title) == "" {
		return nil
	}
	return b.title.get(b.view.d.text, b.Title, boxTitleFont())
}

// band returns the height above the group: 0 without a title.
func (b *Box) band() float64 {
	l := b.titleLayout()
	if l == nil {
		return 0
	}
	return math.Ceil(l.Size().H) + tokens.Controls.BoxTitleGap
}

// ContentInsets implements driver.Box: the group's content insets plus
// the title band.
func (b *Box) ContentInsets() driver.Insets {
	in := tokens.Controls.BoxContentInsets()
	in.Top += b.band()
	return in
}

// FittingSize implements driver.Widget: the insets (an empty content
// area), clamped to max; a stub Fixed size wins.
func (b *Box) FittingSize(max driver.Size) driver.Size {
	if !b.Fixed.IsZero() {
		return b.Box.FittingSize(max)
	}
	in := b.ContentInsets()
	return driver.Size{W: in.Horizontal(), H: in.Vertical()}.Min(max)
}

// place frames the content host at the insets.
func (b *Box) place() { b.content.SetFrame(b.view.Bounds().Inset(b.ContentInsets())) }

func (b *Box) SetFrame(r driver.Rect) { b.peerBase.SetFrame(r); b.place() }

func (b *Box) SetTitle(title string) {
	b.Box.SetTitle(title)
	b.place()
	b.redraw()
}

func (b *Box) SetContent(w driver.Widget) {
	b.Box.SetContent(w)
	b.content.setChild(w)
}

// group is the element of a Box.
type group struct {
	Base
	d     *Driver
	p     *Box
	trunc truncMemo // the title truncated to the width
}

func (e *group) FittingSize(max driver.Size) driver.Size { return e.p.FittingSize(max) }

// Paint draws the title and the group (nothing for an empty frame).
func (e *group) Paint(c render.Canvas) {
	b := e.Bounds()
	if b.Empty() {
		return
	}
	a := appearanceIn(&e.Base, e.d)
	band := e.p.band()
	if band > 0 {
		l := e.p.titleLayout()
		if l != nil && l.Size().W > b.W {
			l = e.trunc.truncate(e.d.text, e.p.Title, boxTitleFont(), b.W)
		}
		if l != nil {
			col := driver.ColorLabel
			if !e.EnabledInTree() {
				col = driver.ColorDisabledControlText
			}
			c.DrawText(l, driver.Point{}, semantic(col, a))
		}
	}
	if band >= b.H {
		return
	}
	ct := &tokens.Controls
	s := scaleIn(&e.Base)
	r := pixelAlign(&e.Base, driver.Rect{Y: band, W: b.W, H: b.H - band}, s)
	if r.Empty() {
		return
	}
	radius := min(ct.BoxCornerRadius, r.W/2, r.H/2)
	c.FillRoundedRect(r, render.UniformRadii(radius), render.Solid(ct.BoxFill.Value(a)))
	if ct.BoxBorderWidth <= 0 {
		return
	}
	strokeInside(c, r, radius, pixelWidth(ct.BoxBorderWidth, s), ct.BoxBorder.Value(a))
}

func (e *group) AccessibleNode() a11y.Node {
	return a11y.Node{Role: a11y.RoleGroup, Label: e.p.Title}
}
