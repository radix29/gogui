package gogui

import (
	"io"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/headless"
)

// The Example functions (example_test.go, package gogui_test) call Run as
// an app does, once each. In the test binary a Run that no test has given a
// driver (setDriver) gets a fresh, silent headless driver with the
// UI-thread checks on, and clears ran, so the next Example can call Run
// again. setDriver's cleanup clears ran too, so the first Example can run
// after the tests.
func init() {
	newDriver = func() (driver.Driver, error) {
		ran.Store(false)
		debugChecks = true
		return headless.New(io.Discard), nil
	}
}
