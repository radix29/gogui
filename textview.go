package gogui

import "github.com/radix29/gogui/internal/driver"

// TextView is a multi-line, plain-text editor that scrolls itself
// (NSTextView in an NSScrollView). The text wraps at the view's width. It
// expands on both axes.
type TextView struct {
	common[*TextView]
	p        driver.TextView
	text     string
	font     Font
	sel      [2]int // the last selection seen
	onChange subscribers[func(string)]
}

// NewTextView returns an empty, editable text view.
func NewTextView() *TextView {
	t := &TextView{}
	p := driverFor("NewTextView").NewTextView(textViewEvents{t})
	t.p = p
	t.init(t, kindTextView, p, nil)
	return t
}

// SetText replaces the text and puts the caret at its end. It doesn't call
// the OnChange handlers.
func (t *TextView) SetText(text string) *TextView {
	t.check("SetText")
	t.text = text
	t.sel = [2]int{len(text), len(text)}
	if t.live() {
		t.p.SetText(text)
		t.readSelection()
	}
	return t
}

// Text returns the text, including the user's edits.
func (t *TextView) Text() string { t.get("Text"); return t.text }

// SetFont sets the font of the whole text. The zero Font is the system
// font at the default size.
func (t *TextView) SetFont(f Font) *TextView {
	t.check("SetFont")
	t.font = f
	if t.live() {
		t.p.SetFont(f)
	}
	return t
}

// Font returns the font set with SetFont.
func (t *TextView) Font() Font { t.get("Font"); return t.font }

// SetTextColor sets the color of the whole text. The zero Color means
// ColorText.
func (t *TextView) SetTextColor(c Color) *TextView {
	t.check("SetTextColor")
	if t.live() {
		t.p.SetTextColor(c)
	}
	return t
}

// SetEditable controls whether the user can change the text (the default
// is true). A read-only view can still be focused and its text selected.
func (t *TextView) SetEditable(editable bool) *TextView {
	t.check("SetEditable")
	if t.live() {
		t.p.SetEditable(editable)
	}
	return t
}

// Selection returns the selected range as UTF-8 byte offsets into Text,
// with start <= end. An empty selection is the caret position.
func (t *TextView) Selection() (start, end int) {
	t.get("Selection")
	if t.live() {
		t.readSelection()
	}
	return t.sel[0], t.sel[1]
}

// SetSelection selects the bytes [start, end) and scrolls them into view.
// The offsets are clamped to the text and moved back to the nearest
// character boundary, and swapped if start > end.
func (t *TextView) SetSelection(start, end int) *TextView {
	t.check("SetSelection")
	if t.live() {
		t.p.SetSelection(start, end)
		t.readSelection()
	}
	return t
}

func (t *TextView) readSelection() { t.sel[0], t.sel[1] = t.p.Selection() }

// OnChange calls fn with the whole text after every edit by the user,
// including IME commits, paste and undo. SetText doesn't call it, and
// neither does a disabled view.
func (t *TextView) OnChange(fn func(text string)) Unsubscribe {
	t.check("OnChange")
	return t.onChange.add(fn)
}

// textViewEvents implements driver.TextViewHandler for a TextView. The
// peer event carries no text, so it is read once per edit and passed to
// every handler.
type textViewEvents struct{ t *TextView }

func (e textViewEvents) TextChanged() {
	t := e.t
	if t.dead {
		return
	}
	t.text = t.p.Text() // the cache follows the native text even when disabled (B14)
	t.readSelection()
	if t.disabled {
		return
	}
	text := t.text
	t.onChange.eachLive(&t.widget, func(fn func(string)) { fn(text) })
}
