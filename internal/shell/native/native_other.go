//go:build !windows && !linux

package native

import "github.com/radix29/gogui/internal/shell"

// New returns ErrNotImplemented: this OS has no shell (macOS uses the
// appkit driver).
func New() (shell.Shell, error) { return nil, ErrNotImplemented }
