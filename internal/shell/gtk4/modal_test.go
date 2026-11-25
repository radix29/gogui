package gtk4

import (
	"errors"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// The modal surfaces' host tests (S5.7f, modal.go) over the fakeWindow:
// the window calls (setTransientFor, setSensitive, present) and what the
// handlers see. The real GTK half (gtk_window_set_modal, the window
// manager's placement, the insensitive windows) is TestModalGTK's, on
// Linux.

// modalCfg is a modal surface's config, owned by owner (nil: app-modal).
func modalCfg(owner shell.Surface) shell.SurfaceConfig {
	return shell.SurfaceConfig{Title: "m", Size: driver.Size{W: 260, H: 180},
		Style: driver.StyleResizable | driver.StyleMiniaturizable, Modal: true, Owner: owner}
}

// openShown opens a surface titled title, shows and allocates it and,
// with key, makes it the key surface; the logs are cleared.
func openShown(t *testing.T, f *winFactory, title string, key bool) (*surface, *fakeWindow, *sfHandler) {
	t.Helper()
	h := &sfHandler{allow: true}
	cfg := sfResizable
	cfg.Title = title
	sf, fw := f.open(t, cfg, h)
	sf.Show()
	fw.layout()
	if key {
		setActive(fw, true)
	}
	fw.take()
	h.take()
	return sf, fw, h
}

// setActive changes the fake window's is-active and notifies it.
func setActive(fw *fakeWindow, on bool) {
	fw.isActive = on
	fw.notify(kindActive)
}

// showModal opens and shows a modal surface over owner (nil: app-modal)
// and makes it key, as the window manager would.
func showModal(t *testing.T, f *winFactory, owner shell.Surface, others ...*fakeWindow) (*surface, *fakeWindow, *sfHandler) {
	t.Helper()
	h := &sfHandler{allow: true}
	sf, fw := f.open(t, modalCfg(owner), h)
	sf.Show()
	fw.layout()
	for _, o := range others {
		if o.isActive {
			setActive(o, false)
		}
	}
	setActive(fw, true)
	return sf, fw, h
}

// TestModalOwnerErrors: an Owner without Modal, of another shell, closed,
// or a popup's child is shell.ErrSurfaceOwner, and no window is made.
func TestModalOwnerErrors(t *testing.T) {
	s, f := newSurfaceTest(t)
	sf, _, _ := openShown(t, f, "a", true)
	dead, _, _ := openShown(t, f, "d", false)
	dead.Close()
	p, _ := openPopup(t, f, sf, shell.PopupConfig{Size: driver.Size{W: 10, H: 10}}, &popHandler{})
	other := &surface{s: &Shell{}}
	n := len(f.wins)
	for _, c := range []struct {
		name string
		cfg  shell.SurfaceConfig
	}{
		{"not modal", shell.SurfaceConfig{Size: driver.Size{W: 10, H: 10}, Owner: sf}},
		{"closed", modalCfg(dead)},
		{"other shell", modalCfg(other)},
		{"popup child", modalCfg(p.c)},
	} {
		if got, err := s.NewSurface(c.cfg, &sfHandler{}); got != nil || !errors.Is(err, shell.ErrSurfaceOwner) {
			t.Errorf("%s: NewSurface = %v, %v; want ErrSurfaceOwner", c.name, got, err)
		}
	}
	if len(f.wins) != n {
		t.Errorf("a failing NewSurface made %d windows", len(f.wins)-n)
	}
}

// TestModalWindow: a modal surface's window is modal and never
// resizable; Minimize and Zoom do nothing; nothing is blocked before its
// first Show.
func TestModalWindow(t *testing.T) {
	_, f := newSurfaceTest(t)
	a, fa, ha := openShown(t, f, "a", true)
	m, fm := f.open(t, modalCfg(a), &sfHandler{})
	if !fm.cfg.modal || fm.cfg.resizable {
		t.Errorf("window config = %+v, want modal and not resizable", fm.cfg)
	}
	if fa.insensitive || a.blocked || len(f.s.modals) != 0 {
		t.Error("a hidden modal surface blocks")
	}
	m.Show()
	fm.take()
	m.Minimize()
	m.Zoom()
	m.Center()
	wantLog(t, "Minimize, Zoom, Center", fm.take())
	if m.Zoomed() {
		t.Error("a modal surface is zoomed")
	}
	_ = ha
}

// TestModalBlocks: the first Show makes the modal transient for its owner
// and presents it, then blocks every other surface: a held button gets its
// PointerUp and the pointer its PointerLeave, the grab popup is dismissed,
// the windows go insensitive. A blocked surface drops pointer, scroll and
// key input (consuming the key), refuses a close-request without asking
// and presents the modal, does not become key but presents the modal,
// posted; Show on it presents it, then the modal. A surface created later
// is blocked too.
func TestModalBlocks(t *testing.T) {
	s, f := newSurfaceTest(t)
	a, fa, ha := openShown(t, f, "a", true)
	b, fb, hb := openShown(t, f, "b", false)
	ph := &popHandler{}
	openPopup(t, f, a, menuCfg, ph)
	ph.take()
	fa.take()
	ha.take()
	fb.ptrEnter(5, 6, fakeEv{t: 1})
	fb.ptrPressed(1, 5, 6, fakeEv{t: 2, button: 1})
	hb.take()

	m, fm := f.open(t, modalCfg(a), &sfHandler{})
	m.Show()
	wantLog(t, "modal window", fm.take(), "setTransientFor a", "present")
	wantLog(t, "a's popup", ph.take(), "dismissed")
	wantLog(t, "a", fa.take(), "setSensitive false")
	wantLog(t, "b", fb.take(), "setSensitive false")
	wantLog(t, "b's pointer", hb.take(), "up 5,6 b=1 held=0 n=0", "leave 5,6")
	if !a.blocked || m.blocked || s.activeModal() != m {
		t.Fatalf("blocked a=%v m=%v, active %p", a.blocked, m.blocked, s.activeModal())
	}

	fb.ptrReleased(1, 5, 6, fakeEv{t: 3, button: 1})
	fb.ptrEnter(7, 8, fakeEv{t: 4})
	fb.ptrMotion(9, 9, fakeEv{t: 5})
	fb.ptrPressed(1, 9, 9, fakeEv{t: 6, button: 1})
	fb.scrollBy(0, 1, gdkScrollUnitWheel, fakeEv{t: 7})
	if r := fb.keyPress(0x61, 38, 0, 8); r == 0 {
		t.Error("a blocked surface's key press is not consumed")
	}
	fb.keyRelease(0x61, 38, 0, 9)
	wantLog(t, "b's input", hb.take())

	if r := fa.closeRequest(); r == 0 {
		t.Error("close-request not stopped")
	}
	wantLog(t, "a's close-request", ha.take())
	wantLog(t, "a's close-request", fm.take(), "present")

	setActive(fa, false) // the modal took the key
	ha.take()
	setActive(fa, true)
	wantLog(t, "a activated", ha.take())
	wantLog(t, "a activated, before the post", fm.take())
	s.q.Drain()
	wantLog(t, "a activated, posted", fm.take(), "present")

	b.Show()
	wantLog(t, "Show on b", fb.take(), "present")
	wantLog(t, "Show on b", fm.take(), "present")

	_, fc := f.open(t, sfFixed, &sfHandler{})
	wantLog(t, "a new surface", fc.take(), "setSensitive false")
}

// TestModalAppModal: an app-modal surface is transient for the surface key
// at its first Show, for none without one.
func TestModalAppModal(t *testing.T) {
	_, f := newSurfaceTest(t)
	m, fm := f.open(t, modalCfg(nil), &sfHandler{})
	m.Show()
	wantLog(t, "no key surface", fm.take(), "setTransientFor -", "present")
	m.Close()

	_, fa, _ := openShown(t, f, "a", true)
	m, fm = f.open(t, modalCfg(nil), &sfHandler{})
	m.Show()
	wantLog(t, "key surface a", fm.take(), "setTransientFor a", "present")
	wantLog(t, "a", fa.take(), "setSensitive false")
}

// TestModalStack: the newest modal blocks the older one; when it closes
// key, the older one is unblocked before its window goes and presented
// after its Closed; when that closes, the owner is unblocked and
// presented.
func TestModalStack(t *testing.T) {
	s, f := newSurfaceTest(t)
	a, fa, _ := openShown(t, f, "a", true)
	m1, fm1, _ := showModal(t, f, a, fa)
	m2, fm2, h2 := showModal(t, f, nil, fm1)
	if !m1.blocked || !a.blocked || s.activeModal() != m2 {
		t.Fatalf("blocked m1=%v a=%v", m1.blocked, a.blocked)
	}
	fm1.take()
	fm2.take() // blocked by m1 until its Show, then unblocked
	fa.take()
	var order []string
	h2.onClosed = func() { order = append(order, fm1.take()...); order = append(order, "closed") }
	m2.Close()
	order = append(order, fm1.take()...)
	wantLog(t, "m1", order, "setSensitive true", "closed", "present")
	wantLog(t, "m2", fm2.take(), "destroy")
	wantLog(t, "a stays blocked", fa.take())

	setActive(fm1, true) // the window manager activates m1
	m1.Close()
	wantLog(t, "a", fa.take(), "setSensitive true", "present")
	if a.blocked || len(s.modals) != 0 {
		t.Error("a still blocked")
	}
}

// TestModalCloseKey: a modal that was not key hands nothing on; one that
// was presents the surface key at its first Show when it has no owner;
// Quit hands nothing on.
func TestModalCloseKey(t *testing.T) {
	s, f := newSurfaceTest(t)
	_, fa, _ := openShown(t, f, "a", true)
	m, fm, _ := showModal(t, f, nil, fa)
	setActive(fm, false)
	fa.take()
	m.Close()
	wantLog(t, "not key", fa.take(), "setSensitive true")

	setActive(fa, true)
	m, _, _ = showModal(t, f, nil, fa)
	fa.take()
	m.Close()
	wantLog(t, "key, app-modal", fa.take(), "setSensitive true", "present")

	setActive(fa, true)
	showModal(t, f, nil, fa)
	fa.take()
	s.Quit()
	for _, c := range fa.take() {
		if c == "present" || c == "setSensitive true" {
			t.Errorf("Quit: a got %s", c)
		}
	}
}

// TestModalOwnerCloses: closing the owner closes its modal surfaces first,
// newest first, and nothing is handed to the closing owner.
func TestModalOwnerCloses(t *testing.T) {
	_, f := newSurfaceTest(t)
	a, fa, ha := openShown(t, f, "a", true)
	_, fm1, h1 := showModal(t, f, a, fa)
	_, _, h2 := showModal(t, f, a, fm1)
	var order []string
	h1.onClosed = func() { order = append(order, "m1") }
	h2.onClosed = func() { order = append(order, "m2") }
	ha.onClosed = func() { order = append(order, "a") }
	fa.take()
	a.Close()
	wantLog(t, "order", order, "m2", "m1", "a")
	for _, c := range fa.take() {
		if c == "present" {
			t.Error("the closing owner was presented")
		}
	}
	if len(f.s.modals) != 0 || len(f.s.surfaces) != 0 {
		t.Errorf("left: %d modals, %d surfaces", len(f.s.modals), len(f.s.surfaces))
	}
}
