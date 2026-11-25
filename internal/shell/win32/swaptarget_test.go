//go:build windows

package win32

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/d2d"
)

// TestSwapTarget (review R8): a surface and a popup share one swapTarget
// type for their window and rendering state. The popup's window is the
// popup's own target (no forwarding through a surface), the shell's
// window list holds both swap targets in order, each target paints its
// own content through the same chain code, and the popup's background is
// transparent where the surface's is the window background.
func TestSwapTarget(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := shownParent(t, s, &recorder{onPaint: func(render.Canvas) {}})
		defer sf.Close()
		pr := &popRec{}
		p := newPopup(t, s, sf, shell.PopupConfig{Size: driver.Size{W: 60, H: 40}}, pr)
		var ws []*swapTarget
		for w := range s.allWindows() {
			ws = append(ws, w)
		}
		if len(ws) != 2 || ws[0] != sf.swap() || ws[1] != p.swap() {
			t.Fatalf("windows %v, want the surface's and the popup's swap targets", ws)
		}
		for _, c := range []struct {
			name string
			w    window
		}{{"surface", sf}, {"popup", p}} {
			st := c.w.swap()
			if got, _ := targets.Get(handleOf(st.hwnd)); got != target(c.w) {
				t.Errorf("%s: the window's target is %T, want the %s", c.name, got, c.name)
			}
			if st.win != c.w {
				t.Errorf("%s: win is %T", c.name, st.win)
			}
			before := st.presents
			if _, ok := paintAndRead(t, c.w); !ok || st.presents != before+1 || st.invalid || st.renderErr != nil {
				t.Errorf("%s: drawn %v, presents %d → %d, invalid %v, %v", c.name, ok, before, st.presents,
					st.invalid, st.renderErr)
			}
			chainPx(t, c.name, c.w)
		}
		if pr.paints != 1 {
			t.Errorf("popup Paint ran %d times, want 1", pr.paints)
		}
		if sf.background() == (d2d.ColorF{}) || p.background() != (d2d.ColorF{}) {
			t.Errorf("backgrounds: surface %v, popup %v", sf.background(), p.background())
		}
		p.Close()
		n := 0
		for range s.allWindows() {
			n++
		}
		if !p.dead || !p.chain.IsNil() || n != 1 {
			t.Errorf("closed popup: dead %v, chain %v, %d windows", p.dead, !p.chain.IsNil(), n)
		}
	})
}
