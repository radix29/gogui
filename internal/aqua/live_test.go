package aqua

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// Live appearance changes (PLAN.md S6.1h): each field of driver.Appearance
// changes while windows, a sheet, an app-modal alert, an open menu, a
// pop-up menu and a tooltip are up, over a running driver on the fake
// shell. The check is "live equals fresh": the scene built under the old
// appearance and switched to the new one paints exactly what the same
// scene built under the new appearance from the start paints, on every
// surface and popup, so no element keeps a color, a layout or a state
// from the old one. Each change runs both ways.

// liveField is one field of driver.Appearance and a change of it.
type liveField struct {
	name string
	set  func(a *driver.Appearance) // the changed value
	// visible: the change shows in a still frame of every scene in
	// visibleIn (all of them when empty); the others need no still
	// difference (Reduce Motion, the caret blink), only live = fresh.
	visible   bool
	visibleIn []string
	// probe, when set, checks after the live change what a still frame
	// cannot show.
	probe func(t *testing.T, e *liveEnv, to driver.Appearance)
}

var liveFields = []liveField{
	{name: "Dark", set: func(a *driver.Appearance) { a.Dark = true }, visible: true},
	{name: "Accent", set: func(a *driver.Appearance) { a.Accent = driver.RGB8(0xd0, 0x30, 0x90) }, visible: true},
	{name: "IncreaseContrast", set: func(a *driver.Appearance) { a.IncreaseContrast = true }, visible: true},
	{name: "ReduceMotion", set: func(a *driver.Appearance) { a.ReduceMotion = true }, probe: probeReduceMotion},
	{name: "ReduceTransparency", set: func(a *driver.Appearance) { a.ReduceTransparency = true },
		visible: true, visibleIn: []string{"strip menu", "pop-up menu"}},
	{name: "LegacyScrollers", set: func(a *driver.Appearance) { a.LegacyScrollers = true }, visible: true},
	{name: "CaretBlink", set: func(a *driver.Appearance) {
		a.CaretBlink = driver.CaretBlink{Period: 400 * time.Millisecond}
	}, probe: probeCaretBlink},
}

// probeReduceMotion: the driver's timeline follows the new value.
func probeReduceMotion(t *testing.T, e *liveEnv, to driver.Appearance) {
	if got := e.d.Timeline().ReduceMotion(); got != to.ReduceMotion {
		t.Errorf("the timeline's Reduce Motion is %v, want %v", got, to.ReduceMotion)
	}
}

// probeCaretBlink: where the field's caret shows (every scene without an
// alert: the window keeps its focus under a menu), it blinks at the new
// half period from the change on (the change restarts the blink).
func probeCaretBlink(t *testing.T, e *liveEnv, to driver.Appearance) {
	v := e.field.view
	if shown, want := v.caretShown(), e.scene != "sheet" && e.scene != "app-modal alert"; shown != want {
		t.Fatalf("the caret shown %v, want %v", shown, want)
	}
	if !v.caretShown() {
		return
	}
	half := to.CaretBlink.Period / 2
	if v.blink.off {
		t.Fatal("the caret is off right after the change")
	}
	e.clk.Advance(half - time.Millisecond)
	if v.blink.off {
		t.Errorf("the caret went off before the new half period %v", half)
	}
	e.clk.Advance(2 * time.Millisecond)
	if !v.blink.off {
		t.Errorf("the caret did not go off after the new half period %v", half)
	}
}

// liveScenes open what is up when the appearance changes, over the two
// windows liveRun opens (the first key, the second inactive behind it).
var liveScenes = []liveScene{
	{"windows", func(*liveEnv) {}},
	{"sheet", func(e *liveEnv) {
		e.d.RunAlert(e.w, twoButtonAlert, func(int) {})
		if len(e.w.sheets) != 1 || e.w.sheets[0].surf != nil {
			e.t.Fatal("no sheet in the window")
		}
	}},
	{"app-modal alert", func(e *liveEnv) {
		e.d.RunAlert(nil, threeButtonAlert, func(int) {})
		if e.sh.Modal() == nil {
			e.t.Fatal("no modal surface")
		}
	}},
	{"strip menu", func(e *liveEnv) {
		e.sf.Click(centre(e.w.menuStrip().TitleRect(1)))
		if e.w.menuStrip().OpenMenu() != 1 {
			e.t.Fatal("the strip's menu did not open")
		}
	}},
	{"pop-up menu", func(e *liveEnv) {
		e.sf.Click(e.at(centre(e.popup.Frame)))
		if !e.popup.MenuOpen() {
			e.t.Fatal("the pop-up menu did not open")
		}
	}},
	{"tooltip", func(e *liveEnv) {
		e.sf.Move(e.at(centre(e.btn.Frame)))
		e.clk.Advance(tokens.Tooltip.Delay)
		if e.w.tips.open == nil {
			e.t.Fatal("the tooltip did not show")
		}
	}},
}

// liveScene is a name and what it opens.
type liveScene struct {
	name string
	open func(e *liveEnv)
}

// liveEnv is a liveRun scene.
type liveEnv struct {
	t     *testing.T
	scene string
	d     *Driver
	sh    *fake.Shell
	clk   *fake.Clock
	w     *Window // key
	sf    *fake.Surface
	back  *Window // inactive
	btn   *Button
	field *TextField
	popup *PopUpButton
}

// at converts a content point of the key window to its surface's.
func (e *liveEnv) at(p driver.Point) driver.Point { return pt(p.X, p.Y+TitleBarHeight()) }

// liveContent fills w with one control of each kind in a non-normal state
// where it has one (a default button with a tooltip, an on checkbox and
// radio, a slider and a bar, a pop-up button, a focused field with its
// caret, a text view with a selection and its scroller).
func liveContent(d *Driver, w *Window) (*Button, *TextField, *PopUpButton) {
	c := d.NewContainer().(*Container)
	c.SetFrame(driver.Rect{W: 420, H: 300})
	l := d.NewLabel()
	l.SetText("Label")
	btn := d.NewButton(nil).(*Button)
	btn.SetTitle("Go")
	btn.SetDefault(true)
	btn.SetTooltip("Runs it")
	cb := d.NewCheckbox(nil)
	cb.SetTitle("Check")
	cb.SetState(driver.StateOn)
	rd := d.NewRadio(nil)
	rd.SetTitle("Radio")
	rd.SetOn(true)
	sl := d.NewSlider(nil)
	sl.SetValue(0.4)
	pr := d.NewProgress()
	pr.SetValue(0.6)
	pop := d.NewPopUpButton(nil).(*PopUpButton)
	pop.SetItems([]string{"One", "Two", "Three"})
	pop.SetSelected(1)
	f := d.NewTextField(false, nil).(*TextField)
	f.SetText("Field text")
	tv := d.NewTextView(nil)
	tv.SetText(strings.Repeat("A line of text\n", 30))
	tv.SetSelection(2, 9)
	c.SetChildren([]driver.Widget{l, btn, cb, rd, sl, pr, pop, f, tv})
	w.SetContent(c)
	l.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 16})
	btn.SetFrame(driver.Rect{X: 10, Y: 36, W: 80, H: 20})
	cb.SetFrame(driver.Rect{X: 100, Y: 36, W: 100, H: 18})
	rd.SetFrame(driver.Rect{X: 210, Y: 36, W: 100, H: 18})
	sl.SetFrame(driver.Rect{X: 10, Y: 70, W: 150, H: 20})
	pr.SetFrame(driver.Rect{X: 180, Y: 76, W: 150, H: 6})
	pop.SetFrame(driver.Rect{X: 10, Y: 100, W: 120, H: 22})
	f.SetFrame(driver.Rect{X: 10, Y: 134, W: 150, H: 22})
	tv.SetFrame(driver.Rect{X: 180, Y: 100, W: 220, H: 120})
	w.Router().SetFocus(f.view)
	return btn, f, pop
}

// liveRun runs a driver on the fake shell with sys as the OS appearance
// (translucent popups, so Reduce Transparency shows), opens the two
// windows, opens the scene, lets it settle, calls change (which may change
// the OS appearance), settles again and returns the dumps of every surface
// and popup. Both runs of a comparison take the same steps at the same
// times, so the caret's phase and the timers agree.
func liveRun(t *testing.T, sys driver.Appearance, scene liveScene, change func(*liveEnv)) (dumps []string) {
	t.Helper()
	sh := fake.New()
	sh.SetAppearance(sys)
	sh.SetPopupTranslucent(true)
	d := New(sh)
	d.SetMenuBar(roleSpec)
	h := &appLog{}
	h.onReady = func() {
		defer d.Quit()
		e := &liveEnv{t: t, scene: scene.name, d: d, sh: sh, clk: sh.FakeClock()}
		open := func(title string, at driver.Point) *Window {
			dw, err := d.NewWindow(driver.WindowConfig{Title: title, Size: driver.Size{W: 420, H: 300}, Style: allStyles}, &winLog{})
			if err != nil {
				t.Fatal(err)
			}
			w := dw.(*Window)
			w.Surface().(*fake.Surface).UserMove(at)
			return w
		}
		e.back = open("Back", pt(500, 120))
		liveContent(d, e.back)
		e.back.Show()
		e.w = open("Front", pt(60, 100))
		e.sf = e.w.Surface().(*fake.Surface)
		e.btn, e.field, e.popup = liveContent(d, e.w)
		e.w.Show()
		e.clk.Advance(time.Second)
		if !e.w.IsKey() || e.back.IsKey() {
			t.Fatal("the front window is not the key one")
		}
		scene.open(e)
		e.clk.Advance(time.Second)
		change(e)
		e.clk.Advance(time.Second)
		// The caret's phase is not compared: a change restarts the blink
		// (probeCaretBlink checks it), a fresh start does not.
		e.field.view.restartBlink()
		dumps = snapshot(t, sh)
	}
	if err := runDriver(t, d, h); err != nil {
		t.Fatal(err)
	}
	return dumps
}

// snapshot paints whatever is invalid and returns the dumps of the open
// surfaces and popups, in creation order, each named.
func snapshot(t *testing.T, sh *fake.Shell) []string {
	t.Helper()
	sh.FakeClock().Flush()
	var out []string
	for i, sf := range sh.Surfaces() {
		if err := sf.PaintErr(); err != nil {
			t.Fatalf("surface %d (%q): %v", i, sf.Title(), err)
		}
		out = append(out, fmt.Sprintf("surface %d %q\n%s", i, sf.Title(), sf.Dump()))
	}
	for i, p := range sh.Popups() {
		if err := p.PaintErr(); err != nil {
			t.Fatalf("popup %d: %v", i, err)
		}
		out = append(out, fmt.Sprintf("popup %d grab=%v\n%s", i, p.Grab(), p.Dump()))
	}
	return out
}

// firstDiff returns the first differing line of two dumps.
func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range max(len(al), len(bl)) {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			return fmt.Sprintf("line %d:\n  live:  %s\n  fresh: %s", i+1, x, y)
		}
	}
	return "no difference"
}

// TestLiveAppearance: for every field, every scene and both directions,
// the live change paints what a fresh start under the new appearance
// paints, and a visible change shows.
func TestLiveAppearance(t *testing.T) {
	base := driver.DefaultAppearance()
	for _, f := range liveFields {
		changed := base
		f.set(&changed)
		if changed == base {
			t.Fatalf("%s: the change changes nothing", f.name)
		}
		for _, sc := range liveScenes {
			for _, dir := range []struct {
				name     string
				from, to driver.Appearance
			}{{"on", base, changed}, {"off", changed, base}} {
				t.Run(fmt.Sprintf("%s/%s/%s", f.name, sc.name, dir.name), func(t *testing.T) {
					to := dir.to
					still := liveRun(t, dir.from, sc, func(*liveEnv) {})
					live := liveRun(t, dir.from, sc, func(e *liveEnv) { e.sh.SetAppearance(to) })
					fresh := liveRun(t, dir.to, sc, func(*liveEnv) {})
					if f.probe != nil { // a run of its own: it moves the clock
						liveRun(t, dir.from, sc, func(e *liveEnv) {
							e.sh.SetAppearance(to)
							f.probe(t, e, to)
						})
					}
					if len(live) != len(fresh) {
						t.Fatalf("%d surfaces and popups live, %d fresh", len(live), len(fresh))
					}
					for i := range live {
						if live[i] != fresh[i] {
							t.Errorf("%s differs from a fresh start: %s", strings.SplitN(live[i], "\n", 2)[0], firstDiff(live[i], fresh[i]))
						}
					}
					if !f.visible || (len(f.visibleIn) > 0 && !slices.Contains(f.visibleIn, sc.name)) {
						return
					}
					if strings.Join(live, "\n") == strings.Join(still, "\n") {
						t.Error("the change does not show")
					}
				})
			}
		}
	}
}
