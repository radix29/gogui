//go:build linux

package gtk4

import (
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// nativeClipboard is the clipboardBackend over the default display's
// GdkClipboard (S5.2b). It keeps nothing: GDK owns the clipboard object
// for the display's life, so it is looked up on each call.
type nativeClipboard struct{}

// clipboard returns the default display's GdkClipboard (transfer none),
// or 0 without a display.
func (nativeClipboard) clipboard() uintptr {
	d := gtk.Gdk.DisplayGetDefault()
	if d == 0 {
		return 0
	}
	return gtk.Gdk.DisplayGetClipboard(d)
}

func (n nativeClipboard) setText(text string) {
	if c := n.clipboard(); c != 0 {
		gtk.Gdk.ClipboardSetText(c, text) // GDK copies text
	}
}

func (n nativeClipboard) readText(data ffi.Handle) bool {
	c := n.clipboard()
	if c == 0 {
		return false
	}
	registerTrampolines()
	gtk.Gdk.ClipboardReadTextAsync(c, 0, tramp.asyncReady, uintptr(data))
	return true
}

func (nativeClipboard) finishText(source, result uintptr) (string, bool) {
	var gerr uintptr
	p := gtk.Gdk.ClipboardReadTextFinish(source, result, &gerr)
	if p == 0 {
		if gerr != 0 { // no text on the clipboard, or the transfer failed
			gtk.GLib.ErrorFree(gerr)
		}
		return "", false
	}
	defer gtk.GLib.Free(p)
	return ffi.GoString(p), true
}
