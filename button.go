package gogui

import "github.com/radix29/gogui/internal/driver"

// Button is a push button (NSButton with the push bezel). It doesn't
// expand.
type Button struct {
	common[*Button]
	p       driver.Button
	title   string
	onClick subscribers[func()]
}

// NewButton returns a push button with title.
func NewButton(title string) *Button {
	b := &Button{title: title}
	p := driverFor("NewButton").NewButton(buttonEvents{b})
	b.p = p
	b.init(b, kindButton, p, nil)
	p.SetTitle(title)
	return b
}

// SetTitle sets the title.
func (b *Button) SetTitle(title string) *Button {
	b.check("SetTitle")
	b.title = title
	if b.live() {
		b.p.SetTitle(title)
	}
	b.invalidate()
	return b
}

// Title returns the title.
func (b *Button) Title() string { b.get("Title"); return b.title }

// SetDefault makes this the window's default button: it is drawn with the
// accent color and Return triggers it.
func (b *Button) SetDefault(isDefault bool) *Button {
	b.check("SetDefault")
	if b.live() {
		b.p.SetDefault(isDefault)
	}
	return b
}

// SetCancel makes this the window's cancel button: Escape triggers it.
func (b *Button) SetCancel(isCancel bool) *Button {
	b.check("SetCancel")
	if b.live() {
		b.p.SetCancel(isCancel)
	}
	return b
}

// OnClick calls fn when the user activates the button: a click, Space
// while it is focused, Return for the default button or Escape for the
// cancel button. A disabled button doesn't call it.
func (b *Button) OnClick(fn func()) Unsubscribe {
	b.check("OnClick")
	return b.onClick.add(fn)
}

// buttonEvents implements driver.ButtonHandler for a Button.
type buttonEvents struct{ b *Button }

func (e buttonEvents) Clicked() {
	if e.b.disabled || e.b.dead {
		return
	}
	e.b.onClick.eachLive(&e.b.widget, func(fn func()) { fn() })
}

// Separator is a hairline (NSBox with the separator type). A horizontal
// separator (the default) fills the width of its stack, a vertical one the
// height.
type Separator struct {
	common[*Separator]
	p driver.Separator
}

// NewSeparator returns a horizontal separator.
func NewSeparator() *Separator {
	p := driverFor("NewSeparator").NewSeparator()
	s := &Separator{p: p}
	s.init(s, kindSeparator, p, nil)
	return s
}

// SetVertical switches between a horizontal (false, the default) and a
// vertical line. The separator then expands along its new length.
func (s *Separator) SetVertical(vertical bool) *Separator {
	s.check("SetVertical")
	s.vertical = vertical
	s.applyExpand()
	if s.live() {
		s.p.SetVertical(vertical)
	}
	s.invalidate()
	return s
}
