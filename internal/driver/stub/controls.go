package stub

import (
	"math"
	"strings"

	"github.com/radix29/gogui/internal/driver"
)

// Constructors accept a nil handler, which simplifies tests; the real
// drivers require one.

// Label implements driver.Label.
type Label struct {
	Base
	Text       string
	Font       driver.Font
	TextColor  driver.Color
	Alignment  driver.TextAlignment
	Selectable bool
	Wraps      bool
}

// NewLabel returns a stub label.
func NewLabel() *Label {
	l := &Label{}
	l.natural = func(max driver.Size) driver.Size {
		maxW := math.Inf(1)
		if l.Wraps {
			maxW = max.W
		}
		return wrappedText(l.Text, l.Font, l.ControlSize, maxW)
	}
	return l
}

func (l *Label) SetText(text string)                 { l.Text = text }
func (l *Label) SetFont(f driver.Font)               { l.Font = f }
func (l *Label) SetTextColor(c driver.Color)         { l.TextColor = c }
func (l *Label) SetAlignment(a driver.TextAlignment) { l.Alignment = a }
func (l *Label) SetSelectable(selectable bool)       { l.Selectable = selectable }
func (l *Label) SetWraps(wraps bool)                 { l.Wraps = wraps }

// Button implements driver.Button.
type Button struct {
	Base
	Title     string
	IsDefault bool
	IsCancel  bool
	Handler   driver.ButtonHandler
}

// NewButton returns a stub button.
func NewButton(h driver.ButtonHandler) *Button {
	b := &Button{Handler: h}
	b.natural = func(driver.Size) driver.Size {
		return driver.Size{
			W: TextWidth(b.Title, driver.Font{}, b.ControlSize) + buttonPadding,
			H: ControlHeight(b.ControlSize),
		}
	}
	return b
}

func (b *Button) SetTitle(title string)     { b.Title = title }
func (b *Button) SetDefault(isDefault bool) { b.IsDefault = isDefault }
func (b *Button) SetCancel(isCancel bool)   { b.IsCancel = isCancel }

// UserClick simulates a click.
func (b *Button) UserClick() {
	if b.Disabled || b.Handler == nil {
		return
	}
	b.Handler.Clicked()
}

func toggleSize(title string, cs driver.ControlSize) driver.Size {
	w := toggleGlyph
	if title != "" {
		w += TextWidth(title, driver.Font{}, cs)
	}
	return driver.Size{W: w, H: toggleGlyph}
}

// Checkbox implements driver.Checkbox.
type Checkbox struct {
	Base
	Title   string
	Value   driver.CheckState
	Handler driver.ToggleHandler
}

// NewCheckbox returns a stub checkbox.
func NewCheckbox(h driver.ToggleHandler) *Checkbox {
	c := &Checkbox{Handler: h}
	c.natural = func(driver.Size) driver.Size { return toggleSize(c.Title, c.ControlSize) }
	return c
}

func (c *Checkbox) SetTitle(title string)        { c.Title = title }
func (c *Checkbox) SetState(s driver.CheckState) { c.Value = s }
func (c *Checkbox) State() driver.CheckState     { return c.Value }

// UserClick simulates a click: off turns on, on turns off, mixed turns on.
func (c *Checkbox) UserClick() {
	if c.Disabled {
		return
	}
	if c.Value == driver.StateOn {
		c.Value = driver.StateOff
	} else {
		c.Value = driver.StateOn
	}
	if c.Handler != nil {
		c.Handler.StateChanged(c.Value)
	}
}

// Radio implements driver.Radio.
type Radio struct {
	Base
	Title   string
	IsOn    bool
	Handler driver.ToggleHandler
}

// NewRadio returns a stub radio button.
func NewRadio(h driver.ToggleHandler) *Radio {
	r := &Radio{Handler: h}
	r.natural = func(driver.Size) driver.Size { return toggleSize(r.Title, r.ControlSize) }
	return r
}

func (r *Radio) SetTitle(title string) { r.Title = title }
func (r *Radio) SetOn(on bool)         { r.IsOn = on }
func (r *Radio) On() bool              { return r.IsOn }

// UserClick simulates selecting the radio. It reports StateOn even if the
// radio was already on, like AppKit.
func (r *Radio) UserClick() {
	if r.Disabled {
		return
	}
	r.IsOn = true
	if r.Handler != nil {
		r.Handler.StateChanged(driver.StateOn)
	}
}

// TextField implements driver.TextField.
type TextField struct {
	Base
	Secure      bool
	Value       string
	Placeholder string
	Font        driver.Font
	Alignment   driver.TextAlignment
	ReadOnly    bool
	Handler     driver.TextFieldHandler
}

// NewTextField returns a stub text field.
func NewTextField(secure bool, h driver.TextFieldHandler) *TextField {
	t := &TextField{Secure: secure, Handler: h}
	t.natural = func(driver.Size) driver.Size {
		return driver.Size{W: defaultWidth, H: ControlHeight(t.ControlSize)}
	}
	return t
}

func (t *TextField) SetText(text string)                 { t.Value = text }
func (t *TextField) Text() string                        { return t.Value }
func (t *TextField) SetPlaceholder(text string)          { t.Placeholder = text }
func (t *TextField) SetFont(f driver.Font)               { t.Font = f }
func (t *TextField) SetAlignment(a driver.TextAlignment) { t.Alignment = a }
func (t *TextField) SetEditable(editable bool)           { t.ReadOnly = !editable }

// UserType simulates the user replacing the whole text with text.
func (t *TextField) UserType(text string) {
	if t.Disabled || t.ReadOnly {
		return
	}
	t.Value = text
	if t.Handler != nil {
		t.Handler.TextChanged(text)
	}
}

// UserSubmit simulates pressing Return.
func (t *TextField) UserSubmit() {
	if t.Disabled || t.Handler == nil {
		return
	}
	t.Handler.Submitted(t.Value)
}

// Slider implements driver.Slider.
type Slider struct {
	Base
	Min, Max   float64
	Val        float64
	TickMarks  int
	SnapToTick bool
	Vertical   bool
	Handler    driver.SliderHandler
}

// NewSlider returns a stub slider with the AppKit default range 0…1.
func NewSlider(h driver.SliderHandler) *Slider {
	s := &Slider{Max: 1, Handler: h}
	s.natural = func(driver.Size) driver.Size {
		if s.Vertical {
			return driver.Size{W: ControlHeight(s.ControlSize), H: defaultWidth}
		}
		return driver.Size{W: defaultWidth, H: ControlHeight(s.ControlSize)}
	}
	return s
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func (s *Slider) SetRange(min, max float64) {
	s.Min, s.Max = min, max
	s.Val = clamp(s.Val, min, max)
}
func (s *Slider) SetValue(v float64) { s.Val = clamp(v, s.Min, s.Max) }
func (s *Slider) Value() float64     { return s.Val }
func (s *Slider) SetTickMarks(count int, snap bool) {
	s.TickMarks, s.SnapToTick = count, snap
}
func (s *Slider) SetVertical(vertical bool) { s.Vertical = vertical }

// UserDrag simulates the user moving the knob to v (clamped).
func (s *Slider) UserDrag(v float64) {
	if s.Disabled {
		return
	}
	s.Val = clamp(v, s.Min, s.Max)
	if s.Handler != nil {
		s.Handler.ValueChanged(s.Val)
	}
}

// Progress implements driver.Progress.
type Progress struct {
	Base
	Style         driver.ProgressStyle
	Min, Max      float64
	Value         float64
	Indeterminate bool
}

// NewProgress returns a stub progress indicator with the AppKit default
// range 0…100.
func NewProgress() *Progress {
	p := &Progress{Max: 100}
	p.natural = func(driver.Size) driver.Size {
		if p.Style == driver.ProgressSpinner {
			return driver.Size{W: spinnerSize, H: spinnerSize}
		}
		return driver.Size{W: defaultWidth, H: ControlHeight(p.ControlSize)}
	}
	return p
}

func (p *Progress) SetStyle(s driver.ProgressStyle) { p.Style = s }
func (p *Progress) SetRange(min, max float64) {
	p.Min, p.Max = min, max
	p.Value = clamp(p.Value, min, max)
}
func (p *Progress) SetValue(v float64)                  { p.Value = clamp(v, p.Min, p.Max) }
func (p *Progress) SetIndeterminate(indeterminate bool) { p.Indeterminate = indeterminate }

// PopUpButton implements driver.PopUpButton.
type PopUpButton struct {
	Base
	Items   []string
	Index   int
	Handler driver.PopUpHandler
}

// NewPopUpButton returns a stub pop-up button with no items.
func NewPopUpButton(h driver.PopUpHandler) *PopUpButton {
	p := &PopUpButton{Index: -1, Handler: h}
	p.natural = func(driver.Size) driver.Size {
		w := 0.0
		for _, it := range p.Items {
			w = math.Max(w, TextWidth(it, driver.Font{}, p.ControlSize))
		}
		return driver.Size{W: w + popUpPadding, H: ControlHeight(p.ControlSize)}
	}
	return p
}

func (p *PopUpButton) SetItems(items []string) {
	p.Items = append([]string(nil), items...)
	switch {
	case len(p.Items) == 0:
		p.Index = -1
	case p.Index < 0 || p.Index >= len(p.Items):
		p.Index = 0
	}
}

func (p *PopUpButton) SetSelected(index int) {
	if index < -1 || index >= len(p.Items) {
		index = -1
	}
	p.Index = index
}

func (p *PopUpButton) Selected() int { return p.Index }

// UserSelect simulates the user choosing the item at index. Out-of-range
// indices are ignored.
func (p *PopUpButton) UserSelect(index int) {
	if p.Disabled || index < 0 || index >= len(p.Items) {
		return
	}
	p.Index = index
	if p.Handler != nil {
		p.Handler.SelectionChanged(index)
	}
}

// Separator implements driver.Separator.
type Separator struct {
	Base
	Vertical bool
}

// NewSeparator returns a horizontal stub separator.
func NewSeparator() *Separator {
	s := &Separator{}
	s.natural = func(driver.Size) driver.Size { return driver.Size{W: 1, H: 1} }
	return s
}

func (s *Separator) SetVertical(vertical bool) { s.Vertical = vertical }

// Box implements driver.Box.
type Box struct {
	Base
	Title   string
	Content driver.Widget
}

// NewBox returns an untitled stub box.
func NewBox() *Box {
	b := &Box{}
	b.natural = func(driver.Size) driver.Size {
		in := b.ContentInsets()
		return driver.Size{W: in.Horizontal(), H: in.Vertical()}
	}
	return b
}

func (b *Box) SetTitle(title string) { b.Title = title }

func (b *Box) SetContent(w driver.Widget) {
	if b.Content != nil && b.Content != w {
		release(b, b.Content)
	}
	b.Content = w
	if w != nil {
		adopt(b, w)
	}
}

func (b *Box) ContentInsets() driver.Insets {
	in := driver.UniformInsets(boxInset)
	if strings.TrimSpace(b.Title) != "" {
		in.Top += boxTitleHeight
	}
	return in
}

func (b *Box) removeChild(child *Base) {
	if b.Content != nil && baseOf(b.Content) == child {
		b.Content = nil
	}
}

// ScrollView implements driver.ScrollView.
type ScrollView struct {
	Base
	Document      driver.Widget
	DocumentSize  driver.Size
	HasVertical   bool
	HasHorizontal bool
	Off           driver.Point
	Handler       driver.ScrollHandler

	// ViewportInset is the part of the frame the viewport doesn't cover
	// (legacy scrollers, borders): the viewport is the frame size minus it.
	ViewportInset driver.Insets
}

// NewScrollView returns a stub scroll view. Its viewport is its frame size
// (overlay scrollers take no space) until ViewportInset is set.
func NewScrollView(h driver.ScrollHandler) *ScrollView {
	return &ScrollView{Handler: h}
}

func (s *ScrollView) SetFrame(r driver.Rect) {
	s.Frame = r
	s.Off = s.clampOffset(s.Off)
}

func (s *ScrollView) SetDocument(w driver.Widget) {
	if s.Document != nil && s.Document != w {
		release(s, s.Document)
	}
	s.Document = w
	if w != nil {
		adopt(s, w)
	}
}

func (s *ScrollView) SetDocumentSize(size driver.Size) {
	s.DocumentSize = size
	s.Off = s.clampOffset(s.Off)
}

func (s *ScrollView) SetHasVerticalScroller(has bool)   { s.HasVertical = has }
func (s *ScrollView) SetHasHorizontalScroller(has bool) { s.HasHorizontal = has }
func (s *ScrollView) Offset() driver.Point              { return s.Off }
func (s *ScrollView) SetOffset(offset driver.Point)     { s.Off = s.clampOffset(offset) }

// ViewportSize returns the frame size minus ViewportInset.
func (s *ScrollView) ViewportSize() driver.Size { return s.Frame.Size().Inset(s.ViewportInset) }

// UserResizeViewport simulates the OS changing the viewport without a
// SetFrame call (for example legacy scrollers appearing): it sets
// ViewportInset, clamps the offset and reports ViewportResized.
func (s *ScrollView) UserResizeViewport(in driver.Insets) {
	s.ViewportInset = in
	s.Off = s.clampOffset(s.Off)
	if s.Handler != nil {
		s.Handler.ViewportResized(s.ViewportSize())
	}
}

func (s *ScrollView) ScrollRectToVisible(r driver.Rect) {
	vp := s.ViewportSize()
	off := s.Off
	// Bring the far edge in first, then the near edge, so that a rect
	// larger than the viewport shows its top-left corner.
	if r.MaxX() > off.X+vp.W {
		off.X = r.MaxX() - vp.W
	}
	if r.X < off.X {
		off.X = r.X
	}
	if r.MaxY() > off.Y+vp.H {
		off.Y = r.MaxY() - vp.H
	}
	if r.Y < off.Y {
		off.Y = r.Y
	}
	s.Off = s.clampOffset(off)
}

// MaxOffset returns the largest valid offset.
func (s *ScrollView) MaxOffset() driver.Point {
	vp := s.ViewportSize()
	return driver.Point{
		X: math.Max(0, s.DocumentSize.W-vp.W),
		Y: math.Max(0, s.DocumentSize.H-vp.H),
	}
}

func (s *ScrollView) clampOffset(p driver.Point) driver.Point {
	m := s.MaxOffset()
	return driver.Point{X: clamp(p.X, 0, m.X), Y: clamp(p.Y, 0, m.Y)}
}

// UserScroll simulates the user scrolling to offset (clamped).
func (s *ScrollView) UserScroll(offset driver.Point) {
	s.Off = s.clampOffset(offset)
	if s.Handler != nil {
		s.Handler.Scrolled(s.Off)
	}
}

func (s *ScrollView) removeChild(child *Base) {
	if s.Document != nil && baseOf(s.Document) == child {
		s.Document = nil
	}
}

// TextView implements driver.TextView.
type TextView struct {
	Base
	Value     string
	Font      driver.Font
	TextColor driver.Color
	ReadOnly  bool
	SelStart  int
	SelEnd    int
	Handler   driver.TextViewHandler
}

// NewTextView returns an empty stub text view.
func NewTextView(h driver.TextViewHandler) *TextView {
	return &TextView{Handler: h}
}

// SetText replaces the text and puts the caret at its end.
func (t *TextView) SetText(text string) {
	t.Value = text
	t.SelStart, t.SelEnd = len(text), len(text)
}

func (t *TextView) Text() string                { return t.Value }
func (t *TextView) SetFont(f driver.Font)       { t.Font = f }
func (t *TextView) SetTextColor(c driver.Color) { t.TextColor = c }
func (t *TextView) SetEditable(editable bool)   { t.ReadOnly = !editable }
func (t *TextView) Selection() (start, end int) { return t.SelStart, t.SelEnd }

func (t *TextView) SetSelection(start, end int) {
	start, end = t.boundary(start), t.boundary(end)
	if start > end {
		start, end = end, start
	}
	t.SelStart, t.SelEnd = start, end
}

// boundary clamps i to the text and moves it back to a rune boundary.
func (t *TextView) boundary(i int) int {
	i = int(clamp(float64(i), 0, float64(len(t.Value))))
	for i > 0 && i < len(t.Value) && !isRuneStart(t.Value[i]) {
		i--
	}
	return i
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// UserType simulates the user replacing the selection with text. The caret
// ends up after the inserted text.
func (t *TextView) UserType(text string) {
	if t.Disabled || t.ReadOnly {
		return
	}
	t.Value = t.Value[:t.SelStart] + text + t.Value[t.SelEnd:]
	t.SelStart += len(text)
	t.SelEnd = t.SelStart
	if t.Handler != nil {
		t.Handler.TextChanged()
	}
}
