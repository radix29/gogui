package gtk4

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
)

// fakeGtkProps is a gtkProps over maps. watch keeps a dispatch handle per
// property, as connect does, so a test fires the notify through the real
// sigPointer trampoline; unwatch frees them, as closureNotify does.
type fakeGtkProps struct {
	mu       sync.Mutex
	props    map[string]bool
	ints     map[string]int32
	handles  map[string]ffi.Handle
	unwatchs int
}

func newFakeGtkProps(props map[string]bool) *fakeGtkProps {
	return &fakeGtkProps{props: props, ints: map[string]int32{}, handles: map[string]ffi.Handle{}}
}

// withInts adds the int properties ints.
func (f *fakeGtkProps) withInts(ints map[string]int32) *fakeGtkProps {
	f.ints = ints
	return f
}

func (f *fakeGtkProps) boolProp(name string) (bool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.props[name]
	return v, ok
}

func (f *fakeGtkProps) intProp(name string) (int32, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.ints[name]
	return v, ok
}

func (f *fakeGtkProps) set(name string, v bool) {
	f.mu.Lock()
	f.props[name] = v
	f.mu.Unlock()
}

func (f *fakeGtkProps) setInt(name string, v int32) {
	f.mu.Lock()
	f.ints[name] = v
	f.mu.Unlock()
}

func (f *fakeGtkProps) watch(name string, p pointerSignalPeer, k kind) bool {
	_, isBool := f.props[name]
	_, isInt := f.ints[name]
	if !isBool && !isInt {
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
		gtkEnableAnimations: false, gtkOverlayScrolling: true, gtkPreferDark: true, gtkCursorBlink: true,
	}).withInts(map[string]int32{gtkCursorBlinkTime: 1200, gtkCursorBlinkTimeout: 7})
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
	if v, ok := g.gtkInt(gtkCursorBlinkTime); v != 1200 || !ok {
		t.Errorf("gtkInt(%s) = %v, %v; want 1200, true", gtkCursorBlinkTime, v, ok)
	}
	if _, ok := g.gtkInt("gtk-unknown"); ok {
		t.Error("gtkInt of an unknown property is set")
	}
	if _, ok := g.gtkBool("gtk-unknown"); ok {
		t.Error("gtkBool of an unknown property is set")
	}
	want := driver.DefaultAppearance()
	want.Dark, want.ReduceMotion = true, true
	want.CaretBlink = driver.CaretBlink{Period: 1200 * time.Millisecond, Timeout: 7 * time.Second}
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
	if _, ok := g.gtkInt(gtkCursorBlinkTime); ok {
		t.Error("gtkInt set after close")
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
			_, okB := g.gtkBool(name)
			if _, okI := g.gtkInt(name); okB || okI {
				t.Errorf("%s set without GtkSettings", name)
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

// TestGtkSettingsCaretBlinkChanged: the caret properties are watched like
// the others: turning gtk-cursor-blink off, then changing the blink time
// and the timeout, each re-reads the appearance into one AppearanceChanged
// with the new CaretBlink.
func TestGtkSettingsCaretBlinkChanged(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	props := newFakeGtkProps(map[string]bool{gtkCursorBlink: true}).
		withInts(map[string]int32{gtkCursorBlinkTime: 1200, gtkCursorBlinkTimeout: 10})
	s.openSettings = openSystemSettings(noBusConfig(), func() gtkProps { return props }, nil)
	var got []driver.CaretBlink
	err := run(t, s, &handler{
		ready: func() {
			if b := s.Appearance().CaretBlink; b != (driver.CaretBlink{Period: 1200 * time.Millisecond, Timeout: 10 * time.Second}) {
				t.Errorf("initial CaretBlink %+v, want GTK's defaults", b)
			}
			props.notify(gtkCursorBlinkTime) // no change
			props.set(gtkCursorBlink, false)
			props.notify(gtkCursorBlink)
			s.Post(func() {
				props.set(gtkCursorBlink, true)
				props.setInt(gtkCursorBlinkTime, 800)
				props.notify(gtkCursorBlinkTime)
				s.Post(func() {
					props.setInt(gtkCursorBlinkTimeout, math.MaxInt32)
					props.notify(gtkCursorBlinkTimeout)
					s.Post(s.Quit)
				})
			})
		},
		appearance: func(a driver.Appearance) { got = append(got, a.CaretBlink) },
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []driver.CaretBlink{
		{Period: 0, Timeout: 10 * time.Second},
		{Period: 800 * time.Millisecond, Timeout: 10 * time.Second},
		{Period: 800 * time.Millisecond, Timeout: 0},
	}
	if !slices.Equal(got, want) {
		t.Errorf("CaretBlink changes %+v, want %+v", got, want)
	}
	checkTornDown(t, s, l, base)
}
