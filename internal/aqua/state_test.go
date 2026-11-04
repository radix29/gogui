package aqua

import (
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
)

// stateTree builds the content a (focusable button) with the children b
// (hidden) and c (disabled), c with the child d, all logging into log.
func stateTree(log *[]string) (a, b, c, d *probe) {
	a = newProbe(log, "a", 10, 10, 100, 60)
	a.SetFocusable(true)
	a.node = a11y.Node{Role: a11y.RoleButton, Label: "A"}
	b = newProbe(log, "b", 0, 0, 10, 10)
	b.SetHidden(true)
	c = newProbe(log, "c", 20, 0, 10, 10)
	c.SetEnabled(false)
	d = newProbe(log, "d", 0, 0, 5, 5)
	a.AddChild(b)
	a.AddChild(c)
	c.AddChild(d)
	return a, b, c, d
}

// allDirty reports whether every element of the tree rooted at e is dirty.
func allDirty(e Element) bool {
	for _, x := range preOrder(e, nil) {
		if !x.base().Dirty() {
			return false
		}
	}
	return true
}

func TestWindowStateKey(t *testing.T) {
	s := startShell(t)
	w, sf, h := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	var log []string
	a, _, _, _ := stateTree(&log)
	w.SetContent(contentWidget{a})
	if !a.Focus() {
		t.Fatal("focus")
	}
	checkLog(t, &log, "a focus true")
	if a.FocusState() != FocusedActive || w.Router().FocusState(a) != FocusedActive || !w.Router().Active() {
		t.Fatalf("focus state %v", a.FocusState())
	}
	paint(t, s, sf)

	// The window handler hears about the change after the elements.
	a.onEvent = func(ev Event) {
		if _, ok := ev.(WindowStateEvent); ok && len(h.log) != 0 {
			t.Errorf("KeyChanged before the state event: %v", h.log)
		}
	}
	s.Deactivate()
	// Every element, pre-order, hidden and disabled ones included.
	checkLog(t, &log, "a state key=false dark=false", "b state key=false dark=false",
		"c state key=false dark=false", "d state key=false dark=false")
	if h.take() != "unkey" || !allDirty(w.Root()) || !sf.Invalid() {
		t.Fatal("deactivate: handler or repaint")
	}
	// The focus is kept, reported as inactive.
	if w.Router().Focused() != Element(a) || a.FocusState() != FocusedInactive || w.Router().Active() {
		t.Fatalf("inactive: focused %v, state %v", w.Router().Focused(), a.FocusState())
	}
	// The first two lines of the a11y dump: the window and the content.
	tree := func() string {
		lines := strings.Split(AccessibilityTree(w.Root()).Dump(), "\n")
		return lines[0] + "\n" + lines[1]
	}
	wantInactive := `window "W" @0,0 200x128` + "\n" + `  button "A" [focusable focused] {focus} @10,38 100x60`
	if got := tree(); got != wantInactive {
		t.Errorf("a11y inactive:\n%s\nwant:\n%s", got, wantInactive)
	}
	paint(t, s, sf)

	sf.Activate()
	checkLog(t, &log, "a state key=true dark=false", "b state key=true dark=false",
		"c state key=true dark=false", "d state key=true dark=false")
	if h.take() != "key" || !allDirty(w.Root()) || a.FocusState() != FocusedActive {
		t.Fatalf("activate: state %v", a.FocusState())
	}
	wantKey := `window "W" [focused] @0,0 200x128` + "\n" + `  button "A" [focusable focused] {focus} @10,38 100x60`
	if got := tree(); got != wantKey {
		t.Errorf("a11y key:\n%s\nwant:\n%s", got, wantKey)
	}
	// Key events reach the focus again; a repeated key state sends nothing.
	sf.Key(driver.KeyReturn, 0)
	checkLog(t, &log, "a key 13 true", "a key 13 false")
	surfaceHandler{w}.KeyChanged(true)
	if len(log) != 0 || h.take() != "" {
		t.Errorf("repeated key state: %v", log)
	}
	if err := sf.PaintErr(); err != nil {
		t.Error(err)
	}
}

func TestWindowStateAppearance(t *testing.T) {
	s := startShell(t)
	w, sf, h := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	var log []string
	a, _, _, _ := stateTree(&log)
	w.SetContent(contentWidget{a})
	paint(t, s, sf)

	dark := driver.DefaultAppearance()
	dark.Dark = true
	w.SetAppearance(dark)
	checkLog(t, &log, "a state key=true dark=true", "b state key=true dark=true",
		"c state key=true dark=true", "d state key=true dark=true")
	if !allDirty(w.Root()) || h.take() != "" || w.Appearance() != dark {
		t.Fatal("appearance change: repaint or handler")
	}
	paint(t, s, sf)
	w.SetAppearance(dark) // no change
	if len(log) != 0 || sf.Invalid() {
		t.Errorf("same appearance: %v", log)
	}
	contrast := dark
	contrast.IncreaseContrast = true // any field counts
	w.SetAppearance(contrast)
	if len(log) != 4 {
		t.Errorf("contrast change: %v", log)
	}
	log = nil
	w.Close()
	w.SetAppearance(driver.DefaultAppearance())
	if len(log) != 0 || w.Appearance() != contrast {
		t.Errorf("closed window: %v", log)
	}
}

// TestWindowContext: Base.Window from elements in and out of windows, and
// elements that join an inactive window reading its state.
func TestWindowContext(t *testing.T) {
	var log []string
	lone := newProbe(&log, "lone", 0, 0, 10, 10)
	if lone.Window() != nil || lone.FocusState() != Unfocused {
		t.Fatal("a detached element has a window")
	}
	root := newProbe(&log, "root", 0, 0, 100, 100)
	r := NewRouter(root)
	child := newProbe(&log, "child", 0, 0, 10, 10, "focus")
	child.SetFocusable(true)
	root.AddChild(child)
	if child.Window() != nil || !r.Active() {
		t.Fatal("a plain router's tree: window or inactive")
	}
	child.Focus()
	if child.FocusState() != FocusedActive || r.FocusState(lone) != Unfocused || r.FocusState(nil) != Unfocused {
		t.Fatal("plain router focus state")
	}
	r.SetActive(false)
	if child.FocusState() != FocusedInactive || r.Focused() != Element(child) {
		t.Fatal("plain router inactive")
	}
	if Unfocused.String() != "unfocused" || FocusedInactive.String() != "focused-inactive" ||
		FocusedActive.String() != "focused" || FocusState(9).String() != "focusstate(?)" {
		t.Error("FocusState names")
	}

	// A window before Show is not key; content focused then is inactive.
	s := startShell(t)
	w, err := NewWindow(s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles}, &winLog{})
	if err != nil {
		t.Fatal(err)
	}
	log = nil
	a, _, c, d := stateTree(&log)
	w.SetContent(contentWidget{a})
	a.Focus()
	checkLog(t, &log, "a focus true")
	for _, e := range []*probe{a, c, d} {
		x := e.Window()
		if x != w || x.IsKey() || x.Appearance() != s.Appearance() || x.Theme() != tokens ||
			x.Timeline() != nil || x.Router() != w.Router() {
			t.Fatalf("%s: window context %v", e.name, x)
		}
	}
	if w.Root().base().Window() != w || a.FocusState() != FocusedInactive {
		t.Fatalf("before Show: focus state %v", a.FocusState())
	}
	w.Show()
	checkLog(t, &log, "a state key=true dark=false", "b state key=true dark=false",
		"c state key=true dark=false", "d state key=true dark=false")
	if a.FocusState() != FocusedActive {
		t.Fatal("after Show")
	}

	// An element added to an inactive, dark window gets no event but reads
	// the state.
	s.Deactivate()
	dark := s.Appearance()
	dark.Dark = true
	w.SetAppearance(dark)
	log = nil
	e := newProbe(&log, "e", 0, 0, 5, 5)
	e.SetFocusable(true)
	d.AddChild(e)
	if len(log) != 0 {
		t.Fatalf("joining sent %v", log)
	}
	if x := e.Window(); x != w || x.IsKey() || !x.Appearance().Dark {
		t.Fatal("the new element reads the wrong state")
	}
	// Removed again, it has no window.
	e.RemoveFromParent()
	if e.Window() != nil {
		t.Fatal("a removed element keeps its window")
	}
}

// TestWindowStateReentrancy: handlers that move elements out of the window
// or close it during the state event.
func TestWindowStateReentrancy(t *testing.T) {
	s := startShell(t)
	w, sf, h := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	var log []string
	a, b, c, _ := stateTree(&log)
	w.SetContent(contentWidget{a})

	// b removes c (and with it d): they get nothing.
	b.onEvent = func(ev Event) {
		if _, ok := ev.(WindowStateEvent); ok {
			c.RemoveFromParent()
		}
	}
	s.Deactivate()
	checkLog(t, &log, "a state key=false dark=false", "b state key=false dark=false")
	if h.take() != "unkey" {
		t.Fatal("handler")
	}
	b.onEvent = nil

	// a closes the window: b gets nothing, and KeyChanged is not called.
	a.onEvent = func(ev Event) {
		if _, ok := ev.(WindowStateEvent); ok {
			w.Close()
		}
	}
	sf.Activate()
	checkLog(t, &log, "a state key=true dark=false")
	if got := h.take(); got != "closed" || !w.Closed() || w.IsKey() || w.Router().Active() {
		t.Fatalf("close during the state event: %q", got)
	}
	// A closed window still answers with its last state.
	if a.Window() != w || a.Window().Theme() != tokens {
		t.Fatal("closed window context")
	}
}
