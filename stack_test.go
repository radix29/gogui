package gogui

import (
	"math"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/layout"
)

// fake is a test widget around a stub peer, standing in for the controls
// that later steps add (S0.5g–i).
type fake struct{ common[*fake] }

func newFake(k kind, p driver.Widget) *fake {
	f := &fake{}
	f.init(f, k, p, nil)
	return f
}

func fakeButton(title string) *fake {
	b := stub.NewButton(nil)
	b.Title = title
	return newFake(kindButton, b)
}

func fakeField() *fake { return newFake(kindTextField, stub.NewTextField(false, nil)) }

func fakeCheckbox(title string) *fake {
	c := stub.NewCheckbox(nil)
	c.Title = title
	return newFake(kindCheckbox, c)
}

func wantFrame(t testing.TB, name string, w Widget, want Rect) {
	t.Helper()
	got := frameOf(w)
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if !near(got.X, want.X) || !near(got.Y, want.Y) || !near(got.W, want.W) || !near(got.H, want.H) {
		t.Errorf("%s: frame = %v, want %v", name, got, want)
	}
}

// frameOf returns the frame last given to w's peer: an aqua element's
// (a stack or form on the aqua driver) or the stub state's.
func frameOf(w Widget) Rect {
	if e, ok := w.peerWidget().(interface{ Frame() driver.Rect }); ok {
		return e.Frame()
	}
	return stubBase(w).Frame
}

// stubPeer returns w's peer, or the stub peer inside an aqua
// peer (examples_test.go runs the example trees on both drivers).
func stubPeer(w Widget) driver.Widget {
	p := w.peerWidget()
	if sp, ok := p.(interface{ StubPeer() driver.Widget }); ok {
		return sp.StubPeer()
	}
	return p
}

// stubBase returns the stub state of w's peer.
func stubBase(w Widget) *stub.Base {
	switch p := stubPeer(w).(type) {
	case *stub.Label:
		return &p.Base
	case *stub.Button:
		return &p.Base
	case *stub.TextField:
		return &p.Base
	case *stub.Checkbox:
		return &p.Base
	case *stub.Radio:
		return &p.Base
	case *stub.PopUpButton:
		return &p.Base
	case *stub.Container:
		return &p.Base
	case *stub.Box:
		return &p.Base
	case *stub.ScrollView:
		return &p.Base
	case *stub.TextView:
		return &p.Base
	case *stub.Separator:
		return &p.Base
	case *stub.Slider:
		return &p.Base
	case *stub.Progress:
		return &p.Base
	}
	panic("stubBase: unexpected peer")
}

// children returns the peers a stack or form gave its container peer.
func children(w Widget) []driver.Widget { return w.peerWidget().(*stub.Container).Children }

func wantChildren(t *testing.T, name string, w Widget, want ...Widget) {
	t.Helper()
	got := children(w)
	ok := len(got) == len(want)
	for i := 0; ok && i < len(got); i++ {
		ok = got[i] == want[i].peerWidget()
	}
	if !ok {
		t.Errorf("%s: container children %v, want the peers of %v", name, got, want)
	}
}

func layoutAt(w Widget, size Size) { layout.NewRoot(w.node()).LayoutMin(size) }

func TestHelloTree(t *testing.T) {
	runApp(t, func(*App) {
		greeting := NewLabel("Hello!").SetFont(BoldSystemFont(15))
		name, greet := fakeField(), fakeButton("Greet")
		row := HStack(8, Spacer(), greet)
		col := VStack(12, greeting, name, row).SetPadding(20)
		wantChildren(t, "vstack", col, greeting, name, row)
		wantChildren(t, "hstack", row, greet) // the spacer has no peer
		layoutAt(col, Size{W: 360, H: 180})
		gw, gh := 630.0/13, 240.0/13
		nameY := 20 + gh + 12
		wantFrame(t, "vstack", col, Rect{W: 360, H: 180})
		wantFrame(t, "greeting", greeting, Rect{X: 20, Y: 20, W: gw, H: gh})
		wantFrame(t, "name", name, Rect{X: 20, Y: nameY, W: 320, H: 22})
		wantFrame(t, "hstack", row, Rect{X: 20, Y: nameY + 34, W: 320, H: 22})
		wantFrame(t, "greet", greet, Rect{X: 257, W: 63, H: 22})
	})
}

func TestFormTree(t *testing.T) {
	runApp(t, func(*App) {
		name, email := fakeField(), fakeField()
		password := newFake(kindTextField, stub.NewTextField(true, nil))
		pop := stub.NewPopUpButton(nil)
		pop.SetItems([]string{"Viewer", "Editor", "Administrator"})
		role := newFake(kindPopUp, pop)
		var radios []Widget
		for _, s := range []string{"Free", "Pro", "Team"} {
			r := stub.NewRadio(nil)
			r.Title = s
			radios = append(radios, newFake(kindRadio, r))
		}
		plan := VStack(6, radios...) // a RadioGroup is a VStack of radios
		newsletter := fakeCheckbox("Send me product updates")
		terms := fakeCheckbox("I agree to the terms of service")
		cancel, create := fakeButton("Cancel"), fakeButton("Create Account")
		rows := []FormRow{Row("Name:", name), Row("E-mail:", email), Row("Password:", password),
			Row("Role:", role), Row("Plan:", plan), Row("", newsletter), Row("", terms)}
		form := Form(rows...)
		buttons := HStack(8, Spacer(), cancel, create)
		col := VStack(16, form, Spacer(), buttons).SetPadding(20)

		// Row labels get a Label peer, placed before their control; "" none.
		var want []Widget
		for _, r := range form.rows {
			if r.label != nil {
				want = append(want, r.label)
			}
			want = append(want, r.control)
		}
		if len(want) != 12 || form.rows[5].label != nil {
			t.Fatalf("%d form children, want 12 (5 labels)", len(want))
		}
		wantChildren(t, "form", form, want...)
		wantChildren(t, "vstack", col, form, buttons)

		layoutAt(col, Size{W: 460, H: 380})
		wantFrame(t, "form", form, Rect{X: 20, Y: 20, W: 420, H: 238})
		labels := []Rect{{X: 28, Y: 3, W: 35, H: 16}, {X: 14, Y: 33, W: 49, H: 16},
			{X: 0, Y: 63, W: 63, H: 16}, {X: 28, Y: 93, W: 35, H: 16}, {X: 28, Y: 121, W: 35, H: 16}}
		for i, r := range labels {
			wantFrame(t, form.rows[i].label.Text(), form.rows[i].label, r)
		}
		wantFrame(t, "name", name, Rect{X: 71, W: 349, H: 22})
		wantFrame(t, "password", password, Rect{X: 71, Y: 60, W: 349, H: 22})
		wantFrame(t, "role", role, Rect{X: 71, Y: 90, W: 7*13 + 32, H: 22})
		wantFrame(t, "plan", plan, Rect{X: 71, Y: 120, W: 46, H: 66})
		wantFrame(t, "Pro", radios[1], Rect{Y: 24, W: 39, H: 18})
		wantFrame(t, "terms", terms, Rect{X: 71, Y: 220, W: 18 + 31*7, H: 18})
		wantFrame(t, "buttons", buttons, Rect{X: 20, Y: 338, W: 420, H: 22})
		wantFrame(t, "cancel", cancel, Rect{X: 216, W: 70, H: 22})
		wantFrame(t, "create", create, Rect{X: 294, W: 126, H: 22})
	})
}

func TestStackAddAndReparent(t *testing.T) {
	runApp(t, func(*App) {
		a, b, c := NewLabel("a"), NewLabel("b"), NewLabel("c")
		s := VStack(8)
		if got := s.Add(a).Add(b, c); got != s {
			t.Fatal("Add doesn't chain")
		}
		wantChildren(t, "after Add", s, a, b, c)
		other := HStack(4, b) // moves b
		wantChildren(t, "old parent", s, a, c)
		wantChildren(t, "new parent", other, b)
		if b.parent != Widget(other) || a.parent != Widget(s) {
			t.Error("parent links")
		}
		s.Add(a) // re-adding moves it to the end
		wantChildren(t, "re-added", s, c, a)
		if n := len(s.node().(*layout.Stack).Children()); n != 2 {
			t.Errorf("layout node has %d children, want 2", n)
		}
		// Moving a control out of a form drops its row and destroys the label.
		f := Form(Row("Name:", a), Row("", c))
		lbl := f.rows[0].label
		wantChildren(t, "form", f, lbl, a, c)
		wantChildren(t, "stack after the form took a and c", s)
		s.Add(a)
		wantChildren(t, "form after a moved", f, c)
		if !stubLabel(t, lbl).Destroyed || len(f.f.Rows()) != 1 {
			t.Error("the moved control's row label survived")
		}
		// The same FormRow in a second form (A4): the control moves, and each
		// form has its own label; the first form's label goes with its row.
		r := Row("Plan:", b)
		f1 := Form(r)
		l1 := f1.rows[0].label
		f2 := Form(r)
		l2 := f2.rows[0].label
		wantChildren(t, "first form", f1)
		wantChildren(t, "second form", f2, l2, b)
		if l1 == l2 || !stubLabel(t, l1).Destroyed || stubLabel(t, l2).Destroyed || len(f1.rows) != 0 || l2.Text() != "Plan:" {
			t.Error("reusing a FormRow: labels not per form, or a row left behind")
		}
		for _, bad := range []func(){
			func() { s.Add(nil) },
			func() { s.Add(b, b) },
			func() { s.Add(s) },
			func() { inner := VStack(0); s.Add(inner); inner.Add(s) },
			func() { Row("x", nil) },
			func() { Form(FormRow{}) },
		} {
			func() {
				defer func() {
					if r, _ := recover().(string); !strings.HasPrefix(r, "gogui: ") {
						t.Errorf("recover = %q, want a gogui panic", r)
					}
				}()
				bad()
			}()
		}
	})
}

func TestHiddenChildren(t *testing.T) {
	runApp(t, func(*App) {
		a, b, c := fakeButton("A"), fakeButton("B"), fakeButton("C")
		s := HStack(10, a, b, c)
		b.SetHidden(true)
		wantChildren(t, "hidden child keeps its peer", s, a, b, c)
		layoutAt(s, Size{W: 300, H: 22})
		if stubBase(b).Frame != (Rect{}) {
			t.Error("a hidden child was framed")
		}
		wantFrame(t, "C next to A", c, Rect{X: stubBase(a).Frame.W + 10, W: stubBase(c).Frame.W, H: 22})

		// A hidden form control hides its row label too, and showing it again
		// shows the label.
		name, email := fakeField(), fakeField()
		f := Form(Row("Name:", name), Row("E-mail:", email))
		name.SetHidden(true)
		lbl := f.rows[0].label
		if !stubLabel(t, lbl).Hidden || !lbl.node().Props().Hidden() {
			t.Error("the row label of a hidden control is visible")
		}
		layoutAt(f, Size{W: 300, H: 100})
		wantFrame(t, "email moves up", email, Rect{X: 57, W: 243, H: 22})
		name.SetHidden(false)
		if stubLabel(t, lbl).Hidden {
			t.Error("the row label stayed hidden")
		}
	})
}

func TestStackSettersAndInvalidate(t *testing.T) {
	runApp(t, func(*App) {
		s := VStack(0)
		h := &countingHost{}
		s.host = h
		var got *Stack = s.SetPadding(5).SetPaddingInsets(Insets{Top: 1, Left: 2, Bottom: 3, Right: 4}).
			Add(NewLabel("x")).SetHidden(false)
		if got != s || s.s.Padding() != (Insets{Top: 1, Left: 2, Bottom: 3, Right: 4}) {
			t.Errorf("chain or padding: %v", s.s.Padding())
		}
		if h.n != 4 {
			t.Errorf("invalidated %d times, want 4", h.n)
		}
		var fv *FormView = Form().SetTooltip("t")
		if fv.kind != kindForm || s.kind != kindStack || Spacer().peerWidget() != nil {
			t.Error("kinds or spacer peer")
		}
	})
}
