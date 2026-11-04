package driver

// Label is a static, non-editable text (NSTextField labelWithString:).
type Label interface {
	Widget
	SetText(text string)
	// SetFont sets the font. The zero Font is the system font.
	SetFont(f Font)
	// SetTextColor sets the text color. The zero Color means ColorLabel.
	SetTextColor(c Color)
	SetAlignment(a TextAlignment)
	// SetSelectable lets the user select and copy the text.
	SetSelectable(selectable bool)
	// SetWraps makes the text wrap at the frame width. FittingSize then
	// uses max.W as the wrap width.
	SetWraps(wraps bool)
}

// ButtonHandler receives events from a Button.
type ButtonHandler interface {
	// Clicked is called when the user activates the button: a click, Space
	// while focused, Return for the default button or Escape for the
	// cancel button.
	Clicked()
}

// Button is a push button (NSButton with the push bezel).
type Button interface {
	Widget
	SetTitle(title string)
	// SetDefault makes this the window's default button: drawn with the
	// accent color and triggered by Return.
	SetDefault(isDefault bool)
	// SetCancel makes this the window's cancel button, triggered by Escape.
	SetCancel(isCancel bool)
}

// CheckState is the state of a checkbox or radio button. The values match
// NSControlStateValue.
type CheckState int8

const (
	StateOff   CheckState = 0
	StateOn    CheckState = 1
	StateMixed CheckState = -1
)

// ToggleHandler receives events from a Checkbox or a Radio.
type ToggleHandler interface {
	// StateChanged is called when the user changes the state. For a Radio
	// it is called with StateOn every time the user selects it.
	StateChanged(s CheckState)
}

// Checkbox is a check box (NSButton with the switch style).
type Checkbox interface {
	Widget
	SetTitle(title string)
	// SetState sets the state. StateMixed can be set only from code; when
	// the user clicks a mixed checkbox it turns on.
	SetState(s CheckState)
	State() CheckState
}

// Radio is a single radio button (NSButton with the radio style). The public
// RadioGroup puts several radios in a Container and enforces exclusivity;
// the driver only reports clicks and arrow-key moves.
type Radio interface {
	Widget
	SetTitle(title string)
	// SetOn sets whether the radio is selected.
	SetOn(on bool)
	On() bool
}

// TextFieldHandler receives events from a TextField.
type TextFieldHandler interface {
	// TextChanged is called after every edit by the user, including IME
	// commits, paste and undo.
	TextChanged(text string)
	// Submitted is called when the user presses Return in the field.
	Submitted(text string)
}

// TextField is a single-line editable text field (NSTextField, or
// NSSecureTextField for a secure field).
type TextField interface {
	Widget
	SetText(text string)
	Text() string
	SetPlaceholder(text string)
	SetFont(f Font)
	SetAlignment(a TextAlignment)
	// SetEditable controls whether the user can change the text. A
	// non-editable field can still be focused and its text selected.
	SetEditable(editable bool)
}

// SliderHandler receives events from a Slider.
type SliderHandler interface {
	// ValueChanged is called continuously while the user drags the knob,
	// and on keyboard changes.
	ValueChanged(v float64)
}

// Slider is a linear slider (NSSlider).
type Slider interface {
	Widget
	// SetRange sets the minimum and maximum values. The current value is
	// clamped to the new range.
	SetRange(min, max float64)
	// SetValue sets the value, clamped to the range.
	SetValue(v float64)
	Value() float64
	// SetTickMarks shows count tick marks (0 hides them). With snap set,
	// the value only takes tick-mark values.
	SetTickMarks(count int, snap bool)
	SetVertical(vertical bool)
}

// ProgressStyle selects the look of a Progress.
type ProgressStyle uint8

const (
	ProgressBar     ProgressStyle = iota // horizontal bar (the default)
	ProgressSpinner                      // spinning indicator
)

// Progress is a progress indicator (NSProgressIndicator).
type Progress interface {
	Widget
	SetStyle(s ProgressStyle)
	SetRange(min, max float64)
	SetValue(v float64)
	// SetIndeterminate switches to the indeterminate animation. The
	// animation runs while the indicator is visible; drivers stop it when
	// it is hidden, its window is hidden, or Reduce Motion is on.
	SetIndeterminate(indeterminate bool)
}

// PopUpHandler receives events from a PopUpButton.
type PopUpHandler interface {
	// SelectionChanged is called when the user picks an item, even if it
	// is the one already selected.
	SelectionChanged(index int)
}

// PopUpButton is a pop-up menu button (NSPopUpButton, pullsDown NO). Its
// menu opens over the selected item.
type PopUpButton interface {
	Widget
	// SetItems replaces the items. The selection is kept if it is still in
	// range, and otherwise reset to 0 (or -1 when there are no items).
	SetItems(items []string)
	// SetSelected selects the item at index; -1 selects nothing.
	SetSelected(index int)
	Selected() int
}

// Separator is a hairline separator (NSBox with the separator type).
type Separator interface {
	Widget
	// SetVertical switches between a horizontal (the default) and a
	// vertical line.
	SetVertical(vertical bool)
}

// Box is a titled group box (NSBox with the primary type) that holds one
// content widget.
type Box interface {
	Widget
	// SetTitle sets the title shown above the box. An empty title hides it.
	SetTitle(title string)
	// SetContent sets the content widget; nil removes it. The content's
	// frame is relative to the box's content area.
	SetContent(w Widget)
	// ContentInsets returns the distance from the box frame to its content
	// area, for the current title and control size. The layout engine uses
	// it to measure and place the content.
	ContentInsets() Insets
}

// ScrollHandler receives events from a ScrollView.
type ScrollHandler interface {
	// Scrolled is called when the visible part of the document moves, by
	// user input or momentum (not by SetOffset).
	Scrolled(offset Point)
	// ViewportResized is called when the visible area changes size without
	// a SetFrame call, for example when legacy scrollers appear. The public
	// layer re-runs the layout in response.
	ViewportResized(size Size)
}

// ScrollView shows a part of one document widget (NSScrollView with a
// flipped document view).
type ScrollView interface {
	Widget
	// SetDocument sets the document widget; nil removes it.
	SetDocument(w Widget)
	// SetDocumentSize sets the size of the document, as computed by the
	// layout engine. The document's frame is (0, 0, size) in document
	// coordinates. The offset is clamped to the new size.
	SetDocumentSize(size Size)
	SetHasVerticalScroller(has bool)
	SetHasHorizontalScroller(has bool)
	// ViewportSize returns the size of the visible document area: the
	// frame minus any legacy scrollers and borders. It depends only on the
	// frame, the enabled scrollers and the appearance (scrollers never
	// auto-hide), so SetDocumentSize never changes it and never causes
	// ViewportResized.
	ViewportSize() Size
	// Offset returns the document point shown at the top-left corner of
	// the viewport.
	Offset() Point
	// SetOffset scrolls to offset, clamped to the document, without
	// animation.
	SetOffset(offset Point)
	// ScrollRectToVisible scrolls the least amount needed to show r, in
	// document coordinates.
	ScrollRectToVisible(r Rect)
}

// TextViewHandler receives events from a TextView.
type TextViewHandler interface {
	// TextChanged is called after every edit by the user. It carries no
	// text, because documents can be large; call TextView.Text if needed.
	TextChanged()
}

// TextView is a multi-line, plain-text editor that scrolls itself. Text
// wraps at the view width.
type TextView interface {
	Widget
	SetText(text string)
	Text() string
	SetFont(f Font)
	// SetTextColor sets the text color. The zero Color means ColorText.
	SetTextColor(c Color)
	SetEditable(editable bool)
	// Selection returns the selected range as byte offsets into Text, with
	// start <= end. An empty selection is the caret position.
	Selection() (start, end int)
	// SetSelection selects [start, end) and scrolls it into view. The
	// offsets are clamped to the text and moved back to the nearest
	// character boundary.
	SetSelection(start, end int)
}
