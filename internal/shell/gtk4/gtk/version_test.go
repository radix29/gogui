package gtk

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestCheckGTK(t *testing.T) {
	for _, c := range []struct {
		v  [3]uint32
		ok bool
	}{
		{[3]uint32{4, 14, 0}, true}, {[3]uint32{4, 14, 5}, true}, {[3]uint32{4, 18, 2}, true},
		{[3]uint32{4, 13, 0}, false}, {[3]uint32{4, 13, 99}, false}, {[3]uint32{4, 12, 4}, false},
		{[3]uint32{4, 10, 0}, false}, {[3]uint32{4, 9, 99}, false}, {[3]uint32{4, 0, 0}, false},
		{[3]uint32{3, 24, 41}, false}, {[3]uint32{5, 0, 0}, false}, {[3]uint32{0, 0, 0}, false},
	} {
		err := checkGTK(c.v[0], c.v[1], c.v[2])
		if (err == nil) != c.ok {
			t.Errorf("checkGTK(%v) = %v, want ok %v", c.v, err, c.ok)
			continue
		}
		if err == nil {
			continue
		}
		var ve *VersionError
		if !errors.As(err, &ve) || !errors.Is(err, ErrMissing) {
			t.Errorf("checkGTK(%v) = %#v, want a *VersionError wrapping ErrMissing", c.v, err)
		}
		msg := err.Error()
		for _, want := range []string{"GTK 4.14", GTKLib, "libgtk-4-1", "gtk4"} {
			if !strings.Contains(msg, want) {
				t.Errorf("checkGTK(%v) message %q lacks %q", c.v, msg, want)
			}
		}
	}
	err := checkGTK(4, 8, 3)
	if !strings.Contains(err.Error(), "is 4.8.3") {
		t.Errorf("message %q does not name the version found", err)
	}
}

func TestCheckPango(t *testing.T) {
	for _, c := range []struct {
		v    int32
		ok   bool
		have string
	}{
		{15200, true, ""}, {15206, true, ""}, {15402, true, ""}, {15600, true, ""},
		{15199, false, "1.51.99"}, {15006, false, "1.50.6"}, {14917, false, "1.49.17"}, {14400, false, "1.44.0"}, {20000, false, "2.0.0"}, {0, false, "0.0.0"},
	} {
		err := checkPango(c.v)
		if (err == nil) != c.ok {
			t.Errorf("checkPango(%d) = %v, want ok %v", c.v, err, c.ok)
			continue
		}
		if err == nil {
			continue
		}
		var ve *VersionError
		if !errors.As(err, &ve) || !errors.Is(err, ErrMissing) {
			t.Errorf("checkPango(%d) = %#v, want a *VersionError wrapping ErrMissing", c.v, err)
		}
		msg := err.Error()
		for _, want := range []string{"Pango 1.52", PangoLib, "is " + c.have, "libpango-1.0-0"} {
			if !strings.Contains(msg, want) {
				t.Errorf("checkPango(%d) message %q lacks %q", c.v, msg, want)
			}
		}
	}
}

func TestMissingError(t *testing.T) {
	cause := errors.New("ffi: dlopen libgtk-4.so.1: cannot open shared object file")
	err := error(&MissingError{Lib: GTKLib, Err: cause})
	if !errors.Is(err, ErrMissing) || !errors.Is(err, cause) {
		t.Errorf("%v wraps neither ErrMissing nor its cause", err)
	}
	msg := err.Error()
	for _, want := range []string{"GTK 4.14", GTKLib, cause.Error(), "libgtk-4-1", "gtk4"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
	// Every library Load opens has a package hint.
	for _, lib := range []string{GTKLib, GObjectLib, GLibLib, GrapheneLib, PangoLib, PangoCairoLib, CairoLib, PangoFT2Lib, FontconfigLib} {
		if packages[lib] == "" {
			t.Errorf("no package names for %s", lib)
		}
		if m := (&MissingError{Lib: lib, Err: cause}).Error(); !strings.Contains(m, packages[lib]) {
			t.Errorf("message %q lacks the packages %q", m, packages[lib])
		}
	}
}

type fakeRun struct {
	name string
	args []string
	n    int
}

func (f *fakeRun) run(name string, args ...string) error {
	f.name, f.args, f.n = name, args, f.n+1
	return errors.New("no display")
}

func onPath(names ...string) func(string) (string, error) {
	return func(n string) (string, error) {
		for _, m := range names {
			if m == n {
				return "/usr/bin/" + n, nil
			}
		}
		return "", fs.ErrNotExist
	}
}

func TestReport(t *testing.T) {
	err := &MissingError{Lib: GTKLib, Err: errors.New("not found <&>")}
	msg := err.Error()
	for _, c := range []struct {
		path     []string
		wantName string
		wantArgs []string
	}{
		{[]string{"zenity", "kdialog"}, "/usr/bin/zenity",
			[]string{"--error", "--no-markup", "--title=" + reportTitle, "--text=" + msg}},
		{[]string{"kdialog"}, "/usr/bin/kdialog", []string{"--title", reportTitle, "--error", msg}},
		{nil, "", nil},
	} {
		var out strings.Builder
		var f fakeRun
		report(err, &out, onPath(c.path...), f.run)
		if got, want := out.String(), "gogui: "+msg+"\n"; got != want {
			t.Errorf("PATH %v: stderr %q, want %q", c.path, got, want)
		}
		if c.wantName == "" {
			if f.n != 0 {
				t.Errorf("PATH %v: ran %s %q, want no dialog", c.path, f.name, f.args)
			}
			continue
		}
		if f.n != 1 || f.name != c.wantName || strings.Join(f.args, "\x00") != strings.Join(c.wantArgs, "\x00") {
			t.Errorf("PATH %v: ran %d× %s %q, want %s %q", c.path, f.n, f.name, f.args, c.wantName, c.wantArgs)
		}
	}

	var out strings.Builder
	var f fakeRun
	report(nil, &out, onPath("zenity"), f.run)
	if out.Len() != 0 || f.n != 0 {
		t.Errorf("report(nil) wrote %q and ran %d dialogs", out.String(), f.n)
	}
}
