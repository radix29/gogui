package gogui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/driver/stub"
)

// The tests in this file build the trees of the six examples through the
// public API, as examples/*/main.go do, show their windows and check a few
// anchor frames: on the headless driver against the numbers of the layout
// engine's own example tests (internal/layout/examples_*_test.go, S0.4), so
// the public layer and the engine can't drift apart; on aqua (aqua_test.go)
// for overlaps and clipping only (wantAnchor, S3.3b). Alerts and event
// wiring are covered by the per-control tests; the form wires its Cancel
// and Create Account actions, for the Return/Escape test in aqua_test.go
// (S3.2f), controls and notes set their menu bars with their actions, for
// the strips pinned in testdata/aqua and the menu tests there (S5.6j), and
// the form and controls are driven by keys only in
// examples_keys_aqua_test.go, controls with all its actions (S5.7h).

// showExample shows win at its configured size and returns it laid out.
func showExample(app *App, win *Window) {
	win.Show()
	drain(app)
}

// onAqua reports whether app runs on the aqua driver (aqua_test.go).
func onAqua(app *App) bool {
	_, ok := app.drv.(*aqua.Driver)
	return ok
}

// wantAnchor checks the frame of an anchor widget of an example. On the
// headless driver the frame must be want, the layout engine's number. On
// aqua the real controls size themselves from the theme (the sizing policy
// of PLAN.md S3.3), so the frames are pinned as dumps instead
// (TestExamplesOnAqua) and wantAnchor checks only that the widget is
// placed sanely: nothing clipped (it and each ancestor lie inside their
// parent's bounds, except a scroll view's document) and no overlap with a
// sibling.
func wantAnchor(t testing.TB, app *App, name string, w Widget, want Rect) {
	t.Helper()
	if !onAqua(app) {
		wantFrame(t, name, w, want)
		return
	}
	for x := w; x.base().parent != nil; x = x.base().parent {
		if msg := misplaced(x); msg != "" {
			t.Errorf("%s on aqua: %s %s", name, x.base().kind, msg)
		}
	}
}

// misplaced returns why w's frame is not sane in its parent ("" when it
// is): a negative size, outside the parent's bounds (a scroll view's
// document may be larger), or overlapping a sibling.
func misplaced(w Widget) string {
	const eps = 1e-9
	r := frameOf(w)
	if r.W < 0 || r.H < 0 {
		return fmt.Sprintf("%v has a negative size", r)
	}
	parent := w.base().parent
	if parent.peerWidget() != nil && parent.base().kind != kindScrollView {
		pr := frameOf(parent)
		if r.X < -eps || r.Y < -eps || r.X+r.W > pr.W+eps || r.Y+r.H > pr.H+eps {
			return fmt.Sprintf("%v is clipped by its %s of %gx%g", r, parent.base().kind, pr.W, pr.H)
		}
	}
	p, ok := parent.(parentWidget)
	if !ok {
		return ""
	}
	for _, s := range p.childWidgets() {
		if s == w || s.peerWidget() == nil {
			continue
		}
		o := frameOf(s)
		if min(r.X+r.W, o.X+o.W)-max(r.X, o.X) > eps && min(r.Y+r.H, o.Y+o.H)-max(r.Y, o.Y) > eps {
			return fmt.Sprintf("%v overlaps the %s at %v", r, s.base().kind, o)
		}
	}
	return ""
}

func TestExampleHelloTree(t *testing.T) { runApp(t, func(app *App) { exampleHello(t, app) }) }

// exampleHello builds the hello example, shows it and checks its frames.
func exampleHello(t testing.TB, app *App) *Window {
	greeting := NewLabel("Hello!").SetFont(BoldSystemFont(15))
	name := NewTextField().SetPlaceholder("Your name")
	greet := NewButton("Greet").SetDefault(true)
	win := app.NewWindow(WindowConfig{
		Title: "Hello", Size: Size{W: 360, H: 180},
		Style:  Titled | Closable | Miniaturizable | Resizable,
		Center: true,
	})
	row := HStack(8, Spacer(), greet)
	win.SetContent(VStack(12, greeting, name, row).SetPadding(20))
	showExample(app, win)

	gh := 240.0 / 13 // bold 15 pt line height
	wantAnchor(t, app, "name", name, Rect{X: 20, Y: 20 + gh + 12, W: 320, H: 22})
	wantAnchor(t, app, "row", row, Rect{X: 20, Y: 20 + gh + 12 + 22 + 12, W: 320, H: 22})
	wantAnchor(t, app, "greet", greet, Rect{X: 257, W: 63, H: 22})
	return win
}

func TestExampleFormTree(t *testing.T) { runApp(t, func(app *App) { exampleForm(t, app) }) }

// exampleForm builds the form example, shows it and checks its frames.
// formExample is the form example's window and the widgets the tests drive.
type formExample struct {
	win            *Window
	name, email    *TextField
	newsletter     *Checkbox
	terms          *Checkbox
	cancel, create *Button
	role           *PopUpButton
	form           Widget
	password       *TextField
	plan           *RadioGroup
}

// newFormExample builds the form example as examples/form does, with its
// Cancel (closes) and Create Account (validates, then alerts) actions.
func newFormExample(app *App) formExample {
	var f formExample
	f.name = NewTextField().SetPlaceholder("Jane Appleseed")
	f.email = NewTextField().SetPlaceholder("name@example.com")
	f.password = NewSecureField()
	f.role = NewPopUpButton("Viewer", "Editor", "Administrator")
	f.plan = NewRadioGroup("Free", "Pro", "Team").SetSelected(0)
	f.newsletter = NewCheckbox("Send me product updates")
	f.terms = NewCheckbox("I agree to the terms of service")
	f.cancel = NewButton("Cancel").SetCancel(true)
	f.create = NewButton("Create Account").SetDefault(true).SetEnabled(false)
	f.terms.OnChange(func(on bool) { f.create.SetEnabled(on) })
	f.win = app.NewWindow(WindowConfig{
		Title: "New Account", Size: Size{W: 460, H: 380},
		Style: Titled | Closable, Center: true,
	})
	f.cancel.OnClick(f.win.Close)
	f.create.OnClick(func() {
		if strings.TrimSpace(f.name.Text()) == "" || !strings.Contains(f.email.Text(), "@") {
			Alert{
				Style:   AlertCritical,
				Message: "Please check your details",
				Detail:  "A name and a valid e-mail address are required.",
			}.Show(f.win, nil)
			return
		}
		Alert{
			Message: "Account created",
			Detail:  "Welcome, " + f.name.Text() + " (" + f.role.Item(f.role.Selected()) + ").",
		}.Show(f.win, func(int) { f.win.Close() })
	})
	f.form = Form(
		Row("Name:", f.name), Row("E-mail:", f.email), Row("Password:", f.password),
		Row("Role:", f.role), Row("Plan:", f.plan),
		Row("", f.newsletter), Row("", f.terms),
	)
	f.win.SetContent(VStack(16, f.form, Spacer(), HStack(8, Spacer(), f.cancel, f.create)).SetPadding(20))
	return f
}

func exampleForm(t testing.TB, app *App) *Window {
	f := newFormExample(app)
	win, form, name, password, role, plan, terms, create := f.win, f.form, f.name, f.password, f.role, f.plan, f.terms, f.create
	showExample(app, win)

	wantAnchor(t, app, "form", form, Rect{X: 20, Y: 20, W: 420, H: 238})
	// The controls start at x = 71 (label column 63 + 8).
	wantAnchor(t, app, "name", name, Rect{X: 71, Y: 0, W: 349, H: 22})
	wantAnchor(t, app, "password", password, Rect{X: 71, Y: 60, W: 349, H: 22})
	wantAnchor(t, app, "role", role, Rect{X: 71, Y: 90, W: 7*13 + 32, H: 22})
	wantAnchor(t, app, "plan", plan, Rect{X: 71, Y: 120, W: 46, H: 66})
	wantAnchor(t, app, "terms", terms, Rect{X: 71, Y: 220, W: 18 + 31*7, H: 18})
	if !stubBase(create).Disabled || !stubPeer(create).(*stub.Button).IsDefault {
		t.Error("create: want a disabled default button")
	}
	return win
}

func TestExampleConverterTree(t *testing.T) { runApp(t, func(app *App) { exampleConverter(t, app) }) }

// exampleConverter builds the converter example, shows it and checks its frames.
func exampleConverter(t testing.TB, app *App) *Window {
	celsius, fahrenheit := NewTextField(), NewTextField()
	slider := NewSlider().SetRange(-40, 100)
	syncBar := NewProgressBar().SetRange(0, 1)
	updated := NewLabel("").SetTextColor(ColorSecondaryLabel)
	win := app.NewWindow(WindowConfig{
		Title: "Temperature", Size: Size{W: 380, H: 220},
		Style: Titled | Closable | Miniaturizable, Center: true,
	})
	win.SetContent(VStack(12,
		Form(Row("Celsius:", celsius), Row("Fahrenheit:", fahrenheit)),
		slider, NewSeparator(), HStack(8, syncBar, updated),
	).SetPadding(20))
	showExample(app, win)

	wantAnchor(t, app, "celsius", celsius, Rect{X: 85, Y: 0, W: 255, H: 22})
	wantAnchor(t, app, "slider", slider, Rect{X: 20, Y: 84, W: 340, H: 22})
	wantAnchor(t, app, "sync bar", syncBar, Rect{W: 332, H: 22})

	// A sync from a goroutine relabels the status through Do; the
	// deferred pass shrinks the bar.
	done := make(chan struct{})
	go func() {
		Do(func() { updated.SetText("Synced at 15:04:05") })
		close(done)
	}()
	<-done
	drain(app)
	wantAnchor(t, app, "sync bar after sync", syncBar, Rect{W: 206, H: 22})
	wantAnchor(t, app, "updated after sync", updated, Rect{X: 214, Y: 3, W: 126, H: 16})
	return win
}

func TestExampleControlsTree(t *testing.T) { runApp(t, func(app *App) { exampleControls(t, app) }) }

// controlsExample is the controls example's window and the widgets the
// tests drive.
type controlsExample struct {
	win              *Window
	status           *Label
	push, def, alert *Button
	check, mixed     *Checkbox
	radios           *RadioGroup
	field, secure    *TextField
	slider           *Slider
	popup            *PopUpButton
	busy, progress   *ProgressBar
	sep              *Separator
	disabled         *Button
	checkOff         *Checkbox
}

// newControlsExample builds the controls example with its menu bar and
// its actions, which report to the status label, as examples/controls
// does.
func newControlsExample(app *App) controlsExample {
	app.SetMenuBar(MenuBar(
		AppMenu(),
		EditMenu(),
		Menu("View",
			MenuItem("Toggle Appearance", Shortcut(KeyPrimary|KeyShift, 'D'),
				func() { app.SetAppearance(app.Appearance().Toggled()) }),
			MenuSeparator(),
			MenuItem("System Accent", KeyShortcut{}, func() { app.SetAccentColor(ColorControlAccent) }),
			MenuItem("Purple Accent", KeyShortcut{}, func() { app.SetAccentColor(RGB8(149, 61, 150)) }),
			MenuItem("Green Accent", KeyShortcut{}, func() { app.SetAccentColor(RGB8(98, 186, 70)) }),
			MenuItem("Yellow Accent", KeyShortcut{}, func() { app.SetAccentColor(RGB8(255, 197, 0)) }),
		),
	))
	var c controlsExample
	c.status = NewLabel("Ready").SetTextColor(ColorSecondaryLabel)
	report := func(format string, a ...any) { c.status.SetText(fmt.Sprintf(format, a...)) }
	c.push = NewButton("Push Button")
	c.def = NewButton("Default").SetDefault(true).SetTooltip("The default button: Return clicks it")
	c.push.OnClick(func() { report("Push Button clicked") })
	c.def.OnClick(func() { report("Default clicked") })
	c.disabled = NewButton("Disabled").SetEnabled(false).SetTooltip("A disabled control still shows its tooltip")
	c.check = NewCheckbox("Checkbox").SetChecked(true)
	c.check.OnChange(func(on bool) { report("Checkbox: %v", on) })
	c.mixed = NewCheckbox("Mixed").SetState(StateMixed)
	c.checkOff = NewCheckbox("Disabled").SetEnabled(false)
	c.radios = NewRadioGroup("Small", "Medium", "Large").SetSelected(1).SetTooltip("The arrow keys move the selection")
	c.radios.OnChange(func(i int) { report("Radio: %d", i) })
	c.field = NewTextField().SetPlaceholder("Text Field")
	c.field.OnSubmit(func(s string) { report("Submitted: %q", s) })
	c.secure = NewSecureField().SetPlaceholder("Password")
	c.progress = NewProgressBar().SetRange(0, 100).SetValue(40)
	c.slider = NewSlider().SetRange(0, 100).SetValue(40).
		SetTooltip("Drag it, or use the arrow, Page Up/Down, Home and End keys")
	c.slider.OnChange(func(v float64) { c.progress.SetValue(v) })
	c.busy = NewProgressBar().SetIndeterminate(true)
	c.popup = NewPopUpButton("Red", "Green", "Blue").SetSelected(2)
	c.popup.OnChange(func(i int) { report("Pop-up: %s", c.popup.Item(i)) })
	c.win = app.NewWindow(WindowConfig{
		Title: "Controls", Size: Size{W: 520, H: 560}, MinSize: Size{W: 420, H: 480},
		Style:  Titled | Closable | Miniaturizable | Resizable,
		Center: true,
	})
	c.alert = NewButton("Show Alert…")
	c.alert.OnClick(func() {
		Alert{
			Style:   AlertWarning,
			Message: "Delete this item?",
			Detail:  "This action cannot be undone.",
			Buttons: []string{"Delete", "Cancel"},
		}.Show(c.win, func(result int) { report("Alert result: %d", result) })
	})
	c.sep = NewSeparator()
	c.win.SetContent(VStack(14,
		NewBox("Buttons").SetContent(HStack(8, c.push, c.def, c.disabled, Spacer())),
		NewBox("Toggles").SetContent(HStack(20, VStack(6, c.check, c.mixed, c.checkOff), c.radios, Spacer())),
		NewBox("Text").SetContent(VStack(8, c.field, c.secure)),
		NewBox("Values").SetContent(VStack(10, c.slider, c.progress, HStack(8, c.busy, Spacer(), c.popup))),
		c.sep,
		HStack(8, c.status, Spacer(), c.alert),
	).SetPadding(20))
	return c
}

// exampleControls builds the controls example, shows it and checks its
// frames.
func exampleControls(t testing.TB, app *App) *Window {
	c := newControlsExample(app)
	showExample(app, c.win)

	wantAnchor(t, app, "default", c.def, Rect{X: 113, W: 77, H: 22})
	wantAnchor(t, app, "radio group", c.radios, Rect{X: 94, W: 60, H: 66})
	wantAnchor(t, app, "secure", c.secure, Rect{Y: 30, W: 464, H: 22})
	// The indeterminate bar shares the leftover 285 pt with the spacer.
	wantAnchor(t, app, "busy", c.busy, Rect{W: 96 + 142.5, H: 22})
	wantAnchor(t, app, "popup", c.popup, Rect{X: 397, W: 67, H: 22})
	wantAnchor(t, app, "separator", c.sep, Rect{X: 20, Y: 430, W: 480, H: 1})
	wantAnchor(t, app, "alert", c.alert, Rect{X: 375, W: 105, H: 22})
	return c.win
}

func TestExampleGalleryTree(t *testing.T) { runApp(t, func(app *App) { exampleGallery(t, app) }) }

// exampleGallery builds the gallery example (the golden-image scene of
// internal/aqua/gallery through the public API, laid out with stacks) as
// examples/gallery does, shows it and checks its frames.
func exampleGallery(t testing.TB, app *App) *Window {
	large := NewButton("Large").SetControlSize(ControlSizeLarge)
	colA := VStack(6,
		NewLabel("Label"),
		NewLabel("Secondary Label").SetTextColor(ColorSecondaryLabel),
		NewLabel("Disabled Label").SetTextColor(ColorDisabledControlText),
		NewLabel("Small Label").SetFont(SystemFont(11)),
		NewLabel("Bold Label").SetFont(BoldSystemFont(0)),
		NewSeparator(),
		NewButton("Push Button"),
		NewButton("Default").SetDefault(true),
		NewButton("Cancel").SetCancel(true),
		NewButton("Disabled").SetEnabled(false),
		NewButton("Small").SetControlSize(ControlSizeSmall),
		NewButton("Mini").SetControlSize(ControlSizeMini),
		large,
		Spacer(),
	)
	focused := NewCheckbox("Focused")
	radios := NewRadioGroup("Small", "Medium", "Large").SetSelected(1)
	colB := VStack(6,
		NewCheckbox("Checkbox"),
		NewCheckbox("Checked").SetChecked(true),
		NewCheckbox("Mixed").SetState(StateMixed),
		NewCheckbox("Disabled").SetChecked(true).SetEnabled(false),
		focused,
		NewBox("Radio Group").SetContent(radios),
		Spacer(),
	)
	rows := make([]Widget, 0, 8)
	for row := range 8 {
		cells := make([]Widget, 0, 3)
		for col := range 3 {
			cells = append(cells, NewLabel(fmt.Sprintf("Item %d.%d", row+1, col+1)))
		}
		rows = append(rows, HStack(40, cells...))
	}
	scroll := NewScrollView(VStack(14, rows...).SetPadding(10)).
		SetHasVerticalScroller(true).
		SetHasHorizontalScroller(true).
		SetMinHeight(120)
	spinner := NewSpinner().SetIndeterminate(true)
	colC := VStack(10,
		NewSlider().SetRange(0, 100).SetValue(40),
		NewSlider().SetRange(0, 100).SetValue(75).SetTickMarks(5, false),
		NewSlider().SetRange(0, 100).SetValue(25).SetEnabled(false),
		NewProgressBar().SetRange(0, 100).SetValue(40),
		NewProgressBar().SetIndeterminate(true),
		HStack(8, spinner, Spacer()),
		scroll,
	)
	win := app.NewWindow(WindowConfig{
		Title: "Gallery", Size: Size{W: 640, H: 400}, MinSize: Size{W: 560, H: 380},
		Style:  Titled | Closable | Miniaturizable | Resizable,
		Center: true,
	})
	win.SetContent(HStack(16,
		colA, NewSeparator().SetVertical(true),
		colB, NewSeparator().SetVertical(true),
		colC,
	).SetPadding(20))
	showExample(app, win)
	focused.Focus()
	scroll.SetOffset(Point{X: 60, Y: 40})
	drain(app)

	wantAnchor(t, app, "large", large, Rect{Y: 273.53846153846155, W: 63, H: 28})
	wantAnchor(t, app, "radio group", radios, Rect{W: 60, H: 66})
	wantAnchor(t, app, "spinner", spinner, Rect{W: 32, H: 32})
	wantAnchor(t, app, "scroll view", scroll, Rect{Y: 202, W: 224.5, H: 158})
	return win
}

func TestExampleNotesTree(t *testing.T) { runApp(t, func(app *App) { exampleNotes(t, app) }) }

// notesWelcome is the text examples/notes starts with (its const welcome).
const notesWelcome = "Welcome to Notes.\n\nType here. Scrolling, selection, undo and IME all work like on a Mac.\n"

// exampleNotes builds the notes example with its menu bar (as examples/notes
// sets it; New does not recount here), shows it and checks its frames.
func exampleNotes(t testing.TB, app *App) *Window {
	editor := NewTextView().SetText(notesWelcome).SetFont(SystemFont(13))
	stats := NewLabel("16 words, 89 characters").SetTextColor(ColorSecondaryLabel)
	settings := VStack(8)
	for i := 1; i <= 30; i++ {
		settings.Add(NewCheckbox(fmt.Sprintf("Option %d", i)))
	}
	sidebar := NewScrollView(settings.SetPadding(12)).SetHasVerticalScroller(true).SetMinWidth(180)
	app.SetMenuBar(MenuBar(
		AppMenu(),
		Menu("File",
			MenuItem("New", Shortcut(KeyPrimary, 'N'), func() { editor.SetText("") }),
		),
		EditMenu(),
		Menu("Format",
			MenuItem("Bigger", Shortcut(KeyPrimary, '+'),
				func() { editor.SetFont(SystemFont(editor.Font().Size + 1)) }),
			MenuItem("Smaller", Shortcut(KeyPrimary, '-'),
				func() { editor.SetFont(SystemFont(editor.Font().Size - 1)) }),
		),
	))
	win := app.NewWindow(WindowConfig{
		Title: "Notes", Size: Size{W: 720, H: 480}, MinSize: Size{W: 480, H: 320},
		Style:  Titled | Closable | Miniaturizable | Resizable,
		Center: true,
	})
	win.SetContent(VStack(0,
		HStack(0, sidebar, NewSeparator().SetVertical(true), editor),
		NewSeparator(),
		HStack(8, stats, Spacer()).SetPadding(6),
	))
	showExample(app, win)
	editor.Focus()

	wantAnchor(t, app, "sidebar", sidebar, Rect{W: 180, H: 451})
	wantAnchor(t, app, "editor", editor, Rect{X: 181, W: 539, H: 451})
	wantAnchor(t, app, "document", settings, Rect{W: 180, H: 796})
	if s := stubPeer(sidebar).(*stub.ScrollView).DocumentSize; !onAqua(app) && s != (Size{W: 180, H: 796}) {
		t.Errorf("document size %v, want 180 × 796", s)
	}
	wantAnchor(t, app, "stats", stats, Rect{X: 6, Y: 6, W: 161, H: 16})
	return win
}

// TestMisplaced checks the aqua anchor check itself on hand-set frames.
func TestMisplaced(t *testing.T) {
	runApp(t, func(app *App) {
		a, b := NewButton("A"), NewButton("B")
		row := HStack(8, a, b)
		doc := NewLabel("doc")
		sv := NewScrollView(doc)
		set := func(w Widget, r Rect) { stubBase(w).Frame = r }
		set(row, Rect{W: 100, H: 30})
		set(a, Rect{W: 50, H: 22})
		for _, tt := range []struct {
			b    Rect
			want string
		}{
			{Rect{X: 58, W: 42, H: 22}, ""},
			{Rect{X: 50, W: 50, H: 22}, ""}, // touching is no overlap
			{Rect{X: 40, W: 50, H: 22}, "overlaps the Button"},
			{Rect{X: 60, W: 50, H: 22}, "is clipped by its Stack of 100x30"},
			{Rect{X: 60, Y: -1, W: 30, H: 22}, "is clipped"},
			{Rect{X: 60, W: -1, H: 22}, "negative size"},
		} {
			set(b, tt.b)
			got := misplaced(b)
			if (tt.want == "") != (got == "") || !strings.Contains(got, tt.want) {
				t.Errorf("b at %v: %q, want %q", tt.b, got, tt.want)
			}
		}
		set(sv, Rect{W: 100, H: 50})
		set(doc, Rect{W: 100, H: 500})
		if got := misplaced(doc); got != "" {
			t.Errorf("scroll view document: %q", got)
		}
		if onAqua(app) {
			t.Error("headless app reported as aqua")
		}
	})
}
