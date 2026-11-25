package aqua

import (
	"slices"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/menubar"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The role items of the strip and their validation (PLAN.md S5.6g), over
// the fake shell.

// roleSpec has the default app and Edit menus and a Window menu with the
// three window roles and a Close role item with an ID (the app's).
var roleSpec = &driver.MenuBarSpec{Menus: []driver.MenuSpec{
	driver.DefaultAppMenu(),
	driver.DefaultEditMenu(),
	{Title: "Window", Role: driver.MenuWindow, Items: []driver.MenuItemSpec{
		{Role: driver.ItemMinimize},
		{Role: driver.ItemZoom},
		{Role: driver.ItemClose},
		{ID: 9, Title: "Close Tab", Role: driver.ItemClose},
	}},
}}

// The menus and items of roleSpec.
const (
	appMenu, editMenu, windowMenu = 0, 1, 2

	itemAbout, itemHide, itemHideOthers, itemShowAll, itemQuit = 0, 2, 3, 4, 6

	itemUndo, itemRedo, itemCut, itemCopy, itemPaste, itemDelete, itemSelectAll = 0, 1, 3, 4, 5, 6, 7

	itemMinimize, itemZoom, itemClose, itemCloseTab = 0, 1, 2, 3
)

// roleApp is an AppHandler that logs quit requests and picks.
type roleApp struct {
	appLog
	quit  bool // QuitRequested's answer
	log   []string
	picks []int
}

func (a *roleApp) QuitRequested() bool     { a.log = append(a.log, "quitRequested"); return a.quit }
func (a *roleApp) MenuItemSelected(id int) { a.picks = append(a.picks, id) }

// roleEnv is a driver on a started fake shell (not Run: its handler and
// name set directly) with roleSpec as its bar.
type roleEnv struct {
	t *testing.T
	s *fake.Shell
	d *Driver
	h *roleApp
}

func newRoleEnv(t *testing.T) *roleEnv {
	t.Helper()
	s := startShell(t)
	d := newDriver(s)
	h := &roleApp{quit: true}
	d.cfg.Name, d.h = "Test", h
	d.SetMenuBar(roleSpec)
	return &roleEnv{t: t, s: s, d: d, h: h}
}

// window opens and shows a 600×100 driver window of style with content
// (nil: none), painted once.
func (e *roleEnv) window(style driver.WindowStyle, content driver.Widget) (*Window, *fake.Surface, *winLog) {
	e.t.Helper()
	h := &winLog{}
	dw, err := e.d.NewWindow(driver.WindowConfig{Title: "W", Size: driver.Size{W: 600, H: 100}, Style: style}, h)
	if err != nil {
		e.t.Fatal(err)
	}
	w := dw.(*Window)
	if content != nil {
		w.SetContent(content)
		content.SetFrame(driver.Rect{X: 10, Y: 10, W: 180, H: 60})
	}
	w.Show()
	sf := w.Surface().(*fake.Surface)
	paint(e.t, e.s, sf)
	h.take()
	return w, sf, h
}

// enabled opens menu i of w's strip and returns its items as "Title+"
// (enabled) or "Title-" (disabled), separators left out; the menu is closed
// again.
func enabled(t *testing.T, w *Window, i int) string {
	t.Helper()
	st := w.menuStrip()
	if !st.openMenu(i, menuOpening{}) {
		t.Fatalf("menu %d did not open", i)
	}
	defer st.CloseMenu()
	var out []string
	for _, it := range st.menu.Items() {
		if it.Separator {
			continue
		}
		mark := "+"
		if it.Disabled {
			mark = "-"
		}
		out = append(out, it.Title+mark)
	}
	return strings.Join(out, " ")
}

// TestMenuRolesAppMenu: Hide Others is disabled (About, S5.7g, in
// TestAboutAlert), Quit asks
// QuitRequested and quits only on true, Hide minimizes every shown window
// and Show All shows them again, the picking window last (key).
func TestMenuRolesAppMenu(t *testing.T) {
	e := newRoleEnv(t)
	w1, sf1, _ := e.window(allStyles, nil)
	w2, sf2, _ := e.window(allStyles, nil)
	hidden, err := e.d.NewWindow(driver.WindowConfig{Title: "H", Size: driver.Size{W: 600, H: 100}, Style: allStyles}, &winLog{})
	if err != nil {
		t.Fatal(err)
	}
	if got := enabled(t, w1, appMenu); got != "About Test+ Hide Test+ Hide Others- Show All+ Quit Test+" {
		t.Errorf("app menu: %s", got)
	}

	w1.menuPicked(menubar.Ref{Menu: appMenu, Item: itemAbout})
	w1.menuPicked(menubar.Ref{Menu: appMenu, Item: itemHideOthers})
	if len(e.h.log) != 0 || sf1.Minimized() || e.d.Quitting() {
		t.Fatalf("disabled items ran: %v", e.h.log)
	}

	e.h.quit = false
	w1.menuPicked(menubar.Ref{Menu: appMenu, Item: itemQuit})
	if strings.Join(e.h.log, " ") != "quitRequested" || e.d.Quitting() {
		t.Errorf("refused quit: %v, quitting %v", e.h.log, e.d.Quitting())
	}

	w2.menuPicked(menubar.Ref{Menu: appMenu, Item: itemHide})
	hs := hidden.(*Window).Surface().(*fake.Surface)
	if !sf1.Minimized() || !sf2.Minimized() || hs.Visible() {
		t.Errorf("hide: minimized %v %v, the hidden window shown %v", sf1.Minimized(), sf2.Minimized(), hs.Visible())
	}
	w1.menuPicked(menubar.Ref{Menu: appMenu, Item: itemShowAll})
	if sf1.Minimized() || sf2.Minimized() || hs.Visible() || !w1.IsKey() {
		t.Errorf("show all: minimized %v %v, hidden shown %v, w1 key %v", sf1.Minimized(), sf2.Minimized(), hs.Visible(), w1.IsKey())
	}

	e.h.quit = true
	w1.menuPicked(menubar.Ref{Menu: appMenu, Item: itemQuit})
	if !e.d.Quitting() {
		t.Error("quit not run")
	}
}

// TestMenuRolesWindowMenu: Minimize, Zoom and Close act on the picking
// window, enabled by its styles; Close asks ShouldClose; a role item with
// an ID is the app's, reported and enabled by its flag.
func TestMenuRolesWindowMenu(t *testing.T) {
	e := newRoleEnv(t)
	w, sf, wl := e.window(allStyles, nil)
	if got := enabled(t, w, windowMenu); got != "Minimize+ Zoom+ Close+ Close Tab+" {
		t.Errorf("window menu: %s", got)
	}
	w.menuPicked(menubar.Ref{Menu: windowMenu, Item: itemZoom})
	if !sf.Zoomed() {
		t.Error("zoom")
	}
	wl.take() // resized
	w.menuPicked(menubar.Ref{Menu: windowMenu, Item: itemCloseTab})
	if !slices.Equal(e.h.picks, []int{9}) || wl.take() != "" || w.Closed() {
		t.Errorf("close tab: picks %v, closed %v", e.h.picks, w.Closed())
	}
	wl.refuse = true
	w.menuPicked(menubar.Ref{Menu: windowMenu, Item: itemClose})
	if wl.take() != "shouldClose" || w.Closed() {
		t.Error("refused close")
	}
	w.menuPicked(menubar.Ref{Menu: windowMenu, Item: itemMinimize})
	if !sf.Minimized() {
		t.Error("minimize")
	}
	sf.Activate()
	wl.refuse = false
	w.menuPicked(menubar.Ref{Menu: windowMenu, Item: itemClose})
	if !w.Closed() {
		t.Error("close")
	}

	// A titled window without the other styles: the lights' rules.
	plain, psf, _ := e.window(driver.StyleTitled, nil)
	if got := enabled(t, plain, windowMenu); got != "Minimize- Zoom- Close- Close Tab+" {
		t.Errorf("plain window menu: %s", got)
	}
	for _, i := range []int{itemMinimize, itemZoom, itemClose} {
		plain.menuPicked(menubar.Ref{Menu: windowMenu, Item: i})
	}
	if psf.Minimized() || psf.Zoomed() || plain.Closed() {
		t.Error("a disabled window item ran")
	}
}

// TestMenuRolesEditField: the Edit items follow the focused TextField:
// all off without one; Undo and Redo with history; Cut, Copy and Delete
// with a selection; Paste while editable; Select All with text. Each runs
// as its key does, reporting TextChanged.
func TestMenuRolesEditField(t *testing.T) {
	e := newRoleEnv(t)
	h := &fieldLog{}
	tf := e.d.NewTextField(false, h).(*TextField)
	w, sf, _ := e.window(allStyles, tf)
	w.Router().SetFocus(nil)
	if got := enabled(t, w, editMenu); got != "Undo- Redo- Cut- Copy- Paste- Delete- Select All-" {
		t.Errorf("no focus: %s", got)
	}
	tf.view.Focus()
	if got := enabled(t, w, editMenu); got != "Undo- Redo- Cut- Copy- Paste+ Delete- Select All-" {
		t.Errorf("empty field: %s", got)
	}
	sf.Type("hello")
	if got := enabled(t, w, editMenu); got != "Undo+ Redo- Cut- Copy- Paste+ Delete- Select All+" {
		t.Errorf("typed: %s", got)
	}
	pick := func(i int) { w.menuPicked(menubar.Ref{Menu: editMenu, Item: i}) }

	h.events = nil
	pick(itemSelectAll)
	if got := selOf(tf); got != "hello|0,5" {
		t.Errorf("select all: %s", got)
	}
	if got := enabled(t, w, editMenu); got != "Undo+ Redo- Cut+ Copy+ Paste+ Delete+ Select All+" {
		t.Errorf("selected: %s", got)
	}
	tf.Editor().SetSelection(0, 2)
	pick(itemCopy)
	pick(itemCut)
	if clipText(e.s) != "he" || selOf(tf) != "llo|0,0" {
		t.Errorf("copy, cut: clipboard %q, %s", clipText(e.s), selOf(tf))
	}
	tf.Editor().SetSelection(2, 3)
	pick(itemDelete)
	if selOf(tf) != "ll|2,2" {
		t.Errorf("delete: %s", selOf(tf))
	}
	pick(itemPaste)
	e.s.Step()
	if selOf(tf) != "llhe|4,4" {
		t.Errorf("paste: %s", selOf(tf))
	}
	pick(itemUndo)
	pick(itemUndo)
	if selOf(tf) != "llo|2,3" {
		t.Errorf("undo twice: %s", selOf(tf))
	}
	if got := enabled(t, w, editMenu); !strings.HasPrefix(got, "Undo+ Redo+") {
		t.Errorf("after undo: %s", got)
	}
	pick(itemRedo)
	if selOf(tf) != "ll|2,2" {
		t.Errorf("redo: %s", selOf(tf))
	}
	if want := "changed llo;changed ll;changed llhe;changed ll;changed llo;changed ll"; strings.Join(h.events, ";") != want {
		t.Errorf("events %q", h.events)
	}

	// Read-only: Copy and Select All only; disabled: nothing.
	tf.SetEditable(false)
	tf.Editor().SetSelection(0, 1)
	if got := enabled(t, w, editMenu); got != "Undo- Redo- Cut- Copy+ Paste- Delete- Select All+" {
		t.Errorf("read-only: %s", got)
	}
	pick(itemDelete)
	pick(itemPaste)
	if selOf(tf) != "ll|0,1" || e.s.ClipboardReads() != 1 {
		t.Errorf("read-only edits ran: %s", selOf(tf))
	}
	tf.SetEditable(true)
	tf.SetEnabled(false)
	if got := enabled(t, w, editMenu); got != "Undo- Redo- Cut- Copy- Paste- Delete- Select All-" {
		t.Errorf("disabled: %s", got)
	}
}

// TestMenuRolesEditSecure: a SecureField never copies or cuts.
func TestMenuRolesEditSecure(t *testing.T) {
	e := newRoleEnv(t)
	tf := e.d.NewTextField(true, nil).(*TextField)
	w, sf, _ := e.window(allStyles, tf)
	tf.view.Focus()
	sf.Type("secret")
	tf.Editor().SelectAll()
	if got := enabled(t, w, editMenu); got != "Undo+ Redo- Cut- Copy- Paste+ Delete+ Select All+" {
		t.Errorf("secure: %s", got)
	}
	w.menuPicked(menubar.Ref{Menu: editMenu, Item: itemCopy})
	w.menuPicked(menubar.Ref{Menu: editMenu, Item: itemCut})
	if _, ok := e.s.Clipboard(); ok || tf.Text() != "secret" {
		t.Errorf("secure copy or cut ran: %q", tf.Text())
	}
}

// TestMenuRolesEditComposition: an Edit item commits a composition first,
// as a key does; Undo is enabled while composing.
func TestMenuRolesEditComposition(t *testing.T) {
	e := newRoleEnv(t)
	tf := e.d.NewTextField(false, nil).(*TextField)
	w, sf, _ := e.window(allStyles, tf)
	tf.view.Focus()
	sf.Preedit("にほ", 6)
	if got := enabled(t, w, editMenu); !strings.HasPrefix(got, "Undo+ Redo-") {
		t.Errorf("composing: %s", got)
	}
	w.menuPicked(menubar.Ref{Menu: editMenu, Item: itemSelectAll})
	if got := selOf(tf); got != "にほ|0,6" || tf.view.composing() {
		t.Errorf("select all while composing: %s", got)
	}
}

// TestMenuRolesEditView: the Edit items act on a focused TextView, Select
// All and Undo through its command path.
func TestMenuRolesEditView(t *testing.T) {
	e := newRoleEnv(t)
	tv := e.d.NewTextView(nil).(*TextView)
	w, sf, _ := e.window(allStyles, tv)
	tv.view.Focus()
	sf.Type("one")
	sf.Key(driver.KeyReturn, 0)
	sf.Type("two")
	if got := enabled(t, w, editMenu); got != "Undo+ Redo- Cut- Copy- Paste+ Delete- Select All+" {
		t.Errorf("view: %s", got)
	}
	w.menuPicked(menubar.Ref{Menu: editMenu, Item: itemSelectAll})
	w.menuPicked(menubar.Ref{Menu: editMenu, Item: itemCopy})
	w.menuPicked(menubar.Ref{Menu: editMenu, Item: itemDelete})
	if clipText(e.s) != "one\ntwo" || tv.Text() != "" {
		t.Errorf("copy, delete: clipboard %q, text %q", clipText(e.s), tv.Text())
	}
	w.menuPicked(menubar.Ref{Menu: editMenu, Item: itemUndo})
	if tv.Text() != "one\ntwo" {
		t.Errorf("undo: %q", tv.Text())
	}
}

// TestMenuRolesByMouse: picking Edit ▸ Copy with the mouse acts on the
// field that had the focus before the menu opened.
func TestMenuRolesByMouse(t *testing.T) {
	e := newRoleEnv(t)
	tf := e.d.NewTextField(false, nil).(*TextField)
	w, sf, _ := e.window(allStyles, tf)
	tf.view.Focus()
	sf.Type("abc")
	tf.Editor().SetSelection(1, 3)
	st := w.menuStrip()
	sf.Click(centre(st.TitleRect(editMenu)))
	pp := stripPopup(t, st, e.s, editMenu)
	if st.menu.Items()[itemUndo].Disabled || !st.menu.Items()[itemRedo].Disabled {
		t.Errorf("items at the opening %+v", st.menu.Items())
	}
	sf.Click(pp.ToParent(centre(st.menu.ItemRect(itemCopy))))
	finishPick(e.s)
	if clipText(e.s) != "bc" || w.Router().Focused() != Element(tf.view) {
		t.Errorf("clipboard %q, focus %v", clipText(e.s), w.Router().Focused())
	}
}
