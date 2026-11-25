package fake

import (
	"errors"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// named logs a surface's Closed and KeyChanged into a shared win with a
// prefix, so the order across surfaces shows.
type named struct {
	*win
	name string
}

func (n named) Closed()           { n.logf("%s closed", n.name) }
func (n named) KeyChanged(k bool) { n.logf("%s key %v", n.name, k) }

// otherSurface is a shell.Surface that is not the fake's.
type otherSurface struct{ shell.Surface }

func newModal(t *testing.T, s *Shell, owner shell.Surface, size driver.Size, h shell.SurfaceHandler) *Surface {
	t.Helper()
	sf, err := s.NewSurface(shell.SurfaceConfig{Size: size, Modal: true, Owner: owner,
		Style: driver.StyleTitled | driver.StyleClosable | driver.StyleMiniaturizable | driver.StyleResizable}, h)
	if err != nil {
		t.Fatal(err)
	}
	return sf.(*Surface)
}

// TestModalOwnerErrors: Owner without Modal, a closed owner and a foreign
// surface are refused; nil and an open owner are not.
func TestModalOwnerErrors(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})
	dead, _ := s.NewSurface(shell.SurfaceConfig{}, &win{})
	dead.Close()
	var nilSurface *Surface
	for _, c := range []shell.SurfaceConfig{
		{Owner: f},
		{Modal: true, Owner: dead},
		{Modal: true, Owner: otherSurface{}},
		{Modal: true, Owner: nilSurface},
	} {
		if _, err := s.NewSurface(c, &win{}); !errors.Is(err, shell.ErrSurfaceOwner) {
			t.Fatalf("NewSurface(%+v) = %v, want ErrSurfaceOwner", c, err)
		}
	}
	n := len(s.Surfaces())
	m := newModal(t, s, f, sz(10, 10), &win{})
	app := newModal(t, s, nil, sz(10, 10), &win{})
	if len(s.Surfaces()) != n+2 || !m.Modal() || m.Owner() != f || app.Owner() != nil || f.Modal() || f.Owner() != nil {
		t.Fatal("modal state")
	}
	if got := m.Style(); got != driver.StyleTitled|driver.StyleClosable {
		t.Fatalf("modal style %b, want titled|closable", got)
	}
}

// TestModalBlocks: a hidden modal blocks nothing; its first Show places it
// on the owner, ends the owner's capture, hover and grab popup, takes the
// key; blocked surfaces drop input and refuse user moves, resizes, closes
// and activation, still paint, and the modal cannot be minimized or
// zoomed.
func TestModalBlocks(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(400, 300), Style: driver.StyleResizable})
	f.UserMove(pt(100, 100))
	var mw win
	m := newModal(t, s, f, sz(200, 100), &mw)
	if f.Blocked() || s.Modal() != nil || m.Centered() != 0 {
		t.Fatal("a hidden modal blocks")
	}
	f.Press(pt(10, 50), shell.ButtonPrimary)
	g := newPopup(t, s, f, shell.PopupConfig{Anchor: rect(0, 0, 10, 10), Size: sz(50, 50), Grab: true}, &pop{w: &w})
	f.Move(pt(5, 5)) // into the popup
	w.take()
	m.Show()
	wantLog(t, &w, "pop up (5,5) b=1; pop leave (5,5); pop dismissed; key false")
	if !g.WasDismissed() || s.GrabPopup() != nil {
		t.Fatal("the grab popup stays")
	}
	if !f.Blocked() || m.Blocked() || s.Modal() != m || s.Key() != m || mw.take() != "key true" {
		t.Fatal("Show of the modal")
	}
	if m.Position() != pt(200, 200) || m.Centered() != 1 {
		t.Fatalf("placed at %v (%d), want (200,200) once", m.Position(), m.Centered())
	}
	g.Close()
	w.take()

	f.Move(pt(20, 20))
	f.Click(pt(20, 20))
	f.Wheel(pt(20, 20), pt(0, 1))
	if f.KeyDown('a', 0) {
		t.Fatal("a key reached a blocked surface")
	}
	f.Type("ab")
	if f.UserClose() || f.UserResize(sz(500, 400)) {
		t.Fatal("user close or resize of a blocked surface")
	}
	f.UserMove(pt(0, 0))
	if f.Position() != pt(100, 100) || f.Size() != sz(400, 300) {
		t.Fatal("user move of a blocked surface")
	}
	wantLog(t, &w, "")

	f.Activate()
	if s.Key() != m || w.take() != "" || mw.take() != "" {
		t.Fatal("a blocked surface was activated")
	}
	s.Deactivate()
	mw.take()
	f.Activate()
	if s.Key() != m || w.take() != "" || mw.take() != "key true" {
		t.Fatal("activating a blocked surface does not make the modal key")
	}
	f.Minimize()
	f.Activate()
	if f.Minimized() || s.Key() != m {
		t.Fatal("a blocked surface is restored, the modal key")
	}

	f.SetSize(sz(300, 200)) // programmatic calls still work
	s.Flush()
	if f.Size() != sz(300, 200) || w.take() != "resized 300x200 @1; paint" {
		t.Fatal("a blocked surface does not resize or paint")
	}

	m.Minimize()
	m.Zoom()
	if m.Minimized() || m.Zoomed() || m.UserResize(sz(300, 300)) {
		t.Fatal("the modal minimized, zoomed or resized")
	}
	mw.take()
	m.KeyDown('a', 0)
	if got := mw.take(); got != "keydown 'A'" {
		t.Fatalf("the modal's key: %q", got)
	}

	// A surface created while the modal is shown is blocked; Show keeps
	// the key on the modal.
	var w2 win
	b, _ := s.NewSurface(shell.SurfaceConfig{Size: sz(50, 50)}, &w2)
	b.Show()
	if !b.(*Surface).Blocked() || !b.(*Surface).Visible() || s.Key() != m || w2.take() != "" {
		t.Fatal("a later surface is not blocked")
	}

	// Center places the modal again, on the owner's new place.
	f.SetSize(sz(400, 300))
	m.Center()
	if m.Position() != pt(200, 200) || m.Centered() != 2 {
		t.Fatalf("Center: %v %d", m.Position(), m.Centered())
	}
}

// TestModalPlacement: an app-modal surface is centered on the work area, a
// modal over a hidden or minimized owner too, and the place stays inside
// the work area.
func TestModalPlacement(t *testing.T) {
	s := started(t)
	s.SetWorkArea(rect(0, 0, 1440, 900))
	m := newModal(t, s, nil, sz(260, 120), &win{})
	m.Show()
	if m.Position() != pt(590, 390) {
		t.Fatalf("app-modal at %v", m.Position())
	}
	m.Close()

	f := open(t, s, &win{}, shell.SurfaceConfig{Size: sz(300, 200)})
	f.UserMove(pt(1300, -50))
	m = newModal(t, s, f, sz(260, 120), &win{})
	m.Show()
	if m.Position() != pt(1180, 0) {
		t.Fatalf("clamped to %v, want (1180,0)", m.Position())
	}
	m.Close()

	f.Minimize()
	m = newModal(t, s, f, sz(260, 120), &win{})
	m.Show()
	if m.Position() != pt(590, 390) {
		t.Fatalf("over a minimized owner at %v", m.Position())
	}
	m.Close()

	big := newModal(t, s, nil, sz(2000, 1000), &win{})
	big.Show()
	if big.Position() != pt(0, 0) {
		t.Fatalf("larger than the work area at %v", big.Position())
	}
}

// TestModalCloseKey: the key goes to the next modal, then to the owner,
// after the closing modal's Closed; an app-modal hands it back to the
// surface that was key; a modal that was not key hands nothing on.
func TestModalCloseKey(t *testing.T) {
	s := started(t)
	var log win
	f := openNamed(t, s, named{&log, "f"}, shell.SurfaceConfig{Size: sz(400, 300)})
	g := openNamed(t, s, named{&log, "g"}, shell.SurfaceConfig{Size: sz(400, 300)})
	log.take()

	m1 := newModal(t, s, f, sz(100, 50), named{&log, "m1"})
	m1.Show()
	m2 := newModal(t, s, nil, sz(100, 50), named{&log, "m2"})
	m2.Show()
	wantLog(t, &log, "g key false; m1 key true; m1 key false; m2 key true")
	if !m1.Blocked() || s.Modal() != m2 {
		t.Fatal("the newer modal blocks the older one")
	}
	m1.Activate()
	if s.Key() != m2 {
		t.Fatal("the older modal was activated")
	}
	m2.Close()
	wantLog(t, &log, "m2 closed; m1 key true")
	if s.Modal() != m1 || !g.Blocked() || m1.Blocked() {
		t.Fatal("the older modal does not block again")
	}
	m1.Close()
	wantLog(t, &log, "m1 closed; f key true")
	if s.Modal() != nil || f.Blocked() || g.Blocked() {
		t.Fatal("still blocked")
	}

	// App-modal: back to the surface that was key.
	g.Activate()
	log.take()
	a := newModal(t, s, nil, sz(100, 50), named{&log, "a"})
	a.Show()
	a.Close()
	wantLog(t, &log, "g key false; a key true; a closed; g key true")

	// Not key when it closes: nothing is handed on.
	a = newModal(t, s, f, sz(100, 50), named{&log, "a"})
	a.Show()
	s.Deactivate()
	a.Close()
	wantLog(t, &log, "g key false; a key true; a key false; a closed")
	if s.Key() != nil {
		t.Fatal("a key was handed on")
	}
}

// TestModalOwnerCloses: closing the owner closes its modal first, and the
// key is not handed to the closing owner (it was the key before the modal).
// Quit closes everything without handing the key on.
func TestModalOwnerCloses(t *testing.T) {
	s := started(t)
	var log win
	g := openNamed(t, s, named{&log, "g"}, shell.SurfaceConfig{Size: sz(400, 300)})
	f := openNamed(t, s, named{&log, "f"}, shell.SurfaceConfig{Size: sz(400, 300)})
	log.take()
	m := newModal(t, s, f, sz(100, 50), named{&log, "m"})
	m.Show()
	f.Close()
	wantLog(t, &log, "f key false; m key true; m closed; f closed")
	if !m.Closed() || s.Key() != nil || g.Blocked() {
		t.Fatal("owner close")
	}

	g.Activate()
	m = newModal(t, s, g, sz(100, 50), named{&log, "m"})
	m.Show()
	log.take()
	s.Quit()
	wantLog(t, &log, "m closed; g closed")
}

// openNamed is open for a named handler.
func openNamed(t *testing.T, s *Shell, h named, cfg shell.SurfaceConfig) *Surface {
	t.Helper()
	sf, err := s.NewSurface(cfg, h)
	if err != nil {
		t.Fatal(err)
	}
	f := sf.(*Surface)
	f.Show()
	s.Flush()
	return f
}

// modalClosed is a named handler whose Closed records what it sees.
type modalClosed struct {
	named
	fn func()
}

func (m modalClosed) Closed() { m.named.Closed(); m.fn() }

// TestModalLeavesBeforeClosed (V3): a closing modal surface leaves modality
// before its Closed, as on the real shells, so inside Closed its owner is
// no longer blocked; the key is handed on after Closed (as gtk4).
func TestModalLeavesBeforeClosed(t *testing.T) {
	s := started(t)
	var log win
	f := openNamed(t, s, named{&log, "f"}, shell.SurfaceConfig{Size: sz(400, 300)})
	log.take()
	var blocked bool
	var modal *Surface
	sf, err := s.NewSurface(shell.SurfaceConfig{Size: sz(100, 50), Modal: true, Owner: f},
		modalClosed{named{&log, "m"}, func() { blocked, modal = f.Blocked(), s.Modal() }})
	if err != nil {
		t.Fatal(err)
	}
	m := sf.(*Surface)
	m.Show()
	log.take()
	m.Close()
	if blocked || modal != nil {
		t.Fatalf("inside the modal's Closed: owner blocked %v, active modal %v", blocked, modal)
	}
	wantLog(t, &log, "m closed; f key true")
}
