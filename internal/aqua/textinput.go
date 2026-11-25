package aqua

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// TextInputClient is an element that takes text from the IME while it has
// the focus (PLAN.md S5.3d): the TextField, and the TextView since S5.4j.
// The window asks the focused element for it and publishes the result on
// its surface (Surface.SetTextInput); any other focus turns the IME off.
type TextInputClient interface {
	Element

	// TextInput reports whether the element takes IME input now (a
	// SecureField, a read-only or a disabled field do not) and the caret
	// rect in the element's own coordinates, as last painted: after its
	// scroll, at the preedit's cursor while composing. The window adds the
	// focus and the key state itself.
	TextInput() (enabled bool, caret driver.Rect)
}

// textInputState returns the text input the window's surface should have:
// enabled with the caret in surface points while the window is key and
// open and its focus is a TextInputClient that takes input, else the zero
// TextInput (off).
func (w *Window) textInputState() shell.TextInput {
	f := w.router.Focused()
	c, ok := f.(TextInputClient)
	if w.closed || !w.key || !w.router.Active() || !ok { // not while the menu strip has the keys (S5.6i)

		return shell.TextInput{}
	}
	on, caret := c.TextInput()
	if !on {
		return shell.TextInput{}
	}
	// The root covers the surface at its origin, so root coordinates are
	// surface points.
	return shell.TextInput{Enabled: true, Caret: caret.Offset(ToRoot(f, driver.Point{}))}
}

// publishTextInput sets the surface's text input to textInputState when
// it changed since the last call (the surface starts off). The window
// calls it after each paint, after a focus change and after a key change.
func (w *Window) publishTextInput() {
	if w.closed {
		return
	}
	ti := w.textInputState()
	if ti == w.textInput {
		return
	}
	w.textInput = ti
	w.surface.SetTextInput(ti)
}

// TextInput returns the text input the window published last on its
// surface.
func (w *Window) TextInput() shell.TextInput { return w.textInput }
