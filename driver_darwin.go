//go:build darwin

package gogui

import (
	"os"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/headless"
)

// newPlatformDriver creates the macOS driver. Until the appkit driver exists
// (S1.1) it is the headless one.
func newPlatformDriver() driver.Driver { return headless.New(os.Stderr) }
