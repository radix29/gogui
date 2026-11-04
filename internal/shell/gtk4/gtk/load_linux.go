//go:build linux

package gtk

import (
	"sync"

	"github.com/radix29/gogui/internal/ffi"
)

// Versions found by Load.
var gtkVersion [3]uint32
var pangoVersion int32

// Load opens the libraries, binds every function above and checks the
// GTK and Pango versions. It runs once; later calls return the first
// result. The libraries stay open for the life of the process. Every error
// wraps ErrMissing and is a *MissingError or a *VersionError; Report shows
// it to the user. Load does not initialise GTK (Gtk.InitCheck does).
func Load() error { return load() }

var load = sync.OnceValue(func() error {
	for _, l := range libraries() {
		lib, err := ffi.Dlopen(l.soname)
		if err != nil {
			return &MissingError{Lib: l.soname, Err: err}
		}
		for _, b := range l.bindings {
			if err := lib.Bind(b.fptr, b.name); err != nil {
				return &MissingError{Lib: l.soname, Err: err}
			}
		}
		for _, b := range l.optional {
			if _, err := lib.Sym(b.name); err != nil {
				continue // newer than this GTK: the field stays nil
			}
			if err := lib.Bind(b.fptr, b.name); err != nil {
				return &MissingError{Lib: l.soname, Err: err}
			}
		}
	}
	gtkVersion = [3]uint32{Gtk.GetMajorVersion(), Gtk.GetMinorVersion(), Gtk.GetMicroVersion()}
	if err := checkGTK(gtkVersion[0], gtkVersion[1], gtkVersion[2]); err != nil {
		return err
	}
	pangoVersion = Pango.Version()
	return checkPango(pangoVersion)
})

// Version returns the loaded GTK's major, minor and micro version (zero
// until Load has read it; a GTK that is too old is still reported).
func Version() [3]uint32 { return gtkVersion }

// PangoVersion returns pango_version() of the loaded Pango (zero
// until Load has read it).
func PangoVersion() int32 { return pangoVersion }
