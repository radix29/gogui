package gtk4

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
)

// fakeGtkProps is a gtkProps over a map. watch keeps a dispatch handle per
// property, as connect does, so a test fires the notify through the real
// sigPointer trampoline; unwatch frees them, as closureNotify does.
type fakeGtkProps struct {
	mu       sync.Mutex
	props    map[string]bool
	handles  map[string]ffi.Handle
	unwatchs int
}

func newFakeGtkProps(props map[string]bool) *fakeGtkProps {
	return &fakeGtkProps{props: props, handles: map[string]ffi.Handle{}}
}

func (f *fakeGtkProps) boolProp(name string) (bool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.props[name]
	return v, ok
}

func (f *fakeGtkProps) set(name string, v bool) {
	f.mu.Lock()
	f.props[name] = v
	f.mu.Unlock()
}

func (f *fakeGtkProps) watch(name string, p pointerSignalPeer, k kind) bool {
	if _, ok := f.props[name]; !ok {
		return false
	}
	f.handles[name] = dispatch.New(entry{peer: p, kind: k})
	return true
}

func (f *fakeGtkProps) unwatch() {
	f.unwatchs++
	for _, h := range f.handles {
		dispatch.Free(h)
	}
}

// notify fires notify::name through sigPointer, as GObject does after a
// set (instance and GParamSpec are made up). A stale handle does nothing.
func (f *fakeGtkProps) notify(name string) {
	sigPointer(0x5e7, 0x9a5, uintptr(f.handles[name]))
}

// TestGtkSettingsWatch: every watched property is connected once; its
// notify reaches changed through sigPointer; the values read through;
// close disconnects, after which nothing is read or reported.
func TestGtkSettingsWatch(t *testing.T) {
	base := dispatch.Len()
	props := newFakeGtkProps(map[string]bool{
		gtkEnableAnimations: false, gtkOverlayScrolling: true, gtkPreferDark: true,
	})
	changes := 0
	g := openGtkSettings(props, func() { changes++ })
	if g.watched != len(gtkWatched) || dispatch.Len() != base+len(gtkWatched) {
		t.Fatalf("watched %d, %d handles; want %d", g.watched, dispatch.Len()-base, len(gtkWatched))
	}
	for i, name := range gtkWatched {
		props.notify(name)
		if changes != i+1 {
			t.Errorf("notify::%s: %d changes, want %d", name, changes, i+1)
		}
	}
	// Another kind on the peer is not a settings notify.
	g.pointerSignal(kindActive, 0, 0)
	if changes != len(gtkWatched) {
		t.Errorf("a kindActive signal reported a change")
	}
	if v, ok := g.gtkBool(gtkPreferDark); !v || !ok {
		t.Errorf("gtkBool(%s) = %v, %v; want true, true", gtkPreferDark, v, ok)
	}
	if _, ok := g.gtkBool("gtk-unknown"); ok {
		t.Error("gtkBool of an unknown property is set")
	}
	want := driver.DefaultAppearance()
	want.Dark, want.ReduceMotion = true, true
	if a := readAppearance(systemSettings{gs: g}); a != want {
		t.Errorf("readAppearance = %+v, want %+v", a, want)
	}
	g.close()
	g.close()
	checkLen(t, base)
	if props.unwatchs != 1 {
		t.Errorf("unwatch called %d times, want 1", props.unwatchs)
	}
	props.notify(gtkPreferDark) // stale handle
	g.pointerSignal(kindSettingNotify, 0, 0)
	if changes != len(gtkWatched) {
		t.Error("a change was reported after close")
	}
	if _, ok := g.gtkBool(gtkPreferDark); ok {
		t.Error("gtkBool set after close")
	}
}

// TestGtkSettingsMissing: a property GTK lacks is not watched and reads as
// unset; no props or a nil *gtkSettings is every value unset.
func TestGtkSettingsMissing(t *testing.T) {
	base := dispatch.Len()
	g := openGtkSettings(newFakeGtkProps(map[string]bool{gtkOverlayScrolling: false}), nil)
	if g.watched != 1 {
		t.Errorf("watched %d, want 1", g.watched)
	}
	g.pointerSignal(kindSettingNotify, 0, 0) // a nil changed is fine
	if _, ok := g.gtkBool(gtkEnableAnimations); ok {
		t.Error("a missing property is set")
	}
	g.close()
	checkLen(t, base)
	for _, g := range []*gtkSettings{openGtkSettings(nil, nil), nil} {
		for _, name := range gtkWatched {
			if _, ok := g.gtkBool(name); ok {
				t.Errorf("gtkBool(%s) set without GtkSettings", name)
			}
		}
		g.close()
	}
}

// noBusConfig is a portal config whose dial fails as without a session
// bus.
func noBusConfig() portalConfig {
	cfg := defaultPortalConfig()
	cfg.dial = func(...dbus.ConnOption) (*dbus.Conn, error) {
		return nil, errors.New("dbus: couldn't determine address of session bus")
	}
	return cfg
}

// TestOpenSystemSettingsGtk: New's source over the GtkSettings alone (no
// bus): their values fill in, a property it cannot watch is logged once.
func TestOpenSystemSettingsGtk(t *testing.T) {
	var logs []string
	logf := func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	props := newFakeGtkProps(map[string]bool{gtkPreferDark: true, gtkOverlayScrolling: false})
	src := openSystemSettings(noBusConfig(), func() gtkProps { return props }, logf)(func() {})
	want := driver.DefaultAppearance()
	want.Dark, want.LegacyScrollers = true, true
	if a := readAppearance(src); a != want {
		t.Errorf("readAppearance = %+v, want %+v", a, want)
	}
	src.close()
	if props.unwatchs != 1 {
		t.Errorf("unwatch called %d times, want 1", props.unwatchs)
	}
	if len(logs) != 2 {
		t.Errorf("logs = %q, want the unwatched property and the missing bus", logs)
	}
}

// TestGtkSettingsAppearanceChanged: a notify of the GtkSettings, through
// sigPointer, re-reads the appearance on the UI thread: exactly one
// AppearanceChanged for a changed value, none for a notify without a
// change; the handlers are gone after Run.
func TestGtkSettingsAppearanceChanged(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	props := newFakeGtkProps(map[string]bool{
		gtkEnableAnimations: true, gtkOverlayScrolling: true, gtkPreferDark: false,
	})
	s.openSettings = openSystemSettings(noBusConfig(), func() gtkProps { return props }, nil)
	var got []driver.Appearance
	err := run(t, s, &handler{
		ready: func() {
			if a := s.Appearance(); a != driver.DefaultAppearance() {
				t.Errorf("initial Appearance() = %+v, want the default", a)
			}
			props.notify(gtkPreferDark) // no change
			props.set(gtkOverlayScrolling, false)
			props.notify(gtkOverlayScrolling)
			s.Post(func() {
				props.notify(gtkEnableAnimations) // no change
				s.Post(s.Quit)
			})
		},
		appearance: func(a driver.Appearance) {
			if !s.q.OnUIThread() {
				t.Error("AppearanceChanged off the UI thread")
			}
			got = append(got, a)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := driver.DefaultAppearance()
	want.LegacyScrollers = true
	if len(got) != 1 || got[0] != want {
		t.Errorf("AppearanceChanged calls %+v, want one with legacy scrollers", got)
	}
	if props.unwatchs != 1 {
		t.Errorf("unwatch called %d times, want 1", props.unwatchs)
	}
	checkTornDown(t, s, l, base)
}
