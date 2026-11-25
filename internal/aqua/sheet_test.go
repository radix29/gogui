package aqua

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/menubar"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The sheets (PLAN.md S5.7c), over a running driver on the fake shell.

// sheetEnv is a running driver with roleSpec as its bar and one shown
// 400×300 window: a container with a default button at content
// (10, 10, 80, 20) and a text field at (10, 40, 150, 22), focused.
type sheetEnv struct {
	t     *testing.T
	d     *Driver
	sh    *fake.Shell
	clk   *fake.Clock
	w     *Window
	sf    *fake.Surface
	h     *winLog
	btn   *Button
	field *TextField
	fl    *fieldLog
	n     int // the content button's clicks
	got   []string
}

// sheetRun runs f on a started driver with the window shown and painted;
// sys is the shell's appearance.
func sheetRun(t *testing.T, sys driver.Appearance, f func(e *sheetEnv)) {
	t.Helper()
	sh := fake.New()
	sh.SetAppearance(sys)
	d := New(sh)
	d.SetMenuBar(roleSpec)
	h := &appLog{}
	h.onReady = func() {
		defer d.Quit()
		e := &sheetEnv{t: t, d: d, sh: sh, clk: sh.FakeClock(), h: &winLog{}, fl: &fieldLog{}}
		dw, err := d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 400, H: 300}, Style: allStyles}, e.h)
		if err != nil {
			t.Fatal(err)
		}
		e.w = dw.(*Window)
		e.sf = e.w.Surface().(*fake.Surface)
		c := d.NewContainer().(*Container)
		c.SetFrame(driver.Rect{W: 400, H: 300})
		e.btn = d.NewButton(clickCounter{&e.n}).(*Button)
		e.btn.SetTitle("Go")
		e.btn.SetDefault(true)
		e.btn.SetFrame(driver.Rect{X: 10, Y: 10, W: 80, H: 20})
		e.field = d.NewTextField(false, e.fl).(*TextField)
		e.field.SetFrame(driver.Rect{X: 10, Y: 40, W: 150, H: 22})
		c.SetChildren([]driver.Widget{e.btn, e.field})
		e.w.SetContent(c)
		e.w.Show()
		e.w.Router().SetFocus(e.field.view)
		e.settle()
		e.h.take()
		f(e)
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
}

// settle advances a second, so every slide ends, and fails unless no
// sheet's ticker runs (the field's caret blink keeps the clock going).
func (e *sheetEnv) settle() {
	e.t.Helper()
	e.clk.Advance(time.Second)
	for _, s := range e.w.sheets {
		if s.ticker != nil && s.ticker.Running() {
			e.t.Fatal("still sliding after a second")
		}
	}
	if err := e.sf.PaintErr(); err != nil {
		e.t.Fatal(err)
	}
}

// show runs an alert on the window that logs its answers.
func (e *sheetEnv) show(spec driver.AlertSpec) {
	e.d.RunAlert(e.w, spec, func(i int) { e.got = append(e.got, fmt.Sprintf("%s=%d", spec.Message, i)) })
}

// answers returns and clears the logged answers.
func (e *sheetEnv) answers() string {
	s := strings.Join(e.got, " ")
	e.got = nil
	return s
}

// sheetButton returns the centre of the shown sheet's button i in surface
// points.
func (e *sheetEnv) sheetButton(i int) driver.Point {
	v := e.w.shownSheet().alert.Button(i).view
	p := ToRoot(v, driver.Point{X: v.Bounds().W / 2, Y: v.Bounds().H / 2})
	return p.Add(e.w.SheetOrigin())
}

// panelTop returns the surface y of the shown sheet's panel.
func (e *sheetEnv) panelTop() float64 {
	return e.w.SheetOrigin().Y + e.w.shownSheet().alert.ShadowMargin().Top
}

// hasRegion reports whether the surface has a chrome region of kind k.
func hasRegion(sf *fake.Surface, k shell.RegionKind) bool {
	return slices.ContainsFunc(sf.ChromeRegions(), func(r shell.ChromeRegion) bool { return r.Kind == k })
}

// TestSheetSlide: the sheet slides down from behind the title bar over
// sheetDuration, centred, its panel hanging from the title bar's bottom
// edge; it is painted between the content and the title bar; the answer
// slides it back up and the app goes idle.
func TestSheetSlide(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.show(twoButtonAlert)
		s := e.w.shownSheet()
		if s == nil || !e.w.Modal() || e.w.layer.Hidden() {
			t.Fatal("no sheet shown")
		}
		e.clk.Advance(100 * time.Millisecond)
		if s.p <= 0.3 || s.p >= 0.7 || e.panelTop() >= tokens.Chrome.TitleBarHeight {
			t.Errorf("half-way: progress %g, panel top %g", s.p, e.panelTop())
		}
		e.settle()
		if s.p != 1 || e.panelTop() != tokens.Chrome.TitleBarHeight {
			t.Errorf("shown: progress %g, panel top %g", s.p, e.panelTop())
		}
		o, sz := e.w.SheetOrigin(), s.root.Frame().Size()
		if o.X+sz.W/2 != 200 {
			t.Errorf("not centred: origin %v size %v", o, sz)
		}
		dump := e.sf.Dump()
		content, msg, title := strings.Index(dump, `"Go"`), strings.Index(dump, `"Delete the item?"`), strings.LastIndex(dump, `"Edit"`)
		if content < 0 || msg < content || title < msg {
			t.Errorf("paint order: content %d, sheet %d, title %d\n%s", content, msg, title, dump)
		}

		e.sf.Click(e.sheetButton(0))
		if e.answers() != "Delete the item?=0" || e.w.Modal() {
			t.Fatal("click on the default button did not answer")
		}
		if e.w.shownSheet() != s {
			t.Fatal("the answered sheet left before its slide-out")
		}
		e.clk.Advance(100 * time.Millisecond)
		if s.p <= 0.3 || s.p >= 0.7 {
			t.Errorf("sliding out: progress %g", s.p)
		}
		e.settle()
		if e.w.shownSheet() != nil || !e.w.layer.Hidden() || len(e.d.Alerts()) != 0 {
			t.Error("the sheet did not go")
		}
		if strings.Contains(e.sf.Dump(), "Delete the item?") {
			t.Error("the sheet is still painted")
		}
	})
}

// TestSheetReduceMotion: with Reduce Motion the sheet fades in and out in
// place over the same time.
func TestSheetReduceMotion(t *testing.T) {
	a := driver.DefaultAppearance()
	a.ReduceMotion = true
	sheetRun(t, a, func(e *sheetEnv) {
		e.show(oneButtonAlert)
		s := e.w.shownSheet()
		e.clk.Advance(100 * time.Millisecond)
		if s.p <= 0.3 || s.p >= 0.7 || e.panelTop() != tokens.Chrome.TitleBarHeight {
			t.Errorf("half-way: progress %g, panel top %g", s.p, e.panelTop())
		}
		if !strings.Contains(e.sf.Dump(), "pushOpacity") {
			t.Error("no fade layer half-way")
		}
		e.settle()
		if strings.Contains(e.sf.Dump(), "pushOpacity") || !strings.Contains(e.sf.Dump(), `"Saved"`) {
			t.Error("shown: still fading, or not painted")
		}
		e.sf.Key(driver.KeyReturn, 0)
		e.clk.Advance(100 * time.Millisecond)
		if s.p <= 0.3 || s.p >= 0.7 || !strings.Contains(e.sf.Dump(), "pushOpacity") {
			t.Errorf("fading out: progress %g", s.p)
		}
		e.settle()
		if e.w.shownSheet() != nil || e.answers() != "Saved=0" {
			t.Error("the sheet did not go")
		}
	})
}

// TestSheetModality: while the sheet is up the content takes no pointer or
// key input, keeps its focus drawn inactive, and the arrow shows over it;
// the close light is disabled, minimize and zoom work; after the answer
// all of it is back.
func TestSheetModality(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		btnAt, fieldAt := pt(50, 48), pt(80, 79)
		e.sf.Move(fieldAt)
		if e.sf.Cursor() != shell.CursorIBeam {
			t.Fatalf("cursor over the field %v", e.sf.Cursor())
		}
		e.show(twoButtonAlert)
		e.settle()
		if e.sf.Cursor() != shell.CursorArrow {
			t.Errorf("cursor over the content with a sheet: %v", e.sf.Cursor())
		}
		if e.w.Router().Focused() != Element(e.field.view) || e.field.view.FocusState() != FocusedInactive {
			t.Errorf("content focus %v", e.field.view.FocusState())
		}
		if e.w.bar.lights[lightClose].Enabled() || hasRegion(e.sf, shell.RegionClose) || !hasRegion(e.sf, shell.RegionMinimize) {
			t.Error("close light still enabled")
		}

		e.sf.Click(btnAt)
		e.sf.Wheel(fieldAt, pt(0, -10))
		e.sf.Type("x")
		e.sf.Key(driver.KeyTab, 0)
		if e.n != 0 || e.field.Text() != "" || len(e.fl.events) != 0 || e.w.Router().Focused() != Element(e.field.view) {
			t.Errorf("content got input: %d clicks, text %q, focus %v", e.n, e.field.Text(), e.w.Router().Focused())
		}
		if e.answers() != "" {
			t.Fatal("a key for the content answered the sheet")
		}
		if e.sf.UserClose() || e.h.take() != "" {
			t.Error("an OS close was not refused")
		}
		e.w.performClose()
		e.sf.Click(closeAt)
		if e.w.Closed() || e.h.take() != "" {
			t.Error("the close light or performClose acted")
		}
		e.sf.Click(zoomAt)
		if !e.sf.Zoomed() {
			t.Error("zoom did not work")
		}
		e.sf.Activate()

		e.sf.Key(driver.KeyEscape, 0) // the Cancel button
		if e.answers() != "Delete the item?=1" || e.w.Modal() {
			t.Fatal("Escape did not answer Cancel")
		}
		if e.field.view.FocusState() != FocusedActive || !e.w.bar.lights[lightClose].Enabled() || !hasRegion(e.sf, shell.RegionClose) {
			t.Error("focus or close light not restored at the answer")
		}
		e.sf.Type("y")
		if e.field.Text() != "y" {
			t.Errorf("content after the answer: %q", e.field.Text())
		}
		e.settle()
		e.sf.Move(fieldAt)
		e.sf.Click(btnAt)
		if e.sf.Cursor() != shell.CursorArrow || e.n != 1 {
			t.Errorf("content click after the sheet: %d", e.n)
		}
	})
}

// TestSheetKeys: Return answers the default button, Tab moves between the
// sheet's buttons, Space presses the focused one; the content's default
// button gets nothing; a window that loses key makes the sheet inactive.
func TestSheetKeys(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.show(threeButtonAlert)
		e.settle()
		s := e.w.shownSheet()
		if s.root.Router().Focused() != Element(s.alert.Button(1).view) {
			t.Fatal("focus does not start on the neighbour")
		}
		e.sf.Key(driver.KeyTab, 0)
		if s.root.Router().Focused() != Element(s.alert.Button(2).view) {
			t.Error("Tab did not move in the sheet")
		}
		e.sh.Deactivate()
		if s.alert.Button(2).view.FocusState() != FocusedInactive {
			t.Error("sheet focus active in an inactive window")
		}
		e.sf.Activate()
		if s.alert.Button(2).view.FocusState() != FocusedActive || e.field.view.FocusState() != FocusedInactive {
			t.Error("reactivation: wrong focus states")
		}
		e.sf.Key(driver.KeySpace, 0)
		if e.answers() != threeButtonAlert.Message+"=2" || e.n != 0 {
			t.Errorf("Space: content clicks %d", e.n)
		}
		e.settle()

		e.show(oneButtonAlert)
		e.settle()
		if !e.sf.Key(driver.KeyReturn, 0) || e.answers() != "Saved=0" || e.n != 0 {
			t.Errorf("Return: content clicks %d", e.n)
		}
	})
}

// TestSheetQueue: a second alert waits until the first has gone, then
// slides in; a queued alert answered by Dismiss leaves the queue; the
// modality lasts while any alert waits.
func TestSheetQueue(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.show(oneButtonAlert)
		e.show(twoButtonAlert)
		e.show(threeButtonAlert)
		as := e.d.Alerts()
		if len(as) != 3 || as[1].Window() != e.w {
			t.Fatalf("alerts %v", as)
		}
		e.settle()
		first := e.w.shownSheet()
		if first.alert.Spec().Message != "Saved" || as[1].sheet.started {
			t.Fatal("not shown in order")
		}
		as[2].Dismiss(0) // queued: it leaves at once
		if e.answers() != threeButtonAlert.Message+"=0" || len(e.w.sheets) != 2 {
			t.Fatalf("queued dismiss: %d sheets", len(e.w.sheets))
		}
		e.sf.Key(driver.KeyReturn, 0)
		if e.answers() != "Saved=0" || !e.w.Modal() || e.w.shownSheet() != first {
			t.Fatal("the modality ended with an alert waiting")
		}
		e.sf.Type("x")
		if e.field.Text() != "" {
			t.Error("content took text between two sheets")
		}
		e.clk.Advance(sheetDuration + 20*time.Millisecond)
		if e.w.shownSheet() == nil || e.w.shownSheet() == first || !e.w.shownSheet().started {
			t.Fatal("the second sheet did not slide in")
		}
		e.settle()
		as[1].Dismiss(1)
		if e.answers() != "Delete the item?=1" || e.w.Modal() || len(e.d.Alerts()) != 0 {
			t.Error("last answer")
		}
		e.settle()
		if len(e.w.sheets) != 0 {
			t.Error("sheets left")
		}
	})
}

// TestSheetDoneShowsAnother: an alert shown from done queues behind the
// one sliding out.
func TestSheetDoneShowsAnother(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.d.RunAlert(e.w, oneButtonAlert, func(int) { e.show(twoButtonAlert) })
		e.settle()
		e.sf.Key(driver.KeyReturn, 0)
		if !e.w.Modal() || len(e.w.sheets) != 2 {
			t.Fatal("the alert from done is not queued")
		}
		e.settle()
		if e.w.shownSheet() == nil || e.w.shownSheet().alert.Spec().Message != "Delete the item?" {
			t.Fatal("the second alert is not shown")
		}
	})
}

// TestSheetWindowClose: Window.Close answers every alert with its Cancel
// index (0 without one) in queue order before the window goes; an alert
// shown on the closing window from a done is app-modal; Quit answers too.
func TestSheetWindowClose(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.show(twoButtonAlert)
		e.show(oneButtonAlert)
		e.d.RunAlert(e.w, threeButtonAlert, func(i int) {
			e.got = append(e.got, fmt.Sprintf("three=%d closed=%v", i, e.w.Closed()))
			e.d.RunAlert(e.w, oneButtonAlert, nil)
		})
		e.clk.Advance(50 * time.Millisecond)
		e.w.Close()
		if got := e.answers(); got != "Delete the item?=1 Saved=0 three=2 closed=false" {
			t.Errorf("answers %q", got)
		}
		// The app-modal alert of the last done takes the key (S5.7g).
		if got := e.h.take(); !e.w.Closed() || got != "unkey closed" {
			t.Errorf("window not closed: %q", got)
		}
		as := e.d.Alerts()
		if len(as) != 1 || as[0].Window() != nil || as[0].Surface() == nil {
			t.Errorf("the alert from done: %v", as)
		}
		as[0].Dismiss(0)
		if len(e.w.sheets) != 0 {
			t.Error("sheets or frames left after the close")
		}

		w2, err := e.d.NewWindow(driver.WindowConfig{Title: "2", Size: driver.Size{W: 400, H: 300}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		w2.Show()
		e.d.RunAlert(w2, twoButtonAlert, func(i int) { e.got = append(e.got, fmt.Sprint("quit=", i)) })
		e.d.Quit()
		if e.answers() != "quit=1" {
			t.Error("Quit did not answer the sheet")
		}
	})
}

// TestSheetMenus: while the sheet is up the Edit items and Close are
// disabled, minimize and zoom are not; the accelerators follow; the strip
// still opens.
func TestSheetMenus(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.field.SetText("abc")
		e.field.view.edit(driver.ItemSelectAll)
		if got := enabled(t, e.w, windowMenu); got != "Minimize+ Zoom+ Close+ Close Tab+" {
			t.Fatalf("window menu before: %s", got)
		}
		e.show(oneButtonAlert)
		e.settle()
		if got := enabled(t, e.w, editMenu); strings.Contains(got, "+") {
			t.Errorf("edit menu with a sheet: %s", got)
		}
		if got := enabled(t, e.w, windowMenu); got != "Minimize+ Zoom+ Close- Close Tab+" {
			t.Errorf("window menu with a sheet: %s", got)
		}
		e.w.menuPicked(menubar.Ref{Menu: windowMenu, Item: itemClose})
		if e.w.Closed() {
			t.Error("Close ran with a sheet")
		}
		// Primary+A (Select All) is the Edit menu's: disabled, so it goes
		// on to the sheet, which does not take it.
		e.sf.Key('a', driver.ModPrimary.Resolve(false))
		if e.answers() != "" || !e.w.Modal() {
			t.Error("an accelerator answered the sheet")
		}
		e.w.menuPicked(menubar.Ref{Menu: windowMenu, Item: itemMinimize})
		if !e.sf.Minimized() {
			t.Error("Minimize did not run with a sheet")
		}
	})
}

// TestSheetAccessibility: the window's tree has the sheet's dialog after
// the content, in surface coordinates, while it takes input.
func TestSheetAccessibility(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.show(twoButtonAlert)
		e.settle()
		tree := AccessibilityTree(e.w.Root())
		var dialog *a11y.Node
		for _, n := range tree.Children {
			if n.Role == a11y.RoleDialog {
				dialog = n
			}
		}
		if dialog == nil {
			t.Fatalf("no dialog in\n%s", tree)
		}
		s := e.w.shownSheet()
		want := driver.RectOf(e.w.SheetOrigin(), s.root.Frame().Size())
		if dialog.Frame != want || len(dialog.Children) != 4 {
			t.Errorf("dialog %v, want frame %v and 4 children", dialog, want)
		}
		if b := dialog.Children[2]; b.Label != "Delete" || !want.Contains(b.Frame.Origin()) || b.States&a11y.StateFocused != 0 {
			t.Errorf("default button node %v", b)
		}
		e.sf.Key(driver.KeyReturn, 0)
		if strings.Contains(AccessibilityTree(e.w.Root()).String(), "dialog") {
			t.Error("the answered sheet is still exposed")
		}
	})
}

// TestSheetSmallWindow: a shown sheet whose window is made lower than its
// panel stays a sheet, clipped to the content area, and still answers its
// keys (a sheet that does not fit when its turn comes opens in a modal
// surface instead: TestSheetSmallWindowSurface).
func TestSheetSmallWindow(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.show(oneButtonAlert)
		e.settle()
		if !e.w.shownSheet().fits() {
			t.Error("a 400x300 window does not fit the sheet")
		}
		e.w.SetContentSize(driver.Size{W: 200, H: 100})
		e.settle()
		s := e.w.shownSheet()
		if s.fits() || e.w.SheetOrigin().X+s.root.Frame().W/2 != 100 {
			t.Errorf("small window: fits %v, origin %v", s.fits(), e.w.SheetOrigin())
		}
		if !strings.Contains(e.sf.Dump(), "clip") {
			t.Error("the sheet is not clipped")
		}
		e.sf.Key(driver.KeyReturn, 0)
		if e.answers() != "Saved=0" {
			t.Error("Return did not answer the clipped sheet")
		}
	})
}

// TestSheetAppearance: an appearance change repaints the sheet in it.
func TestSheetAppearance(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.show(oneButtonAlert)
		e.settle()
		light := e.sf.Dump()
		e.d.SetAppearance(driver.AppearanceDark)
		e.settle()
		if e.sf.Dump() == light || appearanceIn(&e.w.shownSheet().alert.Base, e.d) != e.w.Appearance() {
			t.Error("the sheet did not follow the dark appearance")
		}
	})
}

// TestSheetNoKeyboardAccess: without full keyboard access the sheet has
// no focus, and its keys reach the alert from the root.
func TestSheetNoKeyboardAccess(t *testing.T) {
	sheetRun(t, driver.DefaultAppearance(), func(e *sheetEnv) {
		e.w.Router().SetFullKeyboardAccess(false)
		e.show(twoButtonAlert)
		e.settle()
		if f := e.w.shownSheet().root.Router().Focused(); f != nil {
			t.Fatalf("focus %v without full keyboard access", f)
		}
		e.sf.Key(driver.KeyTab, 0)
		e.sf.Key(driver.KeyEscape, 0)
		if e.answers() != "Delete the item?=1" {
			t.Error("Escape did not reach the alert")
		}
	})
}
