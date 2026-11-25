package gogui

import (
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/layout"
)

// countingHost counts invalidate calls, standing in for a window.
type countingHost struct{ n int }

func (h *countingHost) invalidate() { h.n++ }

func stubLabel(t *testing.T, l *Label) *stub.Label {
	t.Helper()
	p, ok := l.peer.(*stub.Label)
	if !ok {
		t.Fatalf("peer is %T, want *stub.Label", l.peer)
	}
	return p
}

func TestLabelSettersReachPeer(t *testing.T) {
	runApp(t, func(*App) {
		l := NewLabel("Hello")
		p := stubLabel(t, l)
		if p.Text != "Hello" || l.Text() != "Hello" {
			t.Errorf("new label: peer %q, Text %q", p.Text, l.Text())
		}
		bold := BoldSystemFont(15)
		// Chaining through common[*Label] keeps the concrete type.
		var got *Label = l.SetText("Hi").SetFont(bold).SetTextColor(ColorSecondaryLabel).
			SetAlignment(AlignCenter).SetSelectable(true).SetWraps(true).
			SetEnabled(false).SetHidden(true).SetTooltip("tip").SetControlSize(ControlSizeSmall).Focus()
		if got != l {
			t.Fatal("chain returned a different label")
		}
		if p.Text != "Hi" || p.Font != bold || p.TextColor != ColorSecondaryLabel ||
			p.Alignment != AlignCenter || !p.Selectable || !p.Wraps {
			t.Errorf("label peer: %+v", p)
		}
		if !p.Disabled || !p.Hidden || p.Tooltip != "tip" || p.ControlSize != ControlSizeSmall || p.FocusCalls != 1 {
			t.Errorf("base peer: %+v", p.Base)
		}
		if l.Text() != "Hi" || l.Font() != bold {
			t.Error("label getters don't return the set values")
		}
		if !l.disabled {
			t.Error("SetEnabled(false) not recorded")
		}
		if !l.node().Props().Hidden() {
			t.Error("SetHidden didn't reach the layout node")
		}
	})
}

func TestLabelIsWidget(t *testing.T) {
	runApp(t, func(*App) {
		var w Widget = NewLabel("x")
		if w.peerWidget() == nil {
			t.Fatal("no peer")
		}
		leaf, ok := w.node().(*layout.Leaf)
		if !ok || leaf.Item() != w.peerWidget() {
			t.Fatalf("node %T does not wrap the peer", w.node())
		}
		if w.node().Expands(layout.Horizontal) || w.node().Expands(layout.Vertical) {
			t.Error("a label expands")
		}
	})
}

func TestDestroyedWidget(t *testing.T) {
	runApp(t, func(*App) {
		l := NewLabel("a").SetTooltip("t").SetFont(SystemFont(20))
		p := stubLabel(t, l)
		h := &countingHost{}
		l.host = h
		l.destroy()
		l.destroy() // idempotent
		l.Destroy() // so is the public one
		if !p.Destroyed {
			t.Fatal("peer not destroyed")
		}
		// D13: every setter, subscription and action panics; the getters
		// return the last values.
		for name, fn := range map[string]func(){
			"SetText":       func() { l.SetText("b") },
			"SetFont":       func() { l.SetFont(Font{}) },
			"SetTextColor":  func() { l.SetTextColor(ColorSystemRed) },
			"SetAlignment":  func() { l.SetAlignment(AlignTrailing) },
			"SetSelectable": func() { l.SetSelectable(true) },
			"SetWraps":      func() { l.SetWraps(true) },
			"SetEnabled":    func() { l.SetEnabled(false) },
			"SetHidden":     func() { l.SetHidden(true) },
			"SetTooltip":    func() { l.SetTooltip("u") },
			"SetControlSize": func() {
				l.SetControlSize(ControlSizeLarge)
			},
			"Focus": func() { l.Focus() },
		} {
			mustPanic(t, "Label."+name+" on a destroyed widget", fn)
		}
		if p.Text != "a" || p.Wraps || p.Disabled || p.Hidden || p.Tooltip != "t" ||
			p.ControlSize != ControlSizeRegular || p.FocusCalls != 0 {
			t.Errorf("a destroyed peer was called: %+v", p)
		}
		if l.Text() != "a" || l.Font().Size != 20 || l.Tooltip() != "t" || l.disabled || l.node().Props().Hidden() {
			t.Error("getters on a destroyed widget don't return the last values")
		}
		if h.n != 0 {
			t.Errorf("destroyed widget invalidated its window %d times", h.n)
		}
	})
}

// TestDestroy is A1/D13 and B3/B4: Destroy takes a widget out of its
// parent or window and destroys its subtree; destroyed widgets can't be
// placed again, and nothing can be placed in them.
func TestDestroy(t *testing.T) {
	runApp(t, func(app *App) {
		a, b := NewLabel("a"), NewButton("b")
		inner := VStack(0, b)
		s := VStack(0, a, inner)
		win, _ := newTestWindow(app, WindowConfig{Size: Size{W: 100, H: 100}})
		win.SetContent(s)

		inner.Destroy()
		wantChildren(t, "stack after Destroy", s, a)
		if !stubBase(b).Destroyed || !b.dead || !inner.dead || inner.parent != nil {
			t.Error("Destroy didn't destroy the subtree or detach it")
		}
		if _, live := app.live[&b.widget]; live {
			t.Error("a destroyed widget stays in App.live (B4)")
		}

		// The window's content.
		s.Destroy()
		if win.Content() != nil || !a.dead || win.p.(*minCounter).Content != nil {
			t.Error("destroying the content didn't take it out of the window")
		}

		// B3: no re-parenting of a destroyed widget, nothing into one.
		c := NewLabel("c")
		mustPanic(t, "Stack.Add with a destroyed widget", func() { VStack(0).Add(a) })
		mustPanic(t, "VStack with a destroyed widget", func() { VStack(0, a) })
		mustPanic(t, "Form with a destroyed widget", func() { Form(Row("x", a)) })
		mustPanic(t, "Box.SetContent with a destroyed widget", func() { NewBox("").SetContent(a) })
		mustPanic(t, "ScrollView.SetDocument with a destroyed widget", func() { NewScrollView(nil).SetDocument(a) })
		mustPanic(t, "Window.SetContent with a destroyed widget", func() { win.SetContent(a) })
		mustPanic(t, "Stack.Add on a destroyed widget", func() { s.Add(c) })
		if c.parent != nil || c.dead {
			t.Error("a failed Add changed the child")
		}

		// A form's control: its row and label go with it.
		f := Form(Row("Name:", c))
		lbl := f.rows[0].label
		c.Destroy()
		if len(f.rows) != 0 || !lbl.dead {
			t.Error("destroying a form control left its row or label")
		}
	})
}

// TestStackRemoveClear: Remove and Clear take children out without
// destroying them; they can be placed again.
func TestStackRemoveClear(t *testing.T) {
	runApp(t, func(*App) {
		a, b, c := NewLabel("a"), NewLabel("b"), NewLabel("c")
		s := VStack(0, a, b, c)
		other := VStack(0)
		if got := s.Remove(b, other, nil); got != s {
			t.Fatal("Remove doesn't chain")
		}
		wantChildren(t, "after Remove", s, a, c)
		if b.parent != nil || b.dead {
			t.Error("Remove destroyed or kept the child")
		}
		s.Clear()
		wantChildren(t, "after Clear", s)
		if a.parent != nil || c.parent != nil || a.dead || len(s.Children()) != 0 {
			t.Error("Clear destroyed or kept a child")
		}
		other.Add(a, b, c)
		wantChildren(t, "placed again", other, a, b, c)
		if ch := other.Children(); len(ch) != 3 || ch[0] != Widget(a) {
			t.Errorf("Children = %v", ch)
		}
	})
}

func TestInvalidate(t *testing.T) {
	runApp(t, func(*App) {
		l := NewLabel("a")
		l.SetText("no window") // no host: nothing to do
		h := &countingHost{}
		root := &widget{host: h}
		parent := &widget{parent: root}
		l.parent = parent
		l.SetText("b").SetFont(SystemFont(14)).SetWraps(true).SetHidden(false).SetControlSize(ControlSizeSmall)
		if h.n != 5 {
			t.Errorf("size setters invalidated %d times, want 5", h.n)
		}
		l.SetTextColor(ColorLabel).SetAlignment(AlignCenter).SetSelectable(true).SetEnabled(false).
			SetTooltip("x").Focus()
		if h.n != 5 {
			t.Errorf("setters that don't change the size invalidated (%d)", h.n)
		}
		l.parent = nil // the fake parents are not widgets Run could destroy
	})
}

func TestNewLabelOutsideRun(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "NewLabel called outside Run") {
			t.Errorf("recover = %v", r)
		}
	}()
	NewLabel("x")
}

func TestLabelOffUIThread(t *testing.T) {
	runApp(t, func(*App) {
		l := NewLabel("x")
		for _, c := range []struct {
			what string
			call func()
		}{
			{"Label.SetText", func() { l.SetText("y") }},
			{"Label.SetEnabled", func() { l.SetEnabled(false) }},
			{"NewLabel", func() { NewLabel("z") }},
		} {
			done := make(chan any)
			go func() {
				defer func() { done <- recover() }()
				c.call()
			}()
			if r, _ := (<-done).(string); !strings.Contains(r, c.what+" called off the UI thread") {
				t.Errorf("%s off the UI thread: recover = %q", c.what, r)
			}
		}
		if l.Text() != "x" || l.disabled {
			t.Error("an off-thread call changed the label")
		}
	})
}

func TestSubscribers(t *testing.T) {
	var s subscribers[func(int)]
	var got []string
	rec := func(name string) func(int) {
		return func(int) { got = append(got, name) }
	}
	unA := s.add(rec("a"))
	var unB, unC Unsubscribe
	unB = s.add(func(int) {
		got = append(got, "b")
		unB() // unsubscribe itself during dispatch
		unC() // and a later one, which must not be called anymore
		s.add(rec("late"))
	})
	unC = s.add(rec("c"))
	s.add(rec("d"))
	s.each(func(f func(int)) { f(0) })
	if want := "a b d"; strings.Join(got, " ") != want {
		t.Errorf("first dispatch %q, want %q", strings.Join(got, " "), want)
	}
	unB()
	unC() // idempotent
	unA()
	unA()
	got = nil
	s.each(func(f func(int)) { f(0) })
	if want := "d late"; strings.Join(got, " ") != want || len(s.list) != 2 {
		t.Errorf("second dispatch %q (%d handlers), want %q", strings.Join(got, " "), len(s.list), want)
	}
}

func TestDefaultExpand(t *testing.T) {
	cases := []struct {
		w    widget
		h, v bool
	}{
		{widget{kind: kindLabel}, false, false},
		{widget{kind: kindButton}, false, false},
		{widget{kind: kindCheckbox}, false, false},
		{widget{kind: kindRadio}, false, false},
		{widget{kind: kindRadioGroup}, false, false},
		{widget{kind: kindPopUp}, false, false},
		{widget{kind: kindSpinner}, false, false},
		{widget{kind: kindTextField}, true, false},
		{widget{kind: kindProgressBar}, true, false},
		{widget{kind: kindTextView}, true, true},
		{widget{kind: kindSlider}, true, false},
		{widget{kind: kindSlider, vertical: true}, false, true},
		{widget{kind: kindSeparator}, true, false},
		{widget{kind: kindSeparator, vertical: true}, false, true},
		{widget{kind: kindScrollView}, false, false},
		{widget{kind: kindScrollView, scrolls: [2]bool{false, true}}, false, true},
		{widget{kind: kindScrollView, scrolls: [2]bool{true, true}}, true, true},
		{widget{kind: kindBox}, false, false},
		{widget{kind: kindStack}, false, false},
		{widget{kind: kindForm}, false, false},
		{widget{kind: kindSpacer}, false, false},
	}
	for _, c := range cases {
		w := c.w
		w.n = layout.NewLeaf(stub.NewLabel())
		w.applyExpand()
		if h, v := w.n.Expands(layout.Horizontal), w.n.Expands(layout.Vertical); h != c.h || v != c.v {
			t.Errorf("%v (vertical %v, scrolls %v): expands %v/%v, want %v/%v",
				w.kind, w.vertical, w.scrolls, h, v, c.h, c.v)
		}
	}
	if kind(numKinds).String() != "Widget" || kindPopUp.String() != "PopUpButton" {
		t.Error("kind names")
	}
}

// mustPanic runs fn and fails unless it panics with a message that
// contains want.
func mustPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		msg, _ := r.(string)
		if err, ok := r.(error); ok {
			msg = err.Error()
		}
		if r == nil || !strings.Contains(msg, want) {
			t.Errorf("panic %v, want one containing %q", r, want)
		}
	}()
	fn()
}
