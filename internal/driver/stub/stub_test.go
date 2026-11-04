package stub

import (
	"sync"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
)

// Recording handlers.

type clicks struct{ n int }

func (c *clicks) Clicked() { c.n++ }

type toggles struct{ got []driver.CheckState }

func (t *toggles) StateChanged(s driver.CheckState) { t.got = append(t.got, s) }

type fieldEvents struct{ changed, submitted []string }

func (f *fieldEvents) TextChanged(s string) { f.changed = append(f.changed, s) }
func (f *fieldEvents) Submitted(s string)   { f.submitted = append(f.submitted, s) }

type sliderEvents struct{ got []float64 }

func (s *sliderEvents) ValueChanged(v float64) { s.got = append(s.got, v) }

type popUpEvents struct{ got []int }

func (p *popUpEvents) SelectionChanged(i int) { p.got = append(p.got, i) }

type scrollEvents struct{ got []driver.Point }

func (s *scrollEvents) Scrolled(p driver.Point)     { s.got = append(s.got, p) }
func (s *scrollEvents) ViewportResized(driver.Size) {}

type textViewEvents struct{ n int }

func (t *textViewEvents) TextChanged() { t.n++ }

type windowEvents struct {
	resized []driver.Size
	allow   bool
	asked   int
	closed  int
	key     []bool
}

func (w *windowEvents) ContentResized(s driver.Size) { w.resized = append(w.resized, s) }
func (w *windowEvents) ShouldClose() bool            { w.asked++; return w.allow }
func (w *windowEvents) Closed()                      { w.closed++ }
func (w *windowEvents) KeyChanged(k bool)            { w.key = append(w.key, k) }

type appEvents struct {
	ready      int
	appearance []driver.Appearance
	allowQuit  bool
	menu       []int
}

func (a *appEvents) Ready()                                { a.ready++ }
func (a *appEvents) AppearanceChanged(x driver.Appearance) { a.appearance = append(a.appearance, x) }
func (a *appEvents) QuitRequested() bool                   { return a.allowQuit }
func (a *appEvents) MenuItemSelected(id int)               { a.menu = append(a.menu, id) }

func TestBaseState(t *testing.T) {
	b := NewButton(nil)
	b.SetFrame(driver.Rect{X: 1, Y: 2, W: 3, H: 4})
	b.SetEnabled(false)
	b.SetHidden(true)
	b.SetControlSize(driver.ControlSizeSmall)
	b.SetTooltip("tip")
	b.Focus()
	if b.Frame != (driver.Rect{X: 1, Y: 2, W: 3, H: 4}) || !b.Disabled || !b.Hidden ||
		b.ControlSize != driver.ControlSizeSmall || b.Tooltip != "tip" || b.FocusCalls != 1 {
		t.Errorf("state not recorded: %+v", b.Base)
	}
}

func TestFittingSize(t *testing.T) {
	inf := driver.Unbounded()
	b := NewButton(nil)
	b.SetTitle("OK") // 2 runes × 7 + 28
	if got, want := b.FittingSize(inf), (driver.Size{W: 42, H: 22}); got != want {
		t.Errorf("button = %v, want %v", got, want)
	}
	b.SetControlSize(driver.ControlSizeSmall) // 11/13 scale, height 19
	if got := b.FittingSize(inf); got.H != 19 || got.W >= 42 {
		t.Errorf("small button = %v", got)
	}
	if got := b.FittingSize(driver.Size{W: 10, H: 100}); got.W != 10 {
		t.Errorf("FittingSize must clamp to max: %v", got)
	}
	b.Fixed = driver.Size{W: 5, H: 6}
	if got := b.FittingSize(inf); got != b.Fixed {
		t.Errorf("Fixed = %v", got)
	}

	l := NewLabel()
	l.SetText("abcdefghij") // 70 wide
	if got, want := l.FittingSize(driver.Size{W: 30, H: 1000}), (driver.Size{W: 30, H: 16}); got != want {
		t.Errorf("non-wrapping label = %v, want %v (clamped)", got, want)
	}
	l.SetWraps(true)
	if got, want := l.FittingSize(driver.Size{W: 30, H: 1000}), (driver.Size{W: 30, H: 48}); got != want {
		t.Errorf("wrapping label = %v, want %v", got, want)
	}
	l.SetFont(driver.Font{Size: 26})
	l.SetWraps(false)
	if got, want := l.FittingSize(inf), (driver.Size{W: 140, H: 32}); got != want {
		t.Errorf("26 pt label = %v, want %v", got, want)
	}

	c := NewCheckbox(nil)
	c.SetTitle("On")
	if got, want := c.FittingSize(inf), (driver.Size{W: 32, H: 18}); got != want {
		t.Errorf("checkbox = %v, want %v", got, want)
	}

	p := NewPopUpButton(nil)
	p.SetItems([]string{"a", "abcd"})
	if got := p.FittingSize(inf).W; got != 28+32 {
		t.Errorf("pop-up width = %v", got)
	}

	box := NewBox()
	box.SetTitle("Group")
	if got, want := box.ContentInsets(), (driver.Insets{Top: 24, Left: 8, Bottom: 8, Right: 8}); got != want {
		t.Errorf("box insets = %v, want %v", got, want)
	}

	if got := NewContainer().FittingSize(inf); got != (driver.Size{}) {
		t.Errorf("container = %v", got)
	}
}

func TestButton(t *testing.T) {
	h := &clicks{}
	b := NewButton(h)
	b.SetTitle("Go")
	b.SetDefault(true)
	b.SetCancel(true)
	b.UserClick()
	b.SetEnabled(false)
	b.UserClick()
	if h.n != 1 || b.Title != "Go" || !b.IsDefault || !b.IsCancel {
		t.Errorf("clicks=%d button=%+v", h.n, b)
	}
}

func TestCheckbox(t *testing.T) {
	h := &toggles{}
	c := NewCheckbox(h)
	c.SetState(driver.StateMixed) // no event
	c.UserClick()                 // mixed → on
	c.UserClick()                 // on → off
	c.UserClick()                 // off → on
	want := []driver.CheckState{driver.StateOn, driver.StateOff, driver.StateOn}
	if len(h.got) != len(want) {
		t.Fatalf("events = %v, want %v", h.got, want)
	}
	for i := range want {
		if h.got[i] != want[i] {
			t.Fatalf("events = %v, want %v", h.got, want)
		}
	}
	if c.State() != driver.StateOn {
		t.Errorf("state = %v", c.State())
	}
}

func TestRadio(t *testing.T) {
	h := &toggles{}
	r := NewRadio(h)
	r.SetOn(true)
	r.UserClick() // reported even when already on
	if len(h.got) != 1 || !r.On() {
		t.Errorf("events=%v on=%v", h.got, r.On())
	}
}

func TestTextField(t *testing.T) {
	h := &fieldEvents{}
	f := NewTextField(true, h)
	f.SetText("set") // no event
	f.UserType("typed")
	f.UserSubmit()
	f.SetEditable(false)
	f.UserType("ignored")
	if !f.Secure || f.Text() != "typed" || len(h.changed) != 1 || h.submitted[0] != "typed" {
		t.Errorf("field=%+v events=%+v", f, h)
	}
}

func TestSliderAndProgressClamp(t *testing.T) {
	h := &sliderEvents{}
	s := NewSlider(h)
	s.SetRange(-40, 100)
	s.SetValue(500)
	if s.Value() != 100 {
		t.Errorf("SetValue not clamped: %v", s.Value())
	}
	s.SetRange(0, 10)
	if s.Value() != 10 {
		t.Errorf("SetRange did not clamp the value: %v", s.Value())
	}
	s.UserDrag(-3)
	if len(h.got) != 1 || h.got[0] != 0 {
		t.Errorf("drag events = %v", h.got)
	}

	p := NewProgress()
	if p.Max != 100 {
		t.Errorf("default progress range = %v…%v", p.Min, p.Max)
	}
	p.SetRange(0, 1)
	p.SetValue(2)
	if p.Value != 1 {
		t.Errorf("progress value = %v", p.Value)
	}
}

func TestPopUpButton(t *testing.T) {
	h := &popUpEvents{}
	p := NewPopUpButton(h)
	if p.Selected() != -1 {
		t.Errorf("empty pop-up selection = %d", p.Selected())
	}
	items := []string{"Red", "Green", "Blue"}
	p.SetItems(items)
	items[0] = "changed"
	if p.Items[0] != "Red" || p.Selected() != 0 {
		t.Errorf("SetItems: items=%v sel=%d", p.Items, p.Selected())
	}
	p.SetSelected(2)
	p.SetItems([]string{"A", "B"}) // selection out of range → 0
	if p.Selected() != 0 {
		t.Errorf("selection after shrink = %d", p.Selected())
	}
	p.UserSelect(1)
	p.UserSelect(5)
	if len(h.got) != 1 || h.got[0] != 1 || p.Selected() != 1 {
		t.Errorf("events=%v sel=%d", h.got, p.Selected())
	}
	p.SetSelected(9)
	if p.Selected() != -1 {
		t.Errorf("invalid SetSelected = %d", p.Selected())
	}
}

func TestContainerParenting(t *testing.T) {
	a, b := NewLabel(), NewButton(nil)
	c1, c2 := NewContainer(), NewContainer()

	c1.SetChildren([]driver.Widget{a, b})
	if !a.HasParent() || len(c1.Children) != 2 {
		t.Fatal("children not adopted")
	}

	// Moving a widget to another parent removes it from the old one.
	c2.SetChildren([]driver.Widget{a})
	if len(c1.Children) != 1 || c1.Children[0] != driver.Widget(b) {
		t.Errorf("c1 children = %v", c1.Children)
	}

	// Dropping a child releases it without destroying it.
	c1.SetChildren(nil)
	if b.HasParent() || b.Destroyed {
		t.Error("dropped child must be released, not destroyed")
	}

	// Destroy removes the widget from its parent.
	a.Destroy()
	if len(c2.Children) != 0 || !a.Destroyed {
		t.Errorf("after Destroy: children=%v destroyed=%v", c2.Children, a.Destroyed)
	}
}

func TestSingleChildParents(t *testing.T) {
	l := NewLabel()
	box := NewBox()
	box.SetContent(l)

	sv := NewScrollView(nil)
	sv.SetDocument(l) // moves l out of the box
	if box.Content != nil || sv.Document != driver.Widget(l) {
		t.Fatalf("box=%v doc=%v", box.Content, sv.Document)
	}

	w := NewWindow(driver.WindowConfig{}, nil)
	w.SetContent(l)
	if sv.Document != nil {
		t.Fatal("scroll view still holds the moved widget")
	}
	l.Destroy()
	if w.Content != nil {
		t.Error("window still holds the destroyed widget")
	}
}

func TestScrollView(t *testing.T) {
	h := &scrollEvents{}
	s := NewScrollView(h)
	s.SetFrame(driver.Rect{W: 100, H: 50})
	s.SetDocumentSize(driver.Size{W: 100, H: 500})
	s.SetOffset(driver.Point{X: 30, Y: 1000})
	if got, want := s.Offset(), (driver.Point{Y: 450}); got != want {
		t.Errorf("clamped offset = %v, want %v", got, want)
	}
	s.ScrollRectToVisible(driver.Rect{Y: 100, W: 10, H: 20})
	if got := s.Offset().Y; got != 100 {
		t.Errorf("scroll up to rect: y = %v", got)
	}
	s.ScrollRectToVisible(driver.Rect{Y: 200, W: 10, H: 20})
	if got := s.Offset().Y; got != 170 {
		t.Errorf("scroll down to rect: y = %v", got)
	}
	s.ScrollRectToVisible(driver.Rect{Y: 180, W: 10, H: 10}) // already visible
	if got := s.Offset().Y; got != 170 {
		t.Errorf("visible rect moved the offset: y = %v", got)
	}
	s.SetDocumentSize(driver.Size{W: 100, H: 60})
	if got := s.Offset().Y; got != 10 {
		t.Errorf("shrinking the document must clamp: y = %v", got)
	}
	if len(h.got) != 0 {
		t.Errorf("programmatic scrolling reported events: %v", h.got)
	}
	s.UserScroll(driver.Point{Y: -5})
	if len(h.got) != 1 || h.got[0] != (driver.Point{}) {
		t.Errorf("user scroll events = %v", h.got)
	}
}

func TestTextView(t *testing.T) {
	h := &textViewEvents{}
	v := NewTextView(h)
	v.SetText("héllo")
	if s, e := v.Selection(); s != 6 || e != 6 {
		t.Errorf("caret after SetText = %d,%d", s, e)
	}
	v.SetSelection(5, 2) // 2 is inside "é" (bytes 1–2) → 1; swapped
	if s, e := v.Selection(); s != 1 || e != 5 {
		t.Errorf("selection = %d,%d, want 1,5", s, e)
	}
	v.UserType("E")
	if v.Text() != "hEo" || h.n != 1 {
		t.Errorf("text=%q events=%d", v.Text(), h.n)
	}
	if s, e := v.Selection(); s != 2 || e != 2 {
		t.Errorf("caret after typing = %d,%d", s, e)
	}
	v.SetSelection(-5, 99)
	if s, e := v.Selection(); s != 0 || e != 3 {
		t.Errorf("clamped selection = %d,%d", s, e)
	}
}

func TestWindow(t *testing.T) {
	h := &windowEvents{}
	w := NewWindow(driver.WindowConfig{
		Title:   "T",
		Size:    driver.Size{W: 50, H: 500},
		MinSize: driver.Size{W: 100, H: 100},
		MaxSize: driver.Size{H: 400},
	}, h)
	if got, want := w.ContentSize(), (driver.Size{W: 100, H: 400}); got != want {
		t.Errorf("initial size = %v, want %v", got, want)
	}
	w.UserResize(driver.Size{W: 300, H: 50})
	if got, want := h.resized[0], (driver.Size{W: 300, H: 100}); got != want {
		t.Errorf("resize = %v, want %v", got, want)
	}

	w.Show()
	if !w.Visible || !w.IsKey || len(h.key) != 1 {
		t.Errorf("Show: visible=%v key=%v events=%v", w.Visible, w.IsKey, h.key)
	}

	if w.UserClose() || h.asked != 1 || h.closed != 0 {
		t.Error("ShouldClose=false must keep the window open")
	}
	h.allow = true
	if !w.UserClose() || h.closed != 1 {
		t.Error("UserClose did not close")
	}
	w.Close()
	if h.closed != 1 {
		t.Error("Closed must be called once")
	}
}

func TestDriverWindowsAndQuit(t *testing.T) {
	d := NewDriver()
	h1, h2 := &windowEvents{}, &windowEvents{}
	pw1, _ := d.NewWindow(driver.WindowConfig{}, h1)
	pw2, _ := d.NewWindow(driver.WindowConfig{}, h2)
	w1, w2 := pw1.(*Window), pw2.(*Window)

	w1.Show()
	w2.Show() // w1 resigns key
	if d.KeyWindow() != w2 || w1.IsKey || len(h1.key) != 2 {
		t.Errorf("key window handling: key=%v h1=%v", d.KeyWindow(), h1.key)
	}

	w2.Close()
	if len(d.Windows) != 1 {
		t.Errorf("closed window still tracked: %d", len(d.Windows))
	}

	d.Quit()
	if len(d.Windows) != 0 || h1.closed != 1 || !d.Quitting() {
		t.Errorf("Quit must close every window: open=%d closed=%d", len(d.Windows), h1.closed)
	}
}

func TestDriverAppearance(t *testing.T) {
	d := NewDriver()
	h := &appEvents{}
	d.Handler = h

	d.SetAppearance(driver.AppearanceDark)
	if !d.Appearance().Dark || len(h.appearance) != 1 {
		t.Fatalf("dark: %+v events=%d", d.Appearance(), len(h.appearance))
	}
	d.SetSystemAppearance(driver.Appearance{Dark: true, Accent: driver.DefaultAccent})
	if len(h.appearance) != 1 {
		t.Error("no change must not be reported")
	}
	d.SetAppearance(driver.AppearanceLight) // forced light over a dark system
	if d.Appearance().Dark || len(h.appearance) != 2 {
		t.Errorf("light: %+v", d.Appearance())
	}
	d.SetAppearance(driver.AppearanceSystem)
	if !d.Appearance().Dark {
		t.Error("system mode must follow the simulated OS")
	}
}

func TestDriverMenuBar(t *testing.T) {
	d := NewDriver()
	h := &appEvents{}
	d.Handler = h

	d.SetMenuBar(nil)
	if !d.MenuBar.HasMenu(driver.MenuEdit) {
		t.Fatal("nil must install the default menu bar")
	}

	bar := &driver.MenuBarSpec{Menus: []driver.MenuSpec{{Title: "File", Items: []driver.MenuItemSpec{
		{ID: 1, Title: "New"}, {ID: 2, Title: "Off", Disabled: true},
	}}}}
	d.SetMenuBar(bar)
	bar.Menus[0].Items[0].ID = 99 // the driver must hold a copy

	if !d.UserSelectMenuItem(1) || d.UserSelectMenuItem(2) || d.UserSelectMenuItem(99) {
		t.Error("menu selection results are wrong")
	}
	if len(h.menu) != 1 || h.menu[0] != 1 {
		t.Errorf("menu events = %v", h.menu)
	}

	if d.UserQuit() || d.Quitting() {
		t.Error("QuitRequested=false must cancel")
	}
	h.allowQuit = true
	if !d.UserQuit() || !d.Quitting() {
		t.Error("UserQuit did not quit")
	}
}

func TestDriverAlert(t *testing.T) {
	d := NewDriver()
	buttons := []string{"Delete", "Cancel"}
	result := -1
	d.RunAlert(nil, driver.AlertSpec{Message: "m", Buttons: buttons, Cancel: 1}, func(r int) { result = r })
	buttons[0] = "changed"
	if len(d.Alerts) != 1 || d.Alerts[0].Spec.Buttons[0] != "Delete" {
		t.Fatalf("alerts = %+v", d.Alerts)
	}
	a := d.Alerts[0]
	a.Dismiss(1)
	a.Dismiss(0) // second dismiss is ignored
	if result != 1 || len(d.Alerts) != 0 {
		t.Errorf("result=%d open=%d", result, len(d.Alerts))
	}
	d.RunAlert(nil, driver.AlertSpec{Buttons: []string{"OK"}}, nil)
	d.Alerts[0].Dismiss(0) // nil done is allowed
}

func TestDriverRunAndPost(t *testing.T) {
	d := NewDriver()
	h := &appEvents{}

	var order []int
	var mu sync.Mutex
	add := func(i int) { mu.Lock(); order = append(order, i); mu.Unlock() }

	done := make(chan error, 1)
	go func() {
		// Run on its own goroutine here; the real drivers need the main
		// thread, the stub does not.
		done <- d.Run(driver.AppConfig{Name: "Test"}, h)
	}()

	var wg sync.WaitGroup
	for range 10 { // Post is safe from any goroutine
		wg.Add(1)
		go func() { defer wg.Done(); d.Post(func() {}) }()
	}
	wg.Wait()
	d.Post(func() {
		add(1)
		d.Post(func() { add(3); d.Quit() }) // posted from the loop
		add(2)
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Quit")
	}
	if h.ready != 1 || d.Config.Name != "Test" {
		t.Errorf("ready=%d config=%+v", h.ready, d.Config)
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("order = %v", order)
	}
}

func TestDrain(t *testing.T) {
	d := NewDriver()
	n := 0
	d.Post(func() { n++; d.Post(func() { n++ }) })
	if got := d.Drain(); got != 2 || n != 2 {
		t.Errorf("Drain = %d, n = %d", got, n)
	}
}
