package gogui

import (
	"errors"
	"runtime"
	"testing"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/shell/gtk4"
	"github.com/radix29/gogui/internal/shell/native"
)

// TestPlatformDriver checks the driver-selection files without running the
// driver: headless on macOS until S1.1, aqua over the Win32 shell on
// Windows (S2.3f), aqua over the GTK 4 shell on Linux (S4.1d), and on any
// other OS a driver whose Run returns the error of native.New.
func TestPlatformDriver(t *testing.T) {
	d := newPlatformDriver()
	switch runtime.GOOS {
	case "darwin":
		if d.Name() != "headless" {
			t.Fatalf("Name() = %q; want headless", d.Name())
		}
	case "windows":
		if d.Name() != "aqua" {
			t.Fatalf("Name() = %q; want aqua", d.Name())
		}
		if _, ok := d.(*errDriver); ok {
			t.Fatal("Windows got the failed driver")
		}
	case "linux":
		a, ok := d.(*aqua.Driver)
		if !ok {
			t.Fatalf("driver = %T; want *aqua.Driver", d)
		}
		if _, ok := a.Shell().(*gtk4.Shell); !ok {
			t.Fatalf("shell = %T; want *gtk4.Shell", a.Shell())
		}
	default:
		if d.Name() != "aqua" {
			t.Fatalf("Name() = %q; want aqua", d.Name())
		}
		setDriver(t, d)
		if err := runTest(t, nil); !errors.Is(err, native.ErrNotImplemented) {
			t.Fatalf("Run = %v; want native.ErrNotImplemented", err)
		}
	}
}

// TestFailedDriverRun checks that Run returns the failed driver's error
// without calling ready and leaves no current app behind.
func TestFailedDriverRun(t *testing.T) {
	want := errors.New("no shell")
	setDriver(t, failedDriver("aqua", want))
	called := false
	if err := runTest(t, func(*App) { called = true }); !errors.Is(err, want) {
		t.Fatalf("Run = %v; want %v", err, want)
	}
	if called {
		t.Fatal("ready was called")
	}
	if cur.Load() != nil {
		t.Fatal("current app left set")
	}
	if DoSync(func() {}) {
		t.Fatal("DoSync returned true after a failed Run")
	}
}
