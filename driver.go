package gogui

import (
	"fmt"
	"os"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/mainthread"
)

// newDriver creates the platform driver through newPlatformDriver, which
// lives in the driver-selection files driver_darwin.go and driver_other.go
// (the only build-tagged files in this package). The tests replace it.
var newDriver = func() driver.Driver { return newPlatformDriver() }

// debugChecks turns on the UI-thread checks (GOGUI_DEBUG=1). The tests set
// it.
var debugChecks = os.Getenv("GOGUI_DEBUG") == "1"

// uiDriver is a driver that exposes its UI-thread queue, which Do, DoSync
// and the UI-thread checks use. Queue returns nil before the driver's Run
// has created the queue.
type uiDriver interface {
	driver.Driver
	Queue() *mainthread.Queue
}

// selectDriver creates the driver and checks that it has a UI queue.
func selectDriver() (uiDriver, error) {
	d := newDriver()
	u, ok := d.(uiDriver)
	if !ok {
		return nil, fmt.Errorf("gogui: driver %q has no UI-thread queue", d.Name())
	}
	return u, nil
}

// errDriver is the driver of a platform that can't start: its Run returns
// err without calling the handler, and its queue stays nil. The other
// methods are the stub driver's, which Run's early return never reaches.
type errDriver struct {
	*stub.Driver
	name string
	err  error
}

// failedDriver returns a driver named name whose Run returns err.
func failedDriver(name string, err error) driver.Driver {
	return &errDriver{Driver: stub.NewDriver(), name: name, err: err}
}

func (d *errDriver) Name() string                                  { return d.name }
func (d *errDriver) Run(driver.AppConfig, driver.AppHandler) error { return d.err }
func (d *errDriver) Queue() *mainthread.Queue                      { return nil }
