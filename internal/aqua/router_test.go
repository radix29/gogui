package aqua

import (
	"fmt"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// probe is a test element that logs the events it gets into a shared log
// and handles the kinds it is told to.
type probe struct {
	Base
	name    string
	log     *[]string
	handles map[string]bool // event names it handles: "down", "move", "scroll", "key", "text", …
	onEvent func(Event)
	node    a11y.Node
}

func newProbe(log *[]string, name string, x, y, w, h float64, handles ...string) *probe {
	p := &probe{name: name, log: log, handles: map[string]bool{}}
	p.Init(p)
	p.SetFrame(driver.Rect{X: x, Y: y, W: w, H: h})
	for _, k := range handles {
		p.handles[k] = true
	}
	return p
}

func (p *probe) HandleEvent(ev Event) bool {
	var kind, entry string
	switch ev := ev.(type) {
	case PointerEvent:
		kind = ev.Kind.String()
		entry = fmt.Sprintf("%s %s %g,%g", p.name, kind, ev.Pos.X, ev.Pos.Y)
		if ev.Kind == PointerMove || ev.Kind == PointerUp || ev.Kind == PointerDown {
			if !ev.Over {
				entry += " off"
			}
			if ev.Clicks > 0 {
				entry += fmt.Sprintf(" x%d", ev.Clicks)
			}
		}
	case ScrollEvent:
		kind = "scroll"
		entry = fmt.Sprintf("%s scroll %g,%g", p.name, ev.Pos.X, ev.Pos.Y)
	case KeyEvent:
		kind = "key"
		entry = fmt.Sprintf("%s key %d %v", p.name, ev.Key, ev.Down)
	case TextEvent:
		kind = "text"
		entry = fmt.Sprintf("%s text %q", p.name, ev.Text)
	case FocusEvent:
		kind = "focus"
		entry = fmt.Sprintf("%s focus %v", p.name, ev.Focused)
	case WindowStateEvent:
		kind = "state"
		entry = fmt.Sprintf("%s state key=%v dark=%v", p.name, ev.Key, ev.Appearance.Dark)
	}
	*p.log = append(*p.log, entry)
	if p.onEvent != nil {
		p.onEvent(ev)
	}
	return p.handles[kind]
}

func (p *probe) AccessibleNode() a11y.Node { return p.node }

func checkLog(t *testing.T, log *[]string, want ...string) {
	t.Helper()
	got := strings.Join(*log, "\n")
	if w := strings.Join(want, "\n"); got != w {
		t.Fatalf("log:\n%s\nwant:\n%s", got, w)
	}
	*log = nil
}

func move(x, y float64) shell.PointerEvent {
	return shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: x, Y: y}}
}

func down(x, y float64) shell.PointerEvent {
	return shell.PointerEvent{Kind: shell.PointerDown, Pos: driver.Point{X: x, Y: y},
		Button: shell.ButtonPrimary, Buttons: shell.ButtonPrimary, Clicks: 1}
}

func up(x, y float64) shell.PointerEvent {
	return shell.PointerEvent{Kind: shell.PointerUp, Pos: driver.Point{X: x, Y: y},
		Button: shell.ButtonPrimary, Clicks: 1}
}

// window: root 0,0 200x100 with a panel at 10,10 100x80 holding a button
// at 10,10 40x20 (window 20,20 – 60,40), and a sibling at 150,10 40x40.
type window struct {
	log                  []string
	root, panel, btn, sb *probe
	r                    *Router
}

func newWindow(t *testing.T) *window {
	t.Helper()
	w := &window{}
	w.root = newProbe(&w.log, "root", 0, 0, 200, 100)
	w.panel = newProbe(&w.log, "panel", 10, 10, 100, 80)
	w.btn = newProbe(&w.log, "btn", 10, 10, 40, 20, "down", "up", "move")
	w.sb = newProbe(&w.log, "sb", 150, 10, 40, 40)
	w.root.SetChildren(w.panel, w.sb)
	w.panel.AddChild(w.btn)
	w.r = NewRouter(w.root)
	return w
}

func TestNewRouter(t *testing.T) {
	w := newWindow(t)
	if w.r.Root() != Element(w.root) || w.r.Hovered() != nil || w.r.Captured() != nil || w.r.Focused() != nil {
		t.Fatal("new router state")
	}
	mustPanic(t, "second router", func() { NewRouter(w.root) })
	mustPanic(t, "router on a child", func() { NewRouter(w.btn) })
	mustPanic(t, "router before Init", func() { NewRouter(&probe{}) })
	mustPanic(t, "routed root as child", func() { newBox("x", 0, 0, 1, 1).AddChild(w.root) })
	if w.r.Event(nil) {
		t.Fatal("nil event handled")
	}
}

func TestHoverSequence(t *testing.T) {
	w := newWindow(t)
	r := w.r
	r.Event(shell.PointerEvent{Kind: shell.PointerEnter, Pos: driver.Point{X: 5, Y: 5}})
	checkLog(t, &w.log, "root enter 5,5")
	r.Event(move(15, 15)) // into panel
	checkLog(t, &w.log, "panel enter 5,5", "panel move 5,5", "root move 15,15")
	r.Event(move(25, 25)) // into btn, which handles moves
	checkLog(t, &w.log, "btn enter 5,5", "btn move 5,5")
	if r.Hovered() != Element(w.btn) || len(r.HoverChain()) != 3 {
		t.Fatalf("hover chain %v", r.HoverChain())
	}
	r.Event(move(160, 20)) // across to the sibling: leave deepest first
	checkLog(t, &w.log, "btn leave 140,0", "panel leave 150,10", "sb enter 10,10", "sb move 10,10", "root move 160,20")
	r.Event(move(161, 20)) // same chain: no enter/leave
	checkLog(t, &w.log, "sb move 11,10", "root move 161,20")
	r.Event(shell.PointerEvent{Kind: shell.PointerLeave, Pos: driver.Point{X: 300, Y: 20}})
	checkLog(t, &w.log, "sb leave 150,10", "root leave 300,20")
	if r.Hovered() != nil {
		t.Fatal("hover after leave")
	}
	// RefreshHover does nothing while outside.
	r.RefreshHover()
	checkLog(t, &w.log)
}

func TestHoverRefreshAndTreeChanges(t *testing.T) {
	w := newWindow(t)
	r := w.r
	r.Event(move(25, 25))
	checkLog(t, &w.log, "root enter 25,25", "panel enter 15,15", "btn enter 5,5", "btn move 5,5")

	// Moving the button away under a still pointer changes the hover only
	// at RefreshHover.
	w.btn.SetFrame(driver.Rect{X: 60, Y: 10, W: 40, H: 20})
	checkLog(t, &w.log)
	r.RefreshHover()
	checkLog(t, &w.log, "btn leave -45,5")
	w.btn.SetFrame(driver.Rect{X: 10, Y: 10, W: 40, H: 20})
	r.RefreshHover()
	checkLog(t, &w.log, "btn enter 5,5")

	// Disabling an ancestor drops the hover below it at once.
	w.panel.SetEnabled(false)
	checkLog(t, &w.log, "btn leave 5,5", "panel leave 15,15")
	// The pointer over a disabled subtree hovers only the enabled part.
	r.Event(move(26, 25))
	checkLog(t, &w.log, "root move 26,25")
	if r.Hovered() != Element(w.root) {
		t.Fatal("hover in a disabled subtree")
	}
	w.panel.SetEnabled(true)
	checkLog(t, &w.log)
	r.RefreshHover()
	checkLog(t, &w.log, "panel enter 16,15", "btn enter 6,5")

	// Hiding and removing.
	w.btn.SetHidden(true)
	checkLog(t, &w.log, "btn leave 6,5")
	w.btn.SetHidden(false)
	r.RefreshHover()
	checkLog(t, &w.log, "btn enter 6,5")
	w.panel.RemoveFromParent()
	checkLog(t, &w.log, "btn leave 6,5", "panel leave 16,15")
	if len(r.HoverChain()) != 1 {
		t.Fatal("chain after remove")
	}
}

func TestHoverHandlerChangesTree(t *testing.T) {
	w := newWindow(t)
	// panel hides the button when the pointer enters it: btn must not get
	// an Enter afterwards.
	w.panel.onEvent = func(ev Event) {
		if pe, ok := ev.(PointerEvent); ok && pe.Kind == PointerEnter {
			w.btn.SetHidden(true)
		}
	}
	w.r.Event(move(25, 25))
	checkLog(t, &w.log, "root enter 25,25", "panel enter 15,15", "panel move 15,15", "root move 25,25")
}

func TestCapture(t *testing.T) {
	w := newWindow(t)
	r := w.r
	r.Event(move(25, 25))
	w.log = nil

	if !r.Event(down(25, 25)) {
		t.Fatal("press not handled")
	}
	checkLog(t, &w.log, "btn down 5,5 x1")
	if r.Captured() != Element(w.btn) {
		t.Fatal("no capture")
	}
	// Dragging off keeps the target, reports Over false, no hover changes.
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 160, Y: 20}, Buttons: shell.ButtonPrimary})
	checkLog(t, &w.log, "btn move 140,0 off")
	// Back over the button.
	r.Event(shell.PointerEvent{Kind: shell.PointerMove, Pos: driver.Point{X: 30, Y: 30}, Buttons: shell.ButtonPrimary})
	checkLog(t, &w.log, "btn move 10,10")
	// A second button goes to the capture too; its release keeps the capture.
	r.Event(shell.PointerEvent{Kind: shell.PointerDown, Pos: driver.Point{X: 30, Y: 30}, Button: shell.ButtonSecondary,
		Buttons: shell.ButtonPrimary | shell.ButtonSecondary, Clicks: 1})
	r.Event(shell.PointerEvent{Kind: shell.PointerUp, Pos: driver.Point{X: 30, Y: 30}, Button: shell.ButtonSecondary,
		Buttons: shell.ButtonPrimary, Clicks: 1})
	checkLog(t, &w.log, "btn down 10,10 x1", "btn up 10,10 x1")
	if r.Captured() == nil {
		t.Fatal("capture lost before the last release")
	}
	// Release outside: Over false, then the hover follows the pointer.
	if !r.Event(up(160, 20)) {
		t.Fatal("release not handled")
	}
	checkLog(t, &w.log, "btn up 140,0 off x1", "btn leave 140,0", "panel leave 150,10", "sb enter 10,10")
	if r.Captured() != nil || r.Hovered() != Element(w.sb) {
		t.Fatal("state after release")
	}
}

func TestPressBubblesAndUnhandled(t *testing.T) {
	w := newWindow(t)
	r := w.r
	// sb handles nothing: the press bubbles to the root, nobody captures.
	if r.Event(down(160, 20)) {
		t.Fatal("unhandled press reported handled")
	}
	checkLog(t, &w.log, "root enter 160,20", "sb enter 10,10", "sb down 10,10 x1", "root down 160,20 x1")
	if r.Captured() != nil {
		t.Fatal("capture without a handler")
	}
	if r.Event(up(25, 25)) {
		t.Fatal("release without capture handled")
	}
	checkLog(t, &w.log, "sb leave -125,15", "panel enter 15,15", "btn enter 5,5")
	// A release with buttons still held and no capture changes nothing.
	r.Event(shell.PointerEvent{Kind: shell.PointerUp, Pos: driver.Point{X: 160, Y: 20}, Buttons: shell.ButtonSecondary})
	checkLog(t, &w.log)

	// The root handles presses: a press on sb bubbles to the root, which captures.
	w.root.handles["down"] = true
	r.Event(down(160, 20))
	checkLog(t, &w.log, "btn leave 140,0", "panel leave 150,10", "sb enter 10,10", "sb down 10,10 x1", "root down 160,20 x1")
	if r.Captured() != Element(w.root) {
		t.Fatal("root did not capture")
	}
}

func TestDisabledTargets(t *testing.T) {
	w := newWindow(t)
	r := w.r
	w.btn.SetEnabled(false)
	w.panel.handles["down"] = true
	r.Event(move(25, 25))
	r.Event(down(25, 25))
	checkLog(t, &w.log, "root enter 25,25", "panel enter 15,15", "panel move 15,15", "root move 25,25", "panel down 15,15 x1")
	if r.Captured() != Element(w.panel) {
		t.Fatal("press on a disabled button did not go to its parent")
	}
}

func TestCaptureCancelled(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(w *window)
		want   []string
	}{
		{"disable", func(w *window) { w.btn.SetEnabled(false) }, []string{"btn cancel 15,15", "btn leave 15,15"}},
		{"disable ancestor", func(w *window) { w.panel.SetEnabled(false) }, []string{"btn cancel 15,15", "btn leave 15,15", "panel leave 25,25"}},
		{"hide", func(w *window) { w.btn.SetHidden(true) }, []string{"btn cancel 15,15", "btn leave 15,15"}},
		{"remove", func(w *window) { w.btn.RemoveFromParent() }, []string{"btn cancel 25,25", "btn leave 25,25"}},
		{"move to other tree", func(w *window) { newBox("x", 0, 0, 10, 10).AddChild(w.btn) }, []string{"btn cancel 25,25", "btn leave 25,25"}},
		{"shell leave", func(w *window) {
			w.r.Event(shell.PointerEvent{Kind: shell.PointerLeave, Pos: driver.Point{X: 300, Y: 25}})
		}, []string{"btn cancel 280,5", "btn leave 280,5", "panel leave 290,15", "root leave 300,25"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWindow(t)
			w.r.Event(move(35, 35))
			w.r.Event(down(35, 35))
			w.log = nil
			tc.change(w)
			checkLog(t, &w.log, tc.want...)
			if w.r.Captured() != nil {
				t.Fatal("capture kept")
			}
		})
	}
}

func TestCaptureWithinMoveKept(t *testing.T) {
	// Moving the captured element within the same tree keeps the capture.
	w := newWindow(t)
	w.r.Event(down(25, 25))
	w.log = nil
	w.root.AddChild(w.btn)
	w.sb.SetChildren(w.btn)
	checkLog(t, &w.log)
	if w.r.Captured() != Element(w.btn) {
		t.Fatal("capture lost by a move inside the tree")
	}
}

func TestScroll(t *testing.T) {
	w := newWindow(t)
	r := w.r
	ev := shell.ScrollEvent{Pos: driver.Point{X: 25, Y: 25}, Delta: driver.Point{Y: 3}}
	if r.Event(ev) {
		t.Fatal("unhandled scroll")
	}
	checkLog(t, &w.log, "btn scroll 5,5", "panel scroll 15,15", "root scroll 25,25")
	w.panel.handles["scroll"] = true
	if !r.Event(ev) {
		t.Fatal("scroll not handled")
	}
	checkLog(t, &w.log, "btn scroll 5,5", "panel scroll 15,15")
	// A disabled element and its subtree are skipped.
	w.panel.SetEnabled(false)
	w.root.handles["scroll"] = true
	r.Event(ev)
	checkLog(t, &w.log, "root scroll 25,25")
	// Scrolling goes by the pointer even during a capture elsewhere, and
	// over nothing it is not handled.
	if r.Event(shell.ScrollEvent{Pos: driver.Point{X: 500, Y: 500}}) {
		t.Fatal("scroll outside handled")
	}
	checkLog(t, &w.log)
}

func key(k driver.Key, mods driver.Modifiers) shell.KeyEvent {
	return shell.KeyEvent{Down: true, Key: k, Mods: mods}
}

func TestKeyAndTextBubbling(t *testing.T) {
	w := newWindow(t)
	r := w.r
	// No focus: the root gets keys.
	if r.Event(key('A', 0)) {
		t.Fatal("unhandled key")
	}
	checkLog(t, &w.log, "root key 65 true")

	w.btn.SetFocusable(true)
	if !w.btn.Focus() || !w.btn.HasFocus() || r.Focused() != Element(w.btn) {
		t.Fatal("Focus failed")
	}
	checkLog(t, &w.log, "btn focus true")
	r.Event(key('A', 0))
	r.Event(shell.TextEvent{Text: "a"})
	checkLog(t, &w.log, "btn key 65 true", "panel key 65 true", "root key 65 true",
		"btn text \"a\"", "panel text \"a\"", "root text \"a\"")

	w.panel.handles["key"] = true
	w.panel.handles["text"] = true
	if !r.Event(key('A', 0)) || !r.Event(shell.TextEvent{Text: "a"}) {
		t.Fatal("not handled")
	}
	checkLog(t, &w.log, "btn key 65 true", "panel key 65 true", "btn text \"a\"", "panel text \"a\"")
	if !r.Event(shell.KeyEvent{Key: 'A'}) { // key-ups route the same way
		t.Fatal("key-up not handled")
	}
	checkLog(t, &w.log, "btn key 65 false", "panel key 65 false")

	// A disabled root gets no keys when nothing is focused.
	r.SetFocus(nil)
	checkLog(t, &w.log, "btn focus false")
	w.root.SetEnabled(false)
	if r.Event(key('A', 0)) {
		t.Fatal("disabled root handled a key")
	}
	checkLog(t, &w.log)
}

func TestFocus(t *testing.T) {
	var log []string
	root := newProbe(&log, "root", 0, 0, 100, 100)
	a := newProbe(&log, "a", 0, 0, 10, 10)
	g := newProbe(&log, "g", 0, 0, 50, 50)
	b := newProbe(&log, "b", 0, 0, 10, 10)
	c := newProbe(&log, "c", 0, 0, 10, 10)
	d := newProbe(&log, "d", 0, 0, 10, 10)
	root.SetChildren(a, g, d)
	g.SetChildren(b, c)
	for _, e := range []*probe{a, b, c, d} {
		e.SetFocusable(true)
	}
	if !a.Focusable() || g.Focusable() {
		t.Fatal("Focusable")
	}
	// Without a router, Focus fails.
	if a.Focus() || a.HasFocus() {
		t.Fatal("focus without router")
	}
	r := NewRouter(root)

	order := func() string {
		var s []string
		for _, e := range r.FocusOrder() {
			s = append(s, e.(*probe).name)
		}
		return strings.Join(s, " ")
	}
	if got := order(); got != "a b c d" {
		t.Fatalf("order %q", got)
	}
	tab := func(shift bool) string {
		m := driver.Modifiers(0)
		if shift {
			m = driver.ModShift
		}
		if !r.Event(key(driver.KeyTab, m)) {
			t.Fatal("Tab not handled")
		}
		return r.Focused().(*probe).name
	}
	var seq []string
	for range 5 {
		seq = append(seq, tab(false))
	}
	if got := strings.Join(seq, " "); got != "a b c d a" {
		t.Fatalf("Tab sequence %q", got)
	}
	seq = nil
	for range 5 {
		seq = append(seq, tab(true))
	}
	if got := strings.Join(seq, " "); got != "d c b a d" {
		t.Fatalf("Shift-Tab sequence %q", got)
	}
	r.SetFocus(nil)
	if tab(true) != "d" {
		t.Fatal("Shift-Tab without focus")
	}
	r.SetFocus(nil)
	log = nil

	// Focus events: old loses, new gains; refocusing does nothing.
	a.Focus()
	b.Focus()
	b.Focus()
	checkLog(t, &log, "a focus true", "a focus false", "b focus true")

	// Ctrl+Tab and Tab key-ups don't move the focus; a handled Tab neither.
	if r.Event(key(driver.KeyTab, driver.ModControl)) || r.Event(shell.KeyEvent{Key: driver.KeyTab}) {
		t.Fatal("Ctrl-Tab or Tab-up handled")
	}
	b.handles["key"] = true
	r.Event(key(driver.KeyTab, 0))
	if r.Focused() != Element(b) {
		t.Fatal("handled Tab moved the focus")
	}
	b.handles["key"] = false
	log = nil

	// Hidden and disabled subtrees are skipped; such elements cannot be focused.
	g.SetHidden(true)
	checkLog(t, &log, "b focus false")
	if got := order(); got != "a d" {
		t.Fatalf("order with hidden group %q", got)
	}
	if c.Focus() || r.SetFocus(c) {
		t.Fatal("focused a hidden element")
	}
	g.SetHidden(false)
	c.Focus()
	g.SetEnabled(false)
	checkLog(t, &log, "c focus true", "c focus false")
	if got := order(); got != "a d" {
		t.Fatalf("order with disabled group %q", got)
	}
	g.SetEnabled(true)
	log = nil

	// Unfocusable elements lose the focus and cannot get it.
	d.Focus()
	d.SetFocusable(false)
	d.SetFocusable(false)
	checkLog(t, &log, "d focus true", "d focus false")
	if d.Focus() || r.SetFocus(g) {
		t.Fatal("focused an unfocusable element")
	}

	// Removing, and moving to another tree, drop the focus; a move inside
	// the tree keeps it.
	c.Focus()
	root.AddChild(c)
	if !c.HasFocus() {
		t.Fatal("move inside the tree dropped the focus")
	}
	c.RemoveFromParent()
	checkLog(t, &log, "c focus true", "c focus false")
	b.Focus()
	other := newBox("o", 0, 0, 1, 1)
	other.SetChildren(b)
	checkLog(t, &log, "b focus true", "b focus false")
	if r.SetFocus(b) {
		t.Fatal("focused an element of another tree")
	}

	// Removing an ancestor of the focus.
	a.Focus()
	root.SetChildren(g)
	checkLog(t, &log, "a focus true", "a focus false")

	// A focus handler that moves the focus wins.
	var log2 []string
	root2 := newProbe(&log2, "root", 0, 0, 1, 1)
	x := newProbe(&log2, "x", 0, 0, 1, 1)
	y := newProbe(&log2, "y", 0, 0, 1, 1)
	root2.SetChildren(x, y)
	x.SetFocusable(true)
	y.SetFocusable(true)
	r2 := NewRouter(root2)
	x.onEvent = func(ev Event) {
		if fe, ok := ev.(FocusEvent); ok && fe.Focused {
			y.Focus()
		}
	}
	if r2.SetFocus(x) || r2.Focused() != Element(y) {
		t.Fatal("redirected focus")
	}
	checkLog(t, &log2, "x focus true", "x focus false", "y focus true")

	// Nothing focusable: Tab is not handled.
	r3 := NewRouter(newProbe(&log2, "empty", 0, 0, 1, 1))
	if r3.FocusNext(false) || r3.Event(key(driver.KeyTab, 0)) {
		t.Fatal("Tab without focusable elements")
	}
}

func TestAccessibilityTree(t *testing.T) {
	var log []string
	root := newProbe(&log, "root", 0, 0, 300, 200)
	root.node = a11y.Node{Role: a11y.RoleWindow, Label: "Demo"}
	stack := newProbe(&log, "stack", 20, 30, 200, 100) // RoleNone: hoisted
	ok := newProbe(&log, "ok", 5, 5, 80, 22)
	ok.node = a11y.Node{Role: a11y.RoleButton, Label: "OK", Actions: a11y.ActionPress,
		Children: []*a11y.Node{{Role: a11y.RoleStaticText}}} // ignored
	cb := newProbe(&log, "cb", 5, 40, 80, 18)
	cb.node = a11y.Node{Role: a11y.RoleCheckBox, Label: "Agree", States: a11y.StateChecked, Actions: a11y.ActionPress}
	box := newProbe(&log, "box", 230, 30, 60, 60)
	box.node = a11y.Node{Role: a11y.RoleGroup, Label: "Box"}
	lbl := newProbe(&log, "lbl", 4, 4, 40, 16)
	lbl.node = a11y.Node{Role: a11y.RoleStaticText, Label: "Hi", Value: "Hi"}
	hidden := newProbe(&log, "hidden", 0, 0, 1, 1)
	hidden.node = a11y.Node{Role: a11y.RoleButton, Label: "Hidden"}
	hidden.SetHidden(true)
	root.SetChildren(stack, box, hidden)
	stack.SetChildren(ok, cb)
	box.SetChildren(lbl)
	ok.SetFocusable(true)
	cb.SetFocusable(true)

	// Without a router nothing is focused.
	want := []string{
		`window "Demo" @0,0 300x200`,
		`  button "OK" [focusable] {press focus} @25,35 80x22`,
		`  checkbox "Agree" [disabled checked] {press} @25,70 80x18`,
		`  group "Box" @230,30 60x60`,
		`    text "Hi" value="Hi" @234,34 40x16`,
	}
	cb.SetEnabled(false)
	tree := AccessibilityTree(root)
	if got, w := tree.Dump(), strings.Join(want, "\n"); got != w {
		t.Fatalf("tree:\n%s\nwant:\n%s", got, w)
	}
	if tree.Count() != 5 || tree.Find(a11y.RoleCheckBox, "Agree") == nil || tree.Find(a11y.RoleButton, "Hidden") != nil {
		t.Fatal("Count/Find")
	}

	NewRouter(root)
	cb.SetEnabled(true)
	ok.Focus()
	tree = AccessibilityTree(root)
	if n := tree.Find(a11y.RoleButton, "OK"); n.States != a11y.StateFocusable|a11y.StateFocused {
		t.Fatalf("focused node %v", n)
	}

	// A RoleNone root is still returned.
	plain := newProbe(&log, "plain", 1, 2, 3, 4)
	if got := AccessibilityTree(plain).String(); got != `none "" @1,2 3x4` {
		t.Fatalf("plain root %q", got)
	}
	// The Base default exposes nothing.
	if (&Base{}).AccessibleNode().Role != a11y.RoleNone || (&Base{}).HandleEvent(FocusEvent{}) {
		t.Fatal("Base defaults")
	}
}

func TestPointerKindString(t *testing.T) {
	if PointerCancel.String() != "cancel" || PointerKind(99).String() != "kind(99)" {
		t.Fatal("PointerKind.String")
	}
}

func TestRouterOddEvents(t *testing.T) {
	w := newWindow(t)
	if w.r.Event(shell.PointerEvent{Kind: 99}) {
		t.Fatal("unknown pointer kind handled")
	}
	// Events are a closed set; the marker methods exist on each type.
	for _, ev := range []Event{PointerEvent{}, ScrollEvent{}, KeyEvent{}, TextEvent{}, FocusEvent{}} {
		ev.isEvent()
	}
	// A press handler that disables itself does not capture.
	w.btn.onEvent = func(ev Event) {
		if pe, ok := ev.(PointerEvent); ok && pe.Kind == PointerDown {
			w.btn.SetEnabled(false)
		}
	}
	if !w.r.Event(down(25, 25)) || w.r.Captured() != nil {
		t.Fatal("disabled press handler captured")
	}
	// Base paints nothing.
	if got := dump(t, w.root, driver.Size{W: 200, H: 100}); got != "" {
		t.Fatalf("probe tree paints %q", got)
	}
}

// TestKeyboardAccess covers the two keyboard-access modes (S3.2e): with
// full keyboard access Tab reaches every focusable element, without it only
// FocusText ones, starting from a clicked control at its place in the tree.
func TestKeyboardAccess(t *testing.T) {
	var log []string
	root := newProbe(&log, "root", 0, 0, 100, 100)
	b1 := newProbe(&log, "b1", 0, 0, 10, 10)
	t1 := newProbe(&log, "t1", 0, 0, 10, 10)
	g := newProbe(&log, "g", 0, 0, 50, 50)
	b2 := newProbe(&log, "b2", 0, 0, 10, 10)
	t2 := newProbe(&log, "t2", 0, 0, 10, 10)
	t3 := newProbe(&log, "t3", 0, 0, 10, 10)
	b3 := newProbe(&log, "b3", 0, 0, 10, 10)
	root.SetChildren(b1, t1, g, t3, b3)
	g.SetChildren(b2, t2)
	for _, e := range []*probe{b1, t1, b2, t2, t3, b3} {
		e.SetFocusable(true)
	}
	for _, e := range []*probe{t1, t2, t3} {
		e.SetFocusKind(FocusText)
	}
	if b1.FocusKind() != FocusControl || t1.FocusKind() != FocusText {
		t.Fatal("FocusKind")
	}
	if FocusControl.String() != "control" || FocusText.String() != "text" || FocusKind(9).String() != "focuskind(?)" {
		t.Fatal("FocusKind.String")
	}
	r := NewRouter(root)
	if !r.FullKeyboardAccess() {
		t.Fatal("full keyboard access is not the default")
	}
	names := func(es []Element) string {
		var s []string
		for _, e := range es {
			s = append(s, e.(*probe).name)
		}
		return strings.Join(s, " ")
	}
	tabs := func(shift bool, n int) string {
		t.Helper()
		m := driver.Modifiers(0)
		if shift {
			m = driver.ModShift
		}
		var seq []string
		for range n {
			if !r.Event(key(driver.KeyTab, m)) {
				t.Fatal("Tab not handled")
			}
			seq = append(seq, r.Focused().(*probe).name)
		}
		return strings.Join(seq, " ")
	}

	// Full keyboard access: every control, both directions, wrapping.
	if got := names(r.FocusOrder()); got != "b1 t1 b2 t2 t3 b3" {
		t.Fatalf("full order %q", got)
	}
	if got := tabs(false, 7); got != "b1 t1 b2 t2 t3 b3 b1" {
		t.Fatalf("full Tab %q", got)
	}
	if got := tabs(true, 7); got != "b3 t3 t2 b2 t1 b1 b3" {
		t.Fatalf("full Shift-Tab %q", got)
	}

	// Without it: text elements only; the focus is kept by the switch.
	r.SetFullKeyboardAccess(false)
	if r.FullKeyboardAccess() || r.Focused() != Element(b3) {
		t.Fatal("switching the mode")
	}
	if got := names(r.FocusOrder()); got != "t1 t2 t3" {
		t.Fatalf("text order %q", got)
	}
	r.SetFocus(nil)
	if got := tabs(false, 4); got != "t1 t2 t3 t1" {
		t.Fatalf("text Tab %q", got)
	}
	r.SetFocus(nil)
	if got := tabs(true, 4); got != "t3 t2 t1 t3" {
		t.Fatalf("text Shift-Tab %q", got)
	}

	// A control focused by a click (SetFocus works in both modes): Tab goes
	// to the next text element after it in tree order, Shift-Tab to the one
	// before it, wrapping at both ends.
	for _, c := range []struct {
		from          *probe
		next, backTab string
	}{
		{b1, "t1", "t3"}, // before every text element
		{b2, "t2", "t1"}, // inside the group
		{b3, "t1", "t3"}, // after every text element
	} {
		if !c.from.Focus() {
			t.Fatalf("focus %s", c.from.name)
		}
		if got := tabs(false, 1); got != c.next {
			t.Errorf("Tab from %s: %q, want %q", c.from.name, got, c.next)
		}
		c.from.Focus()
		if got := tabs(true, 1); got != c.backTab {
			t.Errorf("Shift-Tab from %s: %q, want %q", c.from.name, got, c.backTab)
		}
	}

	// Hidden and disabled text elements are skipped; a focused control in
	// a hidden subtree has lost the focus, so Tab starts again.
	t2.SetHidden(true)
	if got := names(r.FocusOrder()); got != "t1 t3" {
		t.Fatalf("order with t2 hidden %q", got)
	}
	t1.Focus()
	if got := tabs(false, 2); got != "t3 t1" {
		t.Fatalf("Tab with t2 hidden %q", got)
	}
	t2.SetHidden(false)
	t3.SetEnabled(false)
	b2.Focus()
	if got := tabs(false, 2); got != "t2 t1" {
		t.Fatalf("Tab with t3 disabled %q", got)
	}
	t3.SetEnabled(true)
	b2.Focus()
	g.SetHidden(true)
	if r.Focused() != nil {
		t.Fatal("hidden control kept the focus")
	}
	if got := tabs(true, 1); got != "t3" {
		t.Fatalf("Shift-Tab after the focus was hidden %q", got)
	}
	g.SetHidden(false)

	// Changing an element's kind changes only the Tab order.
	t3.Focus()
	t3.SetFocusKind(FocusControl)
	if !t3.HasFocus() || names(r.FocusOrder()) != "t1 t2" {
		t.Fatal("SetFocusKind")
	}
	if got := tabs(false, 1); got != "t1" {
		t.Fatalf("Tab from a former text element %q", got)
	}

	// No text element: Tab is not handled and the focus stays.
	for _, e := range []*probe{t1, t2} {
		e.SetFocusKind(FocusControl)
	}
	b1.Focus()
	if r.Event(key(driver.KeyTab, 0)) || r.FocusNext(true) || r.Focused() != Element(b1) {
		t.Fatal("Tab without text elements")
	}
	r.SetFullKeyboardAccess(true)
	if got := tabs(false, 1); got != "t1" {
		t.Fatalf("Tab after full access is back %q", got)
	}
}
