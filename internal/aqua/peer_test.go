package aqua

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// startDriver returns an aqua driver on a started fake shell. It is not
// Run: the peer tests need only the shell's text system.
func startDriver(t *testing.T) *Driver {
	t.Helper()
	return newDriver(startShell(t))
}

// nodeOf returns the accessible node of w's element (no tree states).
func nodeOf(w driver.Widget) string {
	n := ElementOf(w).AccessibleNode()
	return strings.TrimSuffix(n.String(), " @0,0 0x0")
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
		t.Error("ElementOf of a peer")
	}
	if l.(*Label).StubPeer() != driver.Widget(l.(*Label).Label) || l.(*Label).Stub() != &l.(*Label).Label.Base {
		t.Error("StubPeer or Stub")
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

// TestFocusKinds: text fields and text views are FocusText, the other
// focusable controls FocusControl (S3.2e).
func TestFocusKinds(t *testing.T) {
	d := startDriver(t)
	for _, c := range []struct {
		w    driver.Widget
		kind FocusKind
	}{
		{d.NewButton(nil), FocusControl}, // a real control since S3.3e, still FocusControl
		{d.NewCheckbox(nil), FocusControl},
		{d.NewRadio(nil), FocusControl},
		{d.NewSlider(nil), FocusControl},
		{d.NewPopUpButton(nil), FocusControl},
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
