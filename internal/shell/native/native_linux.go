//go:build linux

package native

import (
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4"
)

// New returns a new, unstarted GTK 4 shell. It never fails: GTK is loaded
// by the shell's Run, which returns the error if it is missing.
func New() (shell.Shell, error) { return gtk4.New(), nil }
