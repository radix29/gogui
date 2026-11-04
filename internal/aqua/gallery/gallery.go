// Package gallery builds the gallery scene of the golden-image tests
// (PLAN.md S3.6): a fixed-frame grid of every v0.1 control that is real in
// the aqua driver, in the states the driver API can set.
//
// The scene is built on driver.Driver alone, with fixed frames (SetFrame,
// no layout engine), so the aqua driver (S3.6c, golden PNGs) and the appkit
// driver (S1.5, the macOS references) build the same widgets at the same
// frames. It imports only internal/driver: importing gogui would make a
// cycle through shell/native into the win32 shell's tests.
//
// Hover and pressed states cannot be set through the driver API; they are
// pinned by the per-control state dumps in internal/aqua/testdata instead.
// TextField, TextView and PopUpButton are still placeholders in the aqua
// driver and are left out until phase 5, which adds them and regenerates
// the goldens.
package gallery

import (
	"fmt"

	"github.com/radix29/gogui/internal/driver"
)

// Title is the title of the gallery window.
const Title = "Gallery"

// Size is the content size of the gallery scene: the frame of the root
// widget and the content size of the window Open creates.
var Size = driver.Size{W: 600, H: 346}

// Style is the style of the window Open creates: every title-bar button.
const Style = driver.StyleTitled | driver.StyleClosable | driver.StyleMiniaturizable | driver.StyleResizable

// ScrollOffset is the offset of the scroll view: its document is scrolled
// off both axes, so both scrollers show a knob away from the start.
var ScrollOffset = driver.Point{X: 60, Y: 40}

// Build creates the gallery scene with d's factories and returns its root
// widget (a container) and its size. The caller sets the root's frame to
// (0, 0, Size), normally as a window's content. The focused control needs a
// window, so Build focuses nothing; Open does.
func Build(d driver.Driver) (driver.Widget, driver.Size) {
	root, _ := build(d)
	return root, Size
}

// Open builds the scene in a new window of content size Size, shows it and
// focuses the scene's focused control (the "Focused" checkbox). h receives
// the window's events.
func Open(d driver.Driver, h driver.WindowHandler) (driver.Window, error) {
	w, err := d.NewWindow(driver.WindowConfig{Title: Title, Size: Size, Style: Style}, h)
	if err != nil {
		return nil, err
	}
	focus := content(d, w)
	w.Show()
	focus.Focus()
	return w, nil
}

// Fill is Open without the window and without Show: it builds the scene
// as the content of w (whose content size should be Size) and focuses the
// scene's focused control. The golden renders use it to keep their window
// hidden (S3.6c).
func Fill(d driver.Driver, w driver.Window) {
	content(d, w).Focus()
}

// content builds the scene as w's content and returns the control to
// focus.
func content(d driver.Driver, w driver.Window) driver.Widget {
	root, focus := build(d)
	w.SetContent(root)
	root.SetFrame(driver.Rect{W: Size.W, H: Size.H})
	return focus
}

// Column frames: three columns of 170, 170 and 180 points with 20-point
// margins, split by a vertical separator between the first two.
const (
	colA, colAW = 20, 170
	colB, colBW = 210, 170
	colC, colCW = 400, 180
	margin      = 20
)

func rect(x, y, w, h float64) driver.Rect { return driver.Rect{X: x, Y: y, W: w, H: h} }

// noButton, noToggle, noSlider and noScroll are the handlers of the scene:
// it takes no input it has to answer.
type (
	noButton struct{}
	noToggle struct{}
	noSlider struct{}
	noScroll struct{}
)

func (noButton) Clicked()                       {}
func (noToggle) StateChanged(driver.CheckState) {}
func (noSlider) ValueChanged(float64)           {}
func (noScroll) Scrolled(driver.Point)          {}
func (noScroll) ViewportResized(driver.Size)    {}

// scene collects the children of a container in paint and focus order.
type scene struct {
	d        driver.Driver
	children []driver.Widget
}

func (s *scene) add(w driver.Widget, r driver.Rect) {
	w.SetFrame(r)
	s.children = append(s.children, w)
}

func (s *scene) label(text string, f driver.Font, c driver.Color, r driver.Rect) {
	l := s.d.NewLabel()
	l.SetText(text)
	l.SetFont(f)
	l.SetTextColor(c)
	s.add(l, r)
}

func (s *scene) button(title string, cs driver.ControlSize, r driver.Rect) driver.Button {
	b := s.d.NewButton(noButton{})
	b.SetTitle(title)
	b.SetControlSize(cs)
	s.add(b, r)
	return b
}

func (s *scene) checkbox(title string, st driver.CheckState, r driver.Rect) driver.Checkbox {
	c := s.d.NewCheckbox(noToggle{})
	c.SetTitle(title)
	c.SetState(st)
	s.add(c, r)
	return c
}

func (s *scene) slider(v float64, ticks int, r driver.Rect) driver.Slider {
	sl := s.d.NewSlider(noSlider{})
	sl.SetRange(0, 100)
	sl.SetValue(v)
	sl.SetTickMarks(ticks, ticks > 0)
	s.add(sl, r)
	return sl
}

func (s *scene) progress(style driver.ProgressStyle, v float64, indeterminate bool, r driver.Rect) {
	p := s.d.NewProgress()
	p.SetStyle(style)
	p.SetRange(0, 100)
	p.SetValue(v)
	p.SetIndeterminate(indeterminate)
	s.add(p, r)
}

func (s *scene) container() driver.Container {
	c := s.d.NewContainer()
	c.SetChildren(s.children)
	return c
}

// build creates the scene and returns its root and the control Open
// focuses.
func build(d driver.Driver) (root driver.Container, focus driver.Widget) {
	s := &scene{d: d}

	// Column A: labels, a separator and push buttons.
	s.label("Label", driver.Font{}, driver.Color{}, rect(colA, 20, colAW, 16))
	s.label("Secondary Label", driver.Font{}, driver.Semantic(driver.ColorSecondaryLabel), rect(colA, 40, colAW, 16))
	s.label("Disabled Label", driver.Font{}, driver.Semantic(driver.ColorDisabledControlText), rect(colA, 60, colAW, 16))
	s.label("Small Label", driver.Font{Size: 11}, driver.Color{}, rect(colA, 80, colAW, 14))
	s.label("Bold Label", driver.Font{Weight: driver.FontWeightBold}, driver.Color{}, rect(colA, 98, colAW, 16))
	sep := d.NewSeparator()
	s.add(sep, rect(colA, 124, colAW, 1))
	s.button("Push Button", driver.ControlSizeRegular, rect(colA, 134, 120, 20))
	s.button("Default", driver.ControlSizeRegular, rect(colA, 162, 120, 20)).SetDefault(true)
	s.button("Cancel", driver.ControlSizeRegular, rect(colA, 190, 120, 20)).SetCancel(true)
	s.button("Disabled", driver.ControlSizeRegular, rect(colA, 218, 120, 20)).SetEnabled(false)
	s.button("Small", driver.ControlSizeSmall, rect(colA, 246, 120, 17))
	s.button("Mini", driver.ControlSizeMini, rect(colA, 271, 120, 14))
	s.button("Large", driver.ControlSizeLarge, rect(colA, 293, 120, 26))

	vsep := d.NewSeparator()
	vsep.SetVertical(true)
	s.add(vsep, rect(colA+colAW+9, margin, 1, 300))

	// Column B: checkboxes and a radio group in a box.
	s.checkbox("Checkbox", driver.StateOff, rect(colB, 20, colBW, 20))
	s.checkbox("Checked", driver.StateOn, rect(colB, 44, colBW, 20))
	s.checkbox("Mixed", driver.StateMixed, rect(colB, 68, colBW, 20))
	s.checkbox("Disabled", driver.StateOn, rect(colB, 92, colBW, 20)).SetEnabled(false)
	focus = s.checkbox("Focused", driver.StateOff, rect(colB, 116, colBW, 20))

	radios := &scene{d: d}
	for i, title := range []string{"Small", "Medium", "Large", "Disabled"} {
		r := d.NewRadio(noToggle{})
		r.SetTitle(title)
		r.SetOn(i == 1)
		r.SetEnabled(i != 3)
		radios.add(r, rect(0, float64(i)*22, 150, 20))
	}
	box := d.NewBox()
	box.SetTitle("Radio Group")
	group := radios.container()
	box.SetContent(group)
	s.add(box, rect(colB, 150, colBW, 124))
	group.SetFrame(rect(0, 0, 150, 86))

	// Column C: sliders, progress indicators and a scroll view.
	s.slider(40, 0, rect(colC, 20, colCW, 20))
	s.slider(75, 5, rect(colC, 48, colCW, 28))
	s.slider(25, 0, rect(colC, 84, colCW, 20)).SetEnabled(false)
	s.progress(driver.ProgressBar, 40, false, rect(colC, 116, colCW, 6))
	s.progress(driver.ProgressBar, 0, true, rect(colC, 134, colCW, 6))
	s.progress(driver.ProgressSpinner, 0, true, rect(colC, 152, 32, 32))

	// The document: a 3 × 8 grid of labels, larger than the viewport on
	// both axes.
	doc := &scene{d: d}
	for row := range 8 {
		for col := range 3 {
			doc.label(fmt.Sprintf("Item %d.%d", row+1, col+1), driver.Font{}, driver.Color{},
				rect(float64(10+col*120), float64(10+row*30), 100, 16))
		}
	}
	docSize := driver.Size{W: 360, H: 250}
	sv := d.NewScrollView(noScroll{})
	sv.SetHasVerticalScroller(true)
	sv.SetHasHorizontalScroller(true)
	docRoot := doc.container()
	sv.SetDocument(docRoot)
	sv.SetDocumentSize(docSize)
	docRoot.SetFrame(driver.RectOf(driver.Point{}, docSize)) // as the layout engine arranges it
	s.add(sv, rect(colC, 206, colCW, 120))
	sv.SetOffset(ScrollOffset)

	return s.container(), focus
}
