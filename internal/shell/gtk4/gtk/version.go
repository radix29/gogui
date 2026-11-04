package gtk

import (
	"errors"
	"fmt"
)

// The libraries Load opens, by soname. GDK and GSK live inside libgtk-4.
const (
	GTKLib      = "libgtk-4.so.1"
	GObjectLib  = "libgobject-2.0.so.0"
	GLibLib     = "libglib-2.0.so.0"
	GrapheneLib = "libgraphene-1.0.so.0"
	PangoLib    = "libpango-1.0.so.0"
	// PangoCairoLib has the default font map (S4.2g); it comes with
	// libpango, so Pango's version check covers it.
	PangoCairoLib = "libpangocairo-1.0.so.0"
	// CairoLib has the font options of the text system's context (S4.2g;
	// GTK itself links it).
	CairoLib = "libcairo.so.2"
	// PangoFT2Lib and FontconfigLib add the app font (S4.2j). On Linux,
	// libpangocairo is built on both (its font map is a PangoFcFontMap),
	// so they are there wherever it is.
	PangoFT2Lib   = "libpangoft2-1.0.so.0"
	FontconfigLib = "libfontconfig.so.1"
)

// The oldest versions gogui supports: GTK 4.14 (D7, raised from 4.10 in
// S4.2a for GskPath), and the Pango that GTK 4.14 itself requires (its
// meson.build: pango >= 1.52.0).
const (
	MinGTKMinor   = 14
	MinPangoMajor = 1
	MinPangoMinor = 52
)

// packages names, per library, the distribution packages that provide it,
// for the error message.
var packages = map[string]string{
	GTKLib:        "libgtk-4-1 (Debian, Ubuntu) or gtk4 (Fedora, Arch)",
	GObjectLib:    "libglib2.0-0t64 or libglib2.0-0 (Debian, Ubuntu) or glib2 (Fedora, Arch)",
	GLibLib:       "libglib2.0-0t64 or libglib2.0-0 (Debian, Ubuntu) or glib2 (Fedora, Arch)",
	GrapheneLib:   "libgraphene-1.0-0 (Debian, Ubuntu) or graphene (Fedora, Arch)",
	PangoLib:      "libpango-1.0-0 (Debian, Ubuntu) or pango (Fedora, Arch)",
	PangoCairoLib: "libpangocairo-1.0-0 (Debian, Ubuntu) or pango (Fedora, Arch)",
	CairoLib:      "libcairo2 (Debian, Ubuntu) or cairo (Fedora, Arch)",
	PangoFT2Lib:   "libpangoft2-1.0-0 (Debian, Ubuntu) or pango (Fedora, Arch)",
	FontconfigLib: "libfontconfig1 (Debian, Ubuntu) or fontconfig (Fedora, Arch)",
}

// ErrMissing is what every error from Load wraps (errors.Is): GTK 4 or one
// of its libraries can't be used, so the shell can't start. Report shows
// such an error to the user.
var ErrMissing = errors.New("GTK 4 is missing or too old")

// MissingError is a library that could not be opened or bound.
type MissingError struct {
	Lib string // the soname
	Err error  // from ffi.Dlopen or ffi.Bind
}

func (e *MissingError) Error() string {
	pkg := packages[e.Lib]
	if pkg == "" {
		pkg = "GTK 4"
	}
	return fmt.Sprintf("gogui needs GTK 4.%d or later: %s can't be loaded (%v); install the package %s",
		MinGTKMinor, e.Lib, e.Err, pkg)
}

func (e *MissingError) Unwrap() []error { return []error{ErrMissing, e.Err} }

// VersionError is a library that is older than gogui supports.
type VersionError struct {
	Lib  string // the soname
	Have string // the version found
	Want string // the oldest version supported
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("gogui needs %s %s or later, but %s is %s; update the package %s",
		libName(e.Lib), e.Want, e.Lib, e.Have, packages[e.Lib])
}

func (e *VersionError) Unwrap() error { return ErrMissing }

func libName(lib string) string {
	if lib == PangoLib {
		return "Pango"
	}
	return "GTK"
}

// checkGTK returns a *VersionError unless major.minor.micro is GTK 4.14 or a
// later GTK 4.
func checkGTK(major, minor, micro uint32) error {
	if major == 4 && minor >= MinGTKMinor {
		return nil
	}
	return &VersionError{Lib: GTKLib, Have: fmt.Sprintf("%d.%d.%d", major, minor, micro),
		Want: fmt.Sprintf("4.%d", MinGTKMinor)}
}

// checkPango returns a *VersionError unless v (pango_version():
// major*10000 + minor*100 + micro) is Pango 1.52 or a later Pango 1.
func checkPango(v int32) error {
	major, minor := v/10000, v/100%100
	if major == MinPangoMajor && minor >= MinPangoMinor {
		return nil
	}
	return &VersionError{Lib: PangoLib, Have: fmt.Sprintf("%d.%d.%d", major, minor, v%100),
		Want: fmt.Sprintf("%d.%d", MinPangoMajor, MinPangoMinor)}
}
