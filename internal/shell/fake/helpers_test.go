package fake

import (
	"fmt"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// app is a shell.Handler that counts its calls.
type app struct {
	ready       int
	services    shell.Services // the last Ready's
	appearances []driver.Appearance
	onReady     func()
}

func (a *app) Ready(sv shell.Services) {
	a.ready++
	a.services = sv
	if a.onReady != nil {
		a.onReady()
	}
}
func (a *app) AppearanceChanged(ap driver.Appearance) { a.appearances = append(a.appearances, ap) }

// win is a shell.SurfaceHandler that logs its calls as strings.
type win struct {
	log     []string
	paint   func(c render.Canvas)
	event   func(ev shell.Event) bool
	allow   bool // CloseRequested result
	resizes []driver.Size
}

func (w *win) logf(format string, args ...any) { w.log = append(w.log, fmt.Sprintf(format, args...)) }

func (w *win) Paint(c render.Canvas) {
	w.logf("paint")
	if w.paint != nil {
		w.paint(c)
	}
}
func (w *win) Resized(size driver.Size, scale float64) {
	w.resizes = append(w.resizes, size)
	w.logf("resized %gx%g @%g", size.W, size.H, scale)
}
func (w *win) CloseRequested() bool { w.logf("closeRequested"); return w.allow }
func (w *win) Closed()              { w.logf("closed") }
func (w *win) KeyChanged(k bool)    { w.logf("key %v", k) }
func (w *win) Event(ev shell.Event) bool {
	w.logf("%s", eventString(ev))
	if w.event != nil {
		return w.event(ev)
	}
	return false
}

// take returns the log and clears it.
func (w *win) take() string {
	s := strings.Join(w.log, "; ")
	w.log = nil
	return s
}

var pointerKinds = [...]string{"move", "down", "up", "enter", "leave"}
var phases = [...]string{"", "began", "changed", "ended"}

func eventString(ev shell.Event) string {
	switch e := ev.(type) {
	case shell.PointerEvent:
		s := fmt.Sprintf("%s (%g,%g)", pointerKinds[e.Kind], e.Pos.X, e.Pos.Y)
		if e.Button != 0 {
			s += fmt.Sprintf(" b=%d", e.Button)
		}
		if e.Buttons != 0 {
			s += fmt.Sprintf(" held=%d", e.Buttons)
		}
		if e.Clicks != 0 {
			s += fmt.Sprintf(" n=%d", e.Clicks)
		}
		if e.Activating {
			s += " activating"
		}
		if e.Mods != 0 {
			s += fmt.Sprintf(" m=%d", e.Mods)
		}
		return s
	case shell.ScrollEvent:
		s := fmt.Sprintf("scroll (%g,%g) d=(%g,%g)", e.Pos.X, e.Pos.Y, e.Delta.X, e.Delta.Y)
		if e.Precise {
			s += " precise"
		}
		if e.Phase != 0 {
			s += " " + phases[e.Phase]
		}
		if e.Momentum != 0 {
			s += " momentum " + phases[e.Momentum]
		}
		return s
	case shell.KeyEvent:
		s := "keyup"
		if e.Down {
			s = "keydown"
		}
		if e.Repeat {
			s += " repeat"
		}
		s += fmt.Sprintf(" %q", rune(e.Key))
		if e.Mods != 0 {
			s += fmt.Sprintf(" m=%d", e.Mods)
		}
		return s
	case shell.TextEvent:
		return fmt.Sprintf("text %q", e.Text)
	case shell.PreeditEvent:
		if e.Text == "" {
			return "preedit end"
		}
		s := fmt.Sprintf("preedit %q @%d", e.Text, e.Cursor)
		for _, sg := range e.Segments {
			sel := ""
			if sg.Selected {
				sel = "*"
			}
			s += fmt.Sprintf(" [%d,%d)%s", sg.Start, sg.End, sel)
		}
		return s
	}
	return "?"
}

// mustPanic fails t unless fn panics with a message containing want.
func mustPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want %q", want)
		}
		if !strings.Contains(fmt.Sprint(r), want) {
			t.Fatalf("panic %q, want it to contain %q", r, want)
		}
	}()
	fn()
}

// started returns a shell started on the test goroutine.
func started(t *testing.T) *Shell {
	t.Helper()
	s := New()
	if err := s.Start(&app{}); err != nil {
		t.Fatal(err)
	}
	return s
}

// open returns a shown surface of the given size, painted once, with its
// log cleared.
func open(t *testing.T, s *Shell, w *win, cfg shell.SurfaceConfig) *Surface {
	t.Helper()
	sf, err := s.NewSurface(cfg, w)
	if err != nil {
		t.Fatal(err)
	}
	f := sf.(*Surface)
	f.Show()
	s.Flush()
	w.take()
	return f
}

func pt(x, y float64) driver.Point { return driver.Point{X: x, Y: y} }
func sz(w, h float64) driver.Size  { return driver.Size{W: w, H: h} }
func rc(x, y, w, h float64) shell.ChromeRegion {
	return shell.ChromeRegion{Rect: driver.Rect{X: x, Y: y, W: w, H: h}}
}
