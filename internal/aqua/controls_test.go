package aqua

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// startDriver returns an aqua driver on a started fake shell. It is not
// Run: the placeholder tests need only the shell's text system.
func startDriver(t *testing.T) *Driver {
	t.Helper()
	return New(startShell(t))
}

// captionOf returns what w's placeholder draws with unlimited room.
func captionOf(t *testing.T, w driver.Widget) string {
	t.Helper()
	s, ok := Caption(w)
	if !ok {
		t.Fatalf("%T is not a placeholder peer", w)
	}
	return s
}

// nodeOf returns the accessible node of w's element (no tree states).
func nodeOf(w driver.Widget) string {
	n := ElementOf(w).AccessibleNode()
	return strings.TrimSuffix(n.String(), " @0,0 0x0")
}

func wantCaption(t *testing.T, w driver.Widget, want, node string) {
	t.Helper()
	if got := captionOf(t, w); got != want {
		t.Errorf("%T caption %q, want %q", w, got, want)
	}
	if got := nodeOf(w); got != node {
		t.Errorf("%T node %q, want %q", w, got, node)
	}
}

// paintPeer paints w's element alone at frame r and returns the dump.
func paintPeer(t *testing.T, w driver.Widget, r driver.Rect) string {
	t.Helper()
	w.SetFrame(r)
	return dump(t, ElementOf(w), driver.Size{W: 400, H: 300})
}

func TestElementOf(t *testing.T) {
	d := startDriver(t)
	if ElementOf(nil) != nil || ElementOf(stub.NewLabel()) != nil {
		t.Error("ElementOf of a nil or stub widget")
	}
	c := d.NewContainer()
	if ElementOf(c) != Element(c.(*Container)) {
		t.Error("ElementOf of a container")
	}
	l := d.NewLabel()
	if e := ElementOf(l); e == nil || e != l.(*Label).AquaElement() {
		t.Error("ElementOf of a placeholder")
	}
	if _, ok := Caption(c); ok {
		t.Error("Caption of a container")
	}
	if l.(*Label).StubPeer() != driver.Widget(l.(*Label).Label) || l.(*Label).Stub() != &l.(*Label).Label.Base {
		t.Error("StubPeer or Stub")
	}
}

func TestPlaceholderCaptionsAndNodes(t *testing.T) {
	d := startDriver(t)

	tf := d.NewTextField(false, nil)
	wantCaption(t, tf, "TextField", `textfield ""`)
	tf.SetPlaceholder("Name")
	wantCaption(t, tf, "TextField: Name", `textfield "Name"`)
	tf.SetText("Ann")
	tf.SetEditable(false)
	wantCaption(t, tf, "TextField: Ann", `textfield "Name" value="Ann" [readonly]`)
	tf.SetEditable(true)
	tf.(*TextField).UserType("Bob")
	wantCaption(t, tf, "TextField: Bob", `textfield "Name" value="Bob"`)

	sec := d.NewTextField(true, nil)
	sec.SetPlaceholder("Password")
	wantCaption(t, sec, "SecureField: Password", `textfield "Password"`)
	sec.SetText("pä!")
	wantCaption(t, sec, "SecureField: •••", `textfield "Password"`)

	pop := d.NewPopUpButton(nil)
	wantCaption(t, pop, "PopUp", `popup "" {showmenu}`)
	pop.SetItems([]string{"Red", "Blue"})
	wantCaption(t, pop, "PopUp: Red", `popup "" value="Red" {showmenu}`)
	pop.SetSelected(1)
	wantCaption(t, pop, "PopUp: Blue", `popup "" value="Blue" {showmenu}`)
	pop.(*PopUpButton).UserSelect(0)
	wantCaption(t, pop, "PopUp: Red", `popup "" value="Red" {showmenu}`)

	tv := d.NewTextView(nil)
	wantCaption(t, tv, "TextView", `textarea ""`)
	tv.SetText("first\nsecond")
	tv.SetEditable(false)
	wantCaption(t, tv, "TextView: first", "textarea \"\" value=\"first\\nsecond\" [readonly]")
	tv.SetEditable(true)
	tv.(*TextView).UserType("!")
	if got := tv.Text(); got != "first\nsecond!" {
		t.Errorf("text view after UserType: %q", got)
	}
}

func TestPlaceholderPaint(t *testing.T) {
	d := startDriver(t)
	b := d.NewPopUpButton(nil)
	b.SetItems([]string{"OK"})
	checkDump(t, paintPeer(t, b, driver.Rect{X: 10, Y: 20, W: 100, H: 22}),
		"fillRRect (10,20,100,22) r=4 solid #ffffff",
		"strokeRRect (10.5,20.5,99,21) r=4 w=1 solid #00000042",
		`text (14,24.231,53.308,13.538) "PopUp: OK" 11pt #000000d9`,
	)
	// Too narrow for "PopUp: OK": the title alone, truncated if need be.
	checkDump(t, paintPeer(t, b, driver.Rect{W: 20, H: 22}),
		"fillRRect (0,0,20,22) r=4 solid #ffffff",
		"strokeRRect (0.5,0.5,19,21) r=4 w=1 solid #00000042",
		`text (4,4.231,11.846,13.538) "OK" 11pt #000000d9`,
	)
	b.SetItems([]string{"Cancel"})
	if got := paintPeer(t, b, driver.Rect{W: 20, H: 22}); !strings.Contains(got, `"C…"`) {
		t.Errorf("narrow pop-up:\n%s", got)
	}
	// Without an item the kind is truncated instead.
	b.SetItems(nil)
	if got := paintPeer(t, b, driver.Rect{W: 30, H: 22}); !strings.Contains(got, `"Po…"`) {
		t.Errorf("untitled narrow pop-up:\n%s", got)
	}
	// Not even the ellipsis fits, or no room at all: only the rectangle.
	checkDump(t, paintPeer(t, b, driver.Rect{W: 9, H: 22}),
		"fillRRect (0,0,9,22) r=4 solid #ffffff",
		"strokeRRect (0.5,0.5,8,21) r=4 w=1 solid #00000042",
	)
	checkDump(t, paintPeer(t, b, driver.Rect{W: 8, H: 22}),
		"fillRRect (0,0,8,22) r=4 solid #ffffff",
		"strokeRRect (0.5,0.5,7,21) r=3.5 w=1 solid #00000042",
	)
	// Too low for the text; too thin for the border; empty.
	checkDump(t, paintPeer(t, b, driver.Rect{W: 100, H: 10}),
		"fillRRect (0,0,100,10) r=4 solid #ffffff",
		"strokeRRect (0.5,0.5,99,9) r=4 w=1 solid #00000042",
	)
	checkDump(t, paintPeer(t, b, driver.Rect{W: 100, H: 1}),
		"fillRRect (0,0,100,1) r=0.5 solid #ffffff",
	)
	checkDump(t, paintPeer(t, b, driver.Rect{W: 100}))

	// Disabled: dimmed text, also through a disabled ancestor.
	b.SetItems([]string{"OK"})
	b.SetEnabled(false)
	if got := paintPeer(t, b, driver.Rect{W: 100, H: 22}); !strings.Contains(got, `"PopUp: OK" 11pt #00000040`) {
		t.Errorf("disabled pop-up:\n%s", got)
	}
	if !b.(*PopUpButton).Disabled || ElementOf(b).base().Enabled() {
		t.Error("SetEnabled reached only one side")
	}
	b.SetEnabled(true)
}

func TestPlaceholderWidgetMethods(t *testing.T) {
	s := startShell(t)
	d := New(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	w.SetAppearance(driver.Appearance{Dark: true, Accent: driver.DefaultAccent})
	c := d.NewContainer()
	c.SetFrame(driver.Rect{W: 200, H: 100})
	tf := d.NewTextField(false, nil).(*TextField)
	lbl := d.NewLabel().(*Label)
	c.SetChildren([]driver.Widget{lbl, tf})
	w.SetContent(c)
	tf.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 22})
	if tf.Frame != (driver.Rect{X: 10, Y: 10, W: 100, H: 22}) || tf.AquaElement().base().Frame() != tf.Frame {
		t.Errorf("SetFrame: stub %v, element %v", tf.Frame, tf.AquaElement().base().Frame())
	}
	// Dark: the window's appearance, not the driver's.
	got := paint(t, s, sf)
	if !strings.Contains(got, "fillRRect (10,38,100,22) r=4 solid #ffffff40") {
		t.Errorf("dark placeholder:\n%s", got)
	}
	// Focus reaches the element of a focusable placeholder and is counted.
	tf.Focus()
	lbl.Focus()
	if tf.FocusCalls != 1 || lbl.FocusCalls != 1 || !tf.AquaElement().base().HasFocus() {
		t.Errorf("Focus: calls %d %d, focused %v", tf.FocusCalls, lbl.FocusCalls, tf.AquaElement().base().HasFocus())
	}
	tree := AccessibilityTree(w.Root())
	if n := tree.Find(a11y.RoleTextField, ""); n == nil || n.States&a11y.StateFocused == 0 {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
	// Hidden: neither side painted; Destroy leaves both parents.
	tf.SetHidden(true)
	if !tf.Hidden || !tf.AquaElement().base().Hidden() || tf.AquaElement().base().HasFocus() {
		t.Error("SetHidden")
	}
	tf.SetHidden(false)
	tf.Destroy()
	if !tf.Destroyed || tf.AquaElement().base().Parent() != nil || ElementOf(c).base().ChildCount() != 1 {
		t.Error("Destroy")
	}
}

func TestScrollViewFrames(t *testing.T) {
	d := startDriver(t)
	var events []string
	sv := d.NewScrollView(scrollLog{&events}).(*ScrollView)
	doc := d.NewContainer()
	item := d.NewLabel()
	item.SetText("Row")
	doc.SetChildren([]driver.Widget{item})
	sv.SetDocument(doc)
	sv.SetHasVerticalScroller(true)
	sv.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 50})
	sv.SetDocumentSize(driver.Size{W: 100, H: 200})
	doc.SetFrame(driver.Rect{W: 100, H: 200})
	item.SetFrame(driver.Rect{Y: 60, W: 100, H: 16})

	viewport := func() string {
		return fmt.Sprint(sv.viewport.Frame(), " doc ", sv.document.Frame())
	}
	if got := viewport(); got != "{0 0 100 50} doc {0 0 100 200}" {
		t.Errorf("initial: %s", got)
	}
	// The item at y = 60 is outside the 50 pt viewport.
	if got := dump(t, ElementOf(sv), driver.Size{W: 400, H: 300}); strings.Contains(got, `"Row"`) && !strings.Contains(got, "clip=(10,10,100,50)") {
		t.Errorf("unclipped document:\n%s", got)
	}
	sv.SetOffset(driver.Point{Y: 40})
	if got := viewport(); got != "{0 0 100 50} doc {0 -40 100 200}" {
		t.Errorf("offset: %s", got)
	}
	got := dump(t, ElementOf(sv), driver.Size{W: 400, H: 300})
	if !strings.Contains(got, `text (10,30,21,16) "Row" 13pt #000000d9 clip=(10,10,100,50)`) {
		t.Errorf("scrolled document:\n%s", got)
	}
	sv.ScrollRectToVisible(driver.Rect{Y: 190, W: 10, H: 10})
	if sv.Offset() != (driver.Point{Y: 150}) || sv.document.Frame().Y != -150 {
		t.Errorf("ScrollRectToVisible: %v %v", sv.Offset(), sv.document.Frame())
	}
	sv.UserScroll(driver.Point{Y: 10})
	if sv.document.Frame().Y != -10 || strings.Join(events, " ") != "scrolled 10" {
		t.Errorf("UserScroll: %v %v", sv.document.Frame(), events)
	}
	sv.UserResizeViewport(driver.Insets{Right: 15})
	if got := viewport(); got != "{0 0 85 50} doc {0 -10 100 200}" {
		t.Errorf("viewport inset: %s", got)
	}
	sv.SetDocument(nil)
	if sv.document.ChildCount() != 0 {
		t.Error("SetDocument(nil) kept the element")
	}
}

type scrollLog struct{ log *[]string }

func (s scrollLog) Scrolled(p driver.Point) {
	*s.log = append(*s.log, "scrolled "+strconv.FormatFloat(p.Y, 'g', -1, 64))
}
func (s scrollLog) ViewportResized(driver.Size) {}

// TestPlaceholderFocusKinds: text fields and text views are FocusText, the
// other focusable placeholders FocusControl (S3.2e).
func TestPlaceholderFocusKinds(t *testing.T) {
	d := startDriver(t)
	for _, c := range []struct {
		w    driver.Widget
		kind FocusKind
	}{
		{d.NewButton(nil), FocusControl}, // a real control since S3.3e, still FocusControl
		{d.NewCheckbox(nil), FocusControl},
		{d.NewTextField(false, nil), FocusText},
		{d.NewTextField(true, nil), FocusText},
		{d.NewTextView(nil), FocusText},
	} {
		e := ElementOf(c.w).base()
		if !e.Focusable() || e.FocusKind() != c.kind {
			t.Errorf("%T: focusable %v, kind %v, want %v", c.w, e.Focusable(), e.FocusKind(), c.kind)
		}
	}
}
