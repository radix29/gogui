//go:build windows

package win32

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
	"golang.org/x/sys/windows"
)

// TestMessageLoopAllocs (review P17): the message loop's idle steps
// allocate nothing. The window procedure's dispatch (handled and left to
// DefWindowProcW), the clock's pacing set, its wait handles and the paint
// pass run with two shown surfaces and a popup, as between frames.
func TestMessageLoopAllocs(t *testing.T) {
	inShell(t, func(s *Shell) {
		paint := &recorder{onPaint: func(render.Canvas) {}}
		a := shownParent(t, s, paint)
		b := shownParent(t, s, paint)
		p := newPopup(t, s, a, shell.PopupConfig{Size: driver.Size{W: 50, H: 50}}, &popRec{})
		for _, sf := range []window{a, b, p} {
			paintAndRead(t, sf)
		}
		c := s.clock
		const custom = w32.WM_APP + 78
		checks := []struct {
			name string
			f    func()
		}{
			{"wndproc handled", func() { wndproc(a.hwnd, w32.WM_ERASEBKGND, 0, 0) }},
			{"wndproc unhandled", func() { wndproc(a.hwnd, custom, 0, 0) }},
			{"pacingSurfaces", func() { _ = s.pacingSurfaces() }},
			{"chainReady", func() { _ = c.chainReady() }},
			{"arm", func() {
				c.requested, c.chainSince = true, -1
				c.arm()
				c.requested = false
			}},
			{"offScreen", func() { _ = s.offScreen() }},
			{"signaled", func() { c.signaled(windows.Handle(1)) }},
			{"paintInvalid", func() { s.paintInvalid(0) }},
		}
		for _, ch := range checks {
			if n := testing.AllocsPerRun(50, ch.f); n != 0 {
				t.Errorf("%s: %v allocations, want 0", ch.name, n)
			}
		}
		p.Close()
	})
}
