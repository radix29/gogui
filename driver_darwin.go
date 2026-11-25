//go:build darwin

package gogui

import (
	"os"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/headless"
)

// newPlatformDriver creates the macOS driver. Until the appkit driver exists
// (S1.1) it is the headless one, which always starts.
func newPlatformDriver() (driver.Driver, error) { return headless.New(os.Stderr), nil }
