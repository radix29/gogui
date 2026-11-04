package aqua

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The shared harness of the scroller tests (PLAN.md S3.5e, S3.5f).

// scrollEnv is a running driver with one 300 × 200 window whose content
// (a container, surface y + 28) holds a scroll view framed at
// (10, 10, 150, 100) with a vertical scroller and a 150 × 400 document
// (offset 0…300). Points passed to the fake surface are in surface
// coordinates: content + (0, 28).
type scrollEnv struct {
	d     *Driver
	sh    *fake.Shell
	w     *Window
	sf    *fake.Surface
	clk   *fake.Clock
	c     *Container
	sv    *ScrollView
	doc   *Container
	h     *svLog
	frame driver.Rect // the scroll view's frame in the content
}

// svLog records a ScrollView's handler calls.
type svLog struct {
	scrolled []driver.Point
	resized  []driver.Size
}

func (l *svLog) Scrolled(p driver.Point)       { l.scrolled = append(l.scrolled, p) }
func (l *svLog) ViewportResized(s driver.Size) { l.resized = append(l.resized, s) }

// scrollRun runs f on a started driver with the window shown. sys is the
// shell's appearance before the driver starts (LegacyScrollers, Reduce
// Motion …).
func scrollRun(t *testing.T, sys driver.Appearance, f func(e *scrollEnv)) {
	t.Helper()
	sh := fake.New()
	sh.SetAppearance(sys)
	d := New(sh)
	h := &appLog{}
	h.onReady = func() {
		defer d.Quit()
		w, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		e := &scrollEnv{d: d, sh: sh, w: w.(*Window), clk: sh.FakeClock(), h: &svLog{}}
		e.sf = e.w.Surface().(*fake.Surface)
		e.c = d.NewContainer().(*Container)
		e.c.SetFrame(driver.Rect{W: 300, H: 172})
		e.sv = d.NewScrollView(e.h).(*ScrollView)
		e.doc = d.NewContainer().(*Container)
		e.sv.SetDocument(e.doc)
		e.sv.SetHasVerticalScroller(true)
		e.c.SetChildren([]driver.Widget{e.sv})
		w.SetContent(e.c)
		e.frame = driver.Rect{X: 10, Y: 10, W: 150, H: 100}
		e.sv.SetFrame(e.frame)
		e.sv.SetDocumentSize(driver.Size{W: 150, H: 400})
		e.doc.SetFrame(driver.Rect{W: 150, H: 400})
		e.h.resized = nil
		w.Show()
		e.clk.Advance(0)
		f(e)
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}

// at returns the surface point of the content point (x, y) of the scroll
// view (its own coordinates).
func (e *scrollEnv) at(x, y float64) driver.Point {
	return driver.Point{X: e.frame.X + x, Y: 28 + e.frame.Y + y}
}

// vbar returns the vertical scroller.
func (e *scrollEnv) vbar() *scroller { return e.sv.bars[1] }

// hbar returns the horizontal scroller.
func (e *scrollEnv) hbar() *scroller { return e.sv.bars[0] }
