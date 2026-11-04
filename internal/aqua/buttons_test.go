package aqua

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// pressable is a probe whose Activate logs "name press".
type pressable struct {
	*probe
	onPress func()
}

func (p *pressable) Activate() bool {
	*p.log = append(*p.log, p.name+" press")
	if p.onPress != nil {
		p.onPress()
	}
	return true
}

func newPressable(log *[]string, name string, y float64) *pressable {
	p := &pressable{probe: &probe{name: name, log: log, handles: map[string]bool{}}}
	p.Init(p)
	p.SetFrame(driver.Rect{Y: y, W: 50, H: 20})
	p.SetFocusable(true)
	return p
}

// TestDefaultCancelRouting covers S3.2f in a window on the fake shell:
// Return, Escape and Ctrl+. reaching the marked buttons, the focused element
// first, repeats, modifiers, hidden and disabled buttons, replacing and
// removing them.
func TestDefaultCancelRouting(t *testing.T) {
	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	var log []string
	box := newProbe(&log, "box", 0, 0, 200, 72)
	field := newProbe(&log, "field", 0, 0, 50, 20)
	field.SetFocusable(true)
	ok := newPressable(&log, "ok", 25)
	cancel := newPressable(&log, "cancel", 50)
	box.SetChildren(field, ok, cancel)
	// Marked before they join the window.
	ok.SetDefaultButton(true)
	cancel.SetCancelButton(true)
	if !ok.IsDefaultButton() || ok.IsCancelButton() || !cancel.IsCancelButton() {
		t.Fatal("marks")
	}
	if w.DefaultButton() != nil {
		t.Fatal("default before SetContent")
	}
	w.SetContent(contentWidget{box})
	if w.DefaultButton() != Element(ok) || w.CancelButton() != Element(cancel) {
		t.Fatal("DefaultButton/CancelButton")
	}
	field.Focus()
	log = nil

	// Unhandled keys reach the buttons after the focused element and its
	// ancestors; the key-up is not a press.
	if !sf.Key(driver.KeyReturn, 0) {
		t.Fatal("Return not consumed")
	}
	checkLog(t, &log, "field key 13 true", "box key 13 true", "ok press", "field key 13 false", "box key 13 false")
	if !sf.Key(driver.KeyEscape, 0) || !sf.Key('.', driver.ModControl) {
		t.Fatal("Escape or Ctrl+. not consumed")
	}
	checkLog(t, &log, "field key 27 true", "box key 27 true", "cancel press", "field key 27 false", "box key 27 false",
		"field key 46 true", "box key 46 true", "cancel press", "field key 46 false", "box key 46 false")

	// A focused element that handles Return gets it alone.
	field.handles["key"] = true
	sf.Key(driver.KeyReturn, 0)
	checkLog(t, &log, "field key 13 true", "field key 13 false")
	field.handles["key"] = false
	field.SetFocusable(false) // no focus: keys go to the window root, which handles nothing
	log = nil

	// Modifiers, other keys and repeats press nothing; a repeat is still
	// consumed. ⌘. (the physical Command key) is not Primary off macOS.
	send := func(ev shell.KeyEvent) bool { return sf.Send(ev) }
	for _, ev := range []shell.KeyEvent{
		{Down: true, Key: driver.KeyReturn, Mods: driver.ModShift},
		{Down: true, Key: driver.KeyReturn, Mods: driver.ModControl},
		{Down: true, Key: driver.KeyEscape, Mods: driver.ModOption},
		{Down: true, Key: '.', Mods: driver.ModCommand},
		{Down: true, Key: '.'},
		{Down: true, Key: 'A'},
		{Down: false, Key: driver.KeyReturn},
	} {
		if send(ev) {
			t.Errorf("%+v consumed", ev)
		}
	}
	if !send(shell.KeyEvent{Down: true, Repeat: true, Key: driver.KeyReturn}) {
		t.Error("repeat not consumed")
	}
	checkLog(t, &log)

	// Hidden and disabled buttons (or ancestors) are not pressed, the key
	// stays unhandled, and nothing falls back to another button.
	for _, c := range []struct {
		what    string
		off, on func()
		key     driver.Key
	}{
		{"hidden default", func() { ok.SetHidden(true) }, func() { ok.SetHidden(false) }, driver.KeyReturn},
		{"disabled default", func() { ok.SetEnabled(false) }, func() { ok.SetEnabled(true) }, driver.KeyReturn},
		{"disabled ancestor", func() { box.SetEnabled(false) }, func() { box.SetEnabled(true) }, driver.KeyEscape},
		{"hidden cancel", func() { cancel.SetHidden(true) }, func() { cancel.SetHidden(false) }, driver.KeyEscape},
	} {
		c.off()
		if sf.Key(c.key, 0) {
			t.Errorf("%s: key consumed", c.what)
		}
		checkLog(t, &log)
		c.on()
	}

	// A second mark replaces the first; unmarking or removing it brings the
	// first back; one element can be both.
	other := newPressable(&log, "other", 0)
	box.AddChild(other)
	other.SetDefaultButton(true)
	sf.Key(driver.KeyReturn, 0)
	checkLog(t, &log, "other press")
	ok.SetDefaultButton(true) // marking again makes it the latest
	sf.Key(driver.KeyReturn, 0)
	checkLog(t, &log, "ok press")
	ok.SetDefaultButton(false)
	sf.Key(driver.KeyReturn, 0)
	checkLog(t, &log, "other press")
	other.RemoveFromParent()
	if w.DefaultButton() != nil || sf.Key(driver.KeyReturn, 0) {
		t.Error("removed default still pressed")
	}
	ok.SetCancelButton(true)
	sf.Key(driver.KeyEscape, 0)
	checkLog(t, &log, "ok press")
	ok.SetCancelButton(false)

	// A button whose Activate does nothing leaves the key unhandled.
	dud := newProbe(&log, "dud", 0, 0, 1, 1)
	dud.SetDefaultButton(true)
	box.AddChild(dud)
	if sf.Key(driver.KeyReturn, 0) {
		t.Error("Activate false consumed the key")
	}
	dud.SetDefaultButton(false)

	// A cancel that closes the window: the rest is skipped, later keys do
	// nothing.
	closer := newPressable(&log, "closer", 0)
	closer.onPress = w.Close
	closer.SetCancelButton(true)
	box.AddChild(closer)
	if !sf.Key(driver.KeyEscape, 0) || !w.Closed() {
		t.Fatal("closing cancel")
	}
	checkLog(t, &log, "closer press")
	if w.keyEquivalent(shell.KeyEvent{Down: true, Key: driver.KeyReturn}) {
		t.Error("closed window pressed")
	}
}

// TestButtonKeys: the Button's SetDefault and SetCancel mark its
// element, and Activate is a user click, refused while disabled.
func TestButtonKeys(t *testing.T) {
	d := startDriver(t)
	var clicks int
	b := d.NewButton(clickCounter{&clicks}).(*Button)
	b.SetDefault(true)
	b.SetCancel(true)
	if !b.IsDefault || !b.IsCancel || !b.view.IsDefaultButton() || !b.view.IsCancelButton() {
		t.Fatal("SetDefault/SetCancel")
	}
	b.SetDefault(false)
	b.SetCancel(false)
	if b.IsDefault || b.IsCancel || b.view.IsDefaultButton() || b.view.IsCancelButton() {
		t.Fatal("unset")
	}
	if !b.view.Activate() || clicks != 1 {
		t.Fatal("Activate")
	}
	b.SetEnabled(false)
	if b.view.Activate() || clicks != 1 {
		t.Fatal("Activate while disabled")
	}
	// Other placeholders and Base have no primary action.
	if ElementOf(d.NewPopUpButton(nil)).Activate() || (&Base{}).Activate() {
		t.Fatal("Activate without an action")
	}
}

type clickCounter struct{ n *int }

func (c clickCounter) Clicked() { *c.n++ }
