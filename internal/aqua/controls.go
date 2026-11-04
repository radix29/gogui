package aqua

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// The placeholder peers (PLAN.md S2.2e). Each embeds its driver/stub peer,
// which keeps the state, the getters, the User… simulations and the fake
// FittingSize (§11.1 rule 2), so the example layouts are the headless ones,
// and a peerBase, which owns the drawn placeholder element. Every setter
// that changes what the placeholder shows updates the stub and repaints.
// S3.3 onward replace them control by control (the Label since S3.3c,
// label.go; the Separator and the Box since S3.3d, group.go; the Button
// since S3.3e, button.go; the Checkbox since S3.3f, checkbox.go; the Radio
// since S3.3g, radio.go; the Slider since S3.4c, slider.go; the Progress
// since S3.4e, progress.go; the ScrollView since S3.5d, scrollview.go). The focusable
// placeholder PopUpButton has FocusKind FocusControl: Tab reaches it only
// with full keyboard access (the aqua default, Router.SetFullKeyboardAccess).

func fmtNumber(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

// TextField is the placeholder driver.TextField. It is focusable, with
// FocusKind FocusText (as TextView). A secure
// field shows one bullet per rune and exposes no value.
type TextField struct {
	*stub.TextField
	peerBase
}

// NewTextField returns a placeholder text field.
func (d *Driver) NewTextField(secure bool, h driver.TextFieldHandler) driver.TextField {
	t := &TextField{TextField: stub.NewTextField(secure, h)}
	t.init(d, t.TextField, &t.TextField.Base, t.label, t.node)
	t.el.SetFocusable(true)
	t.el.SetFocusKind(FocusText) // Tab reaches it in both keyboard-access modes
	return t
}

func (t *TextField) shown() string {
	if t.Secure {
		return strings.Repeat("•", utf8.RuneCountInString(t.Value))
	}
	return t.Value
}

func (t *TextField) label() caption {
	kind := "TextField"
	if t.Secure {
		kind = "SecureField"
	}
	if s := t.shown(); s != "" {
		return labelled(kind, s)
	}
	return labelled(kind, t.Placeholder)
}

func (t *TextField) node() a11y.Node {
	n := a11y.Node{Role: a11y.RoleTextField, Label: t.Placeholder}
	if !t.Secure {
		n.Value = t.Value
	}
	if t.ReadOnly {
		n.States = a11y.StateReadOnly
	}
	return n
}

func (t *TextField) SetText(text string)        { t.TextField.SetText(text); t.redraw() }
func (t *TextField) SetPlaceholder(text string) { t.TextField.SetPlaceholder(text); t.redraw() }
func (t *TextField) SetEditable(editable bool)  { t.TextField.SetEditable(editable); t.redraw() }

// UserType simulates typing (stub.TextField.UserType) and repaints.
func (t *TextField) UserType(text string) { t.TextField.UserType(text); t.redraw() }

// PopUpButton is the placeholder driver.PopUpButton.
type PopUpButton struct {
	*stub.PopUpButton
	peerBase
}

// NewPopUpButton returns a placeholder pop-up button.
func (d *Driver) NewPopUpButton(h driver.PopUpHandler) driver.PopUpButton {
	p := &PopUpButton{PopUpButton: stub.NewPopUpButton(h)}
	p.init(d, p.PopUpButton, &p.PopUpButton.Base,
		func() caption { return labelled("PopUp", p.item()) },
		func() a11y.Node {
			return a11y.Node{Role: a11y.RolePopUpButton, Value: p.item(), Actions: a11y.ActionShowMenu}
		})
	p.el.SetFocusable(true) // FocusControl
	return p
}

// item returns the selected item, or "".
func (p *PopUpButton) item() string {
	if p.Index < 0 || p.Index >= len(p.Items) {
		return ""
	}
	return p.Items[p.Index]
}

func (p *PopUpButton) SetItems(items []string) { p.PopUpButton.SetItems(items); p.redraw() }
func (p *PopUpButton) SetSelected(index int)   { p.PopUpButton.SetSelected(index); p.redraw() }

// UserSelect simulates a choice (stub.PopUpButton.UserSelect) and repaints.
func (p *PopUpButton) UserSelect(index int) { p.PopUpButton.UserSelect(index); p.redraw() }

// TextView is the placeholder driver.TextView. It is focusable and shows
// the first line of its text.
type TextView struct {
	*stub.TextView
	peerBase
}

// NewTextView returns a placeholder text view.
func (d *Driver) NewTextView(h driver.TextViewHandler) driver.TextView {
	t := &TextView{TextView: stub.NewTextView(h)}
	t.init(d, t.TextView, &t.TextView.Base,
		func() caption {
			first, _, _ := strings.Cut(t.Value, "\n")
			return labelled("TextView", first)
		},
		func() a11y.Node {
			n := a11y.Node{Role: a11y.RoleTextArea, Value: t.Value}
			if t.ReadOnly {
				n.States = a11y.StateReadOnly
			}
			return n
		})
	t.el.SetFocusable(true)
	t.el.SetFocusKind(FocusText) // Tab reaches it in both keyboard-access modes
	return t
}

func (t *TextView) SetText(text string)       { t.TextView.SetText(text); t.redraw() }
func (t *TextView) SetEditable(editable bool) { t.TextView.SetEditable(editable); t.redraw() }

// UserType simulates typing (stub.TextView.UserType) and repaints.
func (t *TextView) UserType(text string) { t.TextView.UserType(text); t.redraw() }
