package aqua

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// clicker is a probe that tracks its presses with a pressTracker and logs
// "name act" on a release over it.
type clicker struct {
	*probe
	press pressTracker
}

func newClicker(log *[]string, name string, x, y, w, h float64) *clicker {
	c := &clicker{probe: &probe{name: name, log: log, handles: map[string]bool{}}}
	c.Init(c)
	c.SetFrame(driver.Rect{X: x, Y: y, W: w, H: h})
	return c
}

func (c *clicker) HandleEvent(ev Event) bool {
	handled, act := c.press.Track(c, ev)
	if act {
		*c.log = append(*c.log, c.name+" act")
	}
	return handled
}

func (c *clicker) state() string { return c.press.State(c).String() }

// clickTree: root 0,0 200x100 holding the clicker btn at 10,10 40x20.
func clickTree(t *testing.T) (*Router, *probe, *clicker, *[]string) {
	t.Helper()
	log := &[]string{}
	root := newProbe(&[]string{}, "root", 0, 0, 200, 100) // its events are not checked
	btn := newClicker(log, "btn", 10, 10, 40, 20)
	root.AddChild(btn)
	return NewRouter(root), root, btn, log
}

// markClean paints nothing but clears the dirty marks, so Dirty shows the next
// invalidation.
func markClean(e Element) {
	for _, x := range preOrder(e, nil) {
		x.base().dirty = false
	}
}

func wantState(t *testing.T, c *clicker, want string) {
	t.Helper()
	if got := c.state(); got != want {
		t.Fatalf("state %q, want %q", got, want)
	}
}

func TestPressTracker(t *testing.T) {
	r, root, btn, log := clickTree(t)
	wantState(t, btn, "normal unfocused key light")

	r.Event(move(20, 20))
	wantState(t, btn, "hover unfocused key light")

	// Press: captured and pressed, repainted.
	markClean(root)
	if !r.Event(down(20, 20)) || r.Captured() != Element(btn) || !btn.press.Down() || !btn.press.Pressed() {
		t.Fatal("press not captured")
	}
	if !btn.Dirty() {
		t.Error("press did not invalidate")
	}
	wantState(t, btn, "pressed unfocused key light")

	// Drag off: un-pressed, normal (not hover: the frozen chain still holds it).
	markClean(root)
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 100, Y: 80}, Buttons: shell.ButtonPrimary})
	if btn.press.Pressed() || !btn.press.Down() || !btn.Dirty() {
		t.Fatal("drag off")
	}
	wantState(t, btn, "normal unfocused key light")
	// Moving further off changes nothing.
	markClean(root)
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 110, Y: 80}, Buttons: shell.ButtonPrimary})
	if btn.Dirty() {
		t.Error("move while off invalidated")
	}

	// Drag back: pressed again.
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 30, Y: 25}, Buttons: shell.ButtonPrimary})
	wantState(t, btn, "pressed unfocused key light")

	// Release over it: acts once, hover again.
	if !r.Event(up(30, 25)) {
		t.Fatal("release not handled")
	}
	checkLog(t, log, "btn act")
	if r.Captured() != nil || btn.press.Down() {
		t.Fatal("capture kept")
	}
	wantState(t, btn, "hover unfocused key light")

	// Release outside: nothing.
	r.Event(down(20, 20))
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 150, Y: 50}, Buttons: shell.ButtonPrimary})
	r.Event(up(150, 50))
	checkLog(t, log)
	wantState(t, btn, "normal unfocused key light")
	if r.Hovered() != Element(root) {
		t.Fatal("hover after a release outside")
	}
}

func TestPressTrackerOtherButtons(t *testing.T) {
	r, _, btn, log := clickTree(t)
	sec := func(kind shell.PointerKind, b shell.PointerButton) shell.PointerEvent {
		return shell.PointerEvent{Kind: kind, Pos: driver.Point{X: 20, Y: 20}, Button: shell.ButtonSecondary, Buttons: b}
	}
	// A secondary press alone is not handled: no capture, no press.
	if r.Event(sec(shell.PointerDown, shell.ButtonSecondary)) || r.Captured() != nil || btn.press.Down() {
		t.Fatal("secondary press taken")
	}
	r.Event(sec(shell.PointerUp, 0))
	// During a primary press other buttons are swallowed and do not end it.
	r.Event(down(20, 20))
	if !r.Event(sec(shell.PointerDown, shell.ButtonPrimary|shell.ButtonSecondary)) ||
		!r.Event(sec(shell.PointerUp, shell.ButtonPrimary)) || !btn.press.Pressed() {
		t.Fatal("second button during a press")
	}
	checkLog(t, log)
	r.Event(up(20, 20))
	checkLog(t, log, "btn act")
	// Non-pointer events and a stray release are not handled.
	if h, act := btn.press.Track(btn, KeyEvent{}); h || act {
		t.Fatal("key event tracked")
	}
	if h, act := btn.press.Track(btn, PointerEvent{Kind: PointerUp, Button: shell.ButtonPrimary, Over: true}); h || act {
		t.Fatal("release without a press acted")
	}
	if h, _ := btn.press.Track(btn, PointerEvent{Kind: PointerMove, Over: true}); h {
		t.Fatal("idle move handled")
	}
}

// TestPressTrackerDisabled: disabling during a press cancels it without
// acting; a disabled control gets no press and is never hovered or
// pressed.
func TestPressTrackerDisabled(t *testing.T) {
	r, root, btn, log := clickTree(t)
	r.Event(down(20, 20))
	btn.SetEnabled(false)
	if btn.press.Down() || btn.press.Pressed() || r.Captured() != nil {
		t.Fatal("disable kept the press")
	}
	wantState(t, btn, "normal unfocused disabled key light")
	r.Event(up(20, 20))
	checkLog(t, log)

	r.Event(move(21, 21))
	if r.Event(down(20, 20)) || btn.press.Down() {
		t.Fatal("disabled control pressed")
	}
	r.Event(up(20, 20))
	wantState(t, btn, "normal unfocused disabled key light")
	checkLog(t, log)

	// Disabled through an ancestor: the same.
	btn.SetEnabled(true)
	root.SetEnabled(false)
	wantState(t, btn, "normal unfocused disabled key light")
	root.SetEnabled(true)
	r.RefreshHover() // no pointer event since: the hover is recomputed on demand
	r.Event(move(20, 20))
	wantState(t, btn, "hover unfocused key light")

	// A cancel by the router (the pointer left the window) does not act.
	r.Event(down(20, 20))
	r.Event(shell.PointerEvent{Kind: shell.PointerLeave})
	if btn.press.Down() {
		t.Fatal("leave kept the press")
	}
	checkLog(t, log)
	wantState(t, btn, "normal unfocused key light")
}

// TestControlStateFocusAndKey: the focus and key parts, through a bare
// router and through a window (key, inactive, appearance).
func TestControlStateFocusAndKey(t *testing.T) {
	r, _, btn, _ := clickTree(t)
	btn.SetFocusable(true)
	btn.Focus()
	wantState(t, btn, "normal focused key light")
	r.SetActive(false)
	wantState(t, btn, "normal focused-inactive inactive light")
	r.SetActive(true)

	// Detached: no router, no window.
	lone := newClicker(&[]string{}, "lone", 0, 0, 10, 10)
	wantState(t, lone, "normal unfocused key light")
	if lone.ControlState() != lone.press.State(lone) {
		t.Fatal("ControlState without a press")
	}

	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	var log []string
	box := newProbe(&[]string{}, "box", 0, 0, 200, 72)
	c := newClicker(&log, "c", 10, 10, 40, 20)
	c.SetFocusable(true)
	box.AddChild(c)
	w.SetContent(contentWidget{box})
	c.Focus()
	wantState(t, c, "normal focused key light")
	at := ToRoot(c, driver.Point{X: 5, Y: 5})
	sf.Move(at)
	wantState(t, c, "hover focused key light")
	s.Deactivate()
	wantState(t, c, "hover focused-inactive inactive light")
	dark := driver.DefaultAppearance()
	dark.Dark = true
	w.SetAppearance(dark)
	if got := c.ControlState(); got.Appearance != dark {
		t.Fatalf("appearance %+v", got.Appearance)
	}
	wantState(t, c, "hover focused-inactive inactive dark")
	sf.Activate()
	sf.Press(at, shell.ButtonPrimary)
	wantState(t, c, "pressed focused key dark")
	sf.Release(at, shell.ButtonPrimary)
	checkLog(t, &log, "c act")
	if err := sf.PaintErr(); err != nil {
		t.Error(err)
	}
}

func TestInteractionAndControlStateString(t *testing.T) {
	if InteractionNormal.String() != "normal" || InteractionHover.String() != "hover" ||
		InteractionPressed.String() != "pressed" || Interaction(7).String() != "interaction(?)" {
		t.Fatal("Interaction.String")
	}
	s := ControlState{Interaction: InteractionPressed, Focus: FocusedActive, Disabled: true, Appearance: driver.Appearance{Dark: true}}
	if got := s.String(); got != "pressed focused disabled inactive dark" {
		t.Fatalf("ControlState.String %q", got)
	}
}

// TestSpaceActivates: Space presses a focused FocusControl element; its
// repeat and Space with modifiers do not; a text element keeps Space; an
// element that handles Space gets it alone; nothing happens without a
// focus, in an inactive window or for a disabled element.
func TestSpaceActivates(t *testing.T) {
	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	var log []string
	box := newProbe(&log, "box", 0, 0, 200, 72)
	btn := newPressable(&log, "btn", 0)
	field := newProbe(&log, "field", 0, 25, 50, 20)
	field.SetFocusable(true)
	field.SetFocusKind(FocusText)
	box.SetChildren(btn, field)
	w.SetContent(contentWidget{box})

	// No focus: Space reaches the root and is not consumed.
	if sf.KeyDown(driver.KeySpace, 0) {
		t.Fatal("Space without a focus consumed")
	}
	sf.KeyUp(driver.KeySpace, 0)
	log = nil

	btn.Focus()
	log = nil
	if !sf.KeyDown(driver.KeySpace, 0) {
		t.Fatal("Space not consumed")
	}
	checkLog(t, &log, "btn key 32 true", "box key 32 true", "btn press")
	// The repeat is consumed but presses nothing; the key-up is no press.
	if !sf.KeyRepeat(driver.KeySpace, 0) {
		t.Fatal("repeat not consumed")
	}
	sf.KeyUp(driver.KeySpace, 0)
	checkLog(t, &log, "btn key 32 true", "box key 32 true", "btn key 32 false", "box key 32 false")
	// Typed Space: consumed, so no text follows.
	sf.Type(" ")
	checkLog(t, &log, "btn key 32 true", "box key 32 true", "btn press", "btn key 32 false", "box key 32 false")

	// Modifiers: no press.
	for _, m := range []driver.Modifiers{driver.ModShift, driver.ModControl, driver.ModOption, driver.ModCommand} {
		if sf.Key(driver.KeySpace, m) {
			t.Fatalf("Space with %v consumed", m)
		}
	}
	for _, l := range log {
		if l == "btn press" {
			t.Fatal("Space with modifiers pressed")
		}
	}
	log = nil

	// The focused element handling Space gets it alone.
	btn.handles["key"] = true
	sf.Key(driver.KeySpace, 0)
	checkLog(t, &log, "btn key 32 true", "btn key 32 false")
	btn.handles["key"] = false

	// A text element keeps Space: it is typed, not a press.
	field.Focus()
	log = nil
	sf.Type(" ")
	checkLog(t, &log, "field key 32 true", "box key 32 true", `field text " "`, `box text " "`,
		"field key 32 false", "box key 32 false")

	// Inactive window: no press.
	btn.Focus()
	r := w.Router()
	r.SetActive(false)
	log = nil
	if r.Event(shell.KeyEvent{Down: true, Key: driver.KeySpace}) {
		t.Fatal("Space in an inactive window consumed")
	}
	checkLog(t, &log, "btn key 32 true", "box key 32 true")
	r.SetActive(true)

	// A focused element disabled through an ancestor loses the focus, so
	// Space reaches the root; check the guard directly as well.
	r.focus = btn
	box.disabled = true
	if r.spaceActivates(shell.KeyEvent{Down: true, Key: driver.KeySpace}) {
		t.Fatal("disabled element pressed")
	}
	box.disabled = false
	if err := sf.PaintErr(); err != nil {
		t.Error(err)
	}
}

// TestSpaceOnButton: Space presses a focused Button like a click (its
// Activate), toggles a focused Checkbox (S3.3f) and turns on a focused
// Radio (S3.3g), and is not consumed on a focused placeholder without a
// primary action yet.
func TestSpaceOnButton(t *testing.T) {
	d := startDriver(t)
	var clicks int
	b := d.NewButton(clickCounter{&clicks}).(*Button)
	cb := d.NewCheckbox(nil).(*Checkbox)
	rb := d.NewRadio(nil).(*Radio)
	pop := d.NewPopUpButton(nil).(*PopUpButton)
	root := newProbe(&[]string{}, "root", 0, 0, 100, 100)
	root.SetChildren(ElementOf(b), ElementOf(cb), ElementOf(rb), ElementOf(pop))
	r := NewRouter(root)
	space := shell.KeyEvent{Down: true, Key: driver.KeySpace}
	if !b.view.Focus() || !r.Event(space) || clicks != 1 {
		t.Fatalf("Space on a Button: %d clicks", clicks)
	}
	if !cb.view.Focus() || !r.Event(space) || cb.Value != driver.StateOn {
		t.Fatal("Space on a Checkbox")
	}
	if !rb.view.Focus() || !r.Event(space) || !rb.IsOn {
		t.Fatal("Space on a Radio")
	}
	if !pop.el.Focus() || r.Event(space) {
		t.Fatal("Space consumed by a placeholder without an action")
	}
}
