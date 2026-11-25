//go:build linux

package gtk4

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// New returns a shell that has not been started. GTK is loaded by Run.
func New() *Shell {
	s := newShell(&gtkLoop{report: gtk.Report})
	s.text.open = openPango
	s.clip.backend = nativeClipboard{}
	var logf func(string, ...any) // a missing bus or portal: debug level only
	if debugChecks {
		logf = log.Printf
	}
	s.openSettings = openSystemSettings(defaultPortalConfig(), newNativeGtkProps, logf)
	return s
}

// gtkLoop is the loop over GTK: gtk_init_check and a GMainLoop on the
// default main context, as in spike/gtk (no GtkApplication, PLAN.md S4.1).
type gtkLoop struct {
	main   uintptr         // GMainLoop *, 0 outside init…release
	report func(err error) // gtk.Report; tests replace it
	// cssDisplays are the displays the popovers' CSS provider was added
	// to (popover_linux.go).
	cssDisplays map[uintptr]bool
}

func (l *gtkLoop) init() error {
	if err := gtk.Load(); err != nil {
		if errors.Is(err, gtk.ErrMissing) {
			l.report(err)
		}
		return err
	}
	if gtk.Gtk.InitCheck() == gtk.False {
		return fmt.Errorf("gtk4: gtk_init_check failed: no display could be opened (DISPLAY=%q, WAYLAND_DISPLAY=%q)",
			os.Getenv("DISPLAY"), os.Getenv("WAYLAND_DISPLAY"))
	}
	if err := registerSurfaceClass(); err != nil { // registers the trampolines too
		return err
	}
	l.main = gtk.GLib.MainLoopNew(0, gtk.False)
	if l.main == 0 {
		return errors.New("gtk4: g_main_loop_new returned NULL")
	}
	return nil
}

func (l *gtkLoop) run() { gtk.GLib.MainLoopRun(l.main) }

func (l *gtkLoop) quit() {
	if l.main != 0 {
		gtk.GLib.MainLoopQuit(l.main)
	}
}

func (l *gtkLoop) idle(p sourcePeer, k kind) (uint32, ffi.Handle) { return addIdle(p, k) }

func (l *gtkLoop) remove(id uint32) { gtk.GLib.SourceRemove(id) }

func (l *gtkLoop) release() {
	if l.main != 0 {
		gtk.GLib.MainLoopUnref(l.main)
		l.main = 0
	}
}

func (l *gtkLoop) monotonic() int64 { return gtk.GLib.GetMonotonicTime() }

func (l *gtkLoop) timeout(ms uint32, p sourcePeer, k kind) (uint32, ffi.Handle) {
	return addTimeout(ms, p, k)
}

func (l *gtkLoop) addTick(widget uintptr, p tickPeer, k kind) (uint32, ffi.Handle) {
	return addTick(widget, p, k)
}

func (l *gtkLoop) removeTick(widget uintptr, id uint32) {
	gtk.Gtk.WidgetRemoveTickCallback(widget, id)
}

func (l *gtkLoop) frameInfo(frameClock uintptr) (frameTime, interval, presentation int64) {
	frameTime = gtk.Gdk.FrameClockGetFrameTime(frameClock)
	gtk.Gdk.FrameClockGetRefreshInfo(frameClock, frameTime, &interval, &presentation)
	return frameTime, interval, presentation
}

func (l *gtkLoop) frameCounter(frameClock uintptr) int64 {
	return gtk.Gdk.FrameClockGetFrameCounter(frameClock)
}

func (l *gtkLoop) presentation(frameClock uintptr, counter int64) (presented, interval int64, complete, gone bool) {
	t := gtk.Gdk.FrameClockGetTimings(frameClock, counter)
	if t == 0 {
		return 0, 0, false, true
	}
	if gtk.Gdk.FrameTimingsGetComplete(t) == gtk.False {
		return 0, 0, false, false
	}
	return gtk.Gdk.FrameTimingsGetPresentationTime(t), gtk.Gdk.FrameTimingsGetRefreshInterval(t), true, false
}
