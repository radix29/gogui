//go:build windows

package win32

import (
	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The system cursors of shell.Cursor (surface.SetCursor, popup.SetCursor).

// cursorIDs maps the cursors to the system cursors (0: hidden).
var cursorIDs = [...]uintptr{
	shell.CursorArrow:           w32.IDC_ARROW,
	shell.CursorIBeam:           w32.IDC_IBEAM,
	shell.CursorPointingHand:    w32.IDC_HAND,
	shell.CursorResizeLeftRight: w32.IDC_SIZEWE,
	shell.CursorResizeUpDown:    w32.IDC_SIZENS,
	shell.CursorCrosshair:       w32.IDC_CROSS,
	shell.CursorNotAllowed:      w32.IDC_NO,
	shell.CursorHidden:          0,
}

// cursors caches the loaded system cursors (shared handles, never
// destroyed). UI thread only.
var cursors [len(cursorIDs)]windows.Handle

// cursorHandle returns the system cursor for c (0 for CursorHidden; the
// arrow for an unknown c).
func cursorHandle(c shell.Cursor) windows.Handle {
	if int(c) >= len(cursorIDs) {
		c = shell.CursorArrow
	}
	if cursorIDs[c] == 0 {
		return 0
	}
	if cursors[c] == 0 {
		h, err := w32.LoadCursor(cursorIDs[c])
		if err != nil {
			return 0
		}
		cursors[c] = h
	}
	return cursors[c]
}
