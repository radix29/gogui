//go:build windows

package native

import (
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32"
)

// New returns a new, unstarted Win32 shell. It never fails.
func New() (shell.Shell, error) { return win32.New(), nil }
