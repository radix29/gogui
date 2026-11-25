package headless

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
)

// app is an AppHandler whose Ready runs a callback.
type app struct {
	ready   int
	onReady func()
}

func (a *app) Ready() {
	a.ready++
	if a.onReady != nil {
		a.onReady()
	}
}
func (a *app) AppearanceChanged(driver.Appearance) {}
func (a *app) QuitRequested() bool                 { return true }
func (a *app) MenuItemSelected(int)                {}

type win struct{ closed int }

func (w *win) ContentResized(driver.Size) {}
func (w *win) ShouldClose() bool          { return true }
func (w *win) Closed()                    { w.closed++ }
func (w *win) KeyChanged(bool)            {}

// run calls d.Run on the test goroutine with a watchdog, so a Run that never
// returns fails the test instead of hanging it.
func run(t *testing.T, d *Driver, h driver.AppHandler) error {
	t.Helper()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-stop:
		case <-time.After(10 * time.Second):
			panic("headless: Run did not return")
		}
	}()
	return d.Run(driver.AppConfig{Name: "Test"}, h)
}

func TestRunReadyOnceAndQuitFromReady(t *testing.T) {
	d := New(nil)
	h := &app{}
	h.onReady = func() {
		if !d.Running || d.Queue() == nil || !d.Queue().OnUIThread() {
			t.Error("Ready: driver not running or queue not bound to the UI goroutine")
		}
		d.Quit()
	}
	if err := run(t, d, h); err != nil {
		t.Fatal(err)
	}
	if h.ready != 1 || d.Config.Name != "Test" || d.Name() != "headless" {
		t.Errorf("ready=%d config=%+v name=%q", h.ready, d.Config, d.Name())
	}
	if d.Running || !d.Quitting() || !d.Queue().Closed() {
		t.Errorf("after Run: running=%v quitting=%v closed=%v", d.Running, d.Quitting(), d.Queue().Closed())
	}
}

func TestQuitFromPostedWork(t *testing.T) {
	d := New(nil)
	var got []int
	h := &app{onReady: func() {
		d.Queue().Do(func() {
			got = append(got, 1)
			d.Queue().Do(func() { got = append(got, 3); d.Quit() }) // queued from the loop
			got = append(got, 2)
		})
	}}
	if err := run(t, d, h); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("order = %v", got)
	}
}

func TestQueuedWorkRunsOnceAfterQuit(t *testing.T) {
	d := New(nil)
	ran := 0
	h := &app{onReady: func() {
		d.Queue().Do(func() { ran++ })
		d.Quit()
		d.Queue().Do(func() { ran += 100 }) // after Quit: dropped
	}}
	if err := run(t, d, h); err != nil {
		t.Fatal(err)
	}
	if ran != 1 {
		t.Errorf("ran = %d, want 1", ran)
	}
}

func TestFIFOFromGoroutines(t *testing.T) {
	const producers, per = 8, 500
	d := New(nil)
	last := make([]int, producers)
	var bad []string
	total := 0
	h := &app{onReady: func() {
		var wg sync.WaitGroup
		for p := range producers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 1; i <= per; i++ {
					d.Queue().Do(func() {
						if i != last[p]+1 {
							bad = append(bad, "out of order")
						}
						last[p] = i
						total++
						if total == producers*per {
							d.Quit()
						}
					})
				}
			}()
		}
		go wg.Wait()
	}}
	if err := run(t, d, h); err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 || total != producers*per {
		t.Errorf("total=%d, %d ordering errors", total, len(bad))
	}
}

func TestDoSync(t *testing.T) {
	d := New(nil)
	var fromGoroutine, inline bool
	h := &app{onReady: func() {
		q := d.Queue()
		// On the UI goroutine DoSync runs inline.
		order := 0
		q.Do(func() { order = 2 })
		if !q.DoSync(func() { inline = order == 0; order = 1 }) {
			t.Error("inline DoSync returned false")
		}
		go func() {
			ok := q.DoSync(func() { fromGoroutine = q.OnUIThread() })
			if !ok {
				t.Error("DoSync from a goroutine returned false")
			}
			d.Queue().Do(d.Quit)
		}()
	}}
	if err := run(t, d, h); err != nil {
		t.Fatal(err)
	}
	if !inline || !fromGoroutine {
		t.Errorf("inline=%v fromGoroutine(on UI)=%v", inline, fromGoroutine)
	}
	after := make(chan bool)
	go func() { after <- d.Queue().DoSync(func() { t.Error("DoSync ran after Quit") }) }()
	if <-after {
		t.Error("DoSync from a goroutine after Quit returned true")
	}
	// On the UI goroutine DoSync stays inline even after Quit (mainthread semantics).
	if !d.Queue().DoSync(func() {}) {
		t.Error("inline DoSync after Quit returned false")
	}
}

// TestQueueBeforeAndAfterRun: there is no queue before Run, and after Run
// the queue is closed, so work queued then is dropped.
func TestQueueBeforeAndAfterRun(t *testing.T) {
	d := New(nil)
	if d.Queue() != nil {
		t.Fatal("Queue before Run is not nil")
	}
	if err := run(t, d, &app{onReady: func() { d.Queue().Do(d.Quit) }}); err != nil {
		t.Fatal(err)
	}
	if d.Queue().Do(func() { t.Error("work queued after Run ran") }) {
		t.Error("Do after Run returned true")
	}
	d.Queue().Drain()
}

func TestRunTwice(t *testing.T) {
	d := New(nil)
	h := &app{onReady: func() {
		if err := d.Run(driver.AppConfig{}, &app{}); !errors.Is(err, ErrRunTwice) {
			t.Errorf("nested Run = %v", err)
		}
		d.Quit()
	}}
	if err := run(t, d, h); err != nil {
		t.Fatal(err)
	}
	if err := d.Run(driver.AppConfig{}, h); !errors.Is(err, ErrRunTwice) {
		t.Errorf("second Run = %v", err)
	}
	if h.ready != 1 {
		t.Errorf("ready = %d", h.ready)
	}
}

func TestNotice(t *testing.T) {
	var buf bytes.Buffer
	d := New(&buf)
	if err := run(t, d, &app{onReady: func() { d.Quit() }}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != Notice {
		t.Errorf("notice = %q", buf.String())
	}
	// A nil writer prints nothing (and does not crash).
	d = New(nil)
	if err := run(t, d, &app{onReady: func() { d.Quit() }}); err != nil {
		t.Fatal(err)
	}
}

func TestQuitClosesWindows(t *testing.T) {
	d := New(nil)
	a, b, late := &win{}, &win{}, &win{}
	h := &app{onReady: func() {
		for _, w := range []*win{a, b} {
			p, err := d.NewWindow(driver.WindowConfig{Title: "W"}, w)
			if err != nil {
				t.Fatal(err)
			}
			p.Show()
		}
		d.Queue().Do(func() {
			d.Queue().Do(func() { d.NewWindow(driver.WindowConfig{}, late) }) // queued before Quit
			d.Quit()
		})
	}}
	if err := run(t, d, h); err != nil {
		t.Fatal(err)
	}
	if a.closed != 1 || b.closed != 1 || late.closed != 1 || len(d.Windows) != 0 {
		t.Errorf("closed: a=%d b=%d late=%d, %d windows left", a.closed, b.closed, late.closed, len(d.Windows))
	}
}

func TestPanicPropagates(t *testing.T) {
	d := New(nil)
	waiter := make(chan bool, 1)
	h := &app{onReady: func() {
		go func() { waiter <- d.Queue().DoSync(func() {}) }()
		panic("boom")
	}}
	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Errorf("recovered %v, want boom", r)
			}
		}()
		run(t, d, h)
	}()
	if !d.Queue().Closed() || d.Running {
		t.Error("queue not closed or driver still running after a panic")
	}
	select {
	case ok := <-waiter:
		if ok {
			t.Error("DoSync waiting during the panic returned true")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DoSync caller not released after the panic")
	}
}

func TestPanicInPostedWork(t *testing.T) {
	d := New(nil)
	h := &app{onReady: func() { d.Queue().Do(func() { panic("posted") }) }}
	defer func() {
		if r := recover(); r != "posted" {
			t.Errorf("recovered %v, want posted", r)
		}
	}()
	run(t, d, h)
	t.Error("Run returned normally")
}

func TestStubFeaturesStillWork(t *testing.T) {
	d := New(nil)
	if err := run(t, d, &app{onReady: func() {
		d.SetMenuBar(nil)
		d.SetAppearance(driver.AppearanceDark)
		if d.MenuBar == nil || !d.Appearance().Dark || d.NewButton(nil) == nil {
			t.Error("embedded stub driver not reachable")
		}
		if !d.UserQuit() {
			t.Error("UserQuit refused")
		}
	}}); err != nil {
		t.Fatal(err)
	}
	if !d.Quitting() {
		t.Error("UserQuit did not quit")
	}
}

// TestRunBlocksWithoutWork runs a Run that nothing quits in a child process
// and checks that it keeps blocking (as `go run ./examples/hello` must)
// instead of dying in the runtime's "all goroutines are asleep" check. The
// child disables the test timeout, whose timer would hide that check.
func TestRunBlocksWithoutWork(t *testing.T) {
	if os.Getenv("HEADLESS_BLOCK_CHILD") == "1" {
		New(os.Stderr).Run(driver.AppConfig{Name: "Child"}, &app{})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunBlocksWithoutWork$", "-test.timeout=0")
	cmd.Env = append(os.Environ(), "HEADLESS_BLOCK_CHILD=1")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		t.Fatalf("Run returned or crashed (%v):\n%s", err, out.String())
	case <-time.After(time.Second):
		cmd.Process.Kill()
		<-exited
	}
	if !bytes.Contains(out.Bytes(), []byte(Notice)) {
		t.Errorf("child output %q, want the notice", out.String())
	}
}

// TestObserveFrames: the headless driver has no frame clock (R7).
func TestObserveFrames(t *testing.T) {
	d := New(nil)
	if now := d.ObserveFrames(func(driver.FrameTiming) { t.Error("frame observed") }); now != nil {
		t.Error("ObserveFrames returned a clock")
	}
}
