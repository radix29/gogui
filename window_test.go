package gogui

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// minCounter wraps a window peer and counts the SetMinSize calls.
type minCounter struct {
	*stub.Window
	n int
}

func (m *minCounter) SetMinSize(s driver.Size) { m.n++; m.Window.SetMinSize(s) }

// newTestWindow creates a window and swaps its peer for a minCounter.
func newTestWindow(app *App, cfg WindowConfig) (*Window, *minCounter) {
	w := app.NewWindow(cfg)
	m := &minCounter{Window: w.p.(*stub.Window)}
	w.p = m
	return w, m
}

// helloTree is the hello example with fakes for the field and the button.
func helloTree() (root *Stack, name, greet *fake) {
	name, greet = fakeField(), fakeButton("Greet")
	root = VStack(12, NewLabel("Hello!").SetFont(BoldSystemFont(15)), name,
		HStack(8, Spacer(), greet)).Padding(20)
	return root, name, greet
}

func sizeNear(a, b Size) bool { return math.Abs(a.W-b.W) < 1e-9 && math.Abs(a.H-b.H) < 1e-9 }

func drain(app *App) {
	for app.drv.Queue().Drain() > 0 {
	}
}

var helloCfg = WindowConfig{Title: "Hello", Size: Size{W: 360, H: 180}, Style: Titled | Closable}

func TestWindowLayout(t *testing.T) {
	runApp(t, func(app *App) {
		w, m := newTestWindow(app, helloCfg)
		if w.Title() != "Hello" || m.Visible || w.ContentSize() != (Size{W: 360, H: 180}) {
			t.Fatalf("new window: title %q, visible %v, size %v", w.Title(), m.Visible, w.ContentSize())
		}
		root, name, greet := helloTree()
		if got := w.SetContent(root).SetTitle("Hi"); got != w || m.Title != "Hi" || w.Title() != "Hi" {
			t.Error("SetContent/SetTitle chain or title")
		}
		if m.Content != root.peerWidget() || w.passes != 0 {
			t.Fatalf("content peer %v, %d passes before the drain", m.Content, w.passes)
		}
		drain(app)
		wantFrame(t, "root", root, Rect{W: 360, H: 180})
		wantFrame(t, "greet", greet, Rect{X: 257, W: 63, H: 22})

		// Many size setters, one pass.
		for range 10 {
			greet.SetHidden(false)
			root.Padding(20)
		}
		drain(app)
		if w.passes != 2 {
			t.Errorf("%d passes, want 2 (one for SetContent, one for 20 setters)", w.passes)
		}
		// A resize lays out at once, without a drain.
		m.UserResize(Size{W: 500, H: 300})
		if w.passes != 3 || w.ContentSize() != (Size{W: 500, H: 300}) {
			t.Errorf("after UserResize: %d passes, size %v", w.passes, w.ContentSize())
		}
		wantFrame(t, "name", name, Rect{X: 20, Y: 20 + 240.0/13 + 12, W: 460, H: 22})
		w.SetContentSize(Size{W: 400, H: 200})
		wantFrame(t, "greet after SetContentSize", greet, Rect{X: 297, W: 63, H: 22})

		// Show lays out a dirty window before showing it.
		root.Padding(10)
		w.Show()
		if !m.Visible || w.passes != 5 {
			t.Errorf("Show: visible %v, %d passes, want 5", m.Visible, w.passes)
		}
		wantFrame(t, "greet after Show", greet, Rect{X: 317, W: 63, H: 22})
		drain(app)
		w.Show()
		if w.passes != 5 {
			t.Errorf("a clean Show or the stale queued pass laid out again (%d)", w.passes)
		}
	})
}

func TestWindowMinSize(t *testing.T) {
	runApp(t, func(app *App) {
		w, m := newTestWindow(app, helloCfg)
		root, name, _ := helloTree()
		w.SetContent(root)
		drain(app)
		want := Size{W: 136, H: 108 + 240.0/13} // Root.MinSize of hello
		if !sizeNear(m.MinSize, want) || m.n != 1 {
			t.Fatalf("min size %v after %d calls, want %v after 1", m.MinSize, m.n, want)
		}
		m.UserResize(Size{W: 300, H: 200})
		name.SetHidden(false)
		drain(app)
		if m.n != 1 {
			t.Errorf("SetMinSize called %d times for an unchanged minimum", m.n)
		}
		// The configured minimum wins where it is larger.
		big, mb := newTestWindow(app, WindowConfig{Size: Size{W: 500, H: 400}, MinSize: Size{W: 400, H: 100}})
		r2, _, _ := helloTree()
		big.SetContent(r2)
		drain(app)
		if !sizeNear(mb.MinSize, Size{W: 400, H: want.H}) || mb.n != 1 {
			t.Errorf("min size %v after %d calls", mb.MinSize, mb.n)
		}
		// Without content the configured minimum is back.
		big.SetContent(nil)
		drain(app)
		if mb.MinSize != (Size{W: 400, H: 100}) || mb.Content != nil || mb.n != 2 {
			t.Errorf("no content: min size %v, content %v, %d calls", mb.MinSize, mb.Content, mb.n)
		}
	})
}

func TestWindowClose(t *testing.T) {
	runApp(t, func(app *App) {
		w, m := newTestWindow(app, helloCfg)
		root, name, greet := helloTree()
		w.SetContent(root)
		var log []string
		w.OnShouldClose(func() bool { log = append(log, "should"); return len(log) > 1 })
		w.OnClose(func() {
			log = append(log, "close")
			if stubBase(greet).Destroyed {
				t.Error("tree destroyed before OnClose")
			}
		})
		un := w.OnClose(func() { log = append(log, "gone") })
		un()
		un()
		if m.UserClose() || m.Closed {
			t.Fatal("OnShouldClose did not veto UserClose")
		}
		if !m.UserClose() || !m.Closed {
			t.Fatal("UserClose did not close")
		}
		if want := []string{"should", "should", "close"}; len(log) != 3 || log[0] != want[0] || log[2] != want[2] {
			t.Errorf("events %v, want %v", log, want)
		}
		// The whole tree is destroyed (the order is checked in TestBottomUp).
		for _, c := range []Widget{root, name, greet, root.children[2]} {
			if !stubBase(c).Destroyed || !c.base().dead {
				t.Errorf("%v not destroyed", c.base().kind)
			}
		}
		// A closed window ignores everything; Close is idempotent.
		w.SetTitle("x").SetContent(NewLabel("y")).SetContentSize(Size{W: 1, H: 1})
		w.Show()
		w.Close()
		if w.Title() != "x" || m.Title == "x" || m.Visible || w.ContentSize() != (Size{W: 360, H: 180}) || len(log) != 3 {
			t.Error("a closed window called its peer or its handlers")
		}
		// Close and App.Quit both fire OnClose without asking OnShouldClose.
		w2, _ := newTestWindow(app, helloCfg)
		closes := 0
		w2.OnShouldClose(func() bool { t.Error("Close asked OnShouldClose"); return false })
		w2.OnClose(func() { closes++ })
		w2.Close()
		w3, _ := newTestWindow(app, helloCfg)
		w3.OnClose(func() { closes++ })
		w3.OnClose(app.Quit) // as in the examples: Quit inside Quit
		t.Cleanup(func() {
			if closes != 2 {
				t.Errorf("OnClose ran %d times, want 2 (Close, Quit)", closes)
			}
		})
	})
}

// TestBottomUp checks the destruction order directly: when a leaf peer is
// destroyed, its container must still be alive.
func TestBottomUp(t *testing.T) {
	runApp(t, func(app *App) {
		w := app.NewWindow(helloCfg)
		leaf := &orderPeer{Button: stub.NewButton(nil)}
		s := VStack(0, newFake(kindButton, leaf))
		leaf.parent = s.peerWidget().(*stub.Container)
		w.SetContent(s)
		w.Close()
		if !leaf.Destroyed || !leaf.parentAlive {
			t.Errorf("leaf destroyed %v, container alive at that time %v", leaf.Destroyed, leaf.parentAlive)
		}
	})
}

type orderPeer struct {
	*stub.Button
	parent      *stub.Container
	parentAlive bool
}

func (p *orderPeer) Destroy() { p.parentAlive = !p.parent.Destroyed; p.Button.Destroy() }

func TestWindowReplaceContent(t *testing.T) {
	runApp(t, func(app *App) {
		w, m := newTestWindow(app, helloCfg)
		old, _, _ := helloTree()
		w.SetContent(old)
		drain(app)
		repl := VStack(0, NewLabel("new"))
		w.SetContent(repl)
		if m.Content != repl.peerWidget() || stubBase(old).HasParent() || stubBase(old).Destroyed {
			t.Error("old content still attached, or destroyed")
		}
		old.Padding(1) // no longer in the window: no pass
		drain(app)
		if w.passes != 2 {
			t.Errorf("%d passes, want 2", w.passes)
		}
		wantFrame(t, "new content", repl, Rect{W: 360, H: 180})

		// Moving the content into a stack takes it out of the window, and
		// moving it into a second window takes it out of the first.
		HStack(0, repl)
		if m.Content != nil || w.content != nil || repl.host != nil {
			t.Error("content moved into a stack stayed in the window")
		}
		w2, m2 := newTestWindow(app, helloCfg)
		w.SetContent(old)
		w2.SetContent(old)
		if m.Content != nil || m2.Content != old.peerWidget() || old.host != layoutHost(w2) {
			t.Error("content moved to a second window stayed in the first")
		}
		w.Close()
		if stubBase(old).Destroyed {
			t.Error("closing a window destroyed content it gave away")
		}
	})
}

// TestRunDestroysLeftovers checks the lifetime policy: widgets that were
// never placed in a window, or were taken out of one, are destroyed
// bottom-up when Run returns; widgets in a closed window only once.
func TestRunDestroysLeftovers(t *testing.T) {
	var lone *Label
	var orphan *Stack
	var leaf *orderPeer
	var old, shown *Label
	runApp(t, func(app *App) {
		lone = NewLabel("never placed")
		leaf = &orderPeer{Button: stub.NewButton(nil)}
		orphan = VStack(0, newFake(kindButton, leaf))
		leaf.parent = orphan.peerWidget().(*stub.Container)
		old, shown = NewLabel("old"), NewLabel("shown")
		w := app.NewWindow(helloCfg)
		w.SetContent(old)
		w.SetContent(shown) // old is taken out, not destroyed
		if old.dead {
			t.Error("replaced content destroyed before Run returned")
		}
	})
	for name, w := range map[string]Widget{"lone": lone, "orphan": orphan, "old": old, "shown": shown} {
		if !w.base().dead || !stubBase(w).Destroyed {
			t.Errorf("%s: not destroyed after Run", name)
		}
	}
	if !leaf.Destroyed || !leaf.parentAlive {
		t.Errorf("orphan tree: leaf destroyed %v, container alive then %v", leaf.Destroyed, leaf.parentAlive)
	}
}
