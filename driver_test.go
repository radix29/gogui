package gogui

import (
	"errors"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/gtk4"
	"github.com/radix29/gogui/internal/shell/native"
)

// TestPlatformDriver checks the driver-selection files without running the
// driver: headless on macOS until S1.1, aqua over the Win32 shell on
// Windows (S2.3f), aqua over the GTK 4 shell on Linux (S4.1d), and on any
// other OS no driver but the error of native.New (R7).
func TestPlatformDriver(t *testing.T) {
	d, err := newPlatformDriver()
	switch runtime.GOOS {
	case "darwin":
		if err != nil || d.Name() != "headless" {
			t.Fatalf("newPlatformDriver() = %v, %v; want headless", d, err)
		}
	case "windows", "linux":
		if err != nil {
			t.Fatalf("newPlatformDriver: %v", err)
		}
		a, ok := d.(*aqua.Driver)
		if !ok {
			t.Fatalf("driver = %T; want *aqua.Driver", d)
		}
		if _, ok := a.Shell().(*gtk4.Shell); ok != (runtime.GOOS == "linux") {
			t.Fatalf("shell = %T on %s", a.Shell(), runtime.GOOS)
		}
	default:
		if d != nil || !errors.Is(err, native.ErrNotImplemented) {
			t.Fatalf("newPlatformDriver() = %v, %v; want native.ErrNotImplemented", d, err)
		}
	}
}

// TestFailedDriverRun checks that Run returns the error of a platform
// whose driver can't start, without calling ready, and leaves no current
// app behind.
func TestFailedDriverRun(t *testing.T) {
	want := errors.New("no shell")
	setDriverFunc(t, func() (driver.Driver, error) { return nil, want })
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

// TestDriverBoundary checks that the public layer reaches the backends
// only through internal/driver (R7): apart from the driver-selection files,
// no non-test file imports a backend, a shell or the stub peers.
func TestDriverBoundary(t *testing.T) {
	allowed := map[string]bool{
		"github.com/radix29/gogui/internal/driver":     true,
		"github.com/radix29/gogui/internal/layout":     true,
		"github.com/radix29/gogui/internal/mainthread": true,
	}
	selection := map[string]bool{"driver_darwin.go": true, "driver_other.go": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || selection[name] {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(path, "github.com/radix29/gogui/") && !allowed[path] {
				t.Errorf("%s imports %s; only the driver-selection files may", name, path)
			}
		}
	}
}
