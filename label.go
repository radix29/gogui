package gogui

import "github.com/radix29/gogui/internal/driver"

// Label is a static text (NSTextField labelWithString:). It doesn't
// expand; with SetWraps it wraps at the width its stack or form gives it.
type Label struct {
	common[*Label]
	p    driver.Label
	text string
	font Font
}

// NewLabel returns a label that shows text in the system font.
func NewLabel(text string) *Label {
	p := driverFor("NewLabel").NewLabel()
	l := &Label{p: p, text: text}
	l.init(l, kindLabel, p, nil)
	p.SetText(text)
	return l
}

// SetText sets the text.
func (l *Label) SetText(text string) *Label {
	l.check("SetText")
	l.text = text
	if l.live() {
		l.p.SetText(text)
	}
	l.invalidate()
	return l
}

// Text returns the text.
func (l *Label) Text() string { l.get("Text"); return l.text }

// SetFont sets the font. The zero Font is the system font at the default
// size for the control size.
func (l *Label) SetFont(f Font) *Label {
	l.check("SetFont")
	l.font = f
	if l.live() {
		l.p.SetFont(f)
	}
	l.invalidate()
	return l
}

// Font returns the font set with SetFont.
func (l *Label) Font() Font { l.get("Font"); return l.font }

// SetTextColor sets the text color. The zero Color means ColorLabel.
func (l *Label) SetTextColor(c Color) *Label {
	l.check("SetTextColor")
	if l.live() {
		l.p.SetTextColor(c)
	}
	return l
}

// SetAlignment sets the horizontal alignment of the text in the label's
// frame. The default is AlignLeading. On Windows and Linux AlignJustified
// is drawn as AlignLeading.
func (l *Label) SetAlignment(a TextAlignment) *Label {
	l.check("SetAlignment")
	if l.live() {
		l.p.SetAlignment(a)
	}
	return l
}

// SetSelectable lets the user select and copy the text. On Windows and
// Linux the setting is recorded but not applied yet: the text can't be
// selected there.
func (l *Label) SetSelectable(selectable bool) *Label {
	l.check("SetSelectable")
	if l.live() {
		l.p.SetSelectable(selectable)
	}
	return l
}

// SetWraps makes the text wrap at the label's width; the label then gets
// taller instead of wider.
func (l *Label) SetWraps(wraps bool) *Label {
	l.check("SetWraps")
	if l.live() {
		l.p.SetWraps(wraps)
	}
	l.invalidate()
	return l
}
