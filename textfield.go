package gogui

import "github.com/radix29/gogui/internal/driver"

// TextField is a single-line editable text field (NSTextField). With
// NewSecureField it hides its text (NSSecureTextField). It expands
// horizontally.
//
// On Windows and Linux the field edits as an NSTextField does (word and
// line movement with Option and Primary, double- and triple-click
// selection, undo), with Ctrl as Primary; plain Home and End move the
// caret to the line's ends. Input methods compose inline (IMM32 on
// Windows; IBus or Fcitx through GTK on Linux), except in a secure field.
type TextField struct {
	common[*TextField]
	p        driver.TextField
	secure   bool
	text     string
	place    string // the placeholder
	font     Font
	onChange subscribers[func(string)]
	onSubmit subscribers[func(string)]
}

// NewTextField returns an empty, editable text field.
func NewTextField() *TextField { return newTextField("NewTextField", false) }

// NewSecureField returns an empty password field: it shows bullets in
// place of its text and doesn't allow copying it. It is a *TextField, so
// it has the same methods; UI-thread check messages name it "TextField".
func NewSecureField() *TextField { return newTextField("NewSecureField", true) }

func newTextField(what string, secure bool) *TextField {
	t := &TextField{secure: secure}
	p := driverFor(what).NewTextField(secure, textFieldEvents{t})
	t.p = p
	t.init(t, kindTextField, p, nil)
	return t
}

// Secure reports whether the field was created by NewSecureField.
func (t *TextField) Secure() bool { t.get("Secure"); return t.secure }

// SetText replaces the text. It doesn't call the OnChange handlers.
func (t *TextField) SetText(text string) *TextField {
	t.check("SetText")
	t.text = text
	if t.live() {
		t.p.SetText(text)
	}
	t.invalidate()
	return t
}

// Text returns the text, including the user's edits.
func (t *TextField) Text() string { t.get("Text"); return t.text }

// SetPlaceholder sets the gray text shown while the field is empty.
func (t *TextField) SetPlaceholder(text string) *TextField {
	t.check("SetPlaceholder")
	t.place = text
	if t.live() {
		t.p.SetPlaceholder(text)
	}
	t.invalidate()
	return t
}

// Placeholder returns the text set with SetPlaceholder.
func (t *TextField) Placeholder() string { t.get("Placeholder"); return t.place }

// SetFont sets the font. The zero Font is the system font at the default
// size for the control size.
func (t *TextField) SetFont(f Font) *TextField {
	t.check("SetFont")
	t.font = f
	if t.live() {
		t.p.SetFont(f)
	}
	t.invalidate()
	return t
}

// Font returns the font set with SetFont.
func (t *TextField) Font() Font { t.get("Font"); return t.font }

// SetAlignment sets the horizontal alignment of the text. The default is
// AlignLeading.
func (t *TextField) SetAlignment(a TextAlignment) *TextField {
	t.check("SetAlignment")
	if t.live() {
		t.p.SetAlignment(a)
	}
	return t
}

// SetEditable controls whether the user can change the text (the default
// is true). A read-only field can still be focused and its text selected.
func (t *TextField) SetEditable(editable bool) *TextField {
	t.check("SetEditable")
	if t.live() {
		t.p.SetEditable(editable)
	}
	return t
}

// OnChange calls fn with the new text after every edit by the user,
// including IME commits, paste and undo. SetText doesn't call it, and
// neither does a disabled field.
func (t *TextField) OnChange(fn func(text string)) Unsubscribe {
	t.check("OnChange")
	return t.onChange.add(fn)
}

// OnSubmit calls fn with the text when the user presses Return in the
// field. A disabled field doesn't call it.
func (t *TextField) OnSubmit(fn func(text string)) Unsubscribe {
	t.check("OnSubmit")
	return t.onSubmit.add(fn)
}

// textFieldEvents implements driver.TextFieldHandler for a TextField.
type textFieldEvents struct{ t *TextField }

func (e textFieldEvents) TextChanged(text string) {
	t := e.t
	if t.dead {
		return
	}
	t.text = text // the cache follows the native text even when disabled (B14)
	if t.disabled {
		return
	}
	t.onChange.eachLive(&t.widget, func(fn func(string)) { fn(text) })
}

func (e textFieldEvents) Submitted(text string) {
	t := e.t
	if t.dead {
		return
	}
	t.text = text
	if t.disabled {
		return
	}
	t.onSubmit.eachLive(&t.widget, func(fn func(string)) { fn(text) })
}
