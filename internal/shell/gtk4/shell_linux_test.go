//go:build linux

package gtk4

import (
	"errors"
	"sync"
	"testing"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell"
)

// The shell over the real GTK loop. Run runs on the GTK goroutine of
// TestMain (class_linux_test.go), the one thread GTK is used on; Run's own
// LockOSThread nests inside that goroutine's. native.sh runs these under
// xvfb; without a display they are skipped.

// newGTKShell returns New() with the UI-thread checks on and a report hook
// that counts its calls.
func newGTKShell(t *testing.T) (s *Shell, reports *int) {
	t.Helper()
	needGTK(t)
	resetPanics(t)
	s = New()
	s.Queue().SetChecks(true)
	reports = new(int)
	s.loop.(*gtkLoop).report = func(error) { *reports++ }
	return s, reports
}

// runGTK runs s with h on the GTK goroutine and returns Run's error and
// what it panicked with, if it did.
func runGTK(t *testing.T, s *Shell, h shell.Handler) (panicked any, err error) {
	t.Helper()
	onGTK(t, func() error {
		defer func() { panicked = recover() }()
		err = run(t, s, h)
		return nil
	})
	return panicked, err
}

// checkGTKTornDown checks the state after Run: no handle left, the main
// loop freed, the queue closed, the panic state cleared, no GTK report and
// still exactly the trampoline set of native callbacks.
func checkGTKTornDown(t *testing.T, s *Shell, reports *int, base int) {
	t.Helper()
	checkLen(t, base)
	if s.loop.(*gtkLoop).main != 0 {
		t.Error("main loop not freed after Run")
	}
	if !s.q.Closed() {
		t.Error("queue not closed after Run")
	}
	if len(s.wakePending) != 0 {
		t.Errorf("%d wake sources left pending", len(s.wakePending))
	}
	if onPanic != nil || pendingPanic != nil {
		t.Error("panic state left set after Run")
	}
	if *reports != 0 {
		t.Errorf("gtk.Report called %d times although GTK loaded", *reports)
	}
	if got := ffi.Callbacks(); got != numTrampolines {
		t.Errorf("ffi.Callbacks() = %d, want %d", got, numTrampolines)
	}
}

// TestShellGTKRunQuit: Ready runs once on the GTK thread with the loop
// set up; a Quit posted from another goroutine ends Run with nil.
func TestShellGTKRunQuit(t *testing.T) {
	s, reports := newGTKShell(t)
	base := dispatch.Len()
	readies := 0
	p, err := runGTK(t, s, &handler{ready: func() {
		readies++
		if !s.q.OnUIThread() {
			t.Error("Ready off the UI thread")
		}
		if s.loop.(*gtkLoop).main == 0 {
			t.Error("no main loop in Ready")
		}
		if s.TextSystem() == nil || s.Clock() == nil {
			t.Error("no TextSystem or Clock in Ready")
		}
		go s.Post(s.Quit)
	}})
	if p != nil || err != nil {
		t.Fatalf("Run = %v, panicked with %v", err, p)
	}
	if readies != 1 {
		t.Errorf("Ready called %d times", readies)
	}
	checkGTKTornDown(t, s, reports, base)
}

// TestShellGTKQuitInReady: Quit in Ready does not run the loop (its quit
// would be lost) and still runs the work posted before it.
func TestShellGTKQuitInReady(t *testing.T) {
	s, reports := newGTKShell(t)
	base := dispatch.Len()
	ran := 0
	p, err := runGTK(t, s, &handler{ready: func() {
		s.Post(func() { ran++ })
		s.Quit()
	}})
	if p != nil || err != nil || ran != 1 {
		t.Fatalf("Run = %v, panicked with %v, ran %d; want nil, none, 1", err, p, ran)
	}
	checkGTKTornDown(t, s, reports, base)
}

// TestShellGTKPostMany posts from 8 goroutines through g_idle_add from
// their threads: all of it runs, in order per goroutine.
func TestShellGTKPostMany(t *testing.T) {
	const goroutines, per = 8, 500
	s, reports := newGTKShell(t)
	base := dispatch.Len()
	next := make([]int, goroutines)
	total := 0
	p, err := runGTK(t, s, &handler{ready: func() {
		var wg sync.WaitGroup
		for g := range goroutines {
			wg.Go(func() {
				for i := range per {
					s.Post(func() {
						if next[g] != i {
							t.Errorf("goroutine %d: got %d, want %d", g, i, next[g])
						}
						next[g] = i + 1
						total++
					})
				}
			})
		}
		go func() {
			wg.Wait()
			s.Post(s.Quit)
		}()
	}})
	if p != nil || err != nil {
		t.Fatalf("Run = %v, panicked with %v", err, p)
	}
	if total != goroutines*per {
		t.Errorf("%d functions ran, want %d", total, goroutines*per)
	}
	t.Logf("%d functions, %d wake sources, %d drains", total, s.wakes.Load(), s.drains.Load())
	checkGTKTornDown(t, s, reports, base)
}

// TestShellGTKPanicInPostedWork: a panic in posted work is trapped in the
// GSourceFunc trampoline, quits the GMainLoop, and Run re-raises it.
func TestShellGTKPanicInPostedWork(t *testing.T) {
	s, reports := newGTKShell(t)
	base := dispatch.Len()
	boom := errors.New("boom")
	p, err := runGTK(t, s, &handler{ready: func() {
		s.Post(func() { panic(boom) })
		s.Post(func() { t.Error("work queued behind the panic ran") })
	}})
	pe, ok := p.(*PanicError)
	if !ok {
		t.Fatalf("Run = %v, panicked with %T %v; want a *PanicError", err, p, p)
	}
	if !errors.Is(pe, boom) || pe.Callback != nameSourceFunc {
		t.Errorf("PanicError = %v in %q, want boom in %s", pe.Value, pe.Callback, nameSourceFunc)
	}
	checkGTKTornDown(t, s, reports, base)
}
