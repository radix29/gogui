package gallery_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/gallery"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// updateGolden rewrites testdata/gallery.dump:
//
//	CGO_ENABLED=0 go test -run TestGalleryDump -update ./internal/aqua/gallery
var updateGolden = flag.Bool("update", false, "rewrite the golden dump in testdata")

// recDriver is the aqua driver with every widget its factories create
// recorded, so the tests can check them all.
type recDriver struct {
	*aqua.Driver
	widgets []driver.Widget
}

func (d *recDriver) rec(w driver.Widget) { d.widgets = append(d.widgets, w) }

func (d *recDriver) NewContainer() driver.Container {
	w := d.Driver.NewContainer()
	d.rec(w)
	return w
}
func (d *recDriver) NewLabel() driver.Label {
	w := d.Driver.NewLabel()
	d.rec(w)
	return w
}
func (d *recDriver) NewButton(h driver.ButtonHandler) driver.Button {
	w := d.Driver.NewButton(h)
	d.rec(w)
	return w
}
func (d *recDriver) NewCheckbox(h driver.ToggleHandler) driver.Checkbox {
	w := d.Driver.NewCheckbox(h)
	d.rec(w)
	return w
}
func (d *recDriver) NewRadio(h driver.ToggleHandler) driver.Radio {
	w := d.Driver.NewRadio(h)
	d.rec(w)
	return w
}
func (d *recDriver) NewTextField(secure bool, h driver.TextFieldHandler) driver.TextField {
	w := d.Driver.NewTextField(secure, h)
	d.rec(w)
	return w
}
func (d *recDriver) NewSlider(h driver.SliderHandler) driver.Slider {
	w := d.Driver.NewSlider(h)
	d.rec(w)
	return w
}
func (d *recDriver) NewProgress() driver.Progress {
	w := d.Driver.NewProgress()
	d.rec(w)
	return w
}
func (d *recDriver) NewPopUpButton(h driver.PopUpHandler) driver.PopUpButton {
	w := d.Driver.NewPopUpButton(h)
	d.rec(w)
	return w
}
func (d *recDriver) NewSeparator() driver.Separator {
	w := d.Driver.NewSeparator()
	d.rec(w)
	return w
}
func (d *recDriver) NewBox() driver.Box {
	w := d.Driver.NewBox()
	d.rec(w)
	return w
}
func (d *recDriver) NewScrollView(h driver.ScrollHandler) driver.ScrollView {
	w := d.Driver.NewScrollView(h)
	d.rec(w)
	return w
}
func (d *recDriver) NewTextView(h driver.TextViewHandler) driver.TextView {
	w := d.Driver.NewTextView(h)
	d.rec(w)
	return w
}

type app struct{ ready func() }

func (a app) Ready()                            { a.ready() }
func (app) AppearanceChanged(driver.Appearance) {}
func (app) QuitRequested() bool                 { return true }
func (app) MenuItemSelected(int)                {}

type winHandler struct{}

func (winHandler) ContentResized(driver.Size) {}
func (winHandler) ShouldClose() bool          { return true }
func (winHandler) Closed()                    {}
func (winHandler) KeyChanged(bool)            {}

// opened is the gallery shown in a window of the aqua driver over the
// fake shell, painted once.
type opened struct {
	d  *recDriver
	s  *fake.Shell
	w  *aqua.Window
	sf *fake.Surface
}

// withGallery opens the gallery with Reduce Motion and legacy scrollers
// (as the golden renders do), in light or dark, and calls fn on the UI
// goroutine once it is painted and no animation runs.
func withGallery(t *testing.T, dark bool, fn func(g opened)) {
	t.Helper()
	s := fake.New()
	a := driver.DefaultAppearance()
	a.ReduceMotion, a.LegacyScrollers = true, true
	s.SetAppearance(a)
	ad := aqua.New(s)
	d := &recDriver{Driver: ad}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-stop:
		case <-time.After(10 * time.Second):
			panic("gallery: Driver.Run did not return")
		}
	}()
	err := ad.Run(driver.AppConfig{Name: "Gallery"}, app{ready: func() {
		defer ad.Quit()
		if dark {
			ad.SetAppearance(driver.AppearanceDark)
		}
		win, err := gallery.Open(d, winHandler{})
		if err != nil {
			t.Error(err)
			return
		}
		w := win.(*aqua.Window)
		sf := w.Surface().(*fake.Surface)
		for range 10 {
			if !s.FakeClock().Flush() {
				break
			}
		}
		if n := ad.Timeline().Running(); n != 0 {
			t.Errorf("%d animations still run", n)
		}
		if err := sf.PaintErr(); err != nil {
			t.Errorf("paint errors: %v", err)
		}
		fn(opened{d, s, w, sf})
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func frameOf(e aqua.Element) driver.Rect {
	return e.(interface{ Frame() driver.Rect }).Frame()
}

func childrenOf(e aqua.Element) []aqua.Element {
	return e.(interface{ Children() []aqua.Element }).Children()
}

func inside(r, bounds driver.Rect) bool {
	return r.X >= bounds.X && r.Y >= bounds.Y && r.X+r.W <= bounds.X+bounds.W && r.Y+r.H <= bounds.Y+bounds.H
}

func overlap(a, b driver.Rect) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

// TestGalleryScene checks the scene's structure on the aqua driver: every
// widget is a real control (no placeholder), every kind the scene promises
// is there, the children of every container lie inside it without
// overlapping, every frame is at least the control's fitting height, and
// the states Open sets (focus, scroll offset) hold.
func TestGalleryScene(t *testing.T) {
	withGallery(t, false, func(g opened) {
		kinds := map[string]int{}
		for _, w := range g.d.widgets {
			if c, ok := aqua.Caption(w); ok {
				t.Errorf("placeholder in the scene: %s", c)
			}
			kinds[strings.TrimPrefix(fmt.Sprintf("%T", w), "*aqua.")]++
		}
		for k, n := range map[string]int{
			"Container": 3, "Label": 29, "Button": 7, "Checkbox": 5, "Radio": 4,
			"Slider": 3, "Progress": 3, "Separator": 2, "Box": 1, "ScrollView": 1,
		} {
			if kinds[k] != n {
				t.Errorf("%d %s widgets, want %d", kinds[k], k, n)
			}
		}
		if len(kinds) != 10 {
			t.Errorf("widget kinds %v", kinds)
		}

		// The root is the window's content, at (0, 0, Size).
		root := g.w.Content()
		if root == nil {
			t.Fatal("no content")
		}
		if got := frameOf(root); got != (driver.Rect{W: gallery.Size.W, H: gallery.Size.H}) {
			t.Errorf("root frame %v", got)
		}
		if got := g.w.ContentSize(); got != gallery.Size {
			t.Errorf("content size %v", got)
		}

		// Containers: children inside, no two overlapping.
		containers := 0
		var walk func(e aqua.Element)
		walk = func(e aqua.Element) {
			cs := childrenOf(e)
			if _, ok := e.(*aqua.Container); ok {
				containers++
				b := frameOf(e)
				b.X, b.Y = 0, 0
				for i, c := range cs {
					r := frameOf(c)
					if r.Empty() || !inside(r, b) {
						t.Errorf("%T at %v outside its container %v", c, r, b)
					}
					for _, o := range cs[:i] {
						if overlap(r, frameOf(o)) {
							t.Errorf("%T at %v overlaps %T at %v", c, r, o, frameOf(o))
						}
					}
				}
			}
			for _, c := range cs {
				walk(c)
			}
		}
		walk(root)
		if containers != 3 {
			t.Errorf("%d containers in the tree, want 3", containers)
		}

		for _, w := range g.d.widgets {
			e := aqua.ElementOf(w)
			r := frameOf(e)
			switch w := w.(type) {
			case *aqua.Container:
				continue
			case *aqua.ScrollView:
				if got := w.Offset(); got != gallery.ScrollOffset {
					t.Errorf("scroll offset %v, want %v", got, gallery.ScrollOffset)
				}
				continue
			case *aqua.Box:
				// The content lies inside the content area.
				in := w.ContentInsets()
				area := driver.Rect{W: r.W - in.Horizontal(), H: r.H - in.Vertical()}
				for _, c := range childrenOf(e) {
					for _, cc := range childrenOf(c) {
						if !inside(frameOf(cc), area) {
							t.Errorf("box content at %v outside %v", frameOf(cc), area)
						}
					}
				}
			}
			fit := w.FittingSize(driver.Unbounded())
			if fit.H > r.H {
				t.Errorf("%T at %v: fitting height %v", w, r, fit.H)
			}
			switch w.(type) {
			case *aqua.Label, *aqua.Button, *aqua.Checkbox, *aqua.Radio:
				if fit.W > r.W {
					t.Errorf("%T at %v: fitting width %v", w, r, fit.W)
				}
			}
		}

		tree := aqua.AccessibilityTree(g.w.Root())
		n := tree.Find(a11y.RoleCheckBox, "Focused")
		if n == nil || n.States&a11y.StateFocused == 0 || !g.w.IsKey() {
			t.Errorf("the Focused checkbox is not focused in the key window:\n%s", tree.Dump())
		}
	})
}

// TestGalleryBuild checks that Build gives the same scene as Open without
// a window: a container of size Size.
func TestGalleryBuild(t *testing.T) {
	withGallery(t, false, func(g opened) {
		root, size := gallery.Build(g.d)
		if size != gallery.Size {
			t.Errorf("size %v", size)
		}
		if _, ok := root.(*aqua.Container); !ok {
			t.Errorf("root %T", root)
		}
		if n := len(childrenOf(aqua.ElementOf(root))); n != len(childrenOf(g.w.Content())) {
			t.Errorf("Build has %d children", n)
		}
	})
}

// TestGalleryFill checks that Fill gives Open's scene in a window it does
// not show, with the Focused checkbox focused.
func TestGalleryFill(t *testing.T) {
	withGallery(t, false, func(g opened) {
		win, err := g.d.NewWindow(driver.WindowConfig{Title: gallery.Title, Size: gallery.Size, Style: gallery.Style},
			winHandler{})
		if err != nil {
			t.Fatal(err)
		}
		defer win.Close()
		gallery.Fill(g.d, win)
		w := win.(*aqua.Window)
		if w.Shown() {
			t.Error("Fill showed the window")
		}
		if got := frameOf(w.Content()); got != (driver.Rect{W: gallery.Size.W, H: gallery.Size.H}) {
			t.Errorf("root frame %v", got)
		}
		if n, want := len(childrenOf(w.Content())), len(childrenOf(g.w.Content())); n != want {
			t.Errorf("Fill has %d children, want %d", n, want)
		}
		tree := aqua.AccessibilityTree(w.Root())
		if n := tree.Find(a11y.RoleCheckBox, "Focused"); n == nil || n.States&a11y.StateFocused == 0 {
			t.Errorf("the Focused checkbox is not focused:\n%s", tree.Dump())
		}
	})
}

// TestGalleryDump pins the recorder dump of the whole gallery window in
// light and dark (testdata/gallery.dump).
func TestGalleryDump(t *testing.T) {
	var out strings.Builder
	for _, dark := range []bool{false, true} {
		withGallery(t, dark, func(g opened) {
			name := "light"
			if dark {
				name = "dark"
			}
			fmt.Fprintf(&out, "## %s, %v\n%s\n", name, g.sf.Size(), g.sf.Dump())
		})
	}
	got := out.String()
	path := filepath.Join("testdata", "gallery.dump")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to pin the dump)", err)
	}
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); got != w {
		t.Errorf("gallery dump differs from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}
