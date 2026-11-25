package aqua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/render/record"
)

// otherAccent is the second accent of the inactive-window matrix: pink,
// far from every theme color, so an accent-derived color shows as a
// changed dump.
var otherAccent = driver.RGB8(255, 45, 85)

// lightAccent is a third accent, yellow, light enough that the labels on
// an accent fill turn dark (S6.1g); in an inactive window it must not
// show either.
var lightAccent = driver.RGB8(255, 214, 10)

// inactiveCase is one control in one state of the inactive-window matrix
// (TestInactiveWindowMatrix).
type inactiveCase struct {
	kind, state string
	// build creates the control with d in w's content at frame (10, 10,
	// …) and returns it; the window is key and has its appearance.
	build func(t *testing.T, d *Driver, w *Window) driver.Widget
	// accent: the key look uses the accent (a fill, the focus ring, the
	// selection); otherwise the control looks the same in a key and an
	// inactive window.
	accent bool
	// focus: the control takes the focus after build; its inactive look
	// must then be its unfocused inactive look (no focus ring), unless
	// keepsFocusLook (a field's unemphasized selection).
	focus, keepsFocusLook bool
	// appColor: the app chose an accent-derived color (a label in
	// ColorControlAccent). It follows the accent and keeps it in an inactive
	// window, as NSColor.controlAccentColor does not track the window.
	appColor bool
	// inactiveAs names the state of the same kind whose inactive look this
	// one has (a default button is plain in an inactive window).
	inactiveAs string
	// appearance edits the window's appearance (legacy scrollers).
	appearance func(*driver.Appearance)
}

// The builders of the matrix.

func mButton(title string, cs driver.ControlSize, isDefault, enabled bool) func(*testing.T, *Driver, *Window) driver.Widget {
	return func(_ *testing.T, d *Driver, w *Window) driver.Widget {
		b := d.NewButton(nil)
		b.SetTitle(title)
		b.SetControlSize(cs)
		b.SetDefault(isDefault)
		b.SetEnabled(enabled)
		h := 20.0
		switch cs {
		case driver.ControlSizeSmall:
			h = 17
		case driver.ControlSizeMini:
			h = 14
		case driver.ControlSizeLarge:
			h = 26
		}
		return place(w, b, 100, h)
	}
}

func mCheckbox(st driver.CheckState, enabled bool) func(*testing.T, *Driver, *Window) driver.Widget {
	return func(_ *testing.T, d *Driver, w *Window) driver.Widget {
		c := d.NewCheckbox(nil)
		c.SetTitle("Check")
		c.SetState(st)
		c.SetEnabled(enabled)
		return place(w, c, 120, 20)
	}
}

func mRadio(on, enabled bool) func(*testing.T, *Driver, *Window) driver.Widget {
	return func(_ *testing.T, d *Driver, w *Window) driver.Widget {
		r := d.NewRadio(nil)
		r.SetTitle("Radio")
		r.SetOn(on)
		r.SetEnabled(enabled)
		return place(w, r, 120, 20)
	}
}

func mSlider(ticks int, enabled bool) func(*testing.T, *Driver, *Window) driver.Widget {
	return func(_ *testing.T, d *Driver, w *Window) driver.Widget {
		s := d.NewSlider(nil)
		s.SetRange(0, 100)
		s.SetValue(40)
		s.SetTickMarks(ticks, ticks > 0)
		s.SetEnabled(enabled)
		h := 20.0
		if ticks > 0 {
			h = 28
		}
		return place(w, s, 160, h)
	}
}

func mProgress(style driver.ProgressStyle, indeterminate, enabled bool) func(*testing.T, *Driver, *Window) driver.Widget {
	return func(_ *testing.T, d *Driver, w *Window) driver.Widget {
		p := d.NewProgress()
		p.SetStyle(style)
		p.SetRange(0, 100)
		p.SetValue(40)
		p.SetIndeterminate(indeterminate)
		p.SetEnabled(enabled)
		if style == driver.ProgressSpinner {
			return place(w, p, 32, 32)
		}
		return place(w, p, 160, 6)
	}
}

func mLabel(c driver.Color) func(*testing.T, *Driver, *Window) driver.Widget {
	return func(_ *testing.T, d *Driver, w *Window) driver.Widget {
		l := d.NewLabel()
		l.SetText("Label")
		l.SetTextColor(c)
		return place(w, l, 120, 16)
	}
}

func mSeparator(vertical bool) func(*testing.T, *Driver, *Window) driver.Widget {
	return func(_ *testing.T, d *Driver, w *Window) driver.Widget {
		s := d.NewSeparator()
		s.SetVertical(vertical)
		if vertical {
			return place(w, s, 1, 60)
		}
		return place(w, s, 160, 1)
	}
}

func mBox(_ *testing.T, d *Driver, w *Window) driver.Widget {
	b := d.NewBox()
	b.SetTitle("Box")
	l := d.NewLabel()
	l.SetText("Inside")
	c := d.NewContainer()
	c.SetChildren([]driver.Widget{l})
	b.SetContent(c)
	place(w, b, 160, 80)
	c.SetFrame(driver.Rect{W: 140, H: 50})
	l.SetFrame(driver.Rect{X: 4, Y: 4, W: 100, H: 16})
	return b
}

func mScroll(_ *testing.T, d *Driver, w *Window) driver.Widget {
	sv := d.NewScrollView(nil)
	sv.SetHasVerticalScroller(true)
	sv.SetHasHorizontalScroller(true)
	l := d.NewLabel()
	l.SetText("Document")
	doc := d.NewContainer()
	doc.SetChildren([]driver.Widget{l})
	sv.SetDocument(doc)
	sv.SetDocumentSize(driver.Size{W: 400, H: 300})
	doc.SetFrame(driver.Rect{W: 400, H: 300})
	l.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 16})
	place(w, sv, 160, 80)
	sv.SetOffset(driver.Point{X: 30, Y: 20})
	return sv
}

func mField(text string, sel [2]int, enabled bool) func(*testing.T, *Driver, *Window) driver.Widget {
	return func(_ *testing.T, d *Driver, w *Window) driver.Widget {
		f := d.NewTextField(false, nil).(*TextField)
		f.SetText(text)
		f.SetPlaceholder("Name")
		f.SetEnabled(enabled)
		place(w, f, 120, 22)
		if sel[0] >= 0 {
			f.Editor().SetSelection(sel[0], sel[1])
		}
		return f
	}
}

func mTextView(sel [2]int, enabled bool) func(*testing.T, *Driver, *Window) driver.Widget {
	return func(_ *testing.T, d *Driver, w *Window) driver.Widget {
		v := d.NewTextView(nil).(*TextView)
		v.SetText("Hello world\nSecond line")
		v.SetEditable(enabled)
		place(w, v, 160, 80)
		if sel[0] >= 0 {
			v.SetSelection(sel[0], sel[1])
		}
		w.Router().SetFocus(nil)
		return v
	}
}

func mPopUp(enabled bool) func(*testing.T, *Driver, *Window) driver.Widget {
	return func(_ *testing.T, d *Driver, w *Window) driver.Widget {
		p := d.NewPopUpButton(nil)
		p.SetItems([]string{"One", "Two"})
		p.SetSelected(1)
		p.SetEnabled(enabled)
		return place(w, p, 120, 22)
	}
}

// place makes c the window's content at (10, 10, wd, h) and returns it.
func place(w *Window, c driver.Widget, wd, h float64) driver.Widget {
	w.SetContent(c)
	c.SetFrame(driver.Rect{X: 10, Y: 10, W: wd, H: h})
	return c
}

var noSel = [2]int{-1, -1}

// inactiveCases lists every control kind of the gallery (and the
// TextField, the TextView and the PopUpButton, which join it in S4.3j's
// follow-up) in each state that has a key look.
var inactiveCases = []inactiveCase{
	{kind: "label", state: "normal", build: mLabel(driver.Color{})},
	{kind: "label", state: "secondary", build: mLabel(driver.Semantic(driver.ColorSecondaryLabel))},
	{kind: "label", state: "disabled", build: mLabel(driver.Semantic(driver.ColorDisabledControlText))},
	{kind: "label", state: "accent color", build: mLabel(driver.Semantic(driver.ColorControlAccent)), appColor: true},
	{kind: "separator", state: "horizontal", build: mSeparator(false)},
	{kind: "separator", state: "vertical", build: mSeparator(true)},
	{kind: "box", state: "normal", build: mBox},

	{kind: "button", state: "normal", build: mButton("OK", driver.ControlSizeRegular, false, true)},
	{kind: "button", state: "default", build: mButton("OK", driver.ControlSizeRegular, true, true), accent: true, inactiveAs: "normal"},
	{kind: "button", state: "focused", build: mButton("OK", driver.ControlSizeRegular, false, true), accent: true, focus: true},
	{kind: "button", state: "focused default", build: mButton("OK", driver.ControlSizeRegular, true, true), accent: true, focus: true, inactiveAs: "normal"},
	{kind: "button", state: "disabled", build: mButton("OK", driver.ControlSizeRegular, false, false)},
	{kind: "button", state: "disabled default", build: mButton("OK", driver.ControlSizeRegular, true, false), inactiveAs: "disabled"},
	{kind: "button", state: "small default", build: mButton("OK", driver.ControlSizeSmall, true, true), accent: true},
	{kind: "button", state: "mini default", build: mButton("OK", driver.ControlSizeMini, true, true), accent: true},
	{kind: "button", state: "large default", build: mButton("OK", driver.ControlSizeLarge, true, true), accent: true},

	{kind: "checkbox", state: "off", build: mCheckbox(driver.StateOff, true)},
	{kind: "checkbox", state: "on", build: mCheckbox(driver.StateOn, true), accent: true},
	{kind: "checkbox", state: "mixed", build: mCheckbox(driver.StateMixed, true), accent: true},
	{kind: "checkbox", state: "focused", build: mCheckbox(driver.StateOff, true), accent: true, focus: true},
	{kind: "checkbox", state: "focused on", build: mCheckbox(driver.StateOn, true), accent: true, focus: true},
	{kind: "checkbox", state: "disabled off", build: mCheckbox(driver.StateOff, false)},
	{kind: "checkbox", state: "disabled on", build: mCheckbox(driver.StateOn, false), accent: true},
	{kind: "checkbox", state: "disabled mixed", build: mCheckbox(driver.StateMixed, false), accent: true},

	{kind: "radio", state: "off", build: mRadio(false, true)},
	{kind: "radio", state: "on", build: mRadio(true, true), accent: true},
	{kind: "radio", state: "focused", build: mRadio(false, true), accent: true, focus: true},
	{kind: "radio", state: "focused on", build: mRadio(true, true), accent: true, focus: true},
	{kind: "radio", state: "disabled off", build: mRadio(false, false)},
	{kind: "radio", state: "disabled on", build: mRadio(true, false), accent: true},

	{kind: "slider", state: "normal", build: mSlider(0, true), accent: true},
	{kind: "slider", state: "ticks", build: mSlider(5, true), accent: true},
	{kind: "slider", state: "focused", build: mSlider(0, true), accent: true, focus: true},
	{kind: "slider", state: "disabled", build: mSlider(0, false), accent: true},
	{kind: "slider", state: "disabled ticks", build: mSlider(5, false), accent: true},

	{kind: "progress", state: "bar", build: mProgress(driver.ProgressBar, false, true), accent: true},
	{kind: "progress", state: "indeterminate bar", build: mProgress(driver.ProgressBar, true, true), accent: true},
	{kind: "progress", state: "disabled bar", build: mProgress(driver.ProgressBar, false, false), accent: true},
	{kind: "progress", state: "spinner", build: mProgress(driver.ProgressSpinner, true, true)},
	{kind: "progress", state: "disabled spinner", build: mProgress(driver.ProgressSpinner, true, false)},

	{kind: "scroll view", state: "overlay", build: mScroll},
	{kind: "scroll view", state: "legacy", build: mScroll, appearance: func(a *driver.Appearance) { a.LegacyScrollers = true }},

	{kind: "text field", state: "placeholder", build: mField("", noSel, true)},
	{kind: "text field", state: "text", build: mField("Hello", noSel, true)},
	{kind: "text field", state: "focused", build: mField("Hello", noSel, true), accent: true, focus: true},
	{kind: "text field", state: "selected", build: mField("Hello", [2]int{1, 4}, true), accent: true, focus: true, keepsFocusLook: true},
	{kind: "text field", state: "disabled", build: mField("Hello", noSel, false)},

	{kind: "text view", state: "normal", build: mTextView(noSel, true)},
	{kind: "text view", state: "focused", build: mTextView(noSel, true), accent: true, focus: true},
	{kind: "text view", state: "selected", build: mTextView([2]int{2, 15}, true), accent: true, focus: true, keepsFocusLook: true},
	{kind: "text view", state: "read-only", build: mTextView(noSel, false)},

	{kind: "pop-up button", state: "normal", build: mPopUp(true), accent: true},
	{kind: "pop-up button", state: "focused", build: mPopUp(true), accent: true, focus: true},
	{kind: "pop-up button", state: "disabled", build: mPopUp(false)},
}

// TestInactiveWindowMatrix checks the macOS rules of an inactive window
// over every control kind in each state that has a key look, light and
// dark, with and without Increase Contrast (S6.1a, S6.1e). Each case is
// painted in a key and an inactive window, with the default, a second
// and a light accent (S6.1g):
//
//   - In an inactive window no color derives from the accent: its three
//     dumps are equal (accent fills grey, the selection unemphasized, the
//     focus ring hidden), and a default button looks like a plain one.
//   - A focused control's inactive look is its unfocused inactive look
//     (no focus ring, no caret), except a field's selection, which stays
//     unemphasized.
//   - A state whose key look uses no accent (labels, separators, boxes,
//     scrollers, fields' text and bezels, disabled controls without an
//     accent fill) looks the same in a key and an inactive window. A state
//     marked accent does change with the accent in a key window, so the
//     first rule is not vacuous; a disabled accent fill (a checked
//     checkbox, a slider's track) is the faded accent in a key window and
//     the faded grey in an inactive one, as in AppKit.
//   - A color the app chose from the accent (a label in
//     ColorControlAccent) follows the accent and keeps it in an inactive
//     window.
func TestInactiveWindowMatrix(t *testing.T) {
	for _, contrast := range []bool{false, true} {
		for _, dark := range []bool{false, true} {
			for _, c := range inactiveCases {
				name := fmt.Sprintf("%s/%s/dark=%v/contrast=%v", c.kind, c.state, dark, contrast)
				t.Run(name, func(t *testing.T) {
					d := inactiveDumps(t, c, dark, contrast, c.focus)
					if c.appColor {
						if d.key != d.inactive || d.inactiveOther != d.keyOther || d.key == d.keyOther {
							t.Errorf("app color: key %s / %s, inactive %s / %s", d.key, d.keyOther, d.inactive, d.inactiveOther)
						}
						return
					}
					if d.inactive != d.inactiveOther || d.inactive != d.inactiveLight {
						t.Errorf("inactive look depends on the accent:\n%s", lineDiff(d.inactive, d.inactiveOther)+lineDiff(d.inactive, d.inactiveLight))
					}
					if c.accent {
						if d.key == d.keyOther {
							t.Errorf("key look does not use the accent:\n%s", d.key)
						}
					} else {
						if d.key != d.keyOther {
							t.Errorf("key look depends on the accent:\n%s", lineDiff(d.key, d.keyOther))
						}
						if d.key != d.inactive {
							t.Errorf("key and inactive look differ:\n%s", lineDiff(d.key, d.inactive))
						}
					}
					if c.inactiveAs != "" {
						o := inactiveDumps(t, findCase(t, c.kind, c.inactiveAs), dark, contrast, false)
						if d.inactive != o.inactive {
							t.Errorf("inactive look is not the %s one:\n%s", c.inactiveAs, lineDiff(o.inactive, d.inactive))
						}
					}
					if c.focus && !c.keepsFocusLook {
						u := inactiveDumps(t, c, dark, contrast, false)
						if d.inactive != u.inactive {
							t.Errorf("focused inactive look is not the unfocused one:\n%s", lineDiff(u.inactive, d.inactive))
						}
						if d.key == u.key {
							t.Errorf("focused key look is the unfocused one:\n%s", d.key)
						}
					}
				})
			}
		}
	}
}

// matrixDumps are the four record dumps of one case.
type matrixDumps struct{ key, keyOther, inactive, inactiveOther, inactiveLight string }

// inactiveDumps builds c in a fresh key window and returns the record
// dumps of the control's subtree in a key and an inactive window, with
// the default and the other accent.
func inactiveDumps(t *testing.T, c inactiveCase, dark, contrast, focus bool) matrixDumps {
	t.Helper()
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 120}, Style: allStyles})
	ap := driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, CaretBlink: driver.DefaultCaretBlink, IncreaseContrast: contrast}
	if c.appearance != nil {
		c.appearance(&ap)
	}
	w.SetAppearance(ap)
	wd := c.build(t, d, w)
	e := ElementOf(wd)
	if focus && !focusIn(e) {
		t.Fatal("no focusable element")
	}
	dump := func(accent driver.Color) string {
		a := ap
		a.Accent = accent
		w.SetAppearance(a)
		paint(t, s, sf)
		rc := record.New(render.Size{W: 200, H: 148})
		PaintTree(rc, e)
		if err := rc.End(); err != nil {
			t.Fatal(err)
		}
		return rc.Dump()
	}
	var m matrixDumps
	m.key = dump(driver.DefaultAccent)
	m.keyOther = dump(otherAccent)
	s.Deactivate()
	m.inactiveOther = dump(otherAccent)
	m.inactiveLight = dump(lightAccent)
	m.inactive = dump(driver.DefaultAccent)
	return m
}

// findCase returns the matrix case of kind in state.
func findCase(t *testing.T, kind, state string) inactiveCase {
	t.Helper()
	for _, c := range inactiveCases {
		if c.kind == kind && c.state == state {
			return c
		}
	}
	t.Fatalf("no case %s/%s", kind, state)
	return inactiveCase{}
}

// focusIn focuses e or its first focusable descendant.
func focusIn(e Element) bool {
	for _, x := range preOrder(e, nil) {
		if x.base().Focusable() && x.base().Focus() {
			return true
		}
	}
	return false
}

// lineDiff prints the lines of a and b that differ, position by position.
func lineDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	var out strings.Builder
	for i := range max(len(al), len(bl)) {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			fmt.Fprintf(&out, "- %s\n+ %s\n", x, y)
		}
	}
	return out.String()
}

// TestAlertInactiveStates pins an alert's buttons in a sheet and in an
// app-modal surface, in a key and an inactive window, light and dark, as
// record dumps in testdata/sheet.states (S6.1a). The focus starts on the
// default button's neighbour, so the key look has a default button and a
// focus ring; the inactive look has neither and no accent-derived color
// (the same dump with a second accent), and the panel, the icon and the
// labels are unchanged.
func TestAlertInactiveStates(t *testing.T) {
	var out strings.Builder
	for _, contrast := range []bool{false, true} {
		for _, dark := range []bool{false, true} {
			for _, appModal := range []bool{false, true} {
				sys := driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, CaretBlink: driver.DefaultCaretBlink, IncreaseContrast: contrast}
				sheetRun(t, sys, func(e *sheetEnv) {
					var a *alert
					kind := "sheet"
					if appModal {
						kind = "app-modal"
						e.showApp(threeButtonAlert)
						e.settle()
						a = e.d.Alerts()[0].Root().(*alertRoot).alert
					} else {
						e.show(threeButtonAlert)
						e.settle()
						a = e.w.shownSheet().alert
					}
					look := func(accent driver.Color) string {
						ap := sys
						ap.Accent = accent
						e.sh.SetAppearance(ap)
						e.settle()
						return dump(t, a, driver.Size{W: 600, H: 600})
					}
					key, keyOther := look(driver.DefaultAccent), look(otherAccent)
					e.sh.Deactivate()
					in, inOther := look(otherAccent), look(driver.DefaultAccent)
					if key == keyOther {
						t.Errorf("%s: key look does not use the accent", kind)
					}
					if in != inOther {
						t.Errorf("%s: inactive look depends on the accent:\n%s", kind, lineDiff(in, inOther))
					}
					if a.Button(1).view.FocusState() != FocusedInactive {
						t.Errorf("%s: focus state %v", kind, a.Button(1).view.FocusState())
					}
					// Only the buttons change: the panel, the icon and the labels
					// come before them and are the same.
					if p, q := alertPanelLines(key), alertPanelLines(in); p == "" || p != q {
						t.Errorf("%s: panel or labels changed:\n%s", kind, lineDiff(p, q))
					}
					mode := "light"
					if dark {
						mode = "dark"
					}
					mode += contrastTag(contrast)
					fmt.Fprintf(&out, "## %s, key, %s\n%s\n## %s, inactive, %s\n%s\n", kind, mode, key, kind, mode, inOther)
				})
			}
		}
	}
	checkStates(t, "sheet.states", out.String())
}

// checkStates compares a states dump with testdata/name, or rewrites the
// file with -update.
func checkStates(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
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
		t.Errorf("states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}

// alertPanelLines returns the lines of an alert dump up to its last
// label (the bold message and the detail), before the buttons.
func alertPanelLines(d string) string {
	ls := strings.Split(d, "\n")
	last := -1
	for i, l := range ls {
		if strings.HasPrefix(l, "text (") && strings.Contains(l, "pt w700") || strings.Contains(l, " 11pt ") {
			last = i
		}
	}
	return strings.Join(ls[:last+1], "\n")
}

// contrastTag returns the suffix of a .states header for an appearance
// with Increase Contrast (S6.1e): ", contrast", or nothing.
func contrastTag(contrast bool) string {
	if contrast {
		return ", contrast"
	}
	return ""
}
