//go:build !darwin

package gogui

import (
	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell/native"
)

// newPlatformDriver creates the Windows or Linux driver: the aqua driver
// over the OS's shell (Win32 since S2.3f, GTK 4 since S4.1d). If the OS
// has no shell (a BSD), it returns the native.New error.
func newPlatformDriver() (driver.Driver, error) {
	sh, err := native.New()
	if err != nil {
		return nil, err
	}
	return aqua.New(sh), nil
}
