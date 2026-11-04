package gogui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/shell"
	fakeshell "github.com/radix29/gogui/internal/shell/fake"
)

// The tests in this file run the five example trees of examples_test.go on
// the aqua driver over the fake shell (PLAN.md S2.2e). Their frames are
// pinned per example in testdata/aqua (the sizing policy of S3.3, from
// S3.3b), and every window must paint without recorder errors.

var examples = []struct {
	name  string
	build func(*testing.T, *App) *Window
}{
	{"hello", exampleHello},
	{"form", exampleForm},
	{"converter", exampleConverter},
	{"controls", exampleControls},
	{"notes", exampleNotes},
}

// runAqua runs body in ready on an aqua driver over a fake shell, drains
// the UI queue and quits.
func runAqua(t *testing.T, body func(app *App, sh *fakeshell.Shell)) {
	t.Helper()
	sh := fakeshell.New()
	d := aqua.New(sh)
	setDriver(t, d)
	err := runTest(t, func(app *App) {
		body(app, sh)
		for d.Queue().Drain() > 0 {
		}
		app.Quit()
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// frameDump lists the frame of every widget in w's window, depth first,
// one per line, as "kind x,y wxh".
func frameDump(win *Window) string {
	var b strings.Builder
	var walk func(w Widget, depth int)
	walk = func(w Widget, depth int) {
		if w.peerWidget() != nil {
			r := frameOf(w)
			fmt.Fprintf(&b, "%s%s %g,%g %gx%g\n", strings.Repeat("  ", depth), w.base().kind, r.X, r.Y, r.W, r.H)
		}
		if p, ok := w.(parentWidget); ok {
			for _, c := range p.childWidgets() {
				walk(c, depth+1)
			}
		}
	}
	walk(win.content, 0)
	return b.String()
}

// paintWindow paints the aqua window behind win and returns its surface.
func paintWindow(t *testing.T, sh *fakeshell.Shell, win *Window) *fakeshell.Surface {
	t.Helper()
	sf := win.p.(*aqua.Window).Surface().(*fakeshell.Surface)
	sh.FakeClock().Flush()
	if sf.Paints() == 0 {
		t.Fatalf("%q: not painted", win.Title())
	}
	if err := sf.PaintErr(); err != nil {
		t.Fatalf("%q: paint errors: %v\n%s", win.Title(), err, sf.Dump())
	}
	return sf
}

// update rewrites the pinned aqua frame dumps of TestExamplesOnAqua (and
// notes_legacy.frames of TestNotesLegacyScrollersOnAqua, with -run Legacy):
//
//	CGO_ENABLED=0 go test -run TestExamplesOnAqua -update .
//
// Run it on purpose, when a step changes how controls size themselves, and
// commit the dump diffs with that step.
var update = flag.Bool("update", false, "rewrite the pinned aqua frame dumps in testdata/aqua")

// TestExamplesOnAqua runs the five examples on the aqua driver and compares
// each window's frame dump with the one pinned in
// testdata/aqua/<example>.frames (PLAN.md S3.3, sizing policy): the real
// controls size themselves from the theme, so their frames may differ from
// the headless ones, and every change shows as a dump diff. The examples'
// anchor widgets are checked for overlaps and clipping (wantAnchor), and
// every window must paint without recorder errors.
func TestExamplesOnAqua(t *testing.T) {
	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			var got string
			runAqua(t, func(app *App, sh *fakeshell.Shell) {
				if !onAqua(app) {
					t.Fatal("not on the aqua driver")
				}
				win := ex.build(t, app)
				got = frameDump(win)
				sf := paintWindow(t, sh, win)
				if !strings.Contains(sf.Dump(), "\ntext ") {
					t.Error("no text painted")
				}
			})
			path := filepath.Join("testdata", "aqua", ex.name+".frames")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
			// Git on Windows may check the file out with CRLF.
			if w := strings.ReplaceAll(string(want), "\r\n", "\n"); got != w {
				t.Errorf("aqua frames differ from %s (rerun with -update if the change is intended):\n%s",
					path, lineDiff(w, got))
			}
		})
	}
}

// lineDiff lists the lines of want and got that differ, by line number,
// as "-want" and "+got" pairs.
func lineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var b strings.Builder
	for i := range max(len(w), len(g)) {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			fmt.Fprintf(&b, "line %d:\n-%s\n+%s\n", i+1, wl, gl)
		}
	}
	return b.String()
}

// TestExampleHelloOnAquaDump pins the paint and the accessibility tree of
// the hello window on the aqua driver: the content background, the real
// label (S3.3c), the real default button (S3.3e: token-sized, accent
// filled, its bezel pixel-aligned) and a placeholder text field at its
// frame (the title bar is 28 pt, so content y is +28), then the chrome.
func TestExampleHelloOnAquaDump(t *testing.T) {
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		win := exampleHello(t, app)
		sf := paintWindow(t, sh, win)
		wantLines(t, "paint", sf.Dump(),
			"fillRRect (0,28,360,180) solid #ececec",
			`text (20,48,48.462,18.462) "Hello!" 15pt w700 #000000d9 clip=(0,28,360,180)`,
			"fillRRect (20,78.462,320,22) r=4 solid #ffffff clip=(0,28,360,180)",
			"strokeRRect (20.5,78.962,319,21) r=4 w=1 solid #00000042 clip=(0,28,360,180)",
			`text (24,82.692,118.462,13.538) "TextField: Your name" 11pt #000000d9 clip=(0,28,360,180)`,
			"shadow (270,112,70,20) r=5 off=(0,0.5) blur=1 #00000033 clip=(0,28,360,180)",
			"fillRRect (270,112,70,20) r=5 solid #007aff clip=(0,28,360,180)",
			`text (287.5,114,35,16) "Greet" 13pt #ffffff clip=(0,28,360,180)`,
			"fillRRect (0,0,360,28) solid #e5e5e5",
			"fillRRect (0,27,360,1) solid #0000001a",
			`text (162.5,6,35,16) "Hello" 13pt w600 #000000d9`,
			"fillRRect (8,8,12,12) r=6 solid #ff5f57",
			"strokeRRect (8.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #e0443e",
			"fillRRect (28,8,12,12) r=6 solid #febc2e",
			"strokeRRect (28.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #dea123",
			"fillRRect (48,8,12,12) r=6 solid #28c840",
			"strokeRRect (48.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #1aab29",
		)
		gh := fmt.Sprint(240.0 / 13)
		wantLines(t, "a11y", aqua.AccessibilityTree(win.p.(*aqua.Window).Root()).Dump(),
			`window "Hello" [focused] @0,0 360x208`,
			`  text "Hello!" @20,48 48.46153846153846x`+gh,
			`  textfield "Your name" [focusable focused] {focus} @20,78.46153846153845 320x22`,
			`  button "Greet" [focusable] {press focus} @270,112.46153846153845 70x20`,
			`  titlebar "Hello" @0,0 360x28`,
			`    button "Close" {press} @8,8 12x12`,
			`    button "Minimize" {press} @28,8 12x12`,
			`    button "Zoom" {press} @48,8 12x12`,
		)
	})
}

func wantLines(t *testing.T, what, got string, want ...string) {
	t.Helper()
	if w := strings.Join(want, "\n"); strings.TrimRight(got, "\n") != w {
		t.Errorf("%s:\n%s\nwant:\n%s", what, got, w)
	}
}

// TestFormKeysOnAqua drives the form example's Cancel and Create Account
// buttons with Return and Escape through the fake shell (PLAN.md S3.2f):
// Return does nothing while Create Account is disabled, presses it once the
// terms are accepted (an alert for the empty name, then the success alert),
// and Escape presses Cancel, which closes the window.
func TestFormKeysOnAqua(t *testing.T) {
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		f := newFormExample(app)
		showExample(app, f.win)
		d := app.drv.(*aqua.Driver)
		aw := f.win.p.(*aqua.Window)
		sf := aw.Surface().(*fakeshell.Surface)
		if aw.DefaultButton() != aqua.ElementOf(f.create.p) || aw.CancelButton() != aqua.ElementOf(f.cancel.p) {
			t.Fatal("default and cancel buttons not registered")
		}
		// The name field has the initial focus and does not take Return.
		if aw.Router().Focused() != aqua.ElementOf(f.name.p) {
			t.Fatal("initial focus is not the name field")
		}
		if sf.Key(driver.KeyReturn, 0) || len(d.Alerts()) != 0 {
			t.Fatal("Return pressed the disabled Create Account")
		}

		f.terms.p.(interface{ UserClick() }).UserClick()
		if stubBase(f.create).Disabled {
			t.Fatal("terms did not enable Create Account")
		}
		if !sf.Key(driver.KeyReturn, 0) {
			t.Fatal("Return not consumed")
		}
		alerts := d.Alerts()
		if len(alerts) != 1 || alerts[0].Spec.Message != "Please check your details" {
			t.Fatalf("validation alert: %+v", alerts)
		}
		alerts[0].Dismiss(0)

		f.name.SetText("Jane")
		f.email.SetText("jane@example.com")
		sf.Key(driver.KeyReturn, 0)
		alerts = d.Alerts()
		if len(alerts) != 1 || alerts[0].Spec.Detail != "Welcome, Jane (Viewer)." {
			t.Fatalf("success alert: %+v", alerts)
		}
		alerts[0].Dismiss(0)
		if !aw.Closed() {
			t.Fatal("dismissing the success alert did not close the window")
		}
	})

	// Escape (and Ctrl+.) press Cancel, which closes the window.
	for _, k := range []struct {
		key  driver.Key
		mods driver.Modifiers
	}{{driver.KeyEscape, 0}, {'.', driver.ModControl}} {
		runAqua(t, func(app *App, sh *fakeshell.Shell) {
			f := newFormExample(app)
			showExample(app, f.win)
			aw := f.win.p.(*aqua.Window)
			if !aw.Surface().(*fakeshell.Surface).Key(k.key, k.mods) || !aw.Closed() {
				t.Fatalf("key %q mods %v did not cancel", k.key, k.mods)
			}
		})
	}
}

// TestRadioGroupOnAqua clicks the real radios of a RadioGroup (PLAN.md
// S3.3g) through the fake shell: a click turns its radio on and the group
// turns the others off and reports the index once; a click on the
// selected radio or in a disabled group reports nothing; Space on the
// focused radio selects it; the a11y nodes are radios, checked when on.
func TestRadioGroupOnAqua(t *testing.T) {
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		g := NewRadioGroup("Free", "Pro", "Team")
		var changes []int
		g.OnChange(func(i int) { changes = append(changes, i) })
		win := app.NewWindow(WindowConfig{Title: "Plan", Size: Size{W: 200, H: 120}, Style: Titled})
		win.SetContent(VStack(0, g).Padding(20))
		showExample(app, win)
		aw := win.p.(*aqua.Window)
		sf := aw.Surface().(*fakeshell.Surface)

		node := func(title string) *a11y.Node {
			t.Helper()
			n := aqua.AccessibilityTree(aw.Root()).Find(a11y.RoleRadioButton, title)
			if n == nil {
				t.Fatalf("no radio %q:\n%s", title, aqua.AccessibilityTree(aw.Root()).Dump())
			}
			return n
		}
		click := func(title string) {
			t.Helper()
			f := node(title).Frame
			sf.Click(driver.Point{X: f.X + 7, Y: f.Y + f.H/2})
			drain(app)
		}
		want := func(sel int, ch ...int) {
			t.Helper()
			if g.Selected() != sel || fmt.Sprint(changes) != fmt.Sprint(ch) {
				t.Fatalf("selected %d, changes %v; want %d, %v", g.Selected(), changes, sel, ch)
			}
			for i, title := range []string{"Free", "Pro", "Team"} {
				if on := node(title).States&a11y.StateChecked != 0; on != (i == sel) {
					t.Fatalf("radio %q checked %v with %d selected", title, on, sel)
				}
			}
		}

		want(0)
		click("Pro")
		want(1, 1)
		click("Pro") // the selected one: no change
		want(1, 1)
		click("Team")
		want(2, 1, 2)
		if f := node("Pro").Frame; f.H != 16 {
			t.Errorf("radio frame %v, want the token height 16", f)
		}

		// Space on the focused radio selects it.
		g.SetSelected(0)
		aqua.ElementOf(g.radios[1].p).(interface{ Focus() bool }).Focus()
		if !sf.Key(driver.KeySpace, 0) {
			t.Fatal("Space not consumed")
		}
		drain(app)
		want(1, 1, 2, 1)

		// A disabled group ignores clicks.
		g.SetEnabled(false)
		click("Free")
		want(1, 1, 2, 1)
		if got := paintWindow(t, sh, win).Dump(); !strings.Contains(got, "pushOpacity 0.5") {
			t.Errorf("disabled group not dimmed:\n%s", got)
		}
	})
}

// TestSliderOnAqua drags and keys the real aqua slider of a converter tree
// (the wiring of examples/converter) through the fake surface: the
// converter's OnChange handler sees every value and updates the text
// fields (PLAN.md S3.4d).
func TestSliderOnAqua(t *testing.T) {
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		celsius, fahrenheit := NewTextField(), NewTextField()
		slider := NewSlider().SetRange(-40, 100)
		var seen []float64
		updating := false
		setCelsius := func(c float64, fromSlider bool) {
			if updating {
				return
			}
			updating = true
			defer func() { updating = false }()
			celsius.SetText(strconv.FormatFloat(c, 'f', 1, 64))
			fahrenheit.SetText(strconv.FormatFloat(c*9/5+32, 'f', 1, 64))
			if !fromSlider {
				slider.SetValue(c)
			}
		}
		slider.OnChange(func(v float64) {
			seen = append(seen, v)
			setCelsius(v, true)
		})
		setCelsius(20, false)
		win := app.NewWindow(WindowConfig{Title: "Temperature", Size: Size{W: 380, H: 220}, Style: Titled | Closable})
		win.SetContent(VStack(12,
			Form(Row("Celsius:", celsius), Row("Fahrenheit:", fahrenheit)),
			slider,
		).Padding(20))
		showExample(app, win)
		aw := win.p.(*aqua.Window)
		sf := aw.Surface().(*fakeshell.Surface)

		n := aqua.AccessibilityTree(aw.Root()).Find(a11y.RoleSlider, "")
		if n == nil {
			t.Fatalf("no slider:\n%s", aqua.AccessibilityTree(aw.Root()).Dump())
		}
		f := n.Frame
		// The 20 pt knob's centre travels from 10 pt in from the left end
		// to 10 pt in from the right.
		at := func(v float64) driver.Point {
			return driver.Point{X: f.X + 10 + (v+40)/140*(f.W-20), Y: f.Y + f.H/2}
		}
		want := func(calls int, c, fh string) {
			t.Helper()
			drain(app)
			if len(seen) != calls {
				t.Fatalf("OnChange calls %v, want %d", seen, calls)
			}
			if calls > 0 && seen[calls-1] != slider.Value() {
				t.Fatalf("OnChange saw %v, the slider is at %v", seen[calls-1], slider.Value())
			}
			if celsius.Text() != c || fahrenheit.Text() != fh {
				t.Fatalf("fields %q, %q; want %q, %q (slider %v)", celsius.Text(), fahrenheit.Text(), c, fh, slider.Value())
			}
		}
		format := func(v float64) (string, string) {
			return strconv.FormatFloat(v, 'f', 1, 64), strconv.FormatFloat(v*9/5+32, 'f', 1, 64)
		}

		// A press on the knob does not move it; the drag does, on every
		// move, and the fields follow.
		want(0, "20.0", "68.0")
		sf.Press(at(20), shell.ButtonPrimary)
		want(0, "20.0", "68.0")
		sf.Move(driver.Point{X: at(20).X + 30, Y: at(20).Y})
		if slider.Value() <= 20 {
			t.Fatalf("dragged right: value %v", slider.Value())
		}
		c, fh := format(slider.Value())
		want(1, c, fh)
		sf.Move(driver.Point{X: at(20).X - 40, Y: at(20).Y + 50}) // off the frame
		if slider.Value() >= 20 {
			t.Fatalf("dragged left: value %v", slider.Value())
		}
		c, fh = format(slider.Value())
		want(2, c, fh)
		sf.Move(driver.Point{X: f.X + f.W + 100, Y: at(20).Y}) // past the end
		want(3, "100.0", "212.0")
		sf.Release(driver.Point{X: f.X + f.W + 100, Y: at(20).Y}, shell.ButtonPrimary)
		want(3, "100.0", "212.0")

		// The keys of the focused slider: Home, then one step (7) up.
		aqua.ElementOf(slider.p).(interface{ Focus() bool }).Focus()
		if !sf.Key(driver.KeyHome, 0) {
			t.Fatal("Home not consumed")
		}
		want(4, "-40.0", "-40.0")
		if !sf.Key(driver.KeyRight, 0) {
			t.Fatal("Right not consumed")
		}
		want(5, "-33.0", "-27.4")
		if !sf.Key(driver.KeyEnd, 0) || !sf.Key(driver.KeyPageDown, 0) { // 100, then 72
			t.Fatal("End or Page Down not consumed")
		}
		want(7, "72.0", "161.6")

		// A press on the track jumps there.
		sf.Click(at(0))
		c, fh = format(slider.Value())
		if slider.Value() < -1 || slider.Value() > 1 {
			t.Fatalf("clicked at 0: value %v", slider.Value())
		}
		want(8, c, fh)

		// A disabled slider ignores both; SetValue does not call OnChange.
		slider.SetEnabled(false)
		drain(app)
		sf.Click(at(50))
		want(8, c, fh)
		slider.SetValue(30)
		if drain(app); len(seen) != 8 || slider.Value() != 30 {
			t.Fatalf("SetValue: OnChange calls %v, value %v", seen, slider.Value())
		}
	})
}

// TestExamplesAccessibleOnAqua walks the widget trees of the five examples
// on the aqua driver (PLAN.md S3.3h, §7.5) and checks the accessible node
// of every widget with a peer against the widget's own state: its role, its
// label, its value, its states (checked or mixed, read-only, disabled with
// an ancestor, focusable when an enabled control, focused when it holds the
// window's focus) and its actions. Every such node must also be in the
// window's accessibility tree, and every exposed node of the content must
// belong to a widget, so no control is missing and none is left over.
// Since S3.5h only text fields, text views and pop-up buttons may still be
// placeholder peers.
func TestExamplesAccessibleOnAqua(t *testing.T) {
	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			runAqua(t, func(app *App, sh *fakeshell.Shell) {
				win := ex.build(t, app)
				aw := win.p.(*aqua.Window)
				tree := aqua.AccessibilityTree(aw.Root())

				// The exposed content nodes, the title bar left out.
				left := map[string]int{}
				tree.Walk(func(n *a11y.Node, depth int) bool {
					if n.Role == a11y.RoleTitleBar {
						return false
					}
					if depth > 0 {
						left[n.String()]++
					}
					return true
				})

				controls := 0
				var walk func(w Widget)
				walk = func(w Widget) {
					if p, ok := w.(parentWidget); ok {
						defer func() {
							for _, c := range p.childWidgets() {
								walk(c)
							}
						}()
					}
					if w.peerWidget() == nil || hiddenInTree(w) {
						return
					}
					e := aqua.ElementOf(w.peerWidget())
					if e == nil {
						t.Errorf("%s: no aqua element", w.base().kind)
						return
					}
					// Only the controls of phase 5 are still
					// placeholders (S3.5h).
					if _, ok := aqua.Caption(w.peerWidget()); ok {
						switch w.(type) {
						case *TextField, *TextView, *PopUpButton:
						default:
							t.Errorf("%s is still a placeholder peer", w.base().kind)
						}
					}
					got := aqua.AccessibilityTree(e)
					want := wantNode(w, e == aw.Router().Focused())
					if want.Role == a11y.RoleNone {
						if got.Role != a11y.RoleNone {
							t.Errorf("%s: exposed as %v, want a layout helper (role none)", w.base().kind, got)
						}
						return
					}
					controls++
					want.Frame = got.Frame // frames: testdata/aqua
					if got.String() != want.String() {
						t.Errorf("%s node:\n got %v\nwant %v", w.base().kind, got, &want)
						return
					}
					s := got.String()
					if left[s] == 0 {
						t.Errorf("%s node %v not in the window's tree:\n%s", w.base().kind, got, tree.Dump())
						return
					}
					left[s]--
				}
				walk(win.content)
				if controls == 0 {
					t.Fatal("no control walked")
				}
				for s, k := range left {
					if k > 0 {
						t.Errorf("node %s (×%d) in the window's tree belongs to no widget", s, k)
					}
				}
			})
		})
	}
}

// hiddenInTree reports whether w or one of its ancestors is hidden.
func hiddenInTree(w Widget) bool {
	for ; w != nil; w = w.base().parent {
		if w.base().n.Props().Hidden() {
			return true
		}
	}
	return false
}

// disabledInTree reports whether w or one of its ancestors is disabled.
func disabledInTree(w Widget) bool {
	for ; w != nil; w = w.base().parent {
		if w.base().disabled {
			return true
		}
	}
	return false
}

// wantNode returns the accessible node widget w should have on aqua, from
// its public getters and, where there is none, its stub state; focused
// tells whether w holds its window's focus. Stacks, forms and radio groups
// are layout helpers (RoleNone): their children are exposed in their place.
func wantNode(w Widget, focused bool) a11y.Node {
	var n a11y.Node
	focusable := false
	switch w := w.(type) {
	case *Label:
		n = a11y.Node{Role: a11y.RoleStaticText, Label: w.Text()}
	case *Button:
		n = a11y.Node{Role: a11y.RoleButton, Label: stubPeer(w).(*stub.Button).Title, Actions: a11y.ActionPress}
		focusable = true
	case *Checkbox:
		n = a11y.Node{Role: a11y.RoleCheckBox, Label: stubPeer(w).(*stub.Checkbox).Title, Actions: a11y.ActionPress}
		switch w.State() {
		case StateOn:
			n.States = a11y.StateChecked
		case StateMixed:
			n.States = a11y.StateMixed
		}
		focusable = true
	case *radio:
		g := w.parent.(*RadioGroup)
		n = a11y.Node{Role: a11y.RoleRadioButton, Label: stubPeer(w).(*stub.Radio).Title, Actions: a11y.ActionPress}
		if g.Selected() >= 0 && g.radios[g.Selected()] == w {
			n.States = a11y.StateChecked
		}
		focusable = true
	case *TextField:
		sp := stubPeer(w).(*stub.TextField)
		n = a11y.Node{Role: a11y.RoleTextField, Label: sp.Placeholder}
		if !w.Secure() {
			n.Value = w.Text()
		}
		if sp.ReadOnly {
			n.States = a11y.StateReadOnly
		}
		focusable = true
	case *TextView:
		n = a11y.Node{Role: a11y.RoleTextArea, Value: w.Text()}
		if stubPeer(w).(*stub.TextView).ReadOnly {
			n.States = a11y.StateReadOnly
		}
		focusable = true
	case *Slider:
		n = a11y.Node{Role: a11y.RoleSlider, Value: fmt.Sprint(w.Value()),
			Actions: a11y.ActionIncrement | a11y.ActionDecrement}
		focusable = true
	case *ProgressBar:
		n = a11y.Node{Role: a11y.RoleProgressIndicator}
		if !w.Indeterminate() {
			n.Value = fmt.Sprint(w.Value())
		}
	case *Spinner:
		n = a11y.Node{Role: a11y.RoleProgressIndicator}
		if !w.Indeterminate() {
			n.Value = fmt.Sprint(w.Value())
		}
	case *PopUpButton:
		n = a11y.Node{Role: a11y.RolePopUpButton, Actions: a11y.ActionShowMenu}
		if i := w.Selected(); i >= 0 {
			n.Value = w.Item(i)
		}
		focusable = true
	case *Separator:
		n = a11y.Node{Role: a11y.RoleSeparator}
	case *Box:
		n = a11y.Node{Role: a11y.RoleGroup, Label: w.Title()}
	case *ScrollView:
		n = a11y.Node{Role: a11y.RoleScrollArea}
	}
	enabled := !disabledInTree(w)
	if !enabled {
		n.States |= a11y.StateDisabled
	}
	if focusable && enabled {
		n.States |= a11y.StateFocusable
		n.Actions |= a11y.ActionFocus
	}
	if focused {
		n.States |= a11y.StateFocused
	}
	return n
}
