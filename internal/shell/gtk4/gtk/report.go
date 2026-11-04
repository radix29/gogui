package gtk

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// reportTitle is the title of the message box Report shows.
const reportTitle = "gogui: GTK 4 is missing"

// Report tells the user that the app can't start because of err (usually
// an ErrMissing from Load): one line on stderr, plus a message box through
// zenity or kdialog when one of them is on PATH (PLAN.md §6.3). GTK itself
// can't show the box, since it is what is missing. It waits until the box
// is closed; a box that fails to open is ignored, as stderr has the text.
func Report(err error) {
	report(err, os.Stderr, exec.LookPath, func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	})
}

// report is Report with stderr, the PATH lookup and the command runner
// injected, for the tests.
func report(err error, stderr io.Writer, lookPath func(string) (string, error),
	run func(name string, args ...string) error) {
	if err == nil {
		return
	}
	msg := err.Error()
	fmt.Fprintln(stderr, "gogui:", msg)
	if name, args := dialogCommand(msg, lookPath); name != "" {
		_ = run(name, args...)
	}
}

// dialogCommand returns the message-box command for msg: zenity if it is
// on PATH, else kdialog, else "". Both get the text as one argument, so
// no shell quoting is involved; zenity's --no-markup keeps a '<' or '&' in
// the error from being read as Pango markup.
func dialogCommand(msg string, lookPath func(string) (string, error)) (string, []string) {
	if p, err := lookPath("zenity"); err == nil {
		return p, []string{"--error", "--no-markup", "--title=" + reportTitle, "--text=" + msg}
	}
	if p, err := lookPath("kdialog"); err == nil {
		return p, []string{"--title", reportTitle, "--error", msg}
	}
	return "", nil
}
