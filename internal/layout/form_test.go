package layout

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// lbl returns a form label: 7 pt per rune × 16 pt.
func lbl(text string) *stub.Label { return label(text, driver.Font{}) }

func radio(title string) *Leaf {
	r := stub.NewRadio(nil)
	r.Title = title
	return ctl(r)
}

func TestFormColumns(t *testing.T) {
	name, pass := lbl("Name:"), lbl("Password:")
	field := ctl(stub.NewTextField(false, nil))
	check := stub.NewCheckbox(nil)
	check.Title = "Remember" // 74 × 18
	ok := button("OK")       // 42 × 22
	f := NewForm(Row{name, field}, Row{pass, ctl(check)}, Row{Control: ctl(ok)})
	if len(f.Rows()) != 3 || f.Rows()[2].Label != nil {
		t.Fatal("rows")
	}
	// Column 63 ("Password:") + 8; widest control 96; rows 22, 18, 22 + 2 × 8.
	wantSize(t, "natural", f.Measure(nil, driver.Unbounded()), driver.Size{W: 167, H: 78})

	verify := func(dx, dy float64) {
		t.Helper()
		wantRect(t, "field fills", field.Item().(*stub.TextField).Frame, driver.Rect{X: dx + 71, Y: dy, W: 229, H: 22})
		wantRect(t, "name right-aligned", name.Frame, driver.Rect{X: dx + 28, Y: dy + 3, W: 35, H: 16})
		wantRect(t, "checkbox natural", check.Frame, driver.Rect{X: dx + 71, Y: dy + 30, W: 74, H: 18})
		wantRect(t, "password", pass.Frame, driver.Rect{X: dx, Y: dy + 31, W: 63, H: 16})
		wantRect(t, "nil label, same x", ok.Frame, driver.Rect{X: dx + 71, Y: dy + 56, W: 42, H: 22})
	}
	f.Arrange(nil, driver.Rect{X: 10, Y: 20, W: 300, H: 78}) // transparent: offsets
	verify(10, 20)
	peer := stub.NewContainer()
	f.SetPeer(peer).Arrange(nil, driver.Rect{X: 10, Y: 20, W: 300, H: 78})
	verify(0, 0)
	if peer.Frame != (driver.Rect{X: 10, Y: 20, W: 300, H: 78}) || f.Peer() != peer {
		t.Errorf("peer frame = %v", peer.Frame)
	}

	// Without any label there is no column and no gap.
	g := NewForm(Row{Control: ctl(button("A"))})
	wantSize(t, "no labels", g.Measure(nil, driver.Unbounded()), driver.Size{W: 35, H: 22})
}

func TestFormRowHeights(t *testing.T) {
	tall, pt := pl(50, 40)
	short, ps := pl(50, 10)
	plan := NewVStack(6, radio("Free"), radio("Pro")) // 42 high, first line 18
	low, pw := pl(50, 10)
	low.Props().SetAlign(Vertical, Trailing)
	lt, ls, lp, lw := lbl("Tall:"), lbl("Short:"), lbl("Plan:"), lbl("Low:")
	f := NewForm(Row{lt, tall}, Row{ls, short}, Row{lp, plan}, Row{lw, low})
	// Column 42 ("Short:") + 8. Rows: 40, max(16, 10) = 16, 42, 16 + 3 × 8.
	wantSize(t, "measure", f.Measure(nil, driver.Unbounded()), driver.Size{W: 100, H: 138})
	f.Arrange(nil, driver.Rect{W: 100, H: 138})
	wantFrame(t, "tall", pt, driver.Rect{X: 50, W: 50, H: 40})
	wantRect(t, "tall label centered", lt.Frame, driver.Rect{X: 7, Y: 12, W: 35, H: 16})
	wantFrame(t, "short at row top", ps, driver.Rect{X: 50, Y: 48, W: 50, H: 10})
	wantRect(t, "short label not above row", ls.Frame, driver.Rect{Y: 48, W: 42, H: 16})
	wantRect(t, "plan label on first radio", lp.Frame, driver.Rect{X: 7, Y: 73, W: 35, H: 16})
	// Trailing in its 16 pt row; the label centers on it (125 = 128 + (10 − 16)/2).
	wantFrame(t, "low trailing", pw, driver.Rect{X: 50, Y: 128, W: 50, H: 10})
	wantRect(t, "low label", lw.Frame, driver.Rect{X: 14, Y: 125, W: 28, H: 16})
}

func TestFormStretchAndAlignment(t *testing.T) {
	full := ctl(stub.NewTextField(false, nil))
	capped := ctl(stub.NewTextField(false, nil))
	capped.Props().SetMax(Horizontal, 150)
	centered := ctl(stub.NewTextField(false, nil))
	centered.Props().SetAlign(Horizontal, Center)
	notes := ctl(stub.NewTextView(nil)) // expands on both axes
	f := NewForm(Row{lbl("A:"), full}, Row{lbl("B:"), capped}, Row{lbl("C:"), centered}, Row{lbl("Notes:"), notes})
	if !f.Expands(Horizontal) || !f.Expands(Vertical) {
		t.Error("expansion not propagated")
	}
	f.Arrange(nil, driver.Rect{W: 300, H: 200})
	// Column 42 + 8; control column 250. Rows 22 × 3 + 3 × 8 = 90; notes gets 110.
	wantRect(t, "full", full.Item().(*stub.TextField).Frame, driver.Rect{X: 50, W: 250, H: 22})
	wantRect(t, "capped", capped.Item().(*stub.TextField).Frame, driver.Rect{X: 50, Y: 30, W: 150, H: 22})
	wantRect(t, "centered", centered.Item().(*stub.TextField).Frame, driver.Rect{X: 127, Y: 60, W: 96, H: 22})
	wantRect(t, "notes fills", notes.Item().(*stub.TextView).Frame, driver.Rect{X: 50, Y: 90, W: 250, H: 110})

	notes.Props().SetMax(Vertical, 10) // below the label: the row is 16, the control 10
	f.Arrange(nil, driver.Rect{W: 300, H: 200})
	wantRect(t, "notes capped", notes.Item().(*stub.TextView).Frame, driver.Rect{X: 50, Y: 90, W: 250, H: 10})
}

func TestFormHiddenAndShort(t *testing.T) {
	field := ctl(stub.NewTextField(false, nil))
	field.Props().SetMin(Horizontal, 40)
	hid, ph := pl(10, 10)
	hid.Props().SetHidden(true)
	wide, name := lbl("A very long label:"), lbl("Name:")
	b, pb := pl(20, 10)
	f := NewForm(Row{wide, hid}, Row{name, field}, Row{Control: b})
	// The hidden row's label doesn't count; no spacing for it.
	wantSize(t, "natural", f.Measure(nil, driver.Unbounded()), driver.Size{W: 139, H: 40})
	wantSize(t, "minimum", f.Measure(nil, driver.Size{H: inf}), driver.Size{W: 83, H: 40})
	f.Arrange(nil, driver.Rect{W: 100, H: 40}) // shrinks the field only
	wantRect(t, "field shrunk", field.Item().(*stub.TextField).Frame, driver.Rect{X: 43, W: 57, H: 22})
	wantRect(t, "label kept", name.Frame, driver.Rect{Y: 3, W: 35, H: 16})
	wantFrame(t, "b", pb, driver.Rect{X: 43, Y: 30, W: 20, H: 10})
	f.Arrange(nil, driver.Rect{W: 60, H: 30}) // overflow at the trailing edges
	wantRect(t, "field at min", field.Item().(*stub.TextField).Frame, driver.Rect{X: 43, W: 40, H: 22})
	wantFrame(t, "b overflows", pb, driver.Rect{X: 43, Y: 30, W: 20, H: 10})
	if ph.measures != 0 || ph.frames != 0 || wide.Frame != (driver.Rect{}) {
		t.Error("hidden row measured or framed")
	}

	field.Props().SetHidden(true)
	b.Props().SetHidden(true)
	f.Props().SetMin(Vertical, 5)
	wantSize(t, "all hidden", f.Measure(nil, driver.Unbounded()), driver.Size{H: 5})
	f.Arrange(nil, driver.Rect{W: 9, H: 9})
	if got := f.FirstLine(nil, driver.Unbounded()); got != 5 {
		t.Errorf("empty first line = %v", got)
	}
	peer := stub.NewContainer()
	f.SetPeer(peer).Props().SetHidden(true)
	f.Arrange(nil, driver.Rect{W: 9, H: 9})
	if f.Measure(nil, driver.Unbounded()) != (driver.Size{}) || peer.Frame != (driver.Rect{}) || f.FirstLine(nil, driver.Unbounded()) != 0 {
		t.Error("hidden form")
	}
}

func TestFormInVStack(t *testing.T) {
	hidden, _ := pl(10, 99)
	hidden.Props().SetHidden(true)
	field := ctl(stub.NewTextField(false, nil))
	plan := NewVStack(6, radio("Free"), radio("Pro"))
	form := NewForm(Row{lbl("Hidden:"), hidden}, Row{lbl("Plan:"), plan}, Row{lbl("E-mail:"), field})
	ok := button("OK")
	col, cp := withPeer(NewVStack(16, form, NewSpacer(), NewHStack(0, NewSpacer(), ctl(ok))))
	col.SetPadding(driver.Insets{Top: 3})
	NewRoot(col).Layout(driver.Size{W: 300, H: 200})
	wantRect(t, "column", cp.Frame, driver.Rect{W: 300, H: 200})
	// The form fills the width because the field expands.
	wantRect(t, "field", field.Item().(*stub.TextField).Frame, driver.Rect{X: 57, Y: 53, W: 243, H: 22})
	wantRect(t, "ok at bottom", ok.Frame, driver.Rect{X: 258, Y: 178, W: 42, H: 22})
	// The form's first line is its first visible row's: the first radio.
	if got := firstLine(nil, col, driver.Size{W: 300, H: 200}); got != 21 {
		t.Errorf("first line = %v, want 21", got)
	}
	tallLabel := NewForm(Row{label("Big:", driver.Font{Size: 26}), field})
	if got := tallLabel.FirstLine(nil, driver.Unbounded()); got != 32 {
		t.Errorf("tall label first line = %v, want 32", got)
	}
}

func TestFormSettersAndPanics(t *testing.T) {
	f := NewForm().SetDirection(RightToLeft)
	if f.Direction() != RightToLeft || f.Peer() != nil || len(f.Rows()) != 0 || f.Expands(Horizontal) {
		t.Error("getters")
	}
	if f.Props().SetExpand(Vertical, true); !f.Expands(Vertical) {
		t.Error("own flag ignored")
	}
	a, _ := pl(10, 10)
	f.SetRows(Row{lbl("X:"), a})
	wantSize(t, "rtl as ltr", f.Measure(nil, driver.Unbounded()), driver.Size{W: 32, H: 16})
	defer func() {
		if recover() == nil {
			t.Error("nil control did not panic")
		}
	}()
	NewForm(Row{Label: lbl("X:")})
}
