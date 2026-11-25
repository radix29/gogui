package headless

import (
	"errors"
	"io"
	"math"
	"sync/atomic"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/mainthread"
)

// Notice is the line Run prints to the notice writer.
const Notice = "gogui: headless driver, no windows are shown\n"

// ErrRunTwice is returned by Run when it is called a second time.
var ErrRunTwice = errors.New("headless: Run called more than once")

// Driver implements driver.Driver without a display.
//
// The embedded stub.Driver provides the factories, the appearance, the menu
// bar, alerts and the User… simulations, and records state for tests.
// Driver adds Name, Run, Quit, Queue, ObserveFrames, Quitting and UserQuit,
// built on a mainthread.Queue.
//
// A Driver runs once: Run may be called only once, and it returns after
// Quit. Every method except Queue must be called on the UI goroutine (the
// one that called Run).
type Driver struct {
	*stub.Driver

	notice  io.Writer
	q       atomic.Pointer[mainthread.Queue]
	started atomic.Bool
	quit    atomic.Bool
	wake    chan struct{}
}

var _ driver.Driver = (*Driver)(nil)

// New returns a headless driver that prints Notice to notice when Run
// starts. A nil notice prints nothing.
func New(notice io.Writer) *Driver {
	return &Driver{
		Driver: stub.NewDriver(),
		notice: notice,
		wake:   make(chan struct{}, 1),
	}
}

// Name implements driver.Driver.
func (d *Driver) Name() string { return "headless" }

// Queue returns the UI-thread queue, or nil before Run. After Quit it
// returns the closed queue, whose Do and DoSync return false.
func (d *Driver) Queue() *mainthread.Queue { return d.q.Load() }

// Run implements driver.Driver. It binds the calling goroutine as the UI
// goroutine, prints the notice, calls h.Ready and then runs queued work
// until Quit. Work that was queued before Quit still runs once, and windows
// opened by it are closed too; then Run returns nil.
//
// A panic in Ready or in queued work propagates out of Run unrecovered; the
// queue is closed on the way out, so goroutines waiting in DoSync return
// false. A second call returns ErrRunTwice.
func (d *Driver) Run(cfg driver.AppConfig, h driver.AppHandler) error {
	if !d.started.CompareAndSwap(false, true) {
		return ErrRunTwice
	}
	q := mainthread.New()
	q.Bind()
	q.SetWake(func() {
		select {
		case d.wake <- struct{}{}:
		default:
		}
	})
	d.q.Store(q)
	defer q.Close()

	d.Config, d.Handler = cfg, h
	d.Running = true
	defer func() { d.Running = false }()

	// A Run that nothing quits blocks forever, as a native event loop does
	// (go run ./examples/hello blocks until killed). With no other
	// goroutine alive, the runtime would call the wait below a deadlock and
	// abort; a pending timer in the timer heap stops its check.
	keepAlive := time.AfterFunc(math.MaxInt64, func() {})
	defer keepAlive.Stop()

	if d.notice != nil {
		io.WriteString(d.notice, Notice)
	}
	if h != nil {
		h.Ready()
	}
	for {
		q.Drain()
		if d.quit.Load() {
			q.Drain() // work queued before Quit runs once
			d.closeWindows()
			return nil
		}
		<-d.wake
	}
}

// Quit implements driver.Driver. It closes every open window (each gets
// WindowHandler.Closed), closes the queue so later Do and DoSync calls
// are dropped, and makes Run return once the current work is done.
func (d *Driver) Quit() {
	d.closeWindows()
	d.quit.Store(true)
	if q := d.q.Load(); q != nil {
		q.Close()
	}
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Quitting reports whether Quit has been called.
func (d *Driver) Quitting() bool { return d.quit.Load() }

// UserQuit simulates the user choosing Quit: it asks QuitRequested and
// quits if allowed. It reports whether the driver quit.
func (d *Driver) UserQuit() bool {
	if d.Handler != nil && !d.Handler.QuitRequested() {
		return false
	}
	d.Quit()
	return true
}

func (d *Driver) closeWindows() {
	for len(d.Windows) > 0 {
		d.Windows[len(d.Windows)-1].Close()
	}
}

// ObserveFrames implements driver.Driver. The headless driver draws
// nothing, so it has no frame clock: it returns nil and never calls fn.
func (d *Driver) ObserveFrames(func(driver.FrameTiming)) func() time.Duration { return nil }
