package gogui

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/headless"
)

// useHeadless makes the next Run use a silent headless driver with the
// UI-thread checks on, and returns that driver.
func useHeadless(t *testing.T) *headless.Driver {
	t.Helper()
	d := headless.New(io.Discard)
	setDriver(t, d)
	return d
}

func setDriver(t testing.TB, d driver.Driver) {
	t.Helper()
	setDriverFunc(t, func() (driver.Driver, error) { return d, nil })
}

// setDriverFunc makes the next Run take its driver from fn, with the
// UI-thread checks on.
func setDriverFunc(t testing.TB, fn func() (driver.Driver, error)) {
	t.Helper()
	oldNew, oldChecks := newDriver, debugChecks
	newDriver, debugChecks = fn, true
	ran.Store(false)
	t.Cleanup(func() {
		newDriver, debugChecks = oldNew, oldChecks
		ran.Store(false) // the next test, or Example, may call Run
	})
}

// runTest calls Run on the test goroutine with a watchdog, so a Run that
// never returns fails loudly instead of hanging. The app is named "Test".
func runTest(t testing.TB, ready func(*App)) error {
	t.Helper()
	return runTestNamed(t, "Test", ready)
}

// runTestNamed is runTest for an app named name (AppConfig.Name, which
// titles the app menu).
func runTestNamed(t testing.TB, name string, ready func(*App)) error {
	t.Helper()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-stop:
		case <-time.After(10 * time.Second):
			panic("gogui: Run did not return")
		}
	}()
	return Run(AppConfig{Name: name}, ready)
}

// runApp runs body in ready on a headless driver, drains the UI queue
// (including work that body's work queues) and quits.
func runApp(t *testing.T, body func(app *App)) {
	t.Helper()
	d := useHeadless(t)
	err := runTest(t, func(app *App) {
		body(app)
		for d.Queue().Drain() > 0 {
		}
		app.Quit()
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunReturnsAfterQuit(t *testing.T) {
	d := useHeadless(t)
	calls := 0
	err := runTest(t, func(app *App) {
		calls++
		if app == nil || !d.Running || !d.Queue().OnUIThread() || !d.Queue().Checks() {
			t.Error("ready: not running on the UI thread with checks on")
		}
		Do(app.Quit)
	})
	if err != nil || calls != 1 {
		t.Fatalf("Run = %v, ready called %d times; want nil, 1", err, calls)
	}
	if d.Config.Name != "Test" || d.Running {
		t.Errorf("driver config %q, running %v", d.Config.Name, d.Running)
	}
	if cur.Load() != nil {
		t.Error("app still current after Run returned")
	}
}

func TestRunNilReady(t *testing.T) {
	d := useHeadless(t)
	go func() {
		for d.Queue() == nil {
			time.Sleep(time.Millisecond)
		}
		d.Queue().Do(d.Quit)
	}()
	if err := runTest(t, nil); err != nil {
		t.Fatal(err)
	}
}

// failing is a driver whose Run fails, as a missing GTK would.
type failing struct{ *headless.Driver }

var errNoGTK = errors.New("gtk missing")

func (failing) Run(driver.AppConfig, driver.AppHandler) error { return errNoGTK }

func TestRunDriverError(t *testing.T) {
	setDriver(t, failing{headless.New(io.Discard)})
	if err := runTest(t, func(*App) { t.Error("ready called") }); !errors.Is(err, errNoGTK) {
		t.Fatalf("Run = %v, want %v", err, errNoGTK)
	}
}

func TestRunTwice(t *testing.T) {
	runApp(t, func(*App) {})
	if err := Run(AppConfig{}, func(*App) { t.Error("ready called") }); err != errRunTwice {
		t.Fatalf("second Run = %v, want %v", err, errRunTwice)
	}
	// Nested: Run called from inside ready.
	runApp(t, func(*App) {
		if err := Run(AppConfig{}, nil); err != errRunTwice {
			t.Errorf("nested Run = %v, want %v", err, errRunTwice)
		}
	})
}

func TestDoFromGoroutinesKeepsOrder(t *testing.T) {
	const producers, n = 8, 200
	var got [producers][]int
	runApp(t, func(app *App) {
		var wg sync.WaitGroup
		for p := range producers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range n {
					Do(func() { got[p] = append(got[p], i) })
				}
			}()
		}
		// Wait on the UI thread without blocking the queue: DoSync from
		// here would run inline, so poll by draining.
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		for {
			select {
			case <-done:
				return
			default:
				queue().Drain()
			}
		}
	})
	for p := range producers {
		if len(got[p]) != n {
			t.Fatalf("producer %d: %d calls ran, want %d", p, len(got[p]), n)
		}
		for i, v := range got[p] {
			if v != i {
				t.Fatalf("producer %d: call %d ran as %d", p, i, v)
			}
		}
	}
}

func TestDoSync(t *testing.T) {
	var order []string
	runApp(t, func(app *App) {
		Do(func() { order = append(order, "queued") })
		if !DoSync(func() { order = append(order, "inline") }) {
			t.Error("DoSync on the UI thread returned false")
		}
		res := make(chan bool)
		go func() {
			res <- DoSync(func() { order = append(order, "goroutine") })
		}()
		for {
			select {
			case ok := <-res:
				if !ok {
					t.Error("DoSync from a goroutine returned false")
				}
				return
			default:
				queue().Drain()
			}
		}
	})
	want := "inline queued goroutine"
	if got := strings.Join(order, " "); got != want {
		t.Errorf("order %q, want %q", got, want)
	}
}

func TestDoOutsideRun(t *testing.T) {
	useHeadless(t)
	called := false
	Do(func() { called = true })
	if DoSync(func() { called = true }) {
		t.Error("DoSync before Run returned true")
	}
	runApp(t, func(*App) {})
	Do(func() { called = true })
	if DoSync(func() { called = true }) {
		t.Error("DoSync after Run returned true")
	}
	if called {
		t.Error("fn ran outside Run")
	}
}

func TestDoSyncFromGoroutineAfterQuit(t *testing.T) {
	res := make(chan bool, 1)
	runApp(t, func(app *App) {
		app.Quit()
		go func() { res <- DoSync(func() { t.Error("fn ran after Quit") }) }()
		if <-res {
			t.Error("DoSync after Quit returned true")
		}
	})
}

func TestUIThreadCheck(t *testing.T) {
	msg := make(chan any)
	runApp(t, func(app *App) {
		checkUI("on the UI thread") // must not panic
		go func() {
			defer func() { msg <- recover() }()
			app.Quit()
		}()
		r := <-msg
		s, _ := r.(string)
		if !strings.Contains(s, "App.Quit called off the UI thread") {
			t.Errorf("off-thread App.Quit: recovered %v, want the UI-thread panic", r)
		}
	})
}

func TestUIThreadCheckOff(t *testing.T) {
	d := useHeadless(t)
	debugChecks = false
	err := runTest(t, func(app *App) {
		if d.Queue().Checks() {
			t.Error("checks on without GOGUI_DEBUG")
		}
		done := make(chan struct{})
		go func() { defer close(done); checkUI("off") }() // must not panic
		<-done
		app.Quit()
	})
	if err != nil {
		t.Fatal(err)
	}
}
