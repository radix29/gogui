//go:build linux

package gtk4

import (
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// setSettingBool sets a boolean property of the default GtkSettings
// through g_object_set_property, as a settings daemon change does.
func setSettingBool(name string, b bool) {
	st := gtk.Gtk.SettingsGetDefault()
	v := gtk.GLib.Malloc0(unsafe.Sizeof(gtk.GValue{}))
	defer gtk.GLib.Free(v)
	gtk.GObject.ValueInit(v, gtk.TypeBoolean)
	defer gtk.GObject.ValueUnset(v)
	gtk.GObject.ValueSetBoolean(v, gbool(b))
	gtk.GObject.ObjectSetProperty(st, name, v)
}

// TestGtkSettingsNotify: over the real default GtkSettings (no session bus,
// so the portal does not mask them) the watched properties are read and
// watched; flipping gtk-overlay-scrolling with g_object_set_property gives
// exactly one AppearanceChanged; after Run the handlers are disconnected
// (their handles freed), so restoring the value reaches nothing.
func TestGtkSettingsNotify(t *testing.T) {
	s, reports := newGTKShell(t)
	base := dispatch.Len()
	s.openSettings = openSystemSettings(noBusConfig(), newNativeGtkProps, nil)
	var (
		got     []driver.Appearance
		watched int
		before  bool
		read    bool
	)
	p, err := runGTK(t, s, &handler{
		ready: func() {
			defer s.Post(s.Quit) // after the re-read the notify posts
			gs := s.settings.(systemSettings).gs
			watched = gs.watched
			if n := dispatch.Len() - base; n != watched {
				t.Errorf("%d handles for %d watched properties", n, watched)
			}
			for _, name := range gtkBoolProps {
				v, ok := gs.gtkBool(name)
				t.Logf("SETTINGS-RESULT: %s = %v (readable %v)", name, v, ok)
			}
			for _, name := range gtkIntProps {
				v, ok := gs.gtkInt(name)
				t.Logf("SETTINGS-RESULT: %s = %v (readable %v)", name, v, ok)
				if !ok {
					t.Errorf("%s not readable", name)
				}
			}
			if b, want := s.Appearance().CaretBlink, readCaretBlink(s.settings); b != want {
				t.Errorf("CaretBlink %+v, want %+v", b, want)
			}
			t.Logf("SETTINGS-RESULT: caret blink %+v", s.Appearance().CaretBlink)
			before, read = gs.gtkBool(gtkOverlayScrolling)
			if !read {
				t.Errorf("%s not readable", gtkOverlayScrolling)
				return
			}
			if s.Appearance().LegacyScrollers != !before {
				t.Errorf("LegacyScrollers %v with %s %v", s.Appearance().LegacyScrollers, gtkOverlayScrolling, before)
			}
			setSettingBool(gtkOverlayScrolling, !before)
		},
		appearance: func(a driver.Appearance) {
			if !s.q.OnUIThread() {
				t.Error("AppearanceChanged off the UI thread")
			}
			got = append(got, a)
		},
	})
	if p != nil || err != nil {
		t.Fatalf("Run = %v, panicked with %v", err, p)
	}
	if read {
		onGTK(t, func() error { setSettingBool(gtkOverlayScrolling, before); return nil })
	}
	if watched != len(gtkWatched) {
		t.Errorf("watched %d properties, want %d", watched, len(gtkWatched))
	}
	if read && (len(got) != 1 || got[0].LegacyScrollers != before) {
		t.Errorf("AppearanceChanged calls %+v, want one with LegacyScrollers %v", got, before)
	}
	checkGTKTornDown(t, s, reports, base)
}
